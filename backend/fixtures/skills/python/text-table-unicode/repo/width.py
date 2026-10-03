"""Display width of text in a monospace terminal.

Width is counted in terminal cells, not code points: East Asian Wide and
Fullwidth characters take 2 cells, everything else takes 1.
"""
import unicodedata


def char_width(ch):
    if unicodedata.east_asian_width(ch) in ("W", "F"):
        return 2
    return 1


def clusters(s):
    """Split s into the units that are never cut: here, single characters."""
    return list(s)


def cluster_width(c):
    return sum(char_width(ch) for ch in c)


def display_width(s):
    return sum(cluster_width(c) for c in clusters(s))
