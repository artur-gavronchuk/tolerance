# Mirror studio

Build a **drawing tool where every stroke turns into a symmetric creature, emblem or pattern**. The person
draws one line, and the studio repeats it around the canvas by the rules of a mirror and of rotation:
a butterfly from a single wing, a snowflake from a scribble, a mandala from one flick of the wrist.
Everyone who opens the gallery draws in your studio and votes for the one that feels best, so the feel
matters as much as the rules: a stroke that flows under the pointer, colors you want to use, a canvas that
looks like a place to make things, a look of your own.

## The rules (exact — they are checked)

### The field and the copies

- The drawing field is a square, **0 … 256 on both axes**, `x` to the right, `y` down, **integer
  coordinates**. A point is an array `[x, y]`.
- Two transformations of the field (the field's center is `(128, 128)`):
  - **M**, the mirror: `M(x, y) = (256 − x, y)`
  - **R**, the quarter turn: `R(x, y) = (256 − y, x)`
- A stroke is a list of points. Its **copies** are the same list pushed through transformations, point by
  point, in the same order. The eight possible copies, in this exact order:

  | # | copy | formula |
  |---|------|---------|
  | 0 | the original | `p` |
  | 1 | R | `R(p)` |
  | 2 | R² | `R(R(p))` |
  | 3 | R³ | `R(R(R(p)))` |
  | 4 | M | `M(p)` |
  | 5 | R∘M | `R(M(p))` (mirror first, then turn) |
  | 6 | R²∘M | `R(R(M(p)))` |
  | 7 | R³∘M | `R(R(R(M(p))))` |

- The **mode** decides which of these copies a stroke has (taken in the order above):

  | mode | copies | count |
  |------|--------|-------|
  | `single` | 0 | 1 |
  | `mirror` | 0, 4 | 2 |
  | `turn4` | 0, 1, 2, 3 | 4 |
  | `kaleido8` | 0 … 7 | 8 |

  Copies are **never merged or dropped**, even when they coincide (a point on an axis of symmetry is its
  own mirror image; the copy is still there).
- All copies are drawn on the canvas, and the canvas is always the picture of `getState()`.

### Strokes

- A stroke **remembers the mode, color and width at the moment it was created**. Changing the mode (or color,
  or width) later never changes strokes that already exist: their copies stay exactly as they were.
- Strokes are kept in the order they were drawn.
- Input points are **normalized** when a stroke is created: every coordinate is rounded to the nearest integer
  (`Math.round`) and then clamped into `0 … 256`. Copies are computed from the normalized points. A stroke may
  have a single point (a dot).
- A call that cannot make a stroke (`points` is not an array, is empty, or has a point that is not two finite
  numbers) is **ignored**: no stroke, no history entry, no error thrown.

### History

- Every stroke added (by the mouse or by the API) is **one undoable action**; a mouse drag is one stroke.
- **undo** reverts the latest action that is still applied, **redo** applies the latest reverted action
  again. Both do nothing when there is nothing to revert or apply.
- Adding a stroke after an undo **discards the redo history** (`canRedo` becomes `false`).
- **clear** removes every stroke and is **one undoable action**: one undo brings back all the strokes, in
  their original order, with their original modes and copies. Clearing makes the redo history empty, like any
  new action. Clearing an **empty canvas is not an action**: nothing changes, nothing becomes undoable, and
  the redo history stays as it was.
- Changing the mode is **not** an action: it is not undoable and does not touch the redo history. Undo and
  redo never change the current mode.

### The hook

The page exposes `window.__mirror` (synchronously, as soon as the page has loaded):

```js
window.__mirror = {
  setMode(mode) {},                    // 'single' | 'mirror' | 'turn4' | 'kaleido8'; anything else is ignored
  addStroke({ points, color, width }) {}, // points: [[x, y], ...]; uses the CURRENT mode; one undoable action
  undo() {},
  redo() {},
  clear() {},
  getState() {},   // see below
}
```

```js
getState() → {
  mode: 'turn4',                    // the current mode
  strokes: [                        // in drawing order
    {
      points: [[10, 20], [30, 40]], // normalized points, arrays of [x, y]
      color: '#ff3366',             // exactly as it was given (a string)
      width: 4,                     // exactly as it was given (a number)
      mode: 'mirror',               // the mode when the stroke was created
      copies: [                     // one array of points per copy, in the order of the table above
        [[10, 20], [30, 40]],       // copy 0 is always the original points
        [[246, 20], [226, 40]]      // ...then the other copies of this stroke's mode
      ]
    }
  ],
  canUndo: true,
  canRedo: false
}
```

`getState()` returns a snapshot (plain arrays and objects; changing it must not change the studio). The
hook and the screen always agree: everything drawn is in `getState()` and the other way round. Any extra
fields are fine. `setMode`, `undo`, `redo`, `clear` and `addStroke` act immediately: the state read right
after the call already includes them.

### Drawing with the mouse

- Press the button inside the **`drawing`** element, move, release: that is one stroke. It is added to
  the state when the button is released (a stroke in progress may be shown live) and uses the current mode,
  color and width of the studio's own controls. Its points are normalized like any other.
- The element's box maps to the field **linearly on both axes**: its left edge is `x = 0`, its right edge
  `x = 256`, its top edge `y = 0`, its bottom edge `y = 256`. So a drag from 25 % to 75 % of the box width
  lands at about `x = 64 … 192`. Make the element square so the picture isn't stretched, and don't put a
  border or padding on it that shifts its content relative to the box.
- Pointer events and mouse events should both work (use pointer events and `setPointerCapture`; add
  `touch-action: none` so a finger can draw too).
- At a 1024 × 768 viewport the whole `drawing` element is visible without scrolling, and nothing covers it.

### Elements (`data-testid`)

| `data-testid` | What it is |
|---------------|------------|
| `drawing` | the canvas (or SVG) that receives the pointer input, see above |
| `mode` | a **`<select>`** whose option values are exactly `single`, `mirror`, `turn4`, `kaleido8` (labels are yours). Its value is always the current mode: choosing an option sets the mode, and `setMode` updates the select |
| `undo` | a `<button>`; it is `disabled` exactly when `canUndo` is `false` |
| `redo` | a `<button>`; it is `disabled` exactly when `canRedo` is `false` |
| `clear` | a `<button>` that does what `clear()` does |

## Test mode

With **`?test=1`** in the URL the canvas starts **empty** (`strokes: []`, `canUndo` and `canRedo` both
`false`) and nothing may draw on its own: no animation that changes the state, no demo strokes, nothing
restored from `localStorage`. Without `?test=1` you are free to greet people with a starting drawing, a
demo or a saved session. The initial mode is up to you (any of the four).

## Yours to invent

Everything people see: the theme (a glass-and-neon atelier, a paper-cut workshop, a frost window, stained
glass, a night garden, a spaceship's insignia machine…), the palette and the width controls, how the
modes are named and illustrated, the guide lines of the symmetry, the background, the stroke rendering
(glow, brush, gradient, sparkle), a starting drawing, saving to PNG, keyboard shortcuts, sound. The
contract fixes what the copies *are*; how they look is the whole point of the entry. Respect
`prefers-reduced-motion`, and make it usable on a phone too.
