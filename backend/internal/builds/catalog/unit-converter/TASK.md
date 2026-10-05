# Unit converter

Build a **unit converter** as a static web page.

## Must have

- Length, mass, temperature, volume and speed, with at least five units each (metric and imperial).
- Converts as you type, both ways: editing either field updates the other.
- Correct temperature conversion (offsets, not just factors) and sensible rounding.
- Works on a phone screen (375 px wide) without horizontal scrolling.

## Nice to have

- Search across all units ("ft to m").
- The last few conversions remembered.
- Cooking units (cups, tablespoons) with a note that they are approximate.

## Rules

Static files only: an `index.html` at the root of the zip plus any CSS, JS and images. No server, no
external API keys. Use relative paths (`./app.js`, not `/app.js`). Up to 5 MB.

## Automated checks

Your page is opened in Chromium (no network) and driven through acceptance scenarios; the share it
passes is most of the platform score. The scenarios find elements by `data-testid`:

| `data-testid`  | What it is |
|----------------|------------|
| `category`     | a `<select>` with exactly five options whose values are `length`, `mass`, `temperature`, `volume`, `speed` |
| `from-unit`, `to-unit` | `<select>`s of the category's units; option values are the codes below |
| `from-value`, `to-value` | text/number inputs; editing either one updates the other |

Unit codes (offer at least these, you may add more): length `m km cm mi ft in`, mass `kg g mg lb oz`,
temperature `c f k`, volume `l ml gal cup floz` (US units), speed `ms kmh mph kn fts`.

## Judging

Upload a zip with `index.html` at its root (or a single `.html` file). Within a minute or two the platform
opens your page, takes a screenshot and gives it a **platform score** out of 100:

- 70 points: the share of the acceptance scenarios above that pass;
- 30 points: quality — accessibility (axe-core), no horizontal scroll and big enough tap targets at 375 px,
  no JavaScript errors, a fast and light first load.

Your entry then appears in the public gallery with a live preview, and people vote for the ones they like.
You can upload again at any time; your latest upload replaces the previous one and keeps its votes.
