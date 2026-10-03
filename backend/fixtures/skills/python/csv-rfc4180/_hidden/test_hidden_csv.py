import random

import pytest

from reader import read_records, read_rows
from table import read_table
from tokenizer import CsvError
from writer import write_rows


def test_hidden_line_endings_bom_and_quoted_breaks():
    assert read_rows("a,b\r\nc,d\r\n") == [["a", "b"], ["c", "d"]]
    assert read_rows("a,b\rc,d\r") == [["a", "b"], ["c", "d"]]
    assert read_rows("a\nb\r\nc\rd") == [["a"], ["b"], ["c"], ["d"]]
    assert read_rows("﻿id,name\n1,x\n") == [["id", "name"], ["1", "x"]]
    assert read_rows("﻿") == []
    assert read_rows("") == [] and read_rows("\n\r\n\r") == []
    # line breaks inside quotes survive byte for byte
    assert read_rows('"a\r\nb",c\r\n') == [["a\r\nb", "c"]]
    assert read_rows('"a\rb","c\nd"\n') == [["a\rb", "c\nd"]]
    assert read_rows('"x\r\n\r\ny"') == [["x\r\n\r\ny"]]
    assert read_rows('"line1\r\nline2",2\r\n"3\n",4') == [["line1\r\nline2", "2"], ["3\n", "4"]]
    # only the first character decides whether a field is quoted; a BOM is only dropped at the very start
    assert read_rows('a,﻿b\n') == [["a", "﻿b"]]
    assert read_rows('"﻿q"') == [["﻿q"]]
    assert read_rows("a\tb\n", delimiter="\t") == [["a", "b"]]
    assert read_rows("a|\"b|c\"|d", delimiter="|") == [["a", "b|c", "d"]]
    for bad in ("", ",,", '"', "\n", "\r", "ab"):
        with pytest.raises(ValueError):
            read_rows("a,b", delimiter=bad)


def test_hidden_empty_fields_and_blank_lines():
    assert read_rows('""\n') == [[""]]
    assert read_rows('""') == [[""]]
    assert read_rows('a\n""\nb\n') == [["a"], [""], ["b"]]
    assert read_rows(",\n") == [["", ""]]
    assert read_rows(",,\r\n,,") == [["", "", ""], ["", "", ""]]
    assert read_rows("a,b,\n") == [["a", "b", ""]]
    assert read_rows("a,b,") == [["a", "b", ""]]
    assert read_rows("a,") == [["a", ""]]
    assert read_rows(",") == [["", ""]]
    assert read_rows('a,""') == [["a", ""]]
    assert read_rows('a,"",\n') == [["a", "", ""]]
    assert read_rows("a\n\n\nb\n\n") == [["a"], ["b"]]
    assert read_rows("\n\na,b\r\n\r\n\r\nc,d") == [["a", "b"], ["c", "d"]]
    assert read_rows(" \n") == [[" "]]
    assert read_rows(" a , b \n") == [[" a ", " b "]]
    assert read_rows("a;;b\n", delimiter=";") == [["a", "", "b"]]
    assert read_rows(',"a"\n') == [["", "a"]]
    assert read_rows('"a",\n') == [["a", ""]]
    assert read_rows('"a",') == [["a", ""]]
    assert read_rows('""""\n') == [['"']]
    assert read_rows('"a""b"""\n') == [['a"b"']]


def test_hidden_errors_carry_the_starting_line():
    def err(text, **kw):
        with pytest.raises(CsvError) as e:
            read_rows(text, **kw)
        return e.value

    assert err('a,b\nc,"d\n').line == 2
    assert err('a,b\nc,"d\ne\nf').line == 2  # unterminated: where the record starts, not where the text ends
    assert err('a\n"x\ny",1\nz,"w\r\nv').line == 4
    assert err('"x\ny"\n"p\rq"\nr,"open').line == 5  # every CRLF, LF and CR is one break, quoted or not
    assert err('a,b"c"\n').line == 1
    assert err('ok\n\n\nab"c\n').line == 4  # blank lines count
    assert err('ok\r\n"q\r\nr",x\r\na, "b"\n').line == 4
    assert err('ok\n"a"b,c\n').line == 2
    assert err('"a"x').line == 1
    assert err('one\rtwo\r"x" ,3\r').line == 3
    assert isinstance(err('"'), ValueError)
    assert read_records('a,b\n\n"c\nd",e\r\nf\n') == [(1, ["a", "b"]), (3, ["c\nd", "e"]), (5, ["f"])]
    assert read_records("﻿x\rq") == [(1, ["x"]), (2, ["q"])]


def test_hidden_writer_quoting_and_terminators():
    assert write_rows([]) == ""
    assert write_rows([["a", "b"], ["c", "d"]]) == "a,b\r\nc,d\r\n"
    assert write_rows([[None, "x", 3, 1.5, True]]) == ",x,3,1.5,True\r\n"
    assert write_rows([[""]]) == '""\r\n'
    assert write_rows([[None]]) == '""\r\n'
    assert write_rows([["", ""]]) == ",\r\n"
    assert write_rows([["a"], [""], ["b"]]) == 'a\r\n""\r\nb\r\n'
    assert write_rows([['say "hi"', "a,b"]]) == '"say ""hi""","a,b"\r\n'
    assert write_rows([["l1\nl2", "l1\r\nl2", "l1\rl2"]]) == '"l1\nl2","l1\r\nl2","l1\rl2"\r\n'
    assert write_rows([[" a", "b ", "\tc", "d\t", " ", "a b"]]) == '" a","b ","\tc","d\t"," ",a b\r\n'
    assert write_rows([["a;b", "a,b"]], delimiter=";") == '"a;b";a,b\r\n'
    assert write_rows([["a\tb", "x y"]], delimiter="\t") == '"a\tb"\tx y\r\n'
    assert write_rows([["é", "日本", "plain-text_1.0"]]) == "é,日本,plain-text_1.0\r\n"
    with pytest.raises(ValueError):
        write_rows([["a"], []])
    with pytest.raises(ValueError):
        write_rows([["a"]], delimiter='"')


NASTY = ["", " ", "a", "a,b", 'q"uote', '"', '""', "x\ny", "x\r\ny", "x\ry", " lead", "trail ", "\t", ";", "|", "é日本",
         "﻿", "a;b|c", ",", "\n", "\r\n", "  ", "end\\"]


def test_hidden_round_trip():
    rng = random.Random(4180)
    for delim in [",", ";", "\t", "|"]:
        for _ in range(60):
            rows = [[rng.choice(NASTY) for _ in range(rng.randint(1, 5))] for _ in range(rng.randint(1, 6))]
            text = write_rows(rows, delimiter=delim)
            assert read_rows(text, delimiter=delim) == rows, (delim, rows, text)
    # a BOM that is data in the first field survives, because it is quoted by the writer only if needed:
    # the writer must therefore not emit it bare in first position
    rows = [["﻿name", "b"]]
    assert read_rows(write_rows(rows)) == rows
    assert read_rows(write_rows([["", ""], [""], ["x", ""]])) == [["", ""], [""], ["x", ""]]


def test_hidden_table():
    assert read_table("id,name\r\n1,Ann\r\n2,Bob") == [{"id": "1", "name": "Ann"}, {"id": "2", "name": "Bob"}]
    assert read_table("﻿id,name\n1,Ann\n") == [{"id": "1", "name": "Ann"}]
    assert read_table("id,name\n") == [] and read_table("") == [] and read_table("\n\n") == []
    got = read_table("a,b,c\n1\n2,\n3,4,5\n,,\n")
    assert got == [
        {"a": "1", "b": "", "c": ""}, {"a": "2", "b": "", "c": ""},
        {"a": "3", "b": "4", "c": "5"}, {"a": "", "b": "", "c": ""},
    ]
    assert [list(r) for r in got] == [["a", "b", "c"]] * 4
    assert read_table('k,v\n"a\r\nb","x,y"\n') == [{"k": "a\r\nb", "v": "x,y"}]
    assert read_table('x\n""\n') == [{"x": ""}]
    assert read_table("a;b\n1;2\n", delimiter=";") == [{"a": "1", "b": "2"}]

    def err(text):
        with pytest.raises(CsvError) as e:
            read_table(text)
        return e.value.line

    assert err("a,b,a\n1,2,3\n") == 1
    assert err("\n\nid,name,id\n") == 3
    assert err("a,,c\n") == 1
    assert err("a,b\n1,2\n3,4,5\n") == 3
    assert err('a,b\n"x\ny",2\n1,2,3\n') == 4
    assert err('a,b\n1,2,\n') == 2  # a trailing delimiter makes a third, empty field
    assert err("a,b\n1,\"2\n") == 2  # reader errors propagate with their line
    assert err('"",b\n') == 1
