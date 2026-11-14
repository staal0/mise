package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	xhtml "golang.org/x/net/html"

	"github.com/staal0/mise/internal/recipe"
)

const userAgent = "mise-recipes/1.0 (+https://github.com/staal0/mise)"

var Client = &http.Client{Timeout: 30 * time.Second}

type Found struct {
	Recipe       *recipe.Recipe
	ImageURL     string
	imageFS      fs.FS
	imagePath    string
	explicitLang bool
}

func FromURL(ctx context.Context, rawURL string, header http.Header) ([]Found, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range header {
		req.Header[k] = v
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return nil, err
	}
	ct, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	var found []Found
	if strings.Contains(ct, "json") {
		found, err = ParseJSON(body)
	} else {
		found, err = ParseHTML(bytes.NewReader(body))
	}
	if err != nil {
		return nil, err
	}
	for i := range found {
		f := &found[i]
		if f.Recipe.Source == "" && !strings.Contains(ct, "json") {
			f.Recipe.Source = rawURL
		}
		if f.ImageURL != "" {
			if u, err := resp.Request.URL.Parse(f.ImageURL); err == nil {
				f.ImageURL = u.String()
			}
		}
	}
	return found, nil
}

func FromPath(p string) ([]Found, error) {
	st, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	switch {
	case st.IsDir():
		return fromFS(os.DirFS(p), ".")
	case strings.EqualFold(filepath.Ext(p), ".zip"):
		zr, err := zip.OpenReader(p)
		if err != nil {
			return nil, err
		}
		// Not closed: images are read from the archive when saving.
		return fromFS(zr, ".")
	default:
		return fromFS(os.DirFS(filepath.Dir(p)), filepath.Base(p))
	}
}

func fromFS(fsys fs.FS, root string) ([]Found, error) {
	var all []Found
	var errs []string
	err := fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			return err
		}
		ext := strings.ToLower(path.Ext(p))
		if ext != ".json" && ext != ".html" && ext != ".htm" {
			return nil
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		var found []Found
		if ext == ".json" {
			found, err = ParseJSON(data)
		} else {
			found, err = ParseHTML(bytes.NewReader(data))
		}
		if err != nil {
			if root == p { // a single explicitly named file: report it
				return err
			}
			errs = append(errs, p+": "+err.Error())
			return nil
		}
		for i := range found {
			found[i].findLocalImage(fsys, path.Dir(p))
		}
		all = append(all, found...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		if len(errs) > 0 {
			return nil, errors.New(strings.Join(errs, "; "))
		}
		return nil, errors.New("no recipes found")
	}
	return all, nil
}

func (f *Found) findLocalImage(fsys fs.FS, dir string) {
	if strings.HasPrefix(f.ImageURL, "http://") || strings.HasPrefix(f.ImageURL, "https://") {
		return
	}
	ref := f.ImageURL
	f.ImageURL = ""
	if ref != "" && isImageExt(strings.ToLower(path.Ext(ref))) {
		if p := path.Join(dir, ref); exists(fsys, p) {
			f.imageFS, f.imagePath = fsys, p
			return
		}
	}
	var candidates []string
	for _, d := range []string{dir, path.Join(dir, "images")} {
		entries, _ := fs.ReadDir(fsys, d)
		for _, e := range entries {
			if !e.IsDir() && isImageExt(strings.ToLower(path.Ext(e.Name()))) {
				candidates = append(candidates, path.Join(d, e.Name()))
			}
		}
	}
	// Many images means a shared folder; don't guess.
	if len(candidates) == 0 || len(candidates) > 3 {
		return
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return strings.Contains(candidates[i], "original") && !strings.Contains(candidates[j], "original")
	})
	f.imageFS, f.imagePath = fsys, candidates[0]
}

func exists(fsys fs.FS, p string) bool {
	_, err := fs.Stat(fsys, p)
	return err == nil
}

func ParseHTML(body io.Reader) ([]Found, error) {
	doc, err := xhtml.Parse(body)
	if err != nil {
		return nil, err
	}
	var found []Found
	pageLang := ""
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && n.Data == "html" {
			for _, a := range n.Attr {
				if a.Key == "lang" {
					pageLang = recipe.Lang(a.Val)
				}
			}
		}
		if n.Type == xhtml.ElementNode && n.Data == "script" && n.FirstChild != nil {
			for _, a := range n.Attr {
				if a.Key == "type" && strings.Contains(strings.ToLower(a.Val), "ld+json") {
					var v any
					if json.Unmarshal([]byte(n.FirstChild.Data), &v) == nil {
						found = append(found, collect(v, false)...)
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if len(found) == 0 {
		found = collect(microdata(doc), false)
	}
	if len(found) == 0 {
		return nil, errors.New("no schema.org Recipe found on the page (neither JSON-LD nor Microdata)")
	}
	if r := found[0].Recipe; pageLang != "" && r.HeadingLang != pageLang {
		// <html lang> beats detection, but not an explicit inLanguage.
		if !found[0].explicitLang {
			r.HeadingLang = pageLang
			r.Notes = retitleNotes(r.Notes, pageLang)
		}
	}
	return found[:1], nil // a page is one recipe; extra matches are usually duplicates
}

func ParseJSON(data []byte) ([]Found, error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	found := collect(v, true)
	if len(found) == 0 {
		return nil, errors.New("no recipe found in JSON")
	}
	return found, nil
}

// loose also accepts objects without @type that look like recipes.
func collect(v any, loose bool) []Found {
	var out []Found
	var walk func(any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			if isType(t["@type"], "Recipe") || (loose && t["@type"] == nil && looksLikeRecipe(t)) {
				out = append(out, convert(t))
				return
			}
			for _, e := range t {
				walk(e)
			}
		case []any:
			for _, e := range t {
				walk(e)
			}
		}
	}
	walk(v)
	return out
}

func looksLikeRecipe(m map[string]any) bool {
	if str(m["name"]) == "" {
		return false
	}
	for _, k := range []string{"recipeIngredient", "recipeInstructions", "ingredients"} {
		if m[k] != nil {
			return true
		}
	}
	return false
}

func isType(v any, want string) bool {
	switch t := v.(type) {
	case string:
		return t == want || strings.HasSuffix(t, "/"+want)
	case []any:
		for _, e := range t {
			if isType(e, want) {
				return true
			}
		}
	}
	return false
}

var (
	tagRe   = regexp.MustCompile(`<[^>]*>`)
	spaceRe = regexp.MustCompile(`\s+`)
	punctRe = regexp.MustCompile(`\s+([.,;:!?])`)
)

func clean(s string) string {
	for i := 0; i < 2; i++ { // sites often double-encode entities
		s = html.UnescapeString(s)
	}
	s = tagRe.ReplaceAllString(s, " ")
	s = spaceRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(punctRe.ReplaceAllString(s, "$1"))
}

func str(v any) string {
	switch t := v.(type) {
	case string:
		return clean(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case []any:
		for _, e := range t {
			if s := str(e); s != "" {
				return s
			}
		}
	case map[string]any:
		for _, k := range []string{"name", "text", "url", "@id"} {
			if s := str(t[k]); s != "" {
				return s
			}
		}
	}
	return ""
}

func first(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil && str(v) != "" {
			return v
		}
		if l, ok := m[k].([]any); ok && len(l) > 0 {
			return l
		}
	}
	return nil
}

func list(v any) []string {
	switch t := v.(type) {
	case string:
		return recipe.SplitList(clean(t))
	case []any:
		var out []string
		for _, e := range t {
			if s := str(e); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	if s := str(v); s != "" {
		return []string{s}
	}
	return nil
}

func convert(m map[string]any) Found {
	r := &recipe.Recipe{
		Name:           str(first(m, "name", "headline")),
		Description:    str(m["description"]),
		Author:         str(m["author"]),
		RecipeYield:    recipe.Yield(yield(m)),
		RecipeCategory: list(m["recipeCategory"]),
		RecipeCuisine:  list(m["recipeCuisine"]),
		Keywords:       list(first(m, "keywords", "tags")),
		Source:         str(first(m, "isBasedOn", "orgURL", "source")),
	}
	r.PrepTime, _ = recipe.ToISODuration(str(m["prepTime"]))
	r.CookTime, _ = recipe.ToISODuration(str(first(m, "cookTime", "performTime")))
	r.TotalTime, _ = recipe.ToISODuration(str(m["totalTime"]))
	// Some sources swap cook and total time.
	if c, ok1 := recipe.ParseISODuration(r.CookTime); ok1 {
		if t, ok2 := recipe.ParseISODuration(r.TotalTime); ok2 && t < c {
			r.CookTime, r.TotalTime = r.TotalTime, r.CookTime
		}
	}
	r.Ingredients = groups(first(m, "recipeIngredient", "ingredients"), ingredientText)
	r.Instructions = instructions(m["recipeInstructions"])
	r.HeadingLang = recipe.Lang(str(m["inLanguage"]))
	explicit := r.HeadingLang != ""
	if !explicit {
		r.HeadingLang = DetectLang(r)
	}
	r.Notes = notes(m, r.HeadingLang)
	return Found{Recipe: r, ImageURL: image(m["image"]), explicitLang: explicit}
}

var numRe = regexp.MustCompile(`\d+(?:[.,]\d+)?`)

func yield(m map[string]any) string {
	texts := []string{str(m["recipeYield"])}
	if l, ok := m["recipeYield"].([]any); ok {
		texts = nil
		for _, e := range l {
			texts = append(texts, str(e))
		}
	}
	texts = append(texts, str(m["recipeYieldQuantity"]), str(m["recipeServings"]))
	for _, s := range texts {
		if n := numRe.FindString(s); n != "" && strings.Trim(n, "0.,") != "" {
			return strings.Replace(n, ",", ".", 1)
		}
	}
	return texts[0]
}

func image(v any) string {
	switch t := v.(type) {
	case []any:
		if len(t) > 0 {
			return image(t[0])
		}
	case map[string]any:
		if s := str(t["url"]); s != "" {
			return s
		}
		return str(t["contentUrl"])
	case string:
		return strings.TrimSpace(t)
	}
	return ""
}

func ingredientText(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return str(v)
	}
	if s := str(m["display"]); s != "" {
		return s
	}
	var parts []string
	if q, ok := m["quantity"].(float64); ok && q > 0 {
		parts = append(parts, strconv.FormatFloat(q, 'f', -1, 64))
	}
	unit, food := str(m["unit"]), str(m["food"])
	if unit != "" || food != "" {
		for _, s := range []string{unit, food, str(m["note"])} {
			if s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " ")
	}
	for _, k := range []string{"note", "originalText", "text", "name"} {
		if s := str(m[k]); s != "" {
			return s
		}
	}
	return ""
}

func groups(v any, text func(any) string) []recipe.Group {
	var items []any
	switch t := v.(type) {
	case []any:
		items = t
	case string:
		for _, l := range strings.Split(t, "\n") {
			items = append(items, l)
		}
	default:
		return nil
	}
	var out []recipe.Group
	for _, it := range items {
		title := ""
		if m, ok := it.(map[string]any); ok {
			title = str(m["title"])
		}
		// "### " lines come from HTML sub-headings.
		if line, ok := it.(string); ok && strings.HasPrefix(strings.TrimSpace(line), "### ") {
			out = append(out, recipe.Group{Title: clean(strings.TrimSpace(line)[4:])})
			continue
		}
		if title != "" || len(out) == 0 {
			out = append(out, recipe.Group{Title: title})
		}
		if s := text(it); s != "" {
			out[len(out)-1].Items = append(out[len(out)-1].Items, s)
		}
	}
	return dropEmpty(out)
}

func dropEmpty(gs []recipe.Group) []recipe.Group {
	var out []recipe.Group
	for _, g := range gs {
		if g.Title != "" || len(g.Items) > 0 {
			out = append(out, g)
		}
	}
	return out
}

func stepText(v any) string {
	if m, ok := v.(map[string]any); ok {
		if s := str(m["text"]); s != "" {
			return s
		}
		return str(m["name"])
	}
	return str(v)
}

func instructions(v any) []recipe.Group {
	switch t := v.(type) {
	case string:
		return groups(t, stepText)
	case map[string]any:
		return groups([]any{t}, stepText)
	case []any:
		var out []recipe.Group
		var loose []any
		flush := func() {
			out = append(out, groups(loose, stepText)...)
			loose = nil
		}
		for _, e := range t {
			m, ok := e.(map[string]any)
			if ok && isType(m["@type"], "HowToSection") {
				flush()
				g := recipe.Group{Title: str(m["name"])}
				for _, s := range flatten(m["itemListElement"]) {
					if s := stepText(s); s != "" {
						g.Items = append(g.Items, s)
					}
				}
				out = append(out, g)
				continue
			}
			loose = append(loose, e)
		}
		flush()
		return dropEmpty(out)
	}
	return nil
}

func flatten(v any) []any {
	if l, ok := v.([]any); ok {
		var out []any
		for _, e := range l {
			out = append(out, flatten(e)...)
		}
		return out
	}
	if v == nil {
		return nil
	}
	return []any{v}
}

func notes(m map[string]any, lang string) string {
	h := recipe.SectionHeadings(lang)
	var b strings.Builder
	if tools := list(first(m, "tool", "tools")); len(tools) > 0 {
		b.WriteString("## " + h.Equipment + "\n\n")
		for _, t := range tools {
			b.WriteString("- " + t + "\n")
		}
	}
	var ns []string
	for _, n := range flatten(m["notes"]) {
		if nm, ok := n.(map[string]any); ok {
			txt, _ := nm["text"].(string)
			s := strings.TrimSpace(txt)
			if t := str(nm["title"]); t != "" {
				s = "### " + t + "\n\n" + s
			}
			ns = append(ns, s)
		} else if s := str(n); s != "" {
			ns = append(ns, s)
		}
	}
	if len(ns) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("## " + h.Notes + "\n\n" + strings.Join(ns, "\n\n") + "\n")
	}
	return strings.TrimSpace(b.String())
}

func retitleNotes(notes, lang string) string {
	to := recipe.SectionHeadings(lang)
	for _, from := range []recipe.Headings{recipe.SectionHeadings("en"), recipe.SectionHeadings("da")} {
		notes = strings.ReplaceAll(notes, "## "+from.Notes+"\n", "## "+to.Notes+"\n")
		notes = strings.ReplaceAll(notes, "## "+from.Equipment+"\n", "## "+to.Equipment+"\n")
	}
	return notes
}

var (
	daWords = map[string]bool{"og": true, "med": true, "til": true, "på": true, "af": true, "en": true, "et": true,
		"det": true, "den": true, "er": true, "ikke": true, "skal": true, "så": true, "eller": true, "lidt": true, "minutter": true}
	enWords = map[string]bool{"and": true, "the": true, "with": true, "to": true, "of": true, "into": true, "until": true,
		"for": true, "or": true, "is": true, "minutes": true, "add": true, "a": true, "it": true}
	wordRe = regexp.MustCompile(`[\p{L}]+`)
)

func DetectLang(r *recipe.Recipe) string {
	var text strings.Builder
	text.WriteString(r.Name + " " + r.Description)
	for _, gs := range [][]recipe.Group{r.Ingredients, r.Instructions} {
		for _, g := range gs {
			text.WriteString(" " + strings.Join(g.Items, " "))
		}
	}
	da, en := 0, 0
	for _, w := range wordRe.FindAllString(strings.ToLower(text.String()), -1) {
		if daWords[w] {
			da++
		}
		if enWords[w] {
			en++
		}
		if strings.ContainsAny(w, "æøå") {
			da++
		}
	}
	switch {
	case da >= 3 && da > en*2:
		return "da"
	case en >= 3 && en > da*2:
		return "en"
	}
	return ""
}

func Save(ctx context.Context, dir string, f Found, header http.Header) error {
	r := f.Recipe
	if r.Name == "" {
		r.Name = "Untitled recipe"
	}
	r.Slug = recipe.UniqueSlug(dir, r.Name)
	var name string
	var err error
	switch {
	case f.imageFS != nil:
		name, err = copyImage(f.imageFS, f.imagePath, dir, r.Slug)
	case f.ImageURL != "":
		name, err = downloadImage(ctx, f.ImageURL, header, dir, r.Slug)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %s: image skipped: %v\n", r.Name, err)
	} else if name != "" {
		r.Image = name
	}
	return recipe.Save(dir, r)
}

var imageExt = map[string]string{
	"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp",
	"image/gif": ".gif", "image/avif": ".avif",
}

func isImageExt(ext string) bool {
	for _, e := range imageExt {
		if e == ext {
			return true
		}
	}
	return ext == ".jpeg"
}

func copyImage(fsys fs.FS, src, dir, slug string) (string, error) {
	in, err := fsys.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	return writeImage(in, dir, slug+strings.ToLower(path.Ext(src)))
}

func downloadImage(ctx context.Context, imageURL string, header http.Header, dir, slug string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", imageURL, nil)
	if err != nil {
		return "", err
	}
	for k, v := range header {
		req.Header[k] = v
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", errors.New(resp.Status)
	}
	ct, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	ext, ok := imageExt[ct]
	if !ok {
		if u, err := url.Parse(imageURL); err == nil {
			ext = strings.ToLower(path.Ext(u.Path))
		}
		if !isImageExt(ext) {
			return "", fmt.Errorf("not an image (%s)", ct)
		}
	}
	return writeImage(resp.Body, dir, slug+ext)
}

func writeImage(r io.Reader, dir, name string) (string, error) {
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, io.LimitReader(r, 20<<20)); err != nil {
		_ = f.Close()
		return "", err
	}
	return name, f.Close()
}
