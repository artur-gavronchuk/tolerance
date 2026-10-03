# Text tables for terminals

Three small modules render text tables for a monospace terminal: `width.py`
(how wide is a string), `wrap.py` (`truncate`, `pad`, `wrap`) and `table.py`
(`render_table`). They are fine for ASCII, but tables with CJK text, accents
or emoji come out ragged and words get cut in the wrong places. Fix the code
without editing the `test_*.py` files. Hidden tests exercise the contract below.

## Width

Width is measured in terminal cells, not code points.

- East Asian Wide and Fullwidth characters (`unicodedata.east_asian_width` of
  `W` or `F`) take 2 cells.
- Characters of category `Mn`, `Me` (combining marks), `Cf` (format characters:
  zero width joiner, variation selectors, ...) and `Cc` (control characters)
  take 0 cells.
- Everything else takes 1 cell.

A *cluster* is the unit a reader sees as one character: a character plus all the
zero-width characters that follow it, and, after a zero width joiner (U+200D),
the next character as well (so an emoji family sequence is one cluster). A
cluster is never split by any function. The width of a cluster is the width of
its first character; the width of a string is the sum over its clusters.
`clusters(s)` returns the clusters as a list of strings that join back to `s`
(zero-width characters at the very start form a cluster of their own).

## `truncate(s, width, ellipsis="…")`

Returns `s` unchanged if it fits in `width` cells. Otherwise it keeps whole
clusters from the left while they fit next to the ellipsis and appends the
ellipsis. The first cluster that does not fit ends the text: a later, narrower
cluster is not used to fill the gap. If the ellipsis alone does not fit, the
result is the ellipsis cut by the same rule, without another ellipsis.
A negative width is a `ValueError`.

## `pad(s, width, align="left")`

Pads with spaces to `width` cells; `left`, `right` or `center` (the odd space
goes to the right side). A string already as wide as `width` or wider comes
back unchanged. Unknown alignment is a `ValueError`.

## `wrap(s, width)`

Returns a list of lines of at most `width` cells (`width < 1` is a
`ValueError`).

- `"\n"` is a hard break; an empty paragraph gives an empty line, and
  `wrap("", n)` is `[""]`.
- Words are separated by runs of spaces and tabs **only**. Any other
  character, including the no-break space U+00A0 and the ideographic space
  U+3000, belongs to a word.
- Lines are joined with single spaces; a word that does not fit on the current
  line starts a new one.
- A word wider than `width` is broken between clusters; the last piece stays
  open for the next word. A single cluster wider than `width` (a CJK character
  with `width == 1`) gets a line of its own; no empty line may appear before it.

## `render_table(rows, headers=None, max_col_width=None, align=None)`

Cells are converted with `str()` (`None` becomes `""`) and wrapped with `wrap` to
`max_col_width` cells if given (otherwise they are only split on `"\n"`). A
column is as wide, in cells, as its widest line (at least 1). `align` has one
entry per column and also applies to the header row. All lines of the output are
exactly as wide as the border. Rows of unequal length, or an `align` of the wrong
length, are a `ValueError`; no rows and no headers give `""`. See the docstring
for the layout.
