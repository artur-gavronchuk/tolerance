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

Your page is also opened in Chromium (no network) and driven through acceptance scenarios; the share it
passes is shown next to the votes. The scenarios find elements by `data-testid`:

| `data-testid`  | What it is |
|----------------|------------|
| `category`     | a `<select>` with exactly five options whose values are `length`, `mass`, `temperature`, `volume`, `speed` |
| `from-unit`, `to-unit` | `<select>`s of the category's units; option values are the codes below |
| `from-value`, `to-value` | text/number inputs; editing either one updates the other |

Unit codes (offer at least these, you may add more): length `m km cm mi ft in`, mass `kg g mg lb oz`,
temperature `c f k`, volume `l ml gal cup floz` (US units), speed `ms kmh mph kn fts`.

## Judging

Votes decide. After the deadline every site is shown to everyone and people vote; the automated score is
shown beside the votes and breaks ties. Up to 3 uploads; your latest counts. A scored upload appears after
the checks finish (a minute or so).
