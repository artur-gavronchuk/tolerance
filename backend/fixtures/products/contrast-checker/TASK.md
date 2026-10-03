# Contrast checker

Build a **WCAG color contrast checker** as a static web page: two colors in, the exact contrast ratio and the
four accessibility verdicts out, with a live preview and a button that repairs a failing text color.

## Must have

- Two text inputs: the text color and the background color. Accepted formats: `#RGB` and `#RRGGBB`, with or
  without the `#`, any letter case, ignoring spaces around (`#abc` means `#aabbcc`). The result updates on every
  keystroke.
- The **contrast ratio** of the WCAG 2 definition: relative luminance
  `L = 0.2126 R + 0.7152 G + 0.0722 B` with each channel linearised (`c/255`, then `c / 12.92` if `c <= 0.04045`,
  else `((c + 0.055) / 1.055) ^ 2.4`), and `ratio = (L_lighter + 0.05) / (L_darker + 0.05)`.
- The ratio is shown as `N.NN:1`, **truncated (rounded down) to two decimals, never rounded up**: red on white
  is `3.99:1`, not `4.00:1`, because WCAG pass marks may not be rounded in your favour. Black on white is `21.00:1`.
- Four verdicts, each Pass or Fail against the exact ratio (not the displayed one): AA normal text 4.5, AA large
  text 3, AAA normal text 7, AAA large text 4.5.
- A live **preview** box that shows sample text in the chosen colors.
- **Invalid input** (`#12`, `#ggg`, `red`, empty, ...) in either field: show an error message, no ratio, and
  mark the verdicts as unknown. When the input is fixed the error disappears and the result returns.
- A **Swap** button that exchanges the two colors.
- A **Fix** button that, when the text color fails AA normal text (4.5), changes the text color to the nearest
  color that passes: keep its hue and saturation, move its HSL lightness toward black or toward white in 1 %
  steps, whichever direction needs the smaller change and reaches 4.5 (a dark background needs lighter text, a
  light one darker). Do not overshoot to pure black or white. If the colors already pass, or an input is
  invalid, it changes nothing.
- Works on a phone screen (375 px wide) without horizontal scrolling.

## Nice to have

- Color pickers next to the text fields; a shareable URL that stores the colors.
- Suggestions for the background too, a list of common palettes, an APCA readout.
- A design that looks like a tool you would bookmark.

## Rules

Static files only: an `index.html` at the root of the zip plus any CSS, JS and images. No server, no
external API keys, no external CDNs or fonts (the checks run without a network). Use relative paths
(`./app.js`, not `/app.js`). Up to 5 MB.

## Automated checks

Your page is also opened in Chromium (no network) and driven through acceptance scenarios; the share it
passes is shown next to the votes. The scenarios find elements by `data-testid`:

| `data-testid` | What it is |
|---------------|------------|
| `fg-input`, `bg-input` | the text color and background color `<input>`s; the Fix button writes `#rrggbb` (lower case) into `fg-input` |
| `ratio` | the ratio text, e.g. `4.54:1`; contains no digit while an input is invalid |
| `aa-normal`, `aa-large`, `aaa-normal`, `aaa-large` | one verdict each; attribute `data-result` is `pass`, `fail`, or `na` while an input is invalid; the text contains `Pass` or `Fail` when known |
| `error` | the message for an invalid input; visible only while an input is invalid |
| `preview` | the sample box; its computed `color` and `background-color` are the chosen colors |
| `swap`, `fix-aa` | the Swap and Fix buttons |

## Judging

Votes decide. After the deadline every site is shown to everyone and people vote; the automated score is
shown beside the votes and breaks ties. Up to 3 uploads; your latest counts. A scored upload appears after
the checks finish (a minute or so).
