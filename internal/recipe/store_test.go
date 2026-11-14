package recipe

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreSeesChanges(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		// Make each write visible even on filesystems with coarse timestamps.
		future := time.Now().Add(time.Duration(len(body)) * time.Second)
		_ = os.Chtimes(p, future, future)
	}
	s := NewStore(dir)
	names := func() map[string]string {
		t.Helper()
		entries, errs, err := s.All()
		if err != nil || len(errs) > 0 {
			t.Fatal(err, errs)
		}
		m := map[string]string{}
		for _, e := range entries {
			m[e.Slug] = e.Default.Name
		}
		return m
	}

	write("a.md", "---\nname: A\nimage: a.jpg\n---\n")
	write("a.da.md", "---\nname: A på dansk\n---\n")
	if got := names(); got["a"] != "A" {
		t.Fatalf("got %v", got)
	}

	// Edit the default file: the change shows up, and the Danish version
	// inherits the new image rather than a cached old one.
	write("a.md", "---\nname: A2\nimage: new.jpg\n---\n")
	if got := names(); got["a"] != "A2" {
		t.Errorf("edit not seen: %v", got)
	}
	e, err := s.Get("a")
	if err != nil {
		t.Fatal(err)
	}
	if da := e.Pick("da"); da.Image != "new.jpg" || da.Name != "A på dansk" {
		t.Errorf("stale inheritance: %+v", da)
	}

	// Add and delete files.
	write("b.md", "---\nname: B\n---\n")
	if got := names(); got["b"] != "B" {
		t.Errorf("new file not seen: %v", got)
	}
	if err := os.Remove(filepath.Join(dir, "b.md")); err != nil {
		t.Fatal(err)
	}
	if got := names(); len(got) != 1 {
		t.Errorf("deleted file still listed: %v", got)
	}
	if _, err := s.Get("b"); !os.IsNotExist(err) {
		t.Errorf("Get deleted: %v", err)
	}

	// Broken file: reported, then fixed.
	write("c.md", "---\nname: [unclosed\n---\n")
	if _, errs, _ := s.All(); errs["c.md"] == nil {
		t.Error("parse error not reported")
	}
	write("c.md", "---\nname: Fixed C file\n---\n")
	if got := names(); got["c"] != "Fixed C file" {
		t.Errorf("fix not seen: %v", got)
	}
}

func TestStoreGetRejectsBadSlugs(t *testing.T) {
	s := NewStore(t.TempDir())
	for _, slug := range []string{"", "..", "../etc/passwd", "a/b", "*", "a?", "[x]", ".hidden"} {
		if _, err := s.Get(slug); !os.IsNotExist(err) {
			t.Errorf("%q: %v", slug, err)
		}
	}
}
