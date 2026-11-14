package web

import (
	"compress/gzip"
	"net/http"
	"strings"
	"sync"
)

var gzipPool = sync.Pool{New: func() any { return gzip.NewWriter(nil) }}

func gzipResponses(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Accept-Encoding")
		if r.Method == http.MethodHead || !acceptsGzip(r) {
			h.ServeHTTP(w, r)
			return
		}
		gw := &gzipWriter{ResponseWriter: w}
		defer gw.close()
		h.ServeHTTP(gw, r)
	})
}

func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		enc, q, _ := strings.Cut(strings.TrimSpace(part), ";")
		if strings.TrimSpace(enc) == "gzip" && strings.ReplaceAll(q, " ", "") != "q=0" {
			return true
		}
	}
	return false
}

type gzipWriter struct {
	http.ResponseWriter
	gz          *gzip.Writer
	wroteHeader bool
}

func compressible(h http.Header) bool {
	if h.Get("Content-Encoding") != "" || h.Get("Content-Range") != "" {
		return false
	}
	ct := h.Get("Content-Type")
	return strings.HasPrefix(ct, "text/") || strings.Contains(ct, "javascript") ||
		strings.Contains(ct, "json") || strings.Contains(ct, "svg")
}

func (g *gzipWriter) WriteHeader(code int) {
	if g.wroteHeader {
		return
	}
	g.wroteHeader = true
	h := g.Header()
	if code != http.StatusNoContent && code != http.StatusNotModified && compressible(h) {
		h.Set("Content-Encoding", "gzip")
		h.Del("Content-Length")
		g.gz = gzipPool.Get().(*gzip.Writer)
		g.gz.Reset(g.ResponseWriter)
	}
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipWriter) Write(b []byte) (int, error) {
	if !g.wroteHeader {
		if g.Header().Get("Content-Type") == "" {
			g.Header().Set("Content-Type", http.DetectContentType(b))
		}
		g.WriteHeader(http.StatusOK)
	}
	if g.gz != nil {
		return g.gz.Write(b)
	}
	return g.ResponseWriter.Write(b)
}

func (g *gzipWriter) close() {
	if g.gz != nil {
		_ = g.gz.Close()
		gzipPool.Put(g.gz)
	}
}
