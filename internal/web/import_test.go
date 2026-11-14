package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testPage = `<html><script type="application/ld+json">
{"@type":"Recipe","name":"Boller","recipeIngredient":["4 dl vand","1 tsk salt"],
 "recipeInstructions":["Rør dejen sammen og lad den hvile.","Bag bollerne i ovnen til de er gyldne."]}
</script></html>`

func TestImportPreviewThenSave(t *testing.T) {
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(testPage))
	}))
	defer site.Close()
	dir := t.TempDir()
	h := New(dir).Handler()

	post := func(path string, form url.Values) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Cookie", "lang=da")
		h.ServeHTTP(w, r)
		return w
	}

	w := post("/import", url.Values{"url": {site.URL}})
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, "Gem opskrift") || !strings.Contains(body, "4 dl vand") {
		t.Fatalf("preview: %d\n%s", w.Code, body)
	}
	if files, _ := filepath.Glob(filepath.Join(dir, "*.md")); len(files) != 0 {
		t.Fatalf("preview wrote files: %v", files)
	}

	md := "---\nname: Mine boller\n---\n\n## Ingredienser\n\n- 4 dl vand\n\n## Fremgangsmåde\n\n1. Bag.\n"
	w = post("/import/save", url.Values{"markdown": {md}, "url": {site.URL}})
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/r/mine-boller" {
		t.Fatalf("save: %d %s\n%s", w.Code, w.Header().Get("Location"), w.Body.String())
	}
	saved, err := os.ReadFile(filepath.Join(dir, "mine-boller.md"))
	if err != nil || !strings.Contains(string(saved), "## Ingredienser") {
		t.Fatalf("saved file: %v\n%s", err, saved)
	}

	post("/import/save", url.Values{"markdown": {"---\nname: Boller\n---\n"}})
	if body := post("/import", url.Values{"url": {site.URL}}).Body.String(); !strings.Contains(body, "allerede") {
		t.Error("no duplicate warning")
	}

	if w := post("/import/save", url.Values{"markdown": {"## Ingredienser\n- salt\n"}}); w.Code != 422 {
		t.Errorf("nameless save: %d", w.Code)
	}
}
