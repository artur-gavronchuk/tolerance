"""Parsing of 5-field cron expressions into sets of allowed values."""


class CronError(ValueError):
    """Raised for an invalid expression or one that can never fire."""


MONTHS = {n: i + 1 for i, n in enumerate("jan feb mar apr may jun jul aug sep oct nov dec".split())}
WEEKDAYS = {n: i for i, n in enumerate("sun mon tue wed thu fri sat".split())}

# name, lowest and highest allowed value, where "*" runs to, names
FIELDS = [
    ("minute", 0, 59, 59, None),
    ("hour", 0, 23, 23, None),
    ("day", 1, 31, 31, None),
    ("month", 1, 12, 12, MONTHS),
    ("weekday", 0, 6, 6, WEEKDAYS),
]

MACROS = {
    "@yearly": "0 0 1 1 *",
    "@annually": "0 0 1 1 *",
    "@monthly": "0 0 1 * *",
    "@weekly": "0 0 * * 0",
    "@daily": "0 0 * * *",
    "@midnight": "0 0 * * *",
    "@hourly": "0 * * * *",
}


def _value(tok, lo, hi, names, field):
    tok = tok.strip()
    if names and tok in names:
        return names[tok]
    if not tok.isdecimal() or not tok.isascii():
        raise CronError("bad %s value %r" % (field, tok))
    n = int(tok)
    if not lo <= n <= hi:
        raise CronError("%s value %d out of range %d-%d" % (field, n, lo, hi))
    return n


def parse_field(text, spec):
    """Parse one field into (set of values, starts_with_star)."""
    field, lo, hi, top, names = spec
    if not text:
        raise CronError("empty %s field" % field)
    values = set()
    for part in text.split(","):
        rng, slash, step_text = part.partition("/")
        step = 1
        if slash:
            if not step_text.isdecimal() or not step_text.isascii() or int(step_text) < 1:
                raise CronError("bad step %r in %s" % (step_text, field))
            step = int(step_text)
        if rng == "*":
            start, end = 0, top
        elif "-" in rng:
            a, _, b = rng.partition("-")
            start = _value(a, lo, hi, names, field)
            end = _value(b, lo, hi, names, field)
            if start > end:
                raise CronError("descending range %r in %s" % (rng, field))
        else:
            start = _value(rng, lo, hi, names, field)
            end = start
        values.update(v for v in range(start, end + 1, step) if v >= lo)
    return values, text.startswith("*")


class Schedule:
    """The parsed form of a cron expression."""

    def __init__(self, minutes, hours, days, months, weekdays, dom_star, dow_star):
        self.minutes = frozenset(minutes)
        self.hours = frozenset(hours)
        self.days = frozenset(days)
        self.months = frozenset(months)
        self.weekdays = frozenset(weekdays)
        self.dom_star = dom_star
        self.dow_star = dow_star


def parse(expr):
    """Parse `expr` ("m h dom mon dow" or an @macro) into a Schedule."""
    expr = expr.strip()
    if expr.startswith("@"):
        if expr.lower() not in MACROS:
            raise CronError("unknown macro %r" % expr)
        expr = MACROS[expr.lower()]
    parts = expr.split()
    if len(parts) != 5:
        raise CronError("want 5 fields, got %d" % len(parts))
    parsed = [parse_field(p, spec) for p, spec in zip(parts, FIELDS)]
    return Schedule(
        parsed[0][0], parsed[1][0], parsed[2][0], parsed[3][0], parsed[4][0],
        dom_star=parsed[2][1], dow_star=parsed[4][1],
    )
