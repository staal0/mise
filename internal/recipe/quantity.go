package recipe

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Segment is a piece of an ingredient line. Quantity segments can be scaled.
type Segment struct {
	Text  string
	IsQty bool
	Qty   float64
	Style string // "frac", "comma" or "dec": how to format the scaled value
}

const fracGlyphs = "½⅓⅔¼¾⅛⅜⅝⅞"

var glyphValue = map[rune]float64{
	'½': 1. / 2, '⅓': 1. / 3, '⅔': 2. / 3, '¼': 1. / 4, '¾': 3. / 4,
	'⅛': 1. / 8, '⅜': 3. / 8, '⅝': 5. / 8, '⅞': 7. / 8,
}

const numPat = `(?:\d+\s+\d+/\d+|\d+\s*[` + fracGlyphs + `]|\d+/\d+|\d+(?:[.,]\d+)?|[` + fracGlyphs + `])`

var leadQty = regexp.MustCompile(`^(` + numPat + `)(?:(\s*[-–]\s*)(` + numPat + `))?`)

// SplitQuantity finds a leading quantity (or range like "2-3") in an
// ingredient line so it can be scaled.
func SplitQuantity(line string) []Segment {
	m := leadQty.FindStringSubmatchIndex(line)
	if m == nil {
		return []Segment{{Text: line}}
	}
	var segs []Segment
	add := func(s string) {
		v, style, ok := parseNumber(s)
		if !ok {
			segs = append(segs, Segment{Text: s})
			return
		}
		segs = append(segs, Segment{Text: s, IsQty: true, Qty: v, Style: style})
	}
	add(line[m[2]:m[3]])
	if m[4] >= 0 {
		segs = append(segs, Segment{Text: line[m[4]:m[5]]})
		add(line[m[6]:m[7]])
	}
	if rest := line[m[1]:]; rest != "" {
		segs = append(segs, Segment{Text: rest})
	}
	return segs
}

func parseNumber(s string) (float64, string, bool) {
	s = strings.TrimSpace(s)
	style := "dec"
	var total float64
	// Trailing unicode fraction glyph, e.g. "1½" or "½".
	if r := []rune(s); len(r) > 0 {
		if g, ok := glyphValue[r[len(r)-1]]; ok {
			total += g
			s = strings.TrimSpace(string(r[:len(r)-1]))
			style = "frac"
		}
	}
	fields := strings.Fields(s)
	for _, f := range fields {
		if a, b, ok := strings.Cut(f, "/"); ok {
			n, err1 := strconv.Atoi(a)
			d, err2 := strconv.Atoi(b)
			if err1 != nil || err2 != nil || d == 0 {
				return 0, "", false
			}
			total += float64(n) / float64(d)
			style = "frac"
			continue
		}
		if strings.Contains(f, ",") {
			style = "comma"
			f = strings.Replace(f, ",", ".", 1)
		}
		v, err := strconv.ParseFloat(f, 64)
		if err != nil {
			return 0, "", false
		}
		total += v
	}
	return total, style, true
}

// LeadingNumber returns the first number in s (e.g. 4 from "4 servings").
func LeadingNumber(s string) float64 {
	segs := SplitQuantity(strings.TrimSpace(s))
	if len(segs) > 0 && segs[0].IsQty {
		return segs[0].Qty
	}
	return 0
}

var isoDur = regexp.MustCompile(`^P(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?)?$`)

// ParseISODuration parses the subset of ISO 8601 durations used by recipes.
func ParseISODuration(s string) (time.Duration, bool) {
	m := isoDur.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(s)))
	if m == nil || s == "P" || s == "PT" {
		return 0, false
	}
	n := func(i int) time.Duration { v, _ := strconv.Atoi(m[i]); return time.Duration(v) }
	return n(1)*24*time.Hour + n(2)*time.Hour + n(3)*time.Minute + n(4)*time.Second, true
}

// HumanDuration renders "PT1H30M" as "1 h 30 min"; non-ISO input is returned as-is.
func HumanDuration(s string) string { return FormatDuration(s, "h") }

// FormatDuration is HumanDuration with a custom hour unit (e.g. "t" in Danish).
func FormatDuration(s, hourUnit string) string {
	d, ok := ParseISODuration(s)
	if !ok {
		return s
	}
	d = d.Round(time.Minute)
	h, m := int(d.Hours()), int(d.Minutes())%60
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%d %s %d min", h, hourUnit, m)
	case h > 0:
		return fmt.Sprintf("%d %s", h, hourUnit)
	default:
		return fmt.Sprintf("%d min", m)
	}
}

var (
	humanPart = regexp.MustCompile(`(?i)(\d+(?:[.,]\d+)?)\s*(d|days?|dage?|h|hrs?|hours?|t|timer?|m|mins?|minutes?|minutt?e?r?)\b`)
	plainNum  = regexp.MustCompile(`^\d+$`)
)

// ToISODuration converts free text like "1 hour 30 minutes", "45 min" or
// "1 time 15 minutter" to ISO 8601. Already-ISO input is passed through.
// Unrecognized text is returned unchanged with ok=false.
func ToISODuration(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", true
	}
	if _, ok := ParseISODuration(s); ok {
		return strings.ToUpper(s), true
	}
	if plainNum.MatchString(s) {
		return "PT" + s + "M", true
	}
	var mins float64
	found := false
	for _, m := range humanPart.FindAllStringSubmatch(s, -1) {
		v, _ := strconv.ParseFloat(strings.Replace(m[1], ",", ".", 1), 64)
		switch u := strings.ToLower(m[2]); {
		case strings.HasPrefix(u, "d"):
			mins += v * 24 * 60
		case u == "h" || u == "t" || strings.HasPrefix(u, "hr") || strings.HasPrefix(u, "hour") || strings.HasPrefix(u, "time"):
			mins += v * 60
		default:
			mins += v
		}
		found = true
	}
	if !found {
		return s, false
	}
	total := int(mins + 0.5)
	h, m := total/60, total%60
	out := "PT"
	if h > 0 {
		out += strconv.Itoa(h) + "H"
	}
	if m > 0 || h == 0 {
		out += strconv.Itoa(m) + "M"
	}
	return out, true
}
