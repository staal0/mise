package recipe

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Store caches parsed files and re-reads only those whose mtime or size changed.
type Store struct {
	Dir string

	mu    sync.Mutex
	files map[string]*cachedFile
}

type cachedFile struct {
	mod  time.Time
	size int64
	r    *Recipe
	err  error
}

func NewStore(dir string) *Store {
	return &Store{Dir: dir, files: map[string]*cachedFile{}}
}

func (s *Store) All() ([]*Entry, map[string]error, error) {
	paths, err := filepath.Glob(filepath.Join(s.Dir, "*.md"))
	if err != nil {
		return nil, nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := make(map[string]bool, len(paths))
	parsed := make(map[string]*Recipe, len(paths))
	errs := map[string]error{}
	for _, p := range paths {
		name := filepath.Base(p)
		if strings.EqualFold(name, "README.md") {
			continue
		}
		seen[name] = true
		r, err := s.load(name)
		if err != nil {
			errs[name] = err
			continue
		}
		parsed[name] = r
	}
	for name := range s.files {
		if !seen[name] {
			delete(s.files, name)
		}
	}
	return buildEntries(parsed), errs, nil
}

func (s *Store) Get(slug string) (*Entry, error) {
	// Refuse paths and glob patterns.
	if slug == "" || strings.HasPrefix(slug, ".") || strings.ContainsAny(slug, `/\*?[`) {
		return nil, os.ErrNotExist
	}
	names := []string{slug + ".md"}
	variants, _ := filepath.Glob(filepath.Join(s.Dir, slug+".*.md"))
	for _, p := range variants {
		if base, lang := splitFilename(filepath.Base(p)); base == slug && lang != "" {
			names = append(names, filepath.Base(p))
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	parsed := map[string]*Recipe{}
	for _, name := range names {
		r, err := s.load(name)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		parsed[name] = r
	}
	if len(parsed) == 0 {
		return nil, os.ErrNotExist
	}
	return buildEntries(parsed)[0], nil
}

// s.mu must be held.
func (s *Store) load(name string) (*Recipe, error) {
	path := filepath.Join(s.Dir, name)
	st, err := os.Stat(path)
	if err != nil {
		delete(s.files, name)
		return nil, err
	}
	if c := s.files[name]; c != nil && c.mod.Equal(st.ModTime()) && c.size == st.Size() {
		return c.r, c.err
	}
	c := &cachedFile{mod: st.ModTime(), size: st.Size()}
	c.r, c.err = parseFile(path, name)
	s.files[name] = c
	return c.r, c.err
}

func parseFile(path, name string) (*Recipe, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	slug, lang := splitFilename(name)
	parseSlug := slug
	if lang != "" {
		parseSlug = "" // don't invent a name; inherit it instead
	}
	r, err := Parse(parseSlug, data)
	if err != nil {
		return nil, err
	}
	r.Slug, r.Lang = slug, lang
	return r, nil
}
