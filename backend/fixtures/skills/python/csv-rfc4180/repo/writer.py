"""Writing rows as CSV text."""
from reader import check_delimiter

EOL = "\n"


def format_field(value, delimiter=",", alone=False):
    """One field as CSV text.

    None is the empty string and everything else goes through str(). The field
    is quoted when it holds the delimiter, a quote, CR or LF, or starts or ends
    with a space or tab (so that readers that trim still see it intact), and
    also when it is empty and the only field of its row (`alone`): an empty
    line would be skipped by the reader. Quotes inside are doubled.
    """
    s = str(value)
    needs = any(c in s for c in (delimiter, '"', "\n"))
    if needs:
        return '"' + s.replace('"', '""') + '"'
    return s


def write_row(row, delimiter=","):
    row = list(row)
    if not row:
        raise ValueError("a row needs at least one field")
    alone = len(row) == 1
    return delimiter.join(format_field(v, delimiter, alone) for v in row) + EOL


def write_rows(rows, delimiter=","):
    """Rows as CSV text, every row (the last one too) ended by CRLF.
    read_rows(write_rows(rows)) gives the rows back as strings."""
    check_delimiter(delimiter)
    return "".join(write_row(r, delimiter) for r in rows)
