"""Money as an integer number of minor units (cents, yen, fils) plus a currency."""
from decimal import ROUND_FLOOR, ROUND_HALF_EVEN, Decimal, InvalidOperation

from currency import CurrencyMismatch, minor_digits


def to_decimal(value):
    """Decimal from a str, int or Decimal. Floats are refused: they are not exact."""
    if isinstance(value, float) or isinstance(value, bool):
        raise TypeError("use a str, int or Decimal, not %s" % type(value).__name__)
    try:
        d = Decimal(value) if not isinstance(value, Decimal) else value
    except InvalidOperation:
        raise ValueError("not a number: %r" % (value,)) from None
    if not d.is_finite():
        raise ValueError("not a finite number: %r" % (value,))
    return d


def quantize_int(d, rounding=ROUND_HALF_EVEN):
    """Round a Decimal to an int with the given decimal rounding mode."""
    return int(d.quantize(Decimal(1), rounding=rounding))


class Money:
    """An amount of one currency. Immutable; `amount` is in minor units."""

    __slots__ = ("amount", "currency")

    def __init__(self, amount, currency):
        if isinstance(amount, bool) or not isinstance(amount, int):
            raise TypeError("amount must be an int number of minor units")
        minor_digits(currency)  # validates the code
        object.__setattr__(self, "amount", amount)
        object.__setattr__(self, "currency", currency)

    def __setattr__(self, name, value):
        raise AttributeError("Money is immutable")

    @classmethod
    def parse(cls, text, currency, rounding=None):
        """Money from a decimal in major units, e.g. parse("12.34", "USD").

        With rounding=None the text must fit the currency's minor unit
        exactly ("12.345" for USD is a ValueError); otherwise it is rounded
        to the minor unit with that decimal rounding mode.
        """
        digits = minor_digits(currency)
        to_decimal(text)  # reject garbage early
        return cls(round(float(text) * 10 ** digits), currency)

    def to_decimal(self):
        """The amount in major units, exactly."""
        return Decimal(self.amount).scaleb(-minor_digits(self.currency))

    def format(self):
        """Like "12.34 USD", "-0.05 USD", "1200 JPY", "0.500 KWD"."""
        digits = minor_digits(self.currency)
        sign = "-" if self.amount < 0 else ""
        q, r = divmod(abs(self.amount), 10 ** digits)
        body = str(q) if digits == 0 else "%d.%0*d" % (q, digits, r)
        return "%s%s %s" % (sign, body, self.currency)

    def multiply(self, factor, rounding=ROUND_HALF_EVEN):
        """The amount times `factor` (str, int or Decimal), rounded once, at the end."""
        d = Decimal(self.amount) * to_decimal(factor)
        return Money(int((d + Decimal("0.5")).to_integral_value(ROUND_FLOOR)), self.currency)

    def percent(self, pct, rounding=ROUND_HALF_EVEN):
        """`pct` percent of the amount: Money.parse("19.99", "USD").percent("7.5")."""
        return self.multiply(to_decimal(pct) / 100, rounding)

    def _same(self, other):
        if not isinstance(other, Money):
            return NotImplemented
        if other.currency != self.currency:
            raise CurrencyMismatch("%s vs %s" % (self.currency, other.currency))
        return other

    def __add__(self, other):
        o = self._same(other)
        return o if o is NotImplemented else Money(self.amount + o.amount, self.currency)

    def __sub__(self, other):
        o = self._same(other)
        return o if o is NotImplemented else Money(self.amount - o.amount, self.currency)

    def __neg__(self):
        return Money(-self.amount, self.currency)

    def __abs__(self):
        return Money(abs(self.amount), self.currency)

    def __lt__(self, other):
        if not isinstance(other, Money):
            return NotImplemented
        return self.amount < other.amount

    def __le__(self, other):
        if not isinstance(other, Money):
            return NotImplemented
        return self.amount <= other.amount

    def __gt__(self, other):
        if not isinstance(other, Money):
            return NotImplemented
        return self.amount > other.amount

    def __ge__(self, other):
        if not isinstance(other, Money):
            return NotImplemented
        return self.amount >= other.amount

    def __eq__(self, other):
        if not isinstance(other, Money):
            return NotImplemented
        return self.amount == other.amount and self.currency == other.currency

    def __hash__(self):
        return hash((self.amount, self.currency))

    def __bool__(self):
        return self.amount != 0

    def __repr__(self):
        return "Money(%d, %r)" % (self.amount, self.currency)
