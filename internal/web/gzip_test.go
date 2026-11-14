package web

import (
	"compress/gzip"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGzip(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pic.jpg"), []byte("\xff\xd8\xff\xe0 fake jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := New("../../samples").Handler()
	media := New(dir).Handler()

	for _, c := range []struct {
		path, accept string
		gzipped      bool
		media        bool
	}{
		{"/", "gzip, deflate, br", true, false},
		{"/r/pancakes", "gzip", true, false},
		{"/static/style.css", "gzip", true, false},
		{"/static/app.js", "gzip", true, false},
		{"/", "", false, false},
		{"/", "gzip;q=0", false, false},
		{"/media/pic.jpg", "gzip", false, true},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", c.path, nil)
		if c.accept != "" {
			r.Header.Set("Accept-Encoding", c.accept)
		}
		if c.media {
			media.ServeHTTP(w, r)
		} else {
			h.ServeHTTP(w, r)
		}
		if w.Code != 200 {
			t.Errorf("%s: status %d", c.path, w.Code)
			continue
		}
		got := w.Header().Get("Content-Encoding") == "gzip"
		if got != c.gzipped {
			t.Errorf("%s (Accept-Encoding %q): gzipped=%v, want %v", c.path, c.accept, got, c.gzipped)
			continue
		}
		if got {
			zr, err := gzip.NewReader(w.Body)
			if err != nil {
				t.Fatalf("%s: %v", c.path, err)
			}
			body, err := io.ReadAll(zr)
			if err != nil || len(body) == 0 {
				t.Errorf("%s: bad gzip body: %v", c.path, err)
			}
			if c.path == "/" && !strings.Contains(string(body), "Pancakes") {
				t.Errorf("index body wrong after decompressing")
			}
		}
	}
}
