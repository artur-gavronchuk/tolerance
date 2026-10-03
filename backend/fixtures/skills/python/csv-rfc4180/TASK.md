# CSV reader and writer

`tokenizer.py` (the state machine), `reader.py` (`read_rows`, `read_records`),
`writer.py` (`write_rows`) and `table.py` (`read_table`) read and write CSV
following RFC 4180 with the choices below. Files from real exporters (Windows
line endings, byte order marks, quoted newlines, empty cells) come out wrong,
and what `write_rows` writes does not always read back. Fix the code without
editing `test_*.py` files. Hidden tests exercise the contract below.

## Reading (`read_rows(text, delimiter=",")`)

- `delimiter` is one character, not a quote, CR or LF, otherwise `ValueError`.
- A record ends at CRLF, LF or a lone CR outside quotes; the last record needs no
  terminator, and a final terminator does not produce an extra record.
- A leading U+FEFF (byte order mark) is dropped.
- A line with nothing on it is skipped. That is the only thing that is skipped: a
  line holding just `""` is a record with one empty field, a line holding just
  delimiters (`,,`) is a record of empty fields, a line of spaces is a record
  with that field.
- A field is quoted when its first character is `"`. Inside, `""` is a quote, and
  everything else, line breaks included, is kept **exactly as written** (a CRLF stays
  CRLF, a lone CR stays CR). After the closing quote only a delimiter, a record
  terminator or the end of the text may follow.
- Whitespace is never trimmed. A delimiter right before the end of a record (`a,b,`)
  gives a last empty field, also at the very end of the text.
- Errors are `CsvError` (a `ValueError` with `.line`): a quote inside an unquoted
  field (`ab"c`, or `a, "b"`), text after a closing quote, and an unterminated
  quoted field. `.line` is the 1-based physical line the **record starts on**,
  where every CRLF, LF and CR, inside quotes or not, counts as one line break.
  `read_records` gives the same records as `(line, fields)` pairs.

## Writing (`write_rows(rows, delimiter=",")`)

Every row, the last one too, is ended by CRLF; no rows give `""`. A field is
`None` (written as empty) or anything `str()` accepts. A field is quoted, with
quotes doubled, when it contains the delimiter, a quote, CR or LF, when it
starts or ends with a space or tab, when it starts with U+FEFF (a reader would
take it for a byte order mark), or when it is the empty string and the only
field of its row (otherwise the line would be empty and skipped on reading).
Nothing else is quoted. A row without fields is a `ValueError`.
`read_rows(write_rows(rows))` gives back the rows for any non-empty rows of
strings, with any valid delimiter.

## Tables (`read_table(text, delimiter=",")`)

The first record is the header. Every other record becomes a dict with the header's
names as keys, in header order, with `""` for fields a short record does not have.
`CsvError` (with `.line` of the offending record) for an empty column name, a
column name that appears twice, and a record with more fields than the header.
Text with no records gives `[]`; a header alone gives `[]` too.
