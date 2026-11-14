# 🥄 Mise en place

Flat-file recipes. One Markdown file per recipe, a tiny Go binary to browse them.

- Recipes are plain `.md` files with YAML frontmatter that uses [schema.org/Recipe](https://schema.org/Recipe) field names.
- One generic importer reads schema.org Recipe data from websites, JSON files, folders and zip archives. That covers most recipe sites and exports from other recipe apps.
- The web UI has search, tag filters, serving scaling, and a cook mode that keeps the screen on and lets you tick off ingredients and steps.
- The interface is in English and Danish, and a recipe can have versions in several languages.
- There are no users, no database and nothing to configure. The files are the database, so use git for history.

## Usage

```sh
go build -o mise .

./mise serve -dir samples         # try it with the sample recipes
./mise serve                      # http://localhost:8080, recipes in ./recipes
./mise serve -dir ~/recipes -addr :9000
./mise import https://www.bbcgoodfood.com/recipes/easy-pancakes
./mise import ~/Downloads/recipes-export.zip ./old-recipes/
```

The recipe folder defaults to `$MISE_DIR`, or `./recipes` if that isn't set. `recipes/` is in `.gitignore`, so your own recipes stay out of this repository. Keep them in their own git repository if you want history. `samples/` holds a few example recipes. Edit the files in any editor. The site re-reads them on every request, so there's nothing to rebuild.

### Importing

`mise import SOURCE...` accepts any mix of these sources:

- **A URL.** For an HTML page, mise reads the page's schema.org data, as JSON-LD or, if there is none, as Microdata (`itemprop` attributes). For a JSON response, it parses the JSON directly. Add `-H "Name: value"` (repeatable) to send headers such as an API token, and `-insecure` to accept a self-signed certificate.
- **A `.json` or `.html` file.**
- **A folder or `.zip` archive** of such files, searched recursively. An image file sitting next to a recipe's JSON, or in an `images/` subfolder, is copied along with the recipe.

The importer is lenient. Besides standard schema.org it understands the variants that recipe apps commonly export:
- ingredient objects (`display`, `quantity`/`unit`/`food`/`note`, `originalText`)
- sections marked with `title` on items
- tags given as `{name}` objects
- `orgURL`, `performTime`, `recipeServings`, `notes` and `tools`
- free-text times such as "1 hour 30 minutes", which are converted to ISO 8601

In the web UI, pasting a URL under "Import from URL" first shows a preview of the recipe as it will be saved. You can edit the Markdown there, and nothing is written until you click Save. The preview also warns you if a recipe with the same name already exists.

Each import creates new files. If a name already exists, the new file is saved as `name-2.md` instead of overwriting the old one.

## Docker

Images are published to `ghcr.io/staal0/mise`. `latest` is the newest release; each release is also tagged with its version for pinning:

```sh
docker run -d -p 8080:8080 -v mise-recipes:/recipes ghcr.io/staal0/mise:latest
```

The image is built `FROM scratch`: it contains only the mise binary and CA certificates (for importing over HTTPS). It runs as user `65532`, keeps recipes in the `/recipes` volume, and has a built-in healthcheck (`mise healthcheck`, which calls `/healthz`).

To keep your recipes in a folder on the host instead, so you can edit them directly, bind-mount it and run as the folder's owner:

```sh
docker run -d -p 8080:8080 --user "$(id -u):$(id -g)" -v "$PWD/recipes:/recipes" ghcr.io/staal0/mise:latest
```

Commands other than `serve` work too, for example `docker run --rm -v mise-recipes:/recipes ghcr.io/staal0/mise import https://…`. With Docker Compose, see [`compose.yaml`](compose.yaml).

### Releasing

Push a version tag. GitHub Actions builds the image, pushes it as `X.Y.Z`, `X.Y` and `latest`, and creates a GitHub release:

```sh
git tag v0.1.0 && git push origin v0.1.0
```

Every push also runs the tests, `golangci-lint` and a `gofmt` check.

## Recipe format

```markdown
---
name: Pancakes
description: Fluffy weekend pancakes.
recipeYield: 4
prepTime: PT10M
cookTime: PT15M
totalTime: PT25M
recipeCategory: [breakfast]
recipeCuisine: [american]
keywords: [sweet, quick]
author: Grandma
image: pancakes.jpg
source: https://example.com/pancakes
---

Optional intro text.

## Ingredients

- 200 g flour
- 1½ dl milk

### Topping

- 2-3 tbsp maple syrup

## Instructions

1. Mix.
2. Fry.

## Notes

Anything else, as normal Markdown.
```

**Frontmatter.** Only `name` is required. Field names are schema.org's. The one exception is `source`, which maps to schema.org `isBasedOn`. Times are ISO 8601 durations (`PT1H30M`). List fields accept either `[a, b]` or `a, b`.

**Language versions.** See [Languages](#languages) below.

**Image.** Either a filename relative to the recipe folder, or a full URL.

**Sections.**
- `## Ingredients` holds a bullet list.
- `## Instructions` holds a numbered list. Plain paragraphs also work, one step per paragraph.
- `### Heading` inside either section starts a group, such as "Sauce" or "Dough".
- Any other `##` section is rendered as ordinary Markdown.
- Danish headings work too: `## Ingredienser` and `## Fremgangsmåde`.

**Scaling.** A number at the start of an ingredient line is scaled: `200`, `1,5`, `1.5`, `1 1/2`, `1½`, `½` or a range like `2-3`. Write the amount first if you want it to scale. `recipeYield` is the number of portions, shown as "4 portioner" or "4 servings" depending on the interface language. The servings picker scales from it. If `recipeYield` is text without a number, it is shown as written and the picker shows a multiplier instead.

## Languages

**Interface.** The interface is available in English and Danish. By default it follows the browser's language. The 🌐 button in the header switches it, and the choice is remembered in a cookie.

**Recipe versions.** A recipe can exist in several languages. The filename says which:

```
frikadeller.md       the default version, in whatever language you wrote it
frikadeller.en.md    an English version (optional)
frikadeller.webp     the image, shared by both
```

- mise shows the version that matches the interface language. If there's no such version, it shows the default.
- On a recipe page, "Also in:" links switch between versions. On the main page, each card shows small language badges.
- A language version only needs what it changes, usually the name, description, ingredients and instructions. Anything it leaves out is taken from the default file: the image, times, yield, tags, source, or whole sections.
- Search matches every version of a recipe.
- A recipe that exists only as `name.en.md` works too. That file then acts as the default.

The default file has no language field. Write it in whatever language you cook in, and add `.xx.md` files only for the recipes you want in another language. `samples/pancakes.da.md` is an example.

**JSON-LD.** Every recipe page embeds schema.org JSON-LD, so other recipe apps can import from mise.

## Notes

- Cook mode uses the Screen Wake Lock API. Browsers only allow it over HTTPS or on `localhost`. If you open mise on a tablet over plain `http://192.168…`, the screen may still dim. Putting mise behind a reverse proxy with TLS fixes that.
- There is no authentication. Run it on a trusted network, or behind a proxy that adds auth. The import form can fetch arbitrary URLs.
