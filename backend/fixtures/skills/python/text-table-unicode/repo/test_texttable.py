from table import render_table
from width import display_width
from wrap import pad, truncate, wrap


def test_ascii_width():
    assert display_width("hello") == 5


def test_cjk_width():
    assert display_width("日本") == 4


def test_truncate_ascii():
    assert truncate("hello world", 8) == "hello w…"
    assert truncate("hi", 8) == "hi"


def test_pad_ascii():
    assert pad("ab", 4) == "ab  "
    assert pad("ab", 4, "right") == "  ab"


def test_wrap_ascii():
    assert wrap("the quick brown fox", 9) == ["the quick", "brown fox"]


def test_table_ascii():
    out = render_table([["a", "1"], ["bb", "22"]], headers=["k", "v"])
    assert out.split("\n")[0] == "+----+----+"
