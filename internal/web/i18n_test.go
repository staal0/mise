package web

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUILang(t *testing.T) {
	for _, c := range []struct{ accept, cookie, want string }{
		{"da-DK,da;q=0.9,en;q=0.8", "", "da"},
		{"en-US,en;q=0.9", "", "en"},
		{"de-DE,da;q=0.5", "", "da"},
		{"fr-FR", "", "en"},
		{"da-DK", "en", "en"},
		{"", "da", "da"},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Accept-Language", c.accept)
		if c.cookie != "" {
			r.Header.Set("Cookie", "lang="+c.cookie)
		}
		if got := uiLang(r); got != c.want {
			t.Errorf("%+v: got %q", c, got)
		}
	}
}

// Every language must define every key, or the UI shows blanks.
func TestTranslationsComplete(t *testing.T) {
	for lang, m := range translations {
		for k := range translations["en"] {
			if strings.TrimSpace(m[k]) == "" {
				t.Errorf("%s: missing %q", lang, k)
			}
		}
	}
}

func TestPagesRender(t *testing.T) {
	h := New("../../samples").Handler()
	for _, lang := range []string{"en", "da"} {
		for _, path := range []string{"/", "/r/pancakes"} {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", path, nil)
			r.Header.Set("Cookie", "lang="+lang)
			h.ServeHTTP(w, r)
			if w.Code != 200 || !strings.Contains(w.Body.String(), `<html lang="`+lang+`">`) {
				t.Errorf("%s %s: %d", lang, path, w.Code)
			}
		}
	}
}

func TestNotesMarkdownIsRenderedSafely(t *testing.T) {
	dir := t.TempDir()
	md := "---\nname: X\n---\n\n## Notes\n\nUse **cold** butter.\n\n<script>alert(1)</script>\n"
	if err := os.WriteFile(filepath.Join(dir, "x.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	New(dir).Handler().ServeHTTP(w, httptest.NewRequest("GET", "/r/x", nil))
	body := w.Body.String()
	if !strings.Contains(body, "<strong>cold</strong>") {
		t.Error("markdown not rendered")
	}
	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Error("raw HTML from the recipe file was rendered")
	}
}

func TestVersionFooter(t *testing.T) {
	defer func(v string) { Version = v }(Version)
	for _, c := range []struct{ version, label, link string }{
		{"dev", "mise dev", ""},
		{"1.0.1", "v1.0.1", "https://github.com/staal0/mise/releases/tag/v1.0.1"},
		{"v2.3.0", "v2.3.0", "https://github.com/staal0/mise/releases/tag/v2.3.0"},
	} {
		Version = c.version
		w := httptest.NewRecorder()
		New("../../samples").Handler().ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		body := w.Body.String()
		if !strings.Contains(body, c.label) || (c.link != "" && !strings.Contains(body, c.link)) {
			t.Errorf("%s: footer missing %q / %q", c.version, c.label, c.link)
		}
	}
}
