# cronnext

Build a command-line tool that answers "when will this cron job run next?": given a cron expression and a
starting moment, it prints the next N run times. No clock, no time zones: every time is a plain
local date and time that you compute on the proleptic Gregorian calendar.

## Contract

- The entry point is `main.py` at the root of your zip. It is started as
  `python3 main.py --from YYYY-MM-DDTHH:MM [-n COUNT] "<expression>"` with the Python standard library only
  (Python 3.12, no network, no pip). Arguments may come in any order; the expression is one argument.
- `--from` is required: exactly `YYYY-MM-DDTHH:MM` (four-digit year 1970 to 2100, two-digit everything else, a
  real calendar date, hour 00-23, minute 00-59). `-n` is a whole number from 1 to 1000, default 5. Each flag at
  most once.
- Anything wrong with the arguments or the expression: print nothing to stdout, a message to stderr, exit **2**.

### The expression

Five fields separated by any amount of spaces or tabs: `minute hour day-of-month month day-of-week`.

| field | values | names |
|-------|--------|-------|
| minute | 0-59 | |
| hour | 0-23 | |
| day-of-month | 1-31 | |
| month | 1-12 | `jan` ... `dec` |
| day-of-week | 0-7, where **0 and 7 are both Sunday** | `sun` ... `sat` |

Names are case-insensitive, three letters, and only valid in their own field (also inside lists and ranges).

A field is a comma-separated list (no empty items) of: `*`, a value, a range `a-b` (`a <= b`; ranges do not wrap,
`fri-mon` is an error), each optionally followed by `/step` (a whole number >= 1) when it is `*` or a range.
`*/15` means every 15th value starting at the field's minimum (`*` in the day-of-week field is 0-6). A step on a
single value (`5/15`) is an error. Anything out of range, with a sign, a decimal point or a stray character is an
error.

Macros replace the whole expression: `@yearly` and `@annually` (`0 0 1 1 *`), `@monthly` (`0 0 1 * *`),
`@weekly` (`0 0 * * 0`), `@daily` and `@midnight` (`0 0 * * *`), `@hourly` (`0 * * * *`). Lower case only; any
other `@word` is an error.

**Day-of-month and day-of-week:** a day matches when the month matches and
- both fields are *restricted*: the day-of-month **or** the day-of-week matches (`0 0 13 * fri` is every 13th
  and every Friday);
- otherwise (at least one field is exactly `*`): both must match.

A field is restricted unless its text is exactly `*`; `*/2` is restricted.

### Output

The run times are the minutes that match, **strictly after** `--from`, in ascending order, one per line as
`YYYY-MM-DD HH:MM`. Exit 0.

Only runs up to the end of year `(--from year + 10)` are considered. If fewer than N runs exist in that window
print the ones that do (exit 0), so `0 0 29 2 *` from 2026 with `-n 1000` prints 2028, 2032 and 2036. If there
is none at all (`0 0 31 2 *`), print nothing and exit **1**.

### Limits

Each run has 10 seconds, so do not step minute by minute through ten years: jump to the next candidate day.
Mind the calendar: leap years (2100 is not one), months with 30 days, month and year rollovers.

## Scoring

Your tool is run against a set of scenarios (args, expected stdout and exit code): ordinary schedules, every
syntax rule, the day-of-month/day-of-week rule, leap years, empty results and every kind of invalid argument.
Your score is the share of scenarios that pass. After the deadline all entries are published and people vote
on them. You can upload up to 3 times; your best run counts.
