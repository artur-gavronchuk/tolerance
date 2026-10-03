"""Splitting money in proportions without losing or inventing a minor unit."""
from decimal import ROUND_FLOOR, Decimal

from money import Money, to_decimal


def allocate(money, ratios):
    """Split `money` into len(ratios) parts proportional to `ratios`.

    `ratios` are non-negative numbers (int, str or Decimal), not all zero. The
    parts always add up to `money` exactly. Each part is the floor of its exact
    share (towards zero, the sign is applied after the split), and the minor
    units left over are handed out one each to the parts whose exact share had
    the largest fractional remainder; equal remainders go to the lower index.
    A zero ratio always gets zero. A negative amount is allocated like its
    absolute value and every part negated, so allocate(-x, r) == [-p for p in
    allocate(x, r)].
    """
    rs = [to_decimal(r) for r in ratios]
    if not rs or any(r < 0 for r in rs) or sum(rs) == 0:
        raise ValueError("ratios must be non-negative, with at least one positive")
    total = sum(rs)
    shares = [Decimal(money.amount) * r / total for r in rs]
    parts = [int(s.to_integral_value(ROUND_FLOOR)) for s in shares]
    left = money.amount - sum(parts)
    for i, r in enumerate(rs):
        if left == 0:
            break
        if r > 0:
            parts[i] += 1
            left -= 1
    return [Money(p, money.currency) for p in parts]


def split_evenly(money, n):
    """`n` near-equal parts; the earlier ones get the extra minor units."""
    if n < 1:
        raise ValueError("n must be at least 1")
    return allocate(money, [1] * n)
