import pytest

from table import render_table
from width import clusters, display_width
from wrap import pad, truncate, wrap

FAMILY = "\U0001F468‍\U0001F469‍\U0001F467"  # man ZWJ woman ZWJ girl
E_ACUTE = "é"


def test_hidden_display_width():
    assert display_width("") == 0
    assert display_width("abc") == 3
    assert display_width("日本語") == 6  # CJK
    assert display_width("Ａ") == 2  # fullwidth A
    assert display_width("ｱ") == 1  # halfwidth katakana
    assert display_width(E_ACUTE) == 1
    assert display_width("à́b") == 2  # two combining marks
    assert display_width(FAMILY) == 2
    assert display_width("❤️") == 1  # variation selector adds nothing
    assert display_width("a‍b") == 1  # a ZWJ joins the next character
    assert display_width("a\tb") == 2  # control characters take no cell
    assert display_width("́") == 0  # a lone mark


def test_hidden_clusters():
    assert clusters("") == []
    assert clusters("a" + E_ACUTE + "b") == ["a", E_ACUTE, "b"]
    assert clusters(FAMILY + "x") == [FAMILY, "x"]
    assert clusters("́a") == ["́", "a"]
    assert clusters("a‍b") == ["a‍b"]
    s = "x日" + E_ACUTE + FAMILY + "❤️!"
    assert "".join(clusters(s)) == s
    assert len(clusters(s)) == 6


def test_hidden_truncate():
    assert truncate("abc", 5) == "abc"
    assert truncate("abcdef", 5) == "abcd…"
    assert truncate("abcdef", 1) == "…"
    assert truncate("abcdef", 0) == ""
    assert truncate("abcdef", 4, "...") == "a..."
    assert truncate("abcdef", 2, "...") == ".."
    # never cut between a base character and its combining mark
    s = "héllo world"
    assert truncate(s, 5) == "héll…"
    assert display_width(truncate(s, 5)) == 5
    assert truncate(E_ACUTE * 4, 3) == E_ACUTE * 2 + "…"
    # a wide character that does not fit ends the text; narrower ones after it are not used
    assert truncate("日本語a", 4) == "日…"
    assert truncate("日本語テキスト", 6) == "日本…"
    assert truncate("日本", 1) == "…"
    # emoji sequences stay whole
    assert truncate(FAMILY + "xy", 3, "") == FAMILY + "x"
    assert truncate(FAMILY + FAMILY, 3, "") == FAMILY
    with pytest.raises(ValueError):
        truncate("abc", -1)


def test_hidden_pad():
    assert pad("ab", 5) == "ab   "
    assert pad("日本", 6) == "日本  "
    assert pad(E_ACUTE, 3, "right") == "  " + E_ACUTE
    assert pad("ab", 5, "center") == " ab  "
    assert pad("日", 5, "center") == " 日  "
    assert pad("日本語", 4) == "日本語"  # wider than width: unchanged
    assert pad(FAMILY, 4, "right") == "  " + FAMILY


def test_hidden_wrap():
    assert wrap("", 5) == [""]
    assert wrap("   ", 5) == [""]
    assert wrap("a\n\nb  c\t d", 10) == ["a", "", "b c d"]
    assert wrap("abcdefg hi", 5) == ["abcde", "fg hi"]
    assert wrap("aa bb cc", 5) == ["aa bb", "cc"]
    assert wrap("日本語のテキスト", 6) == ["日本語", "のテキ", "スト"]
    # only spaces and tabs separate words
    assert wrap("a b c", 3) == ["a b", "c"]
    assert wrap("x　y z", 4) == ["x　y", "z"]
    # clusters are never split, and combining marks take no room
    assert wrap(E_ACUTE * 4, 3) == [E_ACUTE * 3, E_ACUTE]
    assert wrap(FAMILY * 2 + "x", 2) == [FAMILY, FAMILY, "x"]
    # a cluster wider than the width gets its own line, with no empty line before it
    assert wrap("日a", 1) == ["日", "a"]
    assert wrap("a日", 1) == ["a", "日"]
    for line in wrap("The quick bröwn 狐 jumps over the lazy 犬 again and again", 7):
        assert display_width(line) <= 7
    with pytest.raises(ValueError):
        wrap("abc", 0)


def test_hidden_render_table():
    out = render_table(
        [["日本", "1"], [E_ACUTE + "a", "22"]],
        headers=["name", "n"],
        align=["left", "right"],
    )
    assert out == "\n".join([
        "+------+----+",
        "| name |  n |",
        "+------+----+",
        "| 日本 |  1 |",
        "| " + E_ACUTE + "a   | 22 |",
        "+------+----+",
    ])

    out = render_table([["日本語のテキスト", "x"]], max_col_width=6)
    assert out == "\n".join([
        "+--------+---+",
        "| 日本語 | x |",
        "| のテキ |   |",
        "| スト   |   |",
        "+--------+---+",
    ])

    out = render_table([["a\nbb", None, 3]], align=["center", "left", "right"])
    assert out == "\n".join([
        "+----+---+---+",
        "| a  |   | 3 |",
        "| bb |   |   |",
        "+----+---+---+",
    ])
    assert render_table([]) == ""
    assert render_table([], headers=["h"]) == "+---+\n| h |\n+---+\n+---+"
    with pytest.raises(ValueError):
        render_table([["a", "b"], ["c"]])
    with pytest.raises(ValueError):
        render_table([["a", "b"]], align=["left"])
    # every line of a table is exactly as wide as its border
    out = render_table([[FAMILY, "é́", "日"], ["x", "yy", "zzzz"]], headers=["a", "b", "c"], align=["center"] * 3)
    widths = {display_width(line) for line in out.split("\n")}
    assert len(widths) == 1
