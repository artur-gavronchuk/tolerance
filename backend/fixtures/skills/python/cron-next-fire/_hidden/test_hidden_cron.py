from datetime import datetime, timedelta, timezone

import pytest

from cronparse import CronError, parse
from schedule import matches, next_after, next_n


def D(*a):
    return datetime(*a)


def test_hidden_day_of_month_or_day_of_week():
    # both restricted: a day fires if either matches (Feb 2026: Mondays are 2, 9, 16, 23; the 15th is a Sunday)
    s = parse("0 0 15 * mon")
    assert next_n(s, D(2026, 2, 1), 6) == [
        D(2026, 2, 2), D(2026, 2, 9), D(2026, 2, 15), D(2026, 2, 16), D(2026, 2, 23), D(2026, 3, 2),
    ]
    # a "*"-led field makes it an AND: */2 on the day of month still counts as a star
    s = parse("0 0 */2 * mon")
    assert next_n(s, D(2026, 2, 1), 3) == [D(2026, 2, 9), D(2026, 2, 23), D(2026, 3, 9)]
    s = parse("0 0 * * mon")
    assert next_n(s, D(2026, 2, 1), 2) == [D(2026, 2, 2), D(2026, 2, 9)]
    s = parse("0 0 13 * *")
    assert next_n(s, D(2026, 2, 1), 2) == [D(2026, 2, 13), D(2026, 3, 13)]
    # weekday names, lists and ranges take part in the OR as well
    s = parse("0 0 1 jan sat,sun")
    assert next_n(s, D(2026, 1, 1), 3) == [D(2026, 1, 3), D(2026, 1, 4), D(2026, 1, 10)]


def test_hidden_steps_ranges_and_names():
    assert parse("0 0 */10 * *").days == {1, 11, 21, 31}
    assert parse("0 0 * */5 *").months == {1, 6, 11}
    assert parse("0 */7 * * *").hours == {0, 7, 14, 21}
    assert parse("5/20 * * * *").minutes == {5, 25, 45}
    assert parse("10-30/10 * * * *").minutes == {10, 20, 30}
    assert parse("0 0 2/10 * *").days == {2, 12, 22}
    assert parse("0 0 * * */2").weekdays == {0, 2, 4, 6}
    assert parse("0 0 * * 1/3").weekdays == {0, 1, 4}  # 1, 4, 7 and 7 is Sunday
    assert parse("0 0 * * 7").weekdays == {0}
    assert parse("0 0 * * 5-7").weekdays == {5, 6, 0}
    assert parse("0 0 * * 0,7").weekdays == {0}
    assert parse("0 0 * * mon-fri").weekdays == {1, 2, 3, 4, 5}
    assert parse("0 0 * * MON,Sun").weekdays == {1, 0}
    assert parse("0 0 1 Jan-Mar,dec *").months == {1, 2, 3, 12}
    assert parse("1,1,2 * * * *").minutes == {1, 2}
    assert parse("  0   0\t1 1 *  ").days == {1}
    assert parse("0 0 * * *").weekdays == set(range(7))
    assert parse("@hourly").minutes == {0} and parse("@hourly").hours == set(range(24))
    s = parse("@weekly")
    assert (s.minutes, s.hours, s.weekdays, s.dom_star) == ({0}, {0}, {0}, True)
    assert parse("@Yearly").months == {1} and parse("@monthly").days == {1}
    assert parse("@midnight").hours == {0} and parse("@annually").days == {1}


BAD_EXPRESSIONS = [
    "","* * * *", "* * * * * *", "60 * * * *", "* 24 * * *", "* * 0 * *", "* * 32 * *", "* * * 0 *",
    "* * * 13 *", "* * * * 8", "5-1 * * * *", "*/0 * * * *", "1/0 * * * *", "a * * * *", "1,,2 * * * *",
    "1- * * * *", "-1 * * * *", "*/ * * * *", "/5 * * * *", "1-2-3 * * * *", "*-5 * * * *", "@reboot",
    "* * * foo *", "* * * * mon-", "sat-sun * * * *", "0 0 * * sat-sun", "+1 * * * *", "1.5 * * * *",
    "١ * * * *", "*/٣ * * * *",
]


def test_hidden_rejects_bad_expressions():
    accepted = []
    for expr in BAD_EXPRESSIONS:
        try:
            parse(expr)
        except CronError:
            continue
        accepted.append(expr)
    assert accepted == []


def test_hidden_next_is_strictly_after_and_on_whole_minutes():
    s = parse("* * * * *")
    assert next_after(s, D(2026, 1, 1, 10, 0, 0)) == D(2026, 1, 1, 10, 1)
    assert next_after(s, D(2026, 1, 1, 10, 0, 30)) == D(2026, 1, 1, 10, 1)
    assert next_after(s, D(2026, 1, 1, 10, 0, 59, 999999)) == D(2026, 1, 1, 10, 1)
    s = parse("30 10 * * *")
    assert next_after(s, D(2026, 1, 1, 10, 29, 59, 999999)) == D(2026, 1, 1, 10, 30)
    assert next_after(s, D(2026, 1, 1, 10, 30, 0)) == D(2026, 1, 2, 10, 30)
    assert next_after(s, D(2026, 1, 1, 10, 30, 0, 1)) == D(2026, 1, 2, 10, 30)
    s = parse("*/30 * * * *")
    assert next_after(s, D(2026, 1, 1, 10, 45)) == D(2026, 1, 1, 11, 0)
    assert next_after(s, D(2026, 12, 31, 23, 45)) == D(2027, 1, 1, 0, 0)
    s = parse("15 3 * * *")
    assert next_after(s, D(2026, 1, 1, 23, 59)) == D(2026, 1, 2, 3, 15)
    # time zone information is carried over untouched
    tz = timezone(timedelta(hours=5))
    got = next_after(parse("0 9 * * *"), datetime(2026, 1, 1, 9, 0, 5, tzinfo=tz))
    assert got == datetime(2026, 1, 2, 9, 0, tzinfo=tz) and got.tzinfo == tz


def test_hidden_calendar_edges():
    s = parse("0 0 31 * *")
    assert next_n(s, D(2026, 1, 31), 4) == [D(2026, 3, 31), D(2026, 5, 31), D(2026, 7, 31), D(2026, 8, 31)]
    s = parse("0 0 29 2 *")
    assert next_after(s, D(2026, 3, 1)) == D(2028, 2, 29)
    assert next_after(s, D(2096, 3, 1)) == D(2104, 2, 29)  # 2100 is not a leap year
    assert next_after(s, D(2024, 2, 29)) == D(2028, 2, 29)
    for never in ("0 0 30 2 *", "0 0 31 4 *", "0 0 31 6,9,11 *"):
        with pytest.raises(CronError):
            next_after(parse(never), D(2026, 1, 1))
    assert next_after(parse("59 23 31 12 *"), D(2026, 12, 31, 23, 59)) == D(2027, 12, 31, 23, 59)
    assert next_after(parse("@yearly"), D(2026, 6, 1)) == D(2027, 1, 1)
    assert next_after(parse("@weekly"), D(2026, 2, 4, 12)) == D(2026, 2, 8)  # a Wednesday; weeks start on Sunday
    assert next_after(parse("@monthly"), D(2026, 12, 15)) == D(2027, 1, 1)
    # day of month OR weekday in February: every Sunday of the month, and 29 February
    assert next_after(parse("0 0 29 2 sun"), D(2027, 2, 21)) == D(2027, 2, 28)
    assert next_n(parse("0 0 29 2 sun"), D(2028, 2, 20), 3) == [D(2028, 2, 27), D(2028, 2, 29), D(2029, 2, 4)]
    assert next_after(parse("0 0 1 feb,aug *"), D(2026, 8, 1)) == D(2027, 2, 1)


EXPRESSIONS = [
    "*/15 9-17 * * mon-fri",
    "0 0 13 * fri",
    "30 4 1,15 * *",
    "0 */6 29-31 * *",
    "5/20 22 */10 * 0",
    "0 0 29 2 *",
]


def test_hidden_matches_agrees_with_next_after():
    start = D(2026, 1, 30, 22, 17, 41)
    for expr in EXPRESSIONS:
        s = parse(expr)
        fires = next_n(s, start, 8)
        assert fires == sorted(set(fires))
        for f in fires:
            assert f.second == 0 and f.microsecond == 0
            assert matches(s, f), (expr, f)
        # nothing in between fires: scan minute by minute (coarsely over the leap-day gap)
        step = timedelta(minutes=1) if expr != "0 0 29 2 *" else timedelta(hours=1)
        t = start.replace(second=0, microsecond=0) + timedelta(minutes=1)
        limit = min(fires[-1], start + timedelta(days=60))
        found = []
        while t <= limit:
            if matches(s, t):
                found.append(t)
            t += step
        assert found == [f for f in fires if f <= limit], expr
        # seconds do not matter to matches()
        assert matches(s, fires[0].replace(second=59))
