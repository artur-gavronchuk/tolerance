"""The CSV state machine: text in, (line, fields) records out."""


class CsvError(ValueError):
    """Malformed CSV. `line` is the 1-based physical line the record starts on."""

    def __init__(self, message, line):
        super().__init__("line %d: %s" % (line, message))
        self.line = line


def _skip_eol(text, i):
    """Step over the terminator at text[i] (CRLF, LF or CR); returns the new index."""
    if text.startswith("\r\n", i):
        return i + 2
    return i + 1


def tokenize(text, delimiter=","):
    """Split `text` into records.

    Returns a list of (line, fields): the physical line the record starts on
    (every CRLF, LF and CR counts as one line break, quoted ones included) and
    its list of field strings. Empty lines between records yield nothing.
    """
    records = []
    i, n, line = 0, len(text), 1
    stops = (delimiter, "\r", "\n")
    while i < n:
        if text[i] in "\r\n":  # an empty line
            i = _skip_eol(text, i)
            line += 1
            continue
        start_line = line
        fields = []
        while True:
            if i < n and text[i] == '"':
                i += 1
                buf = []
                while True:
                    if i >= n:
                        raise CsvError("unterminated quoted field", line)
                    ch = text[i]
                    if ch == '"':
                        if text.startswith('""', i):
                            buf.append('"')
                            i += 2
                            continue
                        i += 1
                        break
                    if ch in "\r\n":
                        j = _skip_eol(text, i)
                        buf.append(text[i:j])
                        i = j
                        line += 1
                        continue
                    buf.append(ch)
                    i += 1
                if i < n and text[i] not in stops:
                    raise CsvError("unexpected %r after closing quote" % text[i], start_line)
                fields.append("".join(buf))
            else:
                j = i
                while j < n and text[j] not in stops:
                    if text[j] == '"':
                        raise CsvError("quote inside an unquoted field", start_line)
                    j += 1
                fields.append(text[i:j])
                i = j
            if i < n and text[i] == delimiter:
                i += 1
                if i >= n:
                    break
                continue
            break
        records.append((start_line, fields))
        if i < n:  # a CR or LF
            i = _skip_eol(text, i)
            line += 1
    return records
