package importer

import (
	"strings"

	xhtml "golang.org/x/net/html"
)

// microdata returns schema.org Microdata items as JSON-LD-shaped maps.
func microdata(doc *xhtml.Node) []any {
	var items []any
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && hasAttr(n, "itemscope") {
			if !hasAttr(n, "itemprop") {
				items = append(items, parseItem(n))
			}
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
			if !isScope {
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

var headingTags = map[string]bool{"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true}

// isLeadHeading matches sub-headings written as <p><strong>Bake<br></strong>…</p>.
func isLeadHeading(n *xhtml.Node) bool {
	if n.Data != "strong" && n.Data != "b" || n.Parent == nil || !blockTags[n.Parent.Data] {
		return false
	}
	for c := n.Parent.FirstChild; c != n; c = c.NextSibling {
		if c.Type != xhtml.TextNode || strings.TrimSpace(c.Data) != "" {
			return false
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

// textContent stops at the first nested property: some sites wrap the whole recipe in one.
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

// leadingParagraphs keeps only the intro: sites put "description" on containers holding ads etc.
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
					stop = true
				} else {
					walk(c)
				}
			default:
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
