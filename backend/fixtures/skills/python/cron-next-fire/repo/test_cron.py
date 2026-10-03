from datetime import datetime

import pytest

from cronparse import CronError, parse
from schedule import matches, next_after, next_n


def test_parse_simple():
    s = parse("5,10 8-10 * * *")
    assert s.minutes == {5, 10}
    assert s.hours == {8, 9, 10}
    assert s.days == set(range(1, 32))


def test_step_in_minutes():
    assert parse("*/15 * * * *").minutes == {0, 15, 30, 45}


def test_bad_expression():
    with pytest.raises(CronError):
        parse("61 * * * *")
    with pytest.raises(CronError):
        parse("* * * *")


def test_next_after_daily():
    s = parse("30 9 * * *")
    assert next_after(s, datetime(2026, 3, 1, 8, 0)) == datetime(2026, 3, 1, 9, 30)
    assert next_after(s, datetime(2026, 3, 1, 9, 30)) == datetime(2026, 3, 2, 9, 30)


def test_next_n_and_matches():
    s = parse("0 12 * * mon")
    fires = next_n(s, datetime(2026, 3, 1), 3)
    assert fires == [datetime(2026, 3, 2, 12), datetime(2026, 3, 9, 12), datetime(2026, 3, 16, 12)]
    assert all(matches(s, f) for f in fires)
