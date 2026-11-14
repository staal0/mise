package recipe

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Entry is one recipe with all its language versions:
//
//	frikadeller.md     the default version (any language)
//	frikadeller.en.md  an English version
//
// A language version only needs the fields it changes; anything it leaves
// out is taken from the default version.
type Entry struct {
	Slug     string
	Default  *Recipe
	Versions map[string]*Recipe // keyed by language code
}

// Pick returns the version in lang, or the default version.
func (e *Entry) Pick(lang string) *Recipe {
	if v, ok := e.Versions[lang]; ok {
		return v
	}
	return e.Default
}

// Langs returns the language codes of the extra versions, sorted.
func (e *Entry) Langs() []string {
	var out []string
	for l := range e.Versions {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

// All returns the default version followed by the language versions.
func (e *Entry) All() []*Recipe {
	out := []*Recipe{e.Default}
	for _, l := range e.Langs() {
		if v := e.Versions[l]; v != e.Default {
			out = append(out, v)
		}
	}
	return out
}

var variantRe = regexp.MustCompile(`^(.+)\.([a-z]{2,3})$`)

// splitFilename turns "frikadeller.en.md" into ("frikadeller", "en").
func splitFilename(name string) (slug, lang string) {
	base := strings.TrimSuffix(name, ".md")
	if m := variantRe.FindStringSubmatch(base); m != nil {
		return m[1], m[2]
	}
	return base, ""
}

// LoadAll reads every recipe in dir, grouping language versions. Files that
// fail to parse are reported in errs (keyed by filename) rather than
// aborting the load.
func LoadAll(dir string) (entries []*Entry, errs map[string]error, err error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil, nil, err
	}
	errs = map[string]error{}
	bySlug := map[string]*Entry{}
	for _, p := range paths {
		name := filepath.Base(p)
		if strings.EqualFold(name, "README.md") {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			errs[name] = err
			continue
		}
		slug, lang := splitFilename(name)
		parseSlug := slug
		if lang != "" {
			parseSlug = "" // don't invent a name; inherit it instead
		}
		r, err := Parse(parseSlug, data)
		if err != nil {
			errs[name] = err
			continue
		}
		r.Slug, r.Lang = slug, lang
		e := bySlug[slug]
		if e == nil {
			e = &Entry{Slug: slug, Versions: map[string]*Recipe{}}
			bySlug[slug] = e
		}
		if lang == "" {
			e.Default = r
		} else {
			e.Versions[lang] = r
		}
	}
	for _, e := range bySlug {
		if e.Default == nil {
			// Only language versions exist: the first one acts as default.
			e.Default = e.Versions[e.Langs()[0]]
			if e.Default.Name == "" {
				e.Default.Name = e.Slug
			}
		}
		for _, v := range e.Versions {
			if v != e.Default {
				inherit(v, e.Default)
			}
		}
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Default.Name) < strings.ToLower(entries[j].Default.Name)
	})
	return entries, errs, nil
}

// Load returns the entry for slug, or os.ErrNotExist.
func Load(dir, slug string) (*Entry, error) {
	entries, errs, err := LoadAll(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.Slug == slug {
			return e, nil
		}
	}
	if err := errs[slug+".md"]; err != nil {
		return nil, err
	}
	return nil, os.ErrNotExist
}

// inherit fills fields left empty in v from base.
func inherit(v, base *Recipe) {
	str := func(dst *string, src string) {
		if *dst == "" {
			*dst = src
		}
	}
	lst := func(dst *StringList, src StringList) {
		if len(*dst) == 0 {
			*dst = src
		}
	}
	grp := func(dst *[]Group, src []Group) {
		if len(*dst) == 0 {
			*dst = src
		}
	}
	str(&v.Name, base.Name)
	str(&v.Description, base.Description)
	str(&v.Author, base.Author)
	str(&v.RecipeYield, base.RecipeYield)
	str(&v.PrepTime, base.PrepTime)
	str(&v.CookTime, base.CookTime)
	str(&v.TotalTime, base.TotalTime)
	str(&v.Image, base.Image)
	str(&v.Source, base.Source)
	str(&v.Intro, base.Intro)
	str(&v.Notes, base.Notes)
	lst(&v.RecipeCategory, base.RecipeCategory)
	lst(&v.RecipeCuisine, base.RecipeCuisine)
	lst(&v.Keywords, base.Keywords)
	grp(&v.Ingredients, base.Ingredients)
	grp(&v.Instructions, base.Instructions)
}
