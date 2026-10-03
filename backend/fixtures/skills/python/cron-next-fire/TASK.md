# Cron next-fire times

`cronparse.py` parses classic 5-field cron expressions into a `Schedule`
(`minutes`, `hours`, `days`, `months`, `weekdays` as frozensets, plus the flags
`dom_star` and `dow_star`), and `schedule.py` computes fire times from it
(`next_after`, `next_n`, `matches`). Real expressions give wrong days and
wrong times. Fix the code without editing `test_*.py` files. Hidden tests
exercise the contract below.

## Expressions

`minute hour day-of-month month day-of-week`, separated by any run of spaces
and tabs (leading and trailing whitespace is ignored). Ranges: minute 0-59,
hour 0-23, day 1-31, month 1-12, weekday 0-7 where both 0 and 7 are Sunday
(after parsing, `weekdays` only contains 0-6). Each field is a comma-separated list of:

- `*`: the whole range of the field (for weekday: all seven days)
- `a`: one value
- `a-b`: an inclusive range; `a > b` is an error (no wrap-around, so `sat-sun`
  is an error but `5-7` is Friday to Sunday)
- any of the above with `/n` (n >= 1): every n-th value counted from the start of
  the range. `*/n` starts at the field's lowest value (so `*/10` in the day field is
  1, 11, 21, 31), and `a/n` means `a-<top of the field>/n` (so `5/20` in the minute
  field is 5, 25, 45). For weekday the top is 7, so `1/3` is Monday, Thursday and
  Sunday (1, 4 and 7 = 0).

Month and weekday fields also accept three-letter English names
(`jan`..`dec`, `sun`..`sat`) in any letter case, alone or as range ends.
The macros `@yearly`/`@annually`, `@monthly`, `@weekly` (Sunday), `@daily`/`@midnight`
and `@hourly` are accepted in any letter case; other macros are errors. Anything
malformed (wrong field count, empty items, non-ASCII digits, signs, values out of
range, a zero step, ...) raises `CronError`.

## Which days fire

If both the day-of-month and the day-of-week field are restricted, a day fires when
**either** matches (`0 0 15 * mon`: the 15th and every Monday). If either of the two
fields *starts with `*`* (`*`, `*/2`, `*,5`), a day must satisfy **both** (`0 0 */2 * mon`:
odd days that are also Mondays). The month field always has to match.

## Computing fire times

`next_after(sched, dt)` returns the first fire time strictly after `dt`, on a whole
minute (seconds and microseconds zero), keeping `dt`'s `tzinfo`. A `dt` of
`10:30:00.000001` is already past `10:30`. Months have their real lengths and
leap years follow the Gregorian calendar (2100 is not a leap year, so after
29 February 2096 the next one is in 2104). An expression that can never fire
(`0 0 30 2 *`, `0 0 31 4 *`) raises `CronError` from `next_after`.
`matches(sched, dt)` says whether the minute containing `dt` fires and must agree
with `next_after`.
