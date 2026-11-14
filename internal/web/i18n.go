package web

import (
	"net/http"
	"strings"
)

var uiLangs = []string{"en", "da"}

var translations = map[string]map[string]string{
	"en": {
		"langName":        "English",
		"langTitle":       "Switch language",
		"search":          "Search recipes, tags, ingredients…",
		"importToggle":    "+ Import from URL",
		"importBtn":       "Import",
		"badURL":          "Please paste a full http(s) link.",
		"importFailed":    "Import failed: ",
		"readErrors":      "Some files could not be read:",
		"empty":           "No recipes yet. Import one from a URL above, or add a .md file to the recipes folder.",
		"nomatch":         "Nothing matches.",
		"alsoIn":          "Also in:",
		"previewTitle":    "Check before saving",
		"previewHint":     "This is how the recipe will be saved. Nothing is saved until you click Save.",
		"save":            "Save recipe",
		"cancel":          "Cancel",
		"editMarkdown":    "Edit before saving",
		"editHint":        "Changes here are saved; the preview above doesn't update.",
		"existsWarn":      "You already have a recipe with this name. Saving creates a copy.",
		"noName":          "the recipe needs a name",
		"saveFailed":      "Could not save: ",
		"originalVersion": "Original",
		"prep":            "Prep",
		"cook":            "Cook",
		"total":           "Total",
		"yield":           "Yield",
		"fewer":           "Fewer",
		"more":            "More",
		"servings":        "servings",
		"cookMode":        "Cook mode",
		"exitCookMode":    "Exit cook mode",
		"ingredients":     "Ingredients",
		"instructions":    "Instructions",
		"by":              "By",
		"original":        "Original recipe ↗",
		"readMore":        "Read more",
		"showLess":        "Show less",
		"themeTitle":      "Switch theme",
		"themeAuto":       "Auto",
		"themeLight":      "Light",
		"themeDark":       "Dark",
		"hourUnit":        "h",
	},
	"da": {
		"langName":        "Dansk",
		"langTitle":       "Skift sprog",
		"search":          "Søg i opskrifter, tags, ingredienser…",
		"importToggle":    "+ Importér fra URL",
		"importBtn":       "Importér",
		"badURL":          "Indsæt et fuldt http(s)-link.",
		"importFailed":    "Import mislykkedes: ",
		"readErrors":      "Nogle filer kunne ikke læses:",
		"empty":           "Ingen opskrifter endnu. Importér en fra en URL ovenfor, eller læg en .md-fil i opskriftsmappen.",
		"nomatch":         "Ingen opskrifter matcher.",
		"alsoIn":          "Også på:",
		"previewTitle":    "Tjek før du gemmer",
		"previewHint":     "Sådan bliver opskriften gemt. Intet gemmes, før du klikker Gem.",
		"save":            "Gem opskrift",
		"cancel":          "Annullér",
		"editMarkdown":    "Rediger før du gemmer",
		"editHint":        "Ændringer her bliver gemt; forhåndsvisningen ovenfor opdateres ikke.",
		"existsWarn":      "Du har allerede en opskrift med dette navn. Gem laver en kopi.",
		"noName":          "opskriften skal have et navn",
		"saveFailed":      "Kunne ikke gemme: ",
		"originalVersion": "Original",
		"prep":            "Forberedelse",
		"cook":            "Tilberedning",
		"total":           "I alt",
		"yield":           "Antal",
		"fewer":           "Færre",
		"more":            "Flere",
		"servings":        "portioner",
		"cookMode":        "Madlavning",
		"exitCookMode":    "Afslut madlavning",
		"ingredients":     "Ingredienser",
		"instructions":    "Fremgangsmåde",
		"by":              "Af",
		"original":        "Original opskrift ↗",
		"readMore":        "Læs mere",
		"showLess":        "Vis mindre",
		"themeTitle":      "Skift tema",
		"themeAuto":       "Auto",
		"themeLight":      "Lys",
		"themeDark":       "Mørk",
		"hourUnit":        "t",
	},
}

var langNames = map[string]string{"da": "Dansk", "en": "English", "sv": "Svenska", "no": "Norsk", "nb": "Norsk", "de": "Deutsch", "fr": "Français", "it": "Italiano", "es": "Español"}

func langName(code string) string {
	if n, ok := langNames[code]; ok {
		return n
	}
	return strings.ToUpper(code)
}

func uiLang(r *http.Request) string {
	if c, err := r.Cookie("lang"); err == nil && translations[c.Value] != nil {
		return c.Value
	}
	for _, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		tag, _, _ := strings.Cut(part, ";")
		base, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(tag)), "-")
		if translations[base] != nil {
			return base
		}
	}
	return "en"
}

func nextLang(cur string) string {
	for i, l := range uiLangs {
		if l == cur {
			return uiLangs[(i+1)%len(uiLangs)]
		}
	}
	return uiLangs[0]
}
