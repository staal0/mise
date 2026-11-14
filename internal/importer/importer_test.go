package importer

import (
	"github.com/staal0/mise/internal/recipe"
	"reflect"
	"strings"
	"testing"
)

const page = `<html><head>
<script type="application/ld+json">{"@type":"WebSite","name":"x"}</script>
<script type="application/ld+json">
{"@context":"https://schema.org","@graph":[
 {"@type":"WebPage"},
 {"@type":["Recipe"],"name":"Tarte &amp;amp; Co","recipeYield":["4","4 servings"],
  "prepTime":"PT15M","totalTime":"PT1H","keywords":"tart, french",
  "image":[{"@type":"ImageObject","url":"/img/tarte.jpg"}],
  "author":[{"@type":"Person","name":"Chef"}],
  "recipeIngredient":["200 g flour","1 egg, beaten"],
  "recipeInstructions":[
    {"@type":"HowToStep","text":"Preheat."},
    {"@type":"HowToSection","name":"Dough","itemListElement":[{"@type":"HowToStep","text":"Mix <b>well</b>."}]}
  ]}
]}
</script></head><body></body></html>`

func TestFromHTML(t *testing.T) {
	found, err := ParseHTML(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	r, img := found[0].Recipe, found[0].ImageURL
	if r.Name != "Tarte & Co" || r.RecipeYield != "4 servings" || r.Author != "Chef" || img != "/img/tarte.jpg" {
		t.Errorf("got %+v img=%q", r, img)
	}
	if !reflect.DeepEqual([]string(r.Keywords), []string{"tart", "french"}) {
		t.Errorf("keywords %v", r.Keywords)
	}
	if got := r.Ingredients[0].Items; !reflect.DeepEqual(got, []string{"200 g flour", "1 egg, beaten"}) {
		t.Errorf("ingredients %v", got)
	}
	if len(r.Instructions) != 2 || r.Instructions[1].Title != "Dough" || r.Instructions[1].Items[0] != "Mix well." {
		t.Errorf("instructions %#v", r.Instructions)
	}
}

func TestLooseJSON(t *testing.T) {
	const js = `{"id":"abc","name":"Soup","recipeYield":"","recipeServings":4,
	 "prepTime":"10 minutes","performTime":"1 hour","tags":[{"name":"winter"}],
	 "recipeCategory":[{"name":"dinner"}],"orgURL":"https://example.com/soup",
	 "recipeIngredient":[
	   {"display":"1 l stock","title":""},
	   {"title":"Topping","note":"","display":"","quantity":2,"unit":{"name":"tbsp"},"food":{"name":"cream"}},
	   {"note":"salt","display":""}],
	 "recipeInstructions":[{"text":"Boil\nit."},{"title":"Serve","text":"Top it."}],
	 "notes":[{"title":"Tip","text":"Freezes well."}]}`
	found, err := ParseJSON([]byte(js))
	if err != nil || len(found) != 1 {
		t.Fatal(found, err)
	}
	r := found[0].Recipe
	if r.RecipeYield != "4 servings" || r.PrepTime != "PT10M" || r.CookTime != "PT1H" || r.Source != "https://example.com/soup" {
		t.Errorf("%+v", r)
	}
	if len(r.Ingredients) != 2 || r.Ingredients[1].Title != "Topping" ||
		!reflect.DeepEqual(r.Ingredients[1].Items, []string{"2 tbsp cream", "salt"}) {
		t.Errorf("ingredients %#v", r.Ingredients)
	}
	if r.Instructions[0].Items[0] != "Boil it." || r.Instructions[1].Title != "Serve" {
		t.Errorf("instructions %#v", r.Instructions)
	}
	if !strings.Contains(r.Notes, "### Tip") {
		t.Errorf("notes %q", r.Notes)
	}
}

func TestDetectLang(t *testing.T) {
	da := &recipe.Recipe{Name: "Frikadeller", Instructions: []recipe.Group{{Items: []string{
		"Rør alle ingredienserne med en håndmikser.", "Steg dem på en pande med smør og olie til de er gennemstegte."}}}}
	en := &recipe.Recipe{Name: "Pancakes", Instructions: []recipe.Group{{Items: []string{
		"Whisk the flour and the eggs.", "Fry in butter until golden and add syrup to taste."}}}}
	if got := DetectLang(da); got != "da" {
		t.Errorf("danish: got %q", got)
	}
	if got := DetectLang(en); got != "en" {
		t.Errorf("english: got %q", got)
	}
	if got := DetectLang(&recipe.Recipe{Name: "Toast"}); got != "" {
		t.Errorf("too little text: got %q", got)
	}
}

func TestHTMLLangFallback(t *testing.T) {
	const pg = `<html lang="da-DK"><script type="application/ld+json">{"@type":"Recipe","name":"X","recipeIngredient":["1 egg"]}</script></html>`
	found, err := ParseHTML(strings.NewReader(pg))
	if err != nil || found[0].Recipe.HeadingLang != "da" {
		t.Fatalf("got %+v, %v", found, err)
	}
}

const microdataPage = `<html lang="da"><body>
<div itemscope itemtype="http://schema.org/WebPage">
 <article itemprop="mainEntity" itemscope itemtype="http://schema.org/Recipe">
 <div itemprop="description"><p>Gode boller.</p><div class="card"><h2>Boller</h2>Tid i alt 18 timer</div><h2 itemprop="name">Boller</h2></div>
  <span itemprop="author" itemscope itemtype="https://schema.org/Person"><span itemprop="name">Ann</span></span>
  <h1 itemprop="headline">Boller</h1>
  <img itemprop="image" src="/img/boller.jpg">
  <span itemprop="cookTime">PT18H</span><span itemprop="totalTime">PT45M</span>
  <meta itemprop="recipeYield" content="12">
  <span itemprop="keywords">Brød, Brunch</span>
  <ul><li itemprop="recipeIngredient">4 dl vand</li><li itemprop="recipeIngredient">1 tsk salt</li></ul>
  <div itemprop="recipeInstructions"><p>Rør dejen sammen.</p><p><strong>Bagning<br></strong>Bag <b>bollerne</b> i ovnen.</p></div>
  <div itemscope itemtype="http://schema.org/Comment"><span itemprop="text">Lækker!</span></div>
 </article>
</div></body></html>`

func TestMicrodata(t *testing.T) {
	found, err := ParseHTML(strings.NewReader(microdataPage))
	if err != nil {
		t.Fatal(err)
	}
	r := found[0].Recipe
	if r.Description != "Gode boller." {
		t.Errorf("description %q", r.Description)
	}
	if r.Name != "Boller" || r.Author != "Ann" || r.TotalTime != "PT18H" || r.CookTime != "PT45M" || r.RecipeYield != "12" || found[0].ImageURL != "/img/boller.jpg" {
		t.Errorf("got %+v img=%q", r, found[0].ImageURL)
	}
	if !reflect.DeepEqual([]string(r.Keywords), []string{"Brød", "Brunch"}) {
		t.Errorf("keywords %v", r.Keywords)
	}
	if !reflect.DeepEqual(r.Ingredients[0].Items, []string{"4 dl vand", "1 tsk salt"}) {
		t.Errorf("ingredients %v", r.Ingredients)
	}
	want := []recipe.Group{{Items: []string{"Rør dejen sammen."}}, {Title: "Bagning", Items: []string{"Bag bollerne i ovnen."}}}
	if !reflect.DeepEqual(r.Instructions, want) {
		t.Errorf("instructions %#v", r.Instructions)
	}
	if r.HeadingLang != "da" {
		t.Errorf("heading lang %q", r.HeadingLang)
	}
}

func TestServingsWordFollowsLanguage(t *testing.T) {
	const da = `{"name":"Suppe","recipeServings":4,"recipeIngredient":["1 l vand"],
	 "recipeInstructions":[{"text":"Kog vandet og kom grøntsagerne i, og lad det simre til de er møre."}]}`
	found, err := ParseJSON([]byte(da))
	if err != nil {
		t.Fatal(err)
	}
	if got := found[0].Recipe.RecipeYield; got != "4 portioner" {
		t.Errorf("danish: %q", got)
	}
	// Too little text to detect: <html lang> decides.
	const pg = `<html lang="da"><script type="application/ld+json">{"@type":"Recipe","name":"X","recipeServings":1,"recipeIngredient":["salt"]}</script></html>`
	found, err = ParseHTML(strings.NewReader(pg))
	if err != nil {
		t.Fatal(err)
	}
	if got := found[0].Recipe.RecipeYield; got != "1 portion" {
		t.Errorf("html lang: %q", got)
	}
}
