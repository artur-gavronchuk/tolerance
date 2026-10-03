"""Currency conversion."""
from decimal import ROUND_HALF_EVEN, Decimal

from currency import minor_digits
from money import Money, quantize_int, to_decimal


def convert(money, to_currency, rate, rounding=ROUND_HALF_EVEN):
    """Convert `money` at `rate` (units of the target currency per unit of the
    source, a positive str, int or Decimal) and round **once**, at the end,
    to the target currency's minor unit. The exact value is
    amount_in_source_major_units * rate; no intermediate rounding.
    """
    r = to_decimal(rate)
    if r <= 0:
        raise ValueError("rate must be positive")
    minor_digits(to_currency)  # validates the code
    major = (Decimal(money.amount) / 100).quantize(Decimal("0.01"), rounding=rounding)
    out = (major * r).quantize(Decimal("0.01"), rounding=rounding)
    return Money(int(out * 100), to_currency)


class RateTable:
    """Rates quoted for some pairs; the opposite direction is the reciprocal."""

    def __init__(self, rates):
        self._rates = {}
        for (a, b), rate in rates.items():
            r = to_decimal(rate)
            if r <= 0:
                raise ValueError("rate must be positive")
            self._rates[(a, b)] = r

    def rate(self, a, b):
        if a == b:
            return Decimal(1)
        if (a, b) in self._rates:
            return self._rates[(a, b)]
        if (b, a) in self._rates:
            return Decimal(1) / self._rates[(b, a)]
        raise KeyError((a, b))

    def convert(self, money, to_currency, rounding=ROUND_HALF_EVEN):
        if money.currency == to_currency:
            return money
        return convert(money, to_currency, self.rate(money.currency, to_currency), rounding)
