package recipe

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const sample = `---
name: Test
recipeYield: 4
keywords: sweet, quick
---

Intro text.

## Ingredients

- 200 g flour
- 2 eggs

### Sauce

- 1½ dl cream

## Instructions

1. Mix it.
2. Bake it
   until golden.

## Notes

Eat warm.
`

func TestParseRoundTrip(t *testing.T) {
	r, err := Parse("test", []byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if r.RecipeYield != "4" {
		t.Errorf("yield = %q", r.RecipeYield)
	}
	if !reflect.DeepEqual([]string(r.Keywords), []string{"sweet", "quick"}) {
		t.Errorf("keywords = %v", r.Keywords)
	}
	want := []Group{{Items: []string{"200 g flour", "2 eggs"}}, {Title: "Sauce", Items: []string{"1½ dl cream"}}}
	if !reflect.DeepEqual(r.Ingredients, want) {
		t.Errorf("ingredients = %#v", r.Ingredients)
	}
	if got := r.Instructions[0].Items; !reflect.DeepEqual(got, []string{"Mix it.", "Bake it until golden."}) {
		t.Errorf("steps = %#v", got)
	}
	if r.Intro != "Intro text." || r.Notes != "## Notes\n\nEat warm." {
		t.Errorf("intro=%q notes=%q", r.Intro, r.Notes)
	}

	md, err := r.Markdown()
	if err != nil {
		t.Fatal(err)
	}
	r2, err := Parse("test", md)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r, r2) {
		t.Errorf("round trip mismatch:\n%s", md)
	}
	if !strings.Contains(string(md), "keywords: [sweet, quick]") {
		t.Errorf("keywords not in flow style:\n%s", md)
	}
}

func TestNoFrontmatter(t *testing.T) {
	r, err := Parse("x", []byte("# Toast\n\n## Ingredients\n- bread\n\n## Method\nToast it.\n\nButter it.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "Toast" || len(r.Instructions[0].Items) != 2 {
		t.Errorf("%+v", r)
	}
}

func TestSplitQuantity(t *testing.T) {
	cases := map[string][]float64{
		"200 g flour":     {200},
		"1 1/2 cups milk": {1.5},
		"1½ dl cream":     {1.5},
		"½ lemon":         {0.5},
		"7,5 dl milk":     {7.5},
		"2-3 eggs":        {2, 3},
		"salt to taste":   nil,
	}
	for in, want := range cases {
		var got []float64
		var text strings.Builder
		for _, s := range SplitQuantity(in) {
			text.WriteString(s.Text)
			if s.IsQty {
				got = append(got, s.Qty)
			}
		}
		if !reflect.DeepEqual(got, want) || text.String() != in {
			t.Errorf("%q: got %v (%q), want %v", in, got, text.String(), want)
		}
	}
}

func TestDurations(t *testing.T) {
	for in, want := range map[string]string{
		"1 hour 30 minutes": "PT1H30M", "45 min": "PT45M", "1 time 15 minutter": "PT1H15M",
		"20": "PT20M", "pt10m": "PT10M", "1.5 hours": "PT1H30M",
	} {
		if got, ok := ToISODuration(in); !ok || got != want {
			t.Errorf("ToISODuration(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	if got := HumanDuration("PT1H30M"); got != "1 h 30 min" {
		t.Errorf("HumanDuration = %q", got)
	}
	if got := HumanDuration("about an hour"); got != "about an hour" {
		t.Errorf("HumanDuration passthrough = %q", got)
	}
}

func TestSlugify(t *testing.T) {
	if got := Slugify("Æbleskiver & Gløgg!"); got != "aebleskiver-and-gloegg" {
		t.Errorf("got %q", got)
	}
}

func TestLanguageVersions(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("pancakes.md", "---\nname: Pancakes\nimage: p.jpg\nprepTime: PT10M\n---\n## Ingredients\n- 2 eggs\n## Instructions\n1. Fry.\n")
	write("pancakes.da.md", "---\nname: Pandekager\n---\n## Fremgangsmåde\n1. Steg.\n")
	write("only.en.md", "---\nname: Only English\n---\n")

	entries, errs, err := LoadAll(dir)
	if err != nil || len(errs) > 0 {
		t.Fatal(err, errs)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(entries))
	}
	e, err := Load(dir, "pancakes")
	if err != nil {
		t.Fatal(err)
	}
	da := e.Pick("da")
	if da.Name != "Pandekager" || da.Lang != "da" || da.Filename() != "pancakes.da.md" {
		t.Errorf("da version: %+v", da)
	}
	// Inherited from the default file:
	if da.Image != "p.jpg" || da.PrepTime != "PT10M" || len(da.Ingredients) != 1 {
		t.Errorf("not inherited: %+v", da)
	}
	// Own content wins:
	if da.Instructions[0].Items[0] != "Steg." {
		t.Errorf("instructions: %+v", da.Instructions)
	}
	if e.Pick("en") != e.Default || e.Pick("da") == e.Default {
		t.Error("Pick should fall back to the default version")
	}
	if o, _ := Load(dir, "only"); o == nil || o.Default.Name != "Only English" {
		t.Errorf("version without default: %+v", o)
	}
}

func TestDanishHeadingsSurviveRoundTrip(t *testing.T) {
	r, err := Parse("x", []byte("---\nname: X\n---\n\n## Ingredienser\n\n- 1 æg\n\n## Fremgangsmåde\n\n1. Kog.\n"))
	if err != nil {
		t.Fatal(err)
	}
	md, _ := r.Markdown()
	if !strings.Contains(string(md), "## Ingredienser") || !strings.Contains(string(md), "## Fremgangsmåde") {
		t.Errorf("headings lost:\n%s", md)
	}
}
