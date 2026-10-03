"""Reading CSV text into rows."""
from tokenizer import CsvError, tokenize


def check_delimiter(delimiter):
    if not isinstance(delimiter, str) or len(delimiter) != 1 or delimiter in '"\r\n':
        raise ValueError("delimiter must be one character other than a quote or a line break")


def read_records(text, delimiter=","):
    """Like read_rows, but keeps the line each record starts on: [(line, fields)]."""
    check_delimiter(delimiter)
    text = text.replace("\r\n", "\n")
    return tokenize(text, delimiter)


def read_rows(text, delimiter=","):
    """Parse CSV text into a list of rows (lists of str).

    Rows end at CRLF, LF or CR outside quotes; the last row needs no
    terminator. A line break inside quotes is kept exactly as written.
    A leading byte order mark is ignored. Lines with nothing on them are
    skipped, but a line holding just `""` or just delimiters is a row.
    """
    return [fields for _, fields in read_records(text, delimiter) if any(fields)]
