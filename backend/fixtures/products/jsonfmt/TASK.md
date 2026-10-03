# jsonfmt

Build a command-line tool that reads **one JSON document on stdin**, checks it strictly, and either
pretty-prints it or says exactly where it is broken. Think `jq .` with a very picky parser.

## Contract

- The entry point is `main.py` at the root of your zip. It is started as `python3 main.py [flags]` with the
  Python standard library only (Python 3.12, no network, no pip). Input is UTF-8.
- Flags, in any order, each at most once: `--indent N` (N is a whole number from 1 to 8, default 2),
  `--compact`, `--sort-keys`. `--indent` together with `--compact` is a usage error.
- Any other argument, a missing or bad `N`, a repeated flag: print nothing to stdout, a message to stderr,
  exit **2**. Usage errors win over input errors.

### Valid input: exit 0

The document must be exactly RFC 8259 JSON: one value, optionally surrounded by whitespace (only space, tab,
`\n`, `\r`), nothing else. No comments, no trailing commas, no single quotes, no `NaN`/`Infinity`, no leading
zeros (`01`), no `.5`, no `1.`, no `+1`, no raw control characters inside strings. A UTF-8 byte order mark
(U+FEFF) is not whitespace. Object keys must be **unique**; two keys that are equal *after decoding
escapes* (`"a"` and `"a"`) are a duplicate.

Print the document followed by one newline:

- Only the whitespace between tokens changes. **Numbers and strings are copied exactly as written**: `1E5`,
  `-0`, `0.10` and `12345678901234567890123` stay as they are, `"é"` and `"\/"` stay escaped,
  `"é"` stays `"é"`. Do not round-trip through floats or re-escape anything.
- Pretty form: every array element and object member on its own line, indented by N spaces per level,
  `"key": value` with one space after the colon, commas at the end of lines, no trailing whitespace.
  Empty `{}` and `[]` stay on one line.
- `--compact`: no whitespace at all (`{"a":[1,2]}`), not even after `:` or `,`.
- `--sort-keys`: members of every object are ordered by the **decoded** key, compared by Unicode code point
  (so `"B"` comes before `"a"`, and `"😀"` (U+1F600) comes after `"￿"`). Keys are printed as
  written; arrays keep their order.

### Invalid input: exit 1

Print `LINE:COLUMN` and a newline to stdout (nothing else; put a human message on stderr if you like).
The position is that of the **first character that makes the input invalid**: the first character that cannot
continue any valid document that starts with the text before it.

- If the input just ends too early (unterminated string or array, empty input, a lone `-`) the position is the
  one right after the last character.
- For a duplicate key it is the opening quote of the second occurrence.
- Lines and columns start at 1. A line ends at `\n` only (`\r` is an ordinary character in the line before
  it). The column counts **Unicode characters** (code points), not bytes and not UTF-16 units.
- Examples: `[1,]` is `1:4`; `{"a":1,}` is `1:8`; `[01]` is `1:3`; `tru` is `1:4`; `[1,` followed by a
  newline is `2:1`; `["é😀" x]` is `1:7`.

### Limits

Nesting can be very deep: 6 000 levels of `[` must work (use `--compact`; do not recurse on the Python
call stack), and an error after 8 000 open brackets must still be reported at the right column. Each run
has 10 seconds.

## Scoring

Your tool is run against a set of scenarios (stdin + args, expected stdout and exit code), covering
formatting, every flag, dozens of invalid documents with exact positions, usage errors and deep nesting.
Your score is the share of scenarios that pass. After the deadline all entries are published and people vote
on them. You can upload up to 3 times; your best run counts.
