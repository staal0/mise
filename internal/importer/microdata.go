package importer

import (
	"strings"

	xhtml "golang.org/x/net/html"
)

// microdata extracts schema.org Microdata (itemscope/itemprop attributes)
// from a parsed page and returns each top-level item as a JSON-LD-shaped
// map, so it can go through the same conversion as JSON-LD. Nested items
// (e.g. a Recipe inside a WebPage) become nested maps.
func microdata(doc *xhtml.Node) []any {
	var items []any
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && hasAttr(n, "itemscope") {
			if !hasAttr(n, "itemprop") {
				items = append(items, parseItem(n))
			}
			// Items nested via itemprop are handled by parseItem; items
			// nested without itemprop are still separate top-level items.
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return items
}

func parseItem(scope *xhtml.Node) map[string]any {
	props := map[string][]any{}
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != xhtml.ElementNode {
				continue
			}
			names := strings.Fields(attr(c, "itemprop"))
			isScope := hasAttr(c, "itemscope")
			if len(names) > 0 {
				var v any
				switch {
				case isScope:
					v = parseItem(c)
				case len(names) == 1 && names[0] == "description":
					v = leadingParagraphs(c)
				default:
					v = propValue(c)
				}
				for _, name := range names {
					props[name] = append(props[name], v)
				}
			}
			if !isScope { // a nested item's properties belong to it, not us
				walk(c)
			}
		}
	}
	walk(scope)

	m := map[string]any{}
	if t := attr(scope, "itemtype"); t != "" {
		m["@type"] = strings.Fields(t)[0]
	}
	for k, vs := range props {
		if len(vs) == 1 {
			m[k] = vs[0]
		} else {
			m[k] = vs
		}
	}
	return m
}

// propValue follows the Microdata rules for an element's value.
func propValue(n *xhtml.Node) string {
	if v, ok := attrOK(n, "content"); ok {
		return v
	}
	switch n.Data {
	case "img", "audio", "video", "source", "embed", "iframe", "track":
		return attr(n, "src")
	case "a", "area", "link":
		return attr(n, "href")
	case "object":
		return attr(n, "data")
	case "data", "meter":
		return attr(n, "value")
	case "time":
		if v, ok := attrOK(n, "datetime"); ok {
			return v
		}
	}
	return textContent(n)
}

// headingTags render as section headings ("### ...") in extracted text.
var headingTags = map[string]bool{"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true}

// isLeadHeading reports whether n is a <strong>/<b> that opens a block and
// ends with a line break: "<p><strong>Bake<br></strong>Put it in…</p>",
// a common way to write sub-headings in recipe instructions.
func isLeadHeading(n *xhtml.Node) bool {
	if n.Data != "strong" && n.Data != "b" || n.Parent == nil || !blockTags[n.Parent.Data] {
		return false
	}
	for c := n.Parent.FirstChild; c != n; c = c.NextSibling {
		if c.Type != xhtml.TextNode || strings.TrimSpace(c.Data) != "" {
			return false // something precedes it
		}
	}
	last := n.LastChild
	for last != nil && last.Type == xhtml.TextNode && strings.TrimSpace(last.Data) == "" {
		last = last.PrevSibling
	}
	next := n.NextSibling
	for next != nil && next.Type == xhtml.TextNode && strings.TrimSpace(next.Data) == "" {
		next = next.NextSibling
	}
	return (last != nil && last.Data == "br") || (next != nil && next.Data == "br")
}

var blockTags = map[string]bool{
	"p": true, "div": true, "li": true, "br": true, "tr": true, "section": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true, "ol": true, "ul": true,
}

// textContent returns an element's text, with a newline at block
// boundaries so multi-paragraph instructions split into steps, and
// sub-headings marked "### ". Text stops at the first nested property:
// some sites put e.g. "description" on a container that wraps the whole
// recipe, and the other properties' text doesn't belong in it.
func textContent(root *xhtml.Node) string {
	var b strings.Builder
	stopped := false
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if stopped {
			return
		}
		switch n.Type {
		case xhtml.TextNode:
			b.WriteString(n.Data)
		case xhtml.ElementNode:
			if n.Data == "script" || n.Data == "style" || n.Data == "noscript" {
				return
			}
			if n != root && (hasAttr(n, "itemprop") || hasAttr(n, "itemscope")) {
				stopped = true
				return
			}
			if n != root && (headingTags[n.Data] || isLeadHeading(n)) {
				b.WriteString("\n### " + strings.Join(strings.Fields(textContent(n)), " ") + "\n")
				return
			}
			block := blockTags[n.Data]
			if block {
				b.WriteString("\n")
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
			if block {
				b.WriteString("\n")
			}
		}
	}
	walk(root)
	return strings.TrimSpace(b.String())
}

// leadingParagraphs returns the opening paragraphs of an element and stops
// at the first box-like element after them. Some sites put "description"
// on a container that also holds the recipe card, ads and so on; the intro
// paragraphs come first, the rest isn't description.
func leadingParagraphs(root *xhtml.Node) string {
	if _, ok := attrOK(root, "content"); ok || root.Data == "meta" {
		return propValue(root)
	}
	var parts []string
	stop := false
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		for c := n.FirstChild; c != nil && !stop; c = c.NextSibling {
			switch {
			case c.Type == xhtml.TextNode:
				if t := strings.TrimSpace(c.Data); t != "" {
					parts = append(parts, t)
				}
			case c.Type != xhtml.ElementNode, c.Data == "script", c.Data == "style", c.Data == "noscript":
			case hasAttr(c, "itemprop"), hasAttr(c, "itemscope"):
				stop = true
			case c.Data == "p":
				if t := textContent(c); t != "" {
					parts = append(parts, t)
				}
			case blockTags[c.Data] && c.Data != "br":
				if len(parts) > 0 {
					stop = true // intro is over
				} else {
					walk(c) // the intro may be wrapped in a div
				}
			default: // inline element (a, strong, span, …)
				if t := textContent(c); t != "" {
					parts = append(parts, t)
				}
			}
		}
	}
	walk(root)
	return strings.Join(parts, "\n\n")
}

func attrOK(n *xhtml.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}

func attr(n *xhtml.Node, key string) string {
	v, _ := attrOK(n, key)
	return v
}

func hasAttr(n *xhtml.Node, key string) bool {
	_, ok := attrOK(n, key)
	return ok
}
