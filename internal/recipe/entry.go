package recipe

import (
	"regexp"
	"sort"
	"strings"
)

// Entry is x.md plus versions like x.en.md, which inherit the fields they leave out.
type Entry struct {
	Slug     string
	Default  *Recipe
	Versions map[string]*Recipe // by language
}

func (e *Entry) Pick(lang string) *Recipe {
	if v, ok := e.Versions[lang]; ok {
		return v
	}
	return e.Default
}

func (e *Entry) Langs() []string {
	var out []string
	for l := range e.Versions {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

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

func splitFilename(name string) (slug, lang string) {
	base := strings.TrimSuffix(name, ".md")
	if m := variantRe.FindStringSubmatch(base); m != nil {
		return m[1], m[2]
	}
	return base, ""
}

func LoadAll(dir string) (entries []*Entry, errs map[string]error, err error) {
	return NewStore(dir).All()
}

func Load(dir, slug string) (*Entry, error) {
	return NewStore(dir).Get(slug)
}

// buildEntries copies the parsed files, so cached files are never modified.
func buildEntries(files map[string]*Recipe) []*Entry {
	bySlug := map[string]*Entry{}
	for _, parsed := range files {
		r := new(Recipe)
		*r = *parsed
		e := bySlug[r.Slug]
		if e == nil {
			e = &Entry{Slug: r.Slug, Versions: map[string]*Recipe{}}
			bySlug[r.Slug] = e
		}
		if r.Lang == "" {
			e.Default = r
		} else {
			e.Versions[r.Lang] = r
		}
	}
	entries := make([]*Entry, 0, len(bySlug))
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
	return entries
}

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
