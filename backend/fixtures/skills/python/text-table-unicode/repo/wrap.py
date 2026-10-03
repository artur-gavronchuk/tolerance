"""Truncating, padding and wrapping text by display width."""
from width import clusters, cluster_width, display_width


def truncate(s, width, ellipsis="…"):
    """Shorten s to at most `width` cells, ending in `ellipsis` when cut."""
    if width < 0:
        raise ValueError("width must be >= 0")
    if display_width(s) <= width:
        return s
    room = width - display_width(ellipsis)
    if room < 0:
        return ellipsis[:width]
    out, used = [], 0
    for c in clusters(s):
        w = cluster_width(c)
        if used + w > room:
            continue
        out.append(c)
        used += w
    return "".join(out) + ellipsis


def pad(s, width, align="left"):
    """Pad s with spaces to `width` cells."""
    extra = width - len(s)
    if extra <= 0:
        return s
    if align == "left":
        return s + " " * extra
    if align == "right":
        return " " * extra + s
    if align == "center":
        right = extra // 2
        return " " * (extra - right) + s + " " * right
    raise ValueError("unknown align %r" % (align,))


def _break_word(word, width):
    """Split a word that is wider than `width` into pieces of at most `width` cells."""
    pieces = []
    cur, used = "", 0
    for c in clusters(word):
        w = cluster_width(c)
        if used + w > width:
            pieces.append(cur)
            cur, used = "", 0
        cur += c
        used += w
    pieces.append(cur)
    return pieces


def wrap(s, width):
    """Wrap s into lines of at most `width` cells; "\\n" is a hard line break."""
    if width < 1:
        raise ValueError("width must be >= 1")
    lines = []
    for para in s.split("\n"):
        words = para.split()
        if not words:
            lines.append("")
            continue
        cur, used = "", 0
        for word in words:
            w = display_width(word)
            if cur and used + 1 + w <= width:
                cur += " " + word
                used += 1 + w
                continue
            if cur:
                lines.append(cur)
                cur, used = "", 0
            if w <= width:
                cur, used = word, w
            else:
                *full, last = _break_word(word, width)
                lines.extend(full)
                cur, used = last, display_width(last)
        lines.append(cur)
    return lines
