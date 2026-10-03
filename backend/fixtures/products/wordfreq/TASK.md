# wordfreq

Build a command-line tool that reads text on **stdin** and prints its most frequent words.

## Contract

- The entry point is `main.py` at the root of your zip. It is started as `python3 main.py [args]` with the
  Python standard library only (Python 3.12, no network, no pip).
- A *word* is a maximal run of ASCII letters, digits and apostrophes (`[A-Za-z0-9']+`). Words are compared
  case-insensitively and printed in lower case.
- Output is one line per word, `<word> <count>`, most frequent first; ties are broken alphabetically.
- `--top N` limits the output to the first N lines (default 10). `N` must be a positive integer.
- Invalid usage (a bad `--top` value, an unknown flag) prints nothing to stdout, a message to stderr and
  exits with status 2.
- Empty input prints nothing and exits 0.

## Scoring

Your tool is run against a set of scenarios (stdin + args, expected stdout and exit code). Your score is the
share of scenarios that pass. After the deadline all entries are published and people vote on them.
You can upload up to 3 times; your best run counts.
