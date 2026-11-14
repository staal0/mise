// Package recipe defines the on-disk recipe format: Markdown with YAML
// frontmatter whose field names follow schema.org/Recipe.
package recipe

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Recipe is a single recipe. Frontmatter fields use schema.org/Recipe names
// (except Source, which maps to schema.org's isBasedOn).
type Recipe struct {
	Name           string     `yaml:"name"`
	Description    string     `yaml:"description,omitempty"`
	Author         string     `yaml:"author,omitempty"`
	RecipeYield    string     `yaml:"recipeYield,omitempty"`
	PrepTime       string     `yaml:"prepTime,omitempty"`
	CookTime       string     `yaml:"cookTime,omitempty"`
	TotalTime      string     `yaml:"totalTime,omitempty"`
	RecipeCategory StringList `yaml:"recipeCategory,omitempty"`
	RecipeCuisine  StringList `yaml:"recipeCuisine,omitempty"`
	Keywords       StringList `yaml:"keywords,omitempty"`
	Image          string     `yaml:"image,omitempty"`
	Source         string     `yaml:"source,omitempty"`

	Slug         string  `yaml:"-"`
	Lang         string  `yaml:"-"` // from a "<slug>.<lang>.md" filename; "" for the default file
	HeadingLang  string  `yaml:"-"` // language for section headings when writing; defaults to Lang
	Intro        string  `yaml:"-"` // markdown before the first "##" section
	Ingredients  []Group `yaml:"-"`
	Instructions []Group `yaml:"-"`
	Notes        string  `yaml:"-"` // any other "##" sections, as raw markdown
}

// Group is an optionally titled list of ingredients or steps ("### Sauce").
type Group struct {
	Title string
	Items []string
}

// Tags returns categories, cuisines and keywords combined, deduplicated.
func (r *Recipe) Tags() []string {
	seen := map[string]bool{}
	var out []string
	for _, l := range [][]string{r.RecipeCategory, r.RecipeCuisine, r.Keywords} {
		for _, t := range l {
			k := strings.ToLower(t)
			if t != "" && !seen[k] {
				seen[k] = true
				out = append(out, t)
			}
		}
	}
	return out
}

// StringList accepts either a YAML list or a comma-separated string.
type StringList []string

func (s *StringList) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		*s = SplitList(n.Value)
		return nil
	case yaml.SequenceNode:
		var l []string
		if err := n.Decode(&l); err != nil {
			return err
		}
		*s = l
		return nil
	}
	return fmt.Errorf("line %d: expected a string or a list", n.Line)
}

func (s StringList) MarshalYAML() (any, error) {
	n := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
	for _, v := range s {
		n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: v})
	}
	return n, nil
}

// SplitList splits a comma-separated string into trimmed, non-empty parts.
func SplitList(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

var (
	numberedRe = regexp.MustCompile(`^\d+[.)]\s+`)
	bulletRe   = regexp.MustCompile(`^[-*+]\s+`)
)

func sectionKind(heading string) string {
	h := strings.ToLower(strings.TrimRight(strings.TrimSpace(heading), ":"))
	switch h {
	case "ingredients", "ingredienser":
		return "ingredients"
	case "instructions", "directions", "method", "steps", "preparation",
		"fremgangsmåde", "fremgangsmaade", "instruktioner":
		return "instructions"
	}
	return "other"
}

// Parse reads a recipe file. slug is the filename without ".md".
func Parse(slug string, data []byte) (*Recipe, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	r := &Recipe{Slug: slug}
	body := text
	if strings.HasPrefix(text, "---\n") {
		rest := text[4:]
		end := strings.Index(rest, "\n---")
		if end < 0 {
			return nil, fmt.Errorf("frontmatter starts with --- but is never closed")
		}
		if err := yaml.Unmarshal([]byte(rest[:end]), r); err != nil {
			return nil, fmt.Errorf("frontmatter: %w", err)
		}
		body = rest[end+4:]
		if i := strings.IndexByte(body, '\n'); i >= 0 {
			body = body[i+1:]
		} else {
			body = ""
		}
	}

	var intro, notes strings.Builder
	section := ""
	var cur *Group
	prevBlank := true
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			if r.Name == "" {
				r.Name = strings.TrimSpace(line[2:])
			}
			continue
		}
		if strings.HasPrefix(line, "## ") {
			section = sectionKind(line[3:])
			if section != "other" && r.HeadingLang == "" {
				// Remember the heading language so a re-save keeps it.
				switch strings.ToLower(strings.TrimSpace(line[3:])) {
				case "ingredienser", "fremgangsmåde", "fremgangsmaade", "instruktioner":
					r.HeadingLang = "da"
				}
			}
			cur = nil
			prevBlank = true
			if section == "other" {
				notes.WriteString(line + "\n")
			}
			continue
		}
		switch section {
		case "":
			intro.WriteString(line + "\n")
		case "other":
			notes.WriteString(line + "\n")
		case "ingredients", "instructions":
			groups := &r.Ingredients
			if section == "instructions" {
				groups = &r.Instructions
			}
			switch {
			case strings.HasPrefix(t, "### "):
				*groups = append(*groups, Group{Title: strings.TrimSpace(t[4:])})
				cur = &(*groups)[len(*groups)-1]
			case t == "":
				prevBlank = true
				continue
			default:
				if cur == nil {
					*groups = append(*groups, Group{})
					cur = &(*groups)[len(*groups)-1]
				}
				isItem := numberedRe.MatchString(t) || bulletRe.MatchString(t)
				item := bulletRe.ReplaceAllString(numberedRe.ReplaceAllString(t, ""), "")
				continuation := !isItem && len(cur.Items) > 0 &&
					(!prevBlank || line != t /* indented */)
				if continuation {
					cur.Items[len(cur.Items)-1] += " " + item
				} else {
					cur.Items = append(cur.Items, item)
				}
			}
		}
		prevBlank = t == ""
	}
	r.Intro = strings.TrimSpace(intro.String())
	r.Notes = strings.TrimSpace(notes.String())
	r.Ingredients = dropEmpty(r.Ingredients)
	r.Instructions = dropEmpty(r.Instructions)
	if r.Name == "" {
		r.Name = slug
	}
	return r, nil
}

func dropEmpty(gs []Group) []Group {
	var out []Group
	for _, g := range gs {
		if g.Title != "" || len(g.Items) > 0 {
			out = append(out, g)
		}
	}
	return out
}

// Headings are the section names written to recipe files.
type Headings struct{ Ingredients, Instructions, Notes, Equipment string }

// SectionHeadings returns headings in the recipe's language (English default).
func SectionHeadings(lang string) Headings {
	if Lang(lang) == "da" {
		return Headings{"Ingredienser", "Fremgangsmåde", "Noter", "Udstyr"}
	}
	return Headings{"Ingredients", "Instructions", "Notes", "Equipment"}
}

// Lang normalizes a language tag to its lowercase base ("da-DK" → "da").
func Lang(tag string) string {
	base, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(tag)), "-")
	base, _, _ = strings.Cut(base, "_")
	return base
}

// Markdown renders the recipe back to the file format.
func (r *Recipe) Markdown() ([]byte, error) {
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	b.WriteString("---\n")
	if err := enc.Encode(r); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	b.WriteString("---\n")
	if r.Intro != "" {
		b.WriteString("\n" + r.Intro + "\n")
	}
	writeGroups := func(heading string, gs []Group, numbered bool) {
		if len(gs) == 0 {
			return
		}
		b.WriteString("\n## " + heading + "\n")
		for _, g := range gs {
			if g.Title != "" {
				b.WriteString("\n### " + g.Title + "\n")
			}
			b.WriteString("\n")
			for i, it := range g.Items {
				if numbered {
					fmt.Fprintf(&b, "%d. %s\n", i+1, it)
				} else {
					b.WriteString("- " + it + "\n")
				}
			}
		}
	}
	hl := r.HeadingLang
	if hl == "" {
		hl = r.Lang
	}
	h := SectionHeadings(hl)
	writeGroups(h.Ingredients, r.Ingredients, false)
	writeGroups(h.Instructions, r.Instructions, true)
	if r.Notes != "" {
		b.WriteString("\n" + r.Notes + "\n")
	}
	return b.Bytes(), nil
}

var danish = strings.NewReplacer("æ", "ae", "ø", "oe", "å", "aa", "ä", "a", "ö", "o", "ü", "u",
	"é", "e", "è", "e", "ê", "e", "á", "a", "à", "a", "í", "i", "ó", "o", "ú", "u", "ñ", "n", "ç", "c", "ß", "ss", "&", " and ")
var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify turns a recipe name into a filename-safe slug.
func Slugify(name string) string {
	s := nonSlug.ReplaceAllString(danish.Replace(strings.ToLower(name)), "-")
	s = strings.Trim(s, "-")
	if len(s) > 80 {
		s = strings.Trim(s[:80], "-")
	}
	if s == "" {
		s = "recipe"
	}
	return s
}

// UniqueSlug returns a slug for name that doesn't collide with existing files.
func UniqueSlug(dir, name string) string {
	base := Slugify(name)
	slug := base
	for i := 2; ; i++ {
		if _, err := os.Stat(filepath.Join(dir, slug+".md")); os.IsNotExist(err) {
			return slug
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
}

// Filename is the recipe's file name: "<slug>.md" or "<slug>.<lang>.md".
func (r *Recipe) Filename() string {
	if r.Lang != "" {
		return r.Slug + "." + r.Lang + ".md"
	}
	return r.Slug + ".md"
}

// Save writes the recipe to its file in dir. r.Slug must be set.
func Save(dir string, r *Recipe) error {
	data, err := r.Markdown()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, r.Filename()), data, 0o644)
}
