// Command mise is a tiny, flat-file recipe manager.
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/staal0/mise/internal/importer"
	"github.com/staal0/mise/internal/web"
)

const usage = `mise — flat-file recipes

Usage:
  mise serve  [-dir DIR] [-addr :8080]          start the web UI
  mise import [-dir DIR] [-H "K: V"] SOURCE...  import recipes
  mise healthcheck [-addr :8080]                exit 0 if the server is up
  mise version                                  print the version

SOURCE is a URL (web page with schema.org JSON-LD, or a JSON endpoint),
a .json/.html file, a folder of them, or a .zip archive of them.
-H adds a request header for URLs (repeatable), e.g. an Authorization token.
DIR defaults to $MISE_DIR or ./recipes; the address to $MISE_ADDR or :8080.
`

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cmd, args := os.Args[1], os.Args[2:]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	defDir := os.Getenv("MISE_DIR")
	if defDir == "" {
		defDir = "recipes"
	}
	dir := fs.String("dir", defDir, "recipe directory")
	defAddr := os.Getenv("MISE_ADDR")
	if defAddr == "" {
		defAddr = ":8080"
	}

	switch cmd {
	case "version", "-version", "--version":
		fmt.Println(version)

	case "healthcheck":
		addr := fs.String("addr", defAddr, "address the server listens on")
		_ = fs.Parse(args)
		host, port, err := net.SplitHostPort(*addr)
		if err != nil {
			log.Fatal(err)
		}
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = "127.0.0.1"
		}
		c := &http.Client{Timeout: 3 * time.Second}
		resp, err := c.Get("http://" + net.JoinHostPort(host, port) + "/healthz")
		if err != nil {
			log.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			log.Fatalf("healthcheck: %s", resp.Status)
		}

	case "serve":
		addr := fs.String("addr", defAddr, "listen address")
		_ = fs.Parse(args)
		mustDir(*dir)
		srv := &http.Server{Addr: *addr, Handler: web.New(*dir).Handler()}
		go func() {
			<-ctx.Done()
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutdown)
		}()
		log.Printf("mise %s serving %s on %s", version, *dir, *addr)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal(err)
		}

	case "import":
		var headers headerFlag
		fs.Var(&headers, "H", `request header for URL sources, e.g. -H "Authorization: Bearer TOKEN"`)
		insecure := fs.Bool("insecure", false, "skip TLS certificate verification (self-signed certs)")
		_ = fs.Parse(args)
		mustDir(*dir)
		if fs.NArg() == 0 {
			log.Fatal("import: give one or more URLs, files, folders or zip archives")
		}
		if *insecure {
			importer.Client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
		}
		h := http.Header(headers)
		failed := false
		for _, src := range fs.Args() {
			var found []importer.Found
			var err error
			var srcHeader http.Header
			if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
				found, err = importer.FromURL(ctx, src, h)
				srcHeader = h
			} else {
				found, err = importer.FromPath(src)
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", src, err)
				failed = true
				continue
			}
			for _, f := range found {
				if err := importer.Save(ctx, *dir, f, srcHeader); err != nil {
					fmt.Fprintf(os.Stderr, "%s: %s: %v\n", src, f.Recipe.Name, err)
					failed = true
					continue
				}
				fmt.Printf("%s → %s\n", f.Recipe.Name, filepath.Join(*dir, f.Recipe.Slug+".md"))
			}
		}
		if failed {
			os.Exit(1)
		}

	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
}

type headerFlag http.Header

func (h *headerFlag) String() string { return "" }
func (h *headerFlag) Set(v string) error {
	k, val, ok := strings.Cut(v, ":")
	if !ok {
		return fmt.Errorf("header must look like \"Name: value\"")
	}
	if *h == nil {
		*h = headerFlag{}
	}
	http.Header(*h).Add(strings.TrimSpace(k), strings.TrimSpace(val))
	return nil
}

func mustDir(dir string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatal(err)
	}
}
