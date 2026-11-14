package web

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/v2/parser"
	mdhtml "github.com/yuin/goldmark/v2/renderer/html"

	"github.com/staal0/mise/internal/importer"
	"github.com/staal0/mise/internal/recipe"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// Raw HTML in recipe files is not rendered: imported text comes from websites.
var (
	mdParser   = parser.New()
	mdRenderer = mdhtml.New()
)

// Version is set by main from the build.
var Version = "dev"

func versionInfo() (label, url string) {
	v := strings.TrimPrefix(Version, "v")
	if v == "" || v == "dev" || !strings.ContainsAny(v[:1], "0123456789") {
		return Version, ""
	}
	return "v" + v, "https://github.com/staal0/mise/releases/tag/v" + v
}

type Server struct {
	Dir   string
	store *recipe.Store
	tmpl  *template.Template
}

func New(dir string) *Server {
	funcs := template.FuncMap{
		"duration": func(lang, d string) string { return recipe.FormatDuration(d, translations[lang]["hourUnit"]) },
		"segments": recipe.SplitQuantity,
		"markdown": func(s string) template.HTML {
			var b bytes.Buffer
			src := []byte(s)
			if err := mdRenderer.Render(&b, src, mdParser.Parse(src)); err != nil {
				return template.HTML(template.HTMLEscapeString(s))
			}
			return template.HTML(b.String())
		},
		"imageURL": imageURL,
		"num":      func(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) },
		"lower":    strings.ToLower,
		"yieldUnit": func(y string) string {
			segs := recipe.SplitQuantity(strings.TrimSpace(y))
			if len(segs) < 2 || !segs[0].IsQty {
				return ""
			}
			var rest strings.Builder
			for _, sg := range segs[1:] {
				rest.WriteString(sg.Text)
			}
			return strings.TrimSpace(rest.String())
		},
		"versionLabel": func() string { l, _ := versionInfo(); return l },
		"versionURL":   func() string { _, u := versionInfo(); return u },
		"lang":         recipe.Lang,
		"join":         strings.Join,
		"jsonld":       jsonLD,
		"searchText": func(e *recipe.Entry) string {
			var parts []string
			for _, r := range e.All() {
				// Descriptions are left out: long ones bloat the index.
				parts = append(parts, r.Name)
				parts = append(parts, r.Tags()...)
				for _, g := range r.Ingredients {
					parts = append(parts, g.Items...)
				}
			}
			return strings.ToLower(strings.Join(parts, " "))
		},
		"tagsAttr": func(e *recipe.Entry) string {
			var tags []string
			for _, r := range e.All() {
				tags = append(tags, r.Tags()...)
			}
			return strings.ToLower(strings.Join(tags, "|"))
		},
	}
	return &Server{
		Dir:   dir,
		store: recipe.NewStore(dir),
		tmpl:  template.Must(template.New("").Funcs(funcs).ParseFS(templateFS, "templates/*.html")),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	mux.Handle("GET /media/", http.StripPrefix("/media/", imagesOnly(http.FileServerFS(os.DirFS(s.Dir)))))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok\n")) })
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /r/{slug}", s.show)
	mux.HandleFunc("POST /import", s.importURL)
	mux.HandleFunc("POST /import/save", s.saveImport)
	return logRequests(gzipResponses(mux))
}

func imageURL(img string) string {
	if img == "" || strings.HasPrefix(img, "http://") || strings.HasPrefix(img, "https://") {
		return img
	}
	return "/media/" + img
}

var allowedMedia = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true, ".avif": true, ".svg": true}

func imagesOnly(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowedMedia[strings.ToLower(path.Ext(r.URL.Path))] {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "max-age=3600")
		h.ServeHTTP(w, r)
	})
}

type page struct {
	Title    string
	Lang     string // UI language, not the recipe's
	NextLang string
	T        map[string]string

	Cards   []card
	Tags    []string
	Errors  map[string]error
	Message string
	URL     string

	R         *recipe.Recipe
	BaseYield float64

	Markdown string
	ImageURL string
	Exists   bool
	Others   []version
}

type version struct{ URL, Label string }

type card struct {
	E *recipe.Entry
	R *recipe.Recipe
}

func newPage(r *http.Request, title string) *page {
	lang := uiLang(r)
	return &page{Title: title, Lang: lang, NextLang: nextLang(lang), T: translations[lang]}
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	s.renderIndex(w, r, http.StatusOK, "", "")
}

func (s *Server) renderIndex(w http.ResponseWriter, r *http.Request, status int, msg, url string) {
	entries, errs, err := s.store.All()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	p := newPage(r, "Mise en place")
	seen := map[string]bool{}
	for _, e := range entries {
		p.Cards = append(p.Cards, card{e, e.Pick(p.Lang)})
		for _, rc := range e.All() {
			for _, t := range rc.Tags() {
				if k := strings.ToLower(t); !seen[k] {
					seen[k] = true
					p.Tags = append(p.Tags, t)
				}
			}
		}
	}
	sortFold(p.Tags)
	p.Errors, p.Message, p.URL = errs, msg, url
	s.render(w, status, "index.html", p)
}

func (s *Server) show(w http.ResponseWriter, r *http.Request) {
	e, err := s.store.Get(r.PathValue("slug"))
	if errors.Is(err, os.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "Could not read "+r.PathValue("slug")+": "+err.Error(), 500)
		return
	}
	p := newPage(r, "")
	rc := e.Pick(p.Lang)
	if q, ok := r.URL.Query()["lang"]; ok {
		rc = e.Default
		if v, ok := e.Versions[q[0]]; ok {
			rc = v
		}
	}
	for _, v := range e.All() {
		if v == rc {
			continue
		}
		label, url := p.T["originalVersion"], "/r/"+e.Slug+"?lang="
		if v.Lang != "" {
			label, url = langName(v.Lang), "/r/"+e.Slug+"?lang="+v.Lang
		}
		p.Others = append(p.Others, version{url, label})
	}
	p.Title, p.R, p.BaseYield = rc.Name, rc, recipe.LeadingNumber(rc.RecipeYield)
	s.render(w, 200, "recipe.html", p)
}

// importURL only previews; saveImport writes the file.
func (s *Server) importURL(w http.ResponseWriter, r *http.Request) {
	t := translations[uiLang(r)]
	u := strings.TrimSpace(r.FormValue("url"))
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		s.renderIndex(w, r, 400, t["badURL"], u)
		return
	}
	found, err := importer.FromURL(r.Context(), u, nil)
	if err != nil {
		s.renderIndex(w, r, 422, t["importFailed"]+err.Error(), u)
		return
	}
	md, err := found[0].Recipe.Markdown()
	if err != nil {
		s.renderIndex(w, r, 500, t["importFailed"]+err.Error(), u)
		return
	}
	s.renderPreview(w, r, 200, found[0].Recipe, string(md), found[0].ImageURL, u, "")
}

func (s *Server) renderPreview(w http.ResponseWriter, r *http.Request, status int, rc *recipe.Recipe, md, imageURL, sourceURL, msg string) {
	p := newPage(r, rc.Name)
	p.R, p.Markdown, p.ImageURL, p.URL, p.Message = rc, md, imageURL, sourceURL, msg
	if _, err := os.Stat(filepath.Join(s.Dir, recipe.Slugify(rc.Name)+".md")); err == nil {
		p.Exists = true
	}
	s.render(w, status, "preview.html", p)
}

func (s *Server) saveImport(w http.ResponseWriter, r *http.Request) {
	t := translations[uiLang(r)]
	md := r.FormValue("markdown")
	img := strings.TrimSpace(r.FormValue("image"))
	if !strings.HasPrefix(img, "http://") && !strings.HasPrefix(img, "https://") {
		img = ""
	}
	rc, err := recipe.Parse("", []byte(md))
	if err == nil && strings.TrimSpace(rc.Name) == "" {
		err = errors.New(t["noName"])
	}
	if err != nil {
		if rc == nil {
			rc = &recipe.Recipe{}
		}
		s.renderPreview(w, r, 422, rc, md, img, r.FormValue("url"), t["saveFailed"]+err.Error())
		return
	}
	if err := importer.Save(r.Context(), s.Dir, importer.Found{Recipe: rc, ImageURL: img}, nil); err != nil {
		s.renderPreview(w, r, 500, rc, md, img, r.FormValue("url"), t["saveFailed"]+err.Error())
		return
	}
	http.Redirect(w, r, "/r/"+rc.Slug, http.StatusSeeOther)
}

func (s *Server) render(w http.ResponseWriter, status int, name string, data any) {
	var b bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&b, name, data); err != nil {
		log.Printf("template %s: %v", name, err)
		http.Error(w, "template error", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(b.Bytes())
}

func jsonLD(rc *recipe.Recipe) template.JS {
	m := map[string]any{"@context": "https://schema.org", "@type": "Recipe", "name": rc.Name}
	set := func(k string, v any) {
		switch t := v.(type) {
		case string:
			if t != "" {
				m[k] = t
			}
		case []string:
			if len(t) > 0 {
				m[k] = t
			}
		case recipe.StringList:
			if len(t) > 0 {
				m[k] = []string(t)
			}
		default:
			m[k] = v
		}
	}
	set("description", rc.Description)
	set("inLanguage", rc.Lang)
	set("recipeYield", rc.RecipeYield)
	set("prepTime", rc.PrepTime)
	set("cookTime", rc.CookTime)
	set("totalTime", rc.TotalTime)
	set("recipeCategory", rc.RecipeCategory)
	set("recipeCuisine", rc.RecipeCuisine)
	set("keywords", strings.Join(rc.Keywords, ", "))
	set("isBasedOn", rc.Source)
	if rc.Author != "" {
		m["author"] = map[string]string{"@type": "Person", "name": rc.Author}
	}
	var ingr []string
	for _, g := range rc.Ingredients {
		ingr = append(ingr, g.Items...)
	}
	set("recipeIngredient", ingr)
	var steps []any
	for _, g := range rc.Instructions {
		var list []any
		for _, it := range g.Items {
			list = append(list, map[string]string{"@type": "HowToStep", "text": it})
		}
		if g.Title != "" {
			steps = append(steps, map[string]any{"@type": "HowToSection", "name": g.Title, "itemListElement": list})
		} else {
			steps = append(steps, list...)
		}
	}
	if len(steps) > 0 {
		m["recipeInstructions"] = steps
	}
	b, _ := json.Marshal(m)
	return template.JS(b)
}

func sortFold(l []string) {
	slices.SortFunc(l, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
}

func logRequests(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			log.Printf("%s %s", r.Method, r.URL.Path)
		}
		h.ServeHTTP(w, r)
	})
}
