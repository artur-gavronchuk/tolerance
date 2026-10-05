# Pixel Editor

Build a **pixel-art editor** as a static web page — a tiny creative app that people open in the gallery and
immediately want to draw in. A 16 × 16 canvas, a palette of 16 colors, pencil, eraser, fill, undo that never
lets you down. Everyone who opens the gallery draws in your editor and votes for the one that feels best,
so the feel matters as much as the rules: crisp response under the pointer, a satisfying stroke, a layout
that looks like a little studio, a look of your own.

## The rules (exact — they are checked)

- **Canvas.** A grid of **16 × 16 cells**. Every cell is a DOM element with `data-testid="cell-X-Y"` where
  `X` is the column (0…15, left to right) and `Y` the row (0…15, top to bottom): `cell-0-0` is top left,
  `cell-15-0` top right, `cell-0-15` bottom left. The cells receive the pointer input (press, move, release);
  nothing may cover them. You may draw the pixels any way you like (cell backgrounds, a `<canvas>` behind
  or in front of transparent cells…), but the cells must exist, be laid out as the grid and be what the
  pointer lands on. No other element's `data-testid` may start with `cell-`.
- **Colors.** Exactly this palette, in this order, as buttons `color-0` … `color-15`:

  | 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 |
  |---|---|---|---|---|---|---|---|
  | `#000000` | `#1d2b53` | `#7e2553` | `#008751` | `#ab5236` | `#5f574f` | `#c2c3c7` | `#fff1e8` |

  | 8 | 9 | 10 | 11 | 12 | 13 | 14 | 15 |
  |---|---|---|---|---|---|---|---|
  | `#ff004d` | `#ffa300` | `#ffec27` | `#00e436` | `#29adff` | `#83769c` | `#ff77a8` | `#ffccaa` |

  The selected color has `aria-pressed="true"`, all other color buttons `aria-pressed="false"`. A new
  editor has color 0 selected. A cell is either empty (transparent) or one of these 16 colors.
- **Tools.** Buttons `tool-pencil`, `tool-eraser`, `tool-fill`; the active one has `aria-pressed="true"`, the
  others `"false"`. A new editor has the pencil. Keys **B** (pencil), **E** (eraser), **F** (fill) switch
  the tool, upper or lower case, only when no Ctrl, Cmd or Alt is held. Picking a color while the eraser is
  active switches to the pencil (fill stays fill); the eraser itself never changes the selected color.
  - **Pencil** paints the cell under the pointer with the selected color, **eraser** makes it empty.
  - **Fill** replaces the clicked cell's whole region with the selected color. A region is the set of cells
    reachable from the clicked cell by steps **left, right, up, down** (4-connected, *not* diagonals) through
    cells with exactly the same content (the same color, or all empty). Empty regions can be filled too.
    Filling a region that already has the selected color changes nothing.
- **Strokes.** Pressing on a cell paints it right away; moving the pointer with the button held paints every
  cell it enters. The pointer can move fast and skip cells between two events: the cells on the straight
  line between the previous and the new cell are painted too, so there are no gaps (Bresenham's line — the
  checks only use horizontal, vertical and exactly diagonal strokes, where the line is unambiguous). A stroke
  ends on release. The stroke may be drawn with the mouse or a finger (use pointer events, and
  `touch-action: none` so a finger draws instead of scrolling).
- **History.** Buttons `undo` and `redo`, plus **Ctrl+Z** (undo) and **Ctrl+Shift+Z** or **Ctrl+Y** (redo);
  Cmd works like Ctrl on a Mac. The history keeps **at least 100 steps**.
  - **One action is one step**: a whole stroke (press, drag, release) is one step however many cells it
    touched; a fill is one step; `clear` is one step. Picking a color or a tool is not a step.
  - **An action that changes nothing is not a step** and leaves the redo stack alone: pressing a cell that
    already has the pencil's color, erasing empty cells, filling a region that already has the selected
    color, clearing an empty canvas.
  - A new step after undoing clears the redo stack.
  - `undo` has the `disabled` property while there is nothing to undo, `redo` while there is nothing to redo.
- **Clear.** A button `clear` empties the whole canvas as one undoable step. It is never disabled.
- **Memory.** The drawing and the selected color are kept in `localStorage` and restored after a reload. The
  history and the tool do not have to survive a reload.
- **Export.** `window.pixel.exportPNG()` returns a `data:image/png;base64,…` URL (a string, or a promise of
  one) of a PNG exactly **16 × 16 pixels**: pixel (X, Y) has the color of cell (X, Y) fully opaque, and empty
  cells are fully transparent.
- **Hook and screen agree.** Each cell shows its color visibly, and carries `data-color="#rrggbb"`
  (lowercase) while painted; an empty cell has no `data-color` attribute at all.
- **Test mode.** With **`?test=1`** in the URL the editor starts with an **empty** canvas — no starter
  picture, no intro or tutorial that eats pointer input. (Without it you may greet people with a sample
  drawing on a first visit; the gallery screenshot is taken that way, so a pretty first impression is
  welcome.) A restored drawing from `localStorage` is still loaded in test mode.
- On a phone (375 px wide) the whole editor fits without horizontal scrolling and still draws. Cells may be
  smaller than 24 px there — but **do not make cells focusable** (no `<button>`, `tabindex`, `role=button`
  on cells): the tap-target check counts every interactive element under 24 × 24 px, and 256 of them would
  sink it. Plain `div`s with pointer events are what you want. All real buttons (colors, tools, undo, redo,
  clear) must be at least 24 × 24 px.

## Nice to have

A live preview at real size, mirror drawing (symmetry axis), an animation or a "pop" when a cell is painted,
a hover ghost showing the color under the pointer, a grid toggle, a transparency checkerboard, more tools
(line, rectangle, eyedropper, move), color shortcuts (`1`…`9`, `[` and `]`), keyboard drawing (arrows + Space)
for people without a mouse, a download button for the PNG (scaled up so it is not a postage stamp), a
starter sample, themes. Respect `prefers-reduced-motion`.

## Rules for the upload

Static files only: an `index.html` at the root of the zip plus any CSS, JS, images and sounds. No server,
no external API keys, no CDNs or web fonts (the checks run without a network). Use relative paths
(`./editor.js`, not `/editor.js`). Up to 5 MB. A single `.html` file is fine too.

## Automated checks

Your page is opened in Chromium (no network) and driven through acceptance scenarios — real mouse presses,
drags and key presses on your cells, buttons and the page; the share it passes is most of the platform score.
The page must expose this hook (always, not only in test mode):

```js
window.pixel = {
  // The current picture and controls. grid[y][x] is '#rrggbb' (lowercase, from the palette) or null (empty).
  state() {},      // { grid: [16 rows of 16], color: 0..15, tool: 'pencil' | 'eraser' | 'fill', canUndo: boolean, canRedo: boolean }
  exportPNG() {},  // 'data:image/png;base64,…' of a 16×16 PNG (or a promise of that string)
}
```

Keys go to the page (the `document`), so listen on `window` or `document`. Elements found by `data-testid`:

| `data-testid` | What it is |
|---------------|------------|
| `cell-0-0` … `cell-15-15` | the 256 cells (`cell-X-Y`), pointer targets; `data-color` while painted |
| `color-0` … `color-15` | palette buttons, `aria-pressed` marks the selected one |
| `tool-pencil`, `tool-eraser`, `tool-fill` | tool buttons, `aria-pressed` marks the active one |
| `undo`, `redo` | buttons, `disabled` when there is nothing to undo / redo |
| `clear` | button, empties the canvas (one undoable step) |

Anything else (mirror, download, preview…) is yours — give it any `data-testid` that does not start with
`cell-`, `color-` or `tool-`.

The screen and the hook must agree: what `state()` says is what is drawn, and what is drawn is what
`exportPNG()` returns.

## Judging

Upload a zip with `index.html` at its root (or a single `.html` file). Within a minute or two the platform
opens your page, takes a screenshot and gives it a **platform score** out of 100:

- 70 points: the share of the acceptance scenarios above that pass;
- 30 points: quality — accessibility (axe-core), no horizontal scroll and big enough tap targets at 375 px,
  no JavaScript errors, a fast and light first load.

Your entry then appears in the public gallery where everyone can draw in it, and people vote for the ones
they like. You can upload again at any time; your latest upload replaces the previous one and keeps its
votes.
