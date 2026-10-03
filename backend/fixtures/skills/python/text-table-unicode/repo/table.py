"""Plain-text tables."""
from wrap import pad, wrap


def render_table(rows, headers=None, max_col_width=None, align=None):
    """Render rows (lists of cells) as a bordered text table.

        +-------+----+
        | name  | n  |
        +-------+----+
        | alice | 12 |
        +-------+----+

    Cells are converted with str() (None becomes ""), then wrapped to
    `max_col_width` cells when it is given. A column is as wide as its widest
    line. `align` lists "left"/"right"/"center" per column.
    """
    table = [list(r) for r in rows]
    if headers is not None:
        table.insert(0, list(headers))
    if not table:
        return ""
    ncols = len(table[0])
    if any(len(r) != ncols for r in table):
        raise ValueError("rows have different numbers of cells")
    aligns = list(align) if align else ["left"] * ncols
    if len(aligns) != ncols:
        raise ValueError("align must have one entry per column")

    cells = []
    for r in table:
        row = []
        for c in r:
            text = "" if c is None else str(c)
            row.append(wrap(text, max_col_width) if max_col_width else text.split("\n"))
        cells.append(row)

    widths = [max(1, max(len(line) for row in cells for line in row[i])) for i in range(ncols)]
    border = "+" + "+".join("-" * (w + 2) for w in widths) + "+"

    def render_row(row):
        height = max(len(c) for c in row)
        out = []
        for k in range(height):
            parts = []
            for i, c in enumerate(row):
                parts.append(" " + pad(c[k] if k < len(c) else "", widths[i], aligns[i]) + " ")
            out.append("|" + "|".join(parts) + "|")
        return out

    lines = [border]
    for n, row in enumerate(cells):
        lines.extend(render_row(row))
        if n == 0 and headers is not None:
            lines.append(border)
    lines.append(border)
    return "\n".join(lines)
