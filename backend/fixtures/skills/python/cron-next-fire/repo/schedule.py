"""Computing fire times of a parsed cron Schedule."""
from datetime import datetime, time, timedelta

from cronparse import CronError, parse

# 29 February comes round every four years; search that far ahead.
HORIZON_DAYS = 366 * 4


def day_matches(sched, d):
    """Does date `d` satisfy the day-of-month, month and day-of-week fields?

    The day of month and the day of week must both match.
    """
    if d.month not in sched.months:
        return False
    dom = d.day in sched.days
    dow = d.isoweekday() % 7 in sched.weekdays
    return dom and dow


def matches(sched, dt):
    """Does the minute containing `dt` (seconds are ignored) fire?"""
    return day_matches(sched, dt.date()) and dt.hour in sched.hours and dt.minute in sched.minutes


def next_after(sched, dt):
    """The first fire time strictly after `dt`, on a whole minute, with dt's tzinfo.

    Raises CronError if the expression never fires (e.g. 30 February).
    """
    start = dt + timedelta(minutes=1)
    day = start.date()
    for _ in range(HORIZON_DAYS):
        if day_matches(sched, day):
            for h in sorted(sched.hours):
                for m in sorted(sched.minutes):
                    cand = datetime.combine(day, time(h, m), tzinfo=dt.tzinfo)
                    if cand >= start:
                        return cand
        day += timedelta(days=1)
    raise CronError("expression never fires")


def next_n(sched, dt, n):
    out = []
    for _ in range(n):
        dt = next_after(sched, dt)
        out.append(dt)
    return out
