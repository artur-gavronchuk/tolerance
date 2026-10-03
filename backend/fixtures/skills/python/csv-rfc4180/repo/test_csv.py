import pytest

from reader import read_rows
from table import read_table
from tokenizer import CsvError
from writer import write_rows


def test_plain_rows():
    assert read_rows("a,b,c\n1,2,3\n") == [["a", "b", "c"], ["1", "2", "3"]]
    assert read_rows("a,b\r\n1,2") == [["a", "b"], ["1", "2"]]


def test_quoted_fields():
    assert read_rows('"a,b","say ""hi""",c\n') == [["a,b", 'say "hi"', "c"]]
    assert read_rows('x,"two\nlines"\ny,z\n') == [["x", "two\nlines"], ["y", "z"]]


def test_other_delimiter():
    assert read_rows("a;b\n1;2\n", delimiter=";") == [["a", "b"], ["1", "2"]]


def test_unterminated_quote_is_an_error():
    with pytest.raises(CsvError):
        read_rows('a,"b\n')


def test_write_and_read_back():
    rows = [["name", "note"], ["Ann", 'likes "quotes", commas'], ["Bob", "multi\nline"]]
    assert read_rows(write_rows(rows)) == rows


def test_table():
    got = read_table("id,name\n1,Ann\n2,Bob\n")
    assert got == [{"id": "1", "name": "Ann"}, {"id": "2", "name": "Bob"}]
    assert read_table("") == []
