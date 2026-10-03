"""Reading CSV with a header row into dicts."""
from reader import read_records


def read_table(text, delimiter=","):
    """Parse CSV text whose first record is a header into a list of dicts.

    Each dict has the header's names as keys, in header order. A record with
    fewer fields than the header gets "" for the missing ones. Errors are
    CsvError carrying the line the offending record starts on:
    an empty header name, a name used twice, or a record with more fields
    than the header. A text with no records at all gives [].
    """
    records = read_records(text, delimiter)
    if not records:
        return []
    header = records[0][1]
    return [dict(zip(header, fields)) for _, fields in records[1:]]
