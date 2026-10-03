"""Currencies and their minor units."""


class UnknownCurrency(ValueError):
    pass


class CurrencyMismatch(ValueError):
    """Two amounts of different currencies were combined or compared."""


# ISO 4217 code -> digits after the decimal point in the minor unit
DIGITS = {
    "USD": 2, "EUR": 2, "GBP": 2, "CHF": 2,
    "JPY": 0, "KRW": 0, "CLP": 0,
    "KWD": 3, "BHD": 3, "JOD": 3,
}


def minor_digits(code):
    try:
        return DIGITS[code]
    except KeyError:
        raise UnknownCurrency(code) from None
