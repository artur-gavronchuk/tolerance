import random
from decimal import (ROUND_CEILING, ROUND_DOWN, ROUND_FLOOR, ROUND_HALF_DOWN, ROUND_HALF_EVEN, ROUND_HALF_UP,
                     ROUND_UP, Decimal)
from fractions import Fraction

import pytest

from allocate import allocate, split_evenly
from currency import CurrencyMismatch, UnknownCurrency
from fx import RateTable, convert
from ledger import AlreadyReversed, Ledger, Posting, UnbalancedEntry
from money import Money


def usd(n):
    return Money(n, "USD")


def eur(n):
    return Money(n, "EUR")


def jpy(n):
    return Money(n, "JPY")


def kwd(n):
    return Money(n, "KWD")


def test_hidden_parse_and_format():
    assert Money.parse("12.34", "USD") == usd(1234)
    assert Money.parse("12", "JPY") == jpy(12)
    assert Money.parse("0.125", "KWD") == kwd(125)
    assert Money.parse("-0.05", "USD") == usd(-5)
    assert Money.parse("1e2", "USD") == usd(10000)
    assert Money.parse(Decimal("7.10"), "USD") == usd(710)
    assert Money.parse(3, "EUR") == eur(300)
    # exact, never via a float: this amount does not survive a round trip through float
    assert Money.parse("90071992547409.93", "USD") == usd(9007199254740993)
    for bad, cur in [("12.345", "USD"), ("12.5", "JPY"), ("0.0001", "KWD"), ("abc", "USD"), ("", "USD"),
                     ("NaN", "USD"), ("Infinity", "USD"), ("1.2.3", "USD")]:
        with pytest.raises(ValueError):
            Money.parse(bad, cur)
    with pytest.raises(TypeError):
        Money.parse(1.5, "USD")
    with pytest.raises(UnknownCurrency):
        Money.parse("1", "XXX")
    # rounding modes, applied to the exact decimal
    assert Money.parse("1.005", "USD", ROUND_HALF_EVEN) == usd(100)
    assert Money.parse("1.005", "USD", ROUND_HALF_UP) == usd(101)
    assert Money.parse("1.005", "USD", ROUND_HALF_DOWN) == usd(100)
    assert Money.parse("2.675", "USD", ROUND_HALF_EVEN) == usd(268)
    assert Money.parse("2.665", "USD", ROUND_HALF_EVEN) == usd(266)
    assert Money.parse("0.125", "USD", ROUND_HALF_UP) == usd(13)
    assert Money.parse("-2.5", "JPY", ROUND_HALF_EVEN) == jpy(-2)
    assert Money.parse("-2.5", "JPY", ROUND_HALF_UP) == jpy(-3)
    assert Money.parse("-2.5", "JPY", ROUND_UP) == jpy(-3)
    assert Money.parse("-2.5", "JPY", ROUND_DOWN) == jpy(-2)
    assert Money.parse("-2.5", "JPY", ROUND_CEILING) == jpy(-2)
    assert Money.parse("-2.5", "JPY", ROUND_FLOOR) == jpy(-3)
    assert Money.parse("19.999", "USD", ROUND_DOWN) == usd(1999)
    assert Money.parse("12.34", "USD", ROUND_UP) == usd(1234)
    # format
    assert usd(1234).format() == "12.34 USD"
    assert usd(-5).format() == "-0.05 USD"
    assert usd(0).format() == "0.00 USD"
    assert usd(100000).format() == "1000.00 USD"
    assert jpy(1200).format() == "1200 JPY"
    assert jpy(-7).format() == "-7 JPY"
    assert jpy(0).format() == "0 JPY"
    assert kwd(500).format() == "0.500 KWD"
    assert kwd(-1).format() == "-0.001 KWD"
    assert kwd(12345).format() == "12.345 KWD"
    assert Money.parse(usd(-123456).format().split()[0], "USD") == usd(-123456)
    with pytest.raises(TypeError):
        Money(1.0, "USD")
    with pytest.raises(TypeError):
        Money(True, "USD")
    with pytest.raises(UnknownCurrency):
        Money(1, "XXX")


def test_hidden_multiply_and_percent_round_once_half_even():
    assert usd(250).multiply("0.01") == usd(2)
    assert usd(350).multiply("0.01") == usd(4)
    assert usd(-250).multiply("0.01") == usd(-2)
    assert usd(-350).multiply("0.01") == usd(-4)
    assert usd(5).multiply("0.5") == usd(2)
    assert usd(7).multiply("0.5") == usd(4)
    assert usd(-5).multiply("0.5") == usd(-2)
    assert usd(-7).multiply("0.5") == usd(-4)
    assert usd(1000).multiply(3) == usd(3000)
    assert usd(1000).multiply("-1") == usd(-1000)
    assert usd(1000).multiply(Decimal("0")) == usd(0)
    assert jpy(15).multiply("0.1") == jpy(2)  # 1.5
    assert jpy(25).multiply("0.1") == jpy(2)  # 2.5
    assert usd(5).multiply("0.5", ROUND_HALF_UP) == usd(3)
    assert usd(-5).multiply("0.5", ROUND_HALF_UP) == usd(-3)
    assert usd(-7).multiply("0.5", ROUND_DOWN) == usd(-3)
    assert usd(-7).multiply("0.5", ROUND_CEILING) == usd(-3)
    assert usd(-7).multiply("0.5", ROUND_FLOOR) == usd(-4)
    assert usd(7).multiply("0.5", ROUND_UP) == usd(4)
    # rounded once at the end, not step by step
    assert usd(1001).multiply("0.333").multiply("3") == usd(999)
    assert usd(1001).multiply("0.999") == usd(1000)
    with pytest.raises(TypeError):
        usd(1).multiply(0.5)
    # percent
    assert Money.parse("19.99", "USD").percent("7.5") == usd(150)  # 149.925
    assert Money.parse("19.99", "USD").percent("20") == usd(400)   # 399.8
    assert usd(10).percent("25") == usd(2)                          # 2.5
    assert usd(30).percent("25") == usd(8)                          # 7.5
    assert usd(-10).percent("25") == usd(-2)
    assert usd(-30).percent("25") == usd(-8)
    assert usd(1000).percent("0.5") == usd(5)
    assert jpy(1995).percent("10") == jpy(200)   # 199.5
    assert jpy(1985).percent("10") == jpy(198)   # 198.5
    assert jpy(1985).percent("10", ROUND_HALF_UP) == jpy(199)
    assert kwd(12345).percent("15") == kwd(1852)  # 1851.75


def test_hidden_allocate_largest_remainder():
    assert allocate(usd(100), [1, 1, 1]) == [usd(34), usd(33), usd(33)]
    assert allocate(usd(10), [1, 2]) == [usd(3), usd(7)]          # 3.33 and 6.67: the leftover goes to the larger remainder
    assert allocate(usd(101), [1, 1, 2]) == [usd(25), usd(25), usd(51)]
    assert allocate(usd(2), [1, 1, 1]) == [usd(1), usd(1), usd(0)]  # equal remainders: lower index first
    assert allocate(usd(1), [1, 1, 1]) == [usd(1), usd(0), usd(0)]
    assert allocate(usd(5), [1, 1]) == [usd(3), usd(2)]
    assert allocate(usd(100), ["33.3", "33.3", "33.4"]) == [usd(33), usd(33), usd(34)]
    assert allocate(usd(100), [Decimal("0.5"), "0.25", Decimal("0.25")]) == [usd(50), usd(25), usd(25)]
    assert allocate(usd(1000), [70, 20, 10]) == [usd(700), usd(200), usd(100)]
    assert allocate(usd(7), [2, 3, 2]) == [usd(2), usd(3), usd(2)]
    assert allocate(usd(10), [1, 1, 1, 7])[3] == usd(7)
    assert allocate(usd(0), [1, 2, 3]) == [usd(0)] * 3
    # a zero ratio gets nothing, whatever is left over
    assert allocate(usd(10), [0, 1, 0, 1]) == [usd(0), usd(5), usd(0), usd(5)]
    assert allocate(usd(1), [0, 1, 1]) == [usd(0), usd(1), usd(0)]
    assert allocate(usd(7), [0, 1, 1]) == [usd(0), usd(4), usd(3)]
    assert allocate(usd(5), [1, 0]) == [usd(5), usd(0)]
    assert allocate(usd(5), [3]) == [usd(5)]
    # negative amounts are mirrored, not floored
    assert allocate(usd(-100), [1, 1, 1]) == [usd(-34), usd(-33), usd(-33)]
    assert allocate(usd(-10), [1, 2]) == [usd(-3), usd(-7)]
    assert allocate(usd(-1), [1, 1, 1]) == [usd(-1), usd(0), usd(0)]
    assert allocate(usd(-2), [1, 1, 1]) == [usd(-1), usd(-1), usd(0)]
    assert allocate(jpy(-5), [1, 1]) == [jpy(-3), jpy(-2)]
    # other currencies
    assert allocate(kwd(1000), [1, 1, 1]) == [kwd(334), kwd(333), kwd(333)]
    assert split_evenly(usd(10), 3) == [usd(4), usd(3), usd(3)]
    assert split_evenly(jpy(1000), 7) == [jpy(143)] * 6 + [jpy(142)]
    assert sum(p.amount for p in split_evenly(jpy(1000), 7)) == 1000
    assert split_evenly(usd(5), 1) == [usd(5)]
    for bad in ([], [0, 0], [-1, 2], [1, -1], ["x"]):
        with pytest.raises(ValueError):
            allocate(usd(10), bad)
    with pytest.raises(ValueError):
        split_evenly(usd(10), 0)
    check_against_oracle()


def oracle(amount, ratios):
    sign = -1 if amount < 0 else 1
    a = abs(amount)
    rs = [Fraction(str(r)) for r in ratios]
    total = sum(rs)
    shares = [a * r / total for r in rs]
    parts = [int(s) for s in shares]
    left = a - sum(parts)
    order = sorted((i for i, r in enumerate(rs) if r > 0), key=lambda i: (-(shares[i] - parts[i]), i))
    for i in order[:left]:
        parts[i] += 1
    return [sign * p for p in parts]


def check_against_oracle():
    rng = random.Random(2026)
    for _ in range(600):
        cur = rng.choice(["USD", "JPY", "KWD"])
        amount = rng.randint(-100000, 100000)
        ratios = [rng.choice([0, 1, 2, 3, 5, 7, 10, "0.5", "1.25", "0.1"]) for _ in range(rng.randint(1, 7))]
        if all(Fraction(str(r)) == 0 for r in ratios):
            ratios[0] = 1
        got = allocate(Money(amount, cur), ratios)
        assert [p.amount for p in got] == oracle(amount, ratios), (amount, ratios)
        assert all(p.currency == cur for p in got)
        assert sum(p.amount for p in got) == amount
        assert [p.amount for p in allocate(Money(-amount, cur), ratios)] == [-p.amount for p in got]
        for p, r in zip(got, ratios):
            if Fraction(str(r)) == 0:
                assert p.amount == 0


def test_hidden_convert_rounds_once_to_the_target_minor_unit():
    assert convert(usd(10000), "EUR", "0.9") == eur(9000)
    assert convert(usd(1050), "JPY", "151.237") == jpy(1588)        # 10.50 * 151.237 = 1587.9885
    assert convert(jpy(1000), "USD", "0.0066") == usd(660)          # 1000 yen * 0.0066 = 6.60 dollars
    assert convert(kwd(1500), "USD", "3.2545") == usd(488)          # 1.500 * 3.2545 = 4.88175
    assert convert(usd(1999), "KWD", "0.30724") == kwd(6142)        # 19.99 * 0.30724 = 6.1417276
    assert convert(jpy(3), "USD", "0.0075") == usd(2)               # 0.0225 dollars
    assert convert(jpy(12345), "KWD", "0.002") == kwd(24690)        # 24.690
    assert convert(usd(100), "JPY", "150") == jpy(150)
    assert convert(usd(99999), "JPY", 1) == jpy(1000)               # 999.99 yen
    # exact ties follow the rounding mode, with a single rounding
    assert convert(usd(1), "EUR", "0.5") == eur(0)
    assert convert(usd(3), "EUR", "0.5") == eur(2)
    assert convert(usd(5), "EUR", "0.5") == eur(2)
    assert convert(usd(-5), "EUR", "0.5") == eur(-2)
    assert convert(usd(-3), "EUR", "0.5") == eur(-2)
    assert convert(usd(1), "EUR", "0.5", ROUND_HALF_UP) == eur(1)
    assert convert(usd(-5), "EUR", "0.5", ROUND_HALF_UP) == eur(-3)
    assert convert(usd(1), "EUR", "0.5", ROUND_FLOOR) == eur(0)
    assert convert(usd(-1), "EUR", "0.5", ROUND_FLOOR) == eur(-1)
    # a result that two roundings would get wrong: 0.0149 * 0.7 ... in cents 1.49 -> 1.043 -> 1
    assert convert(usd(149), "EUR", "0.7") == eur(104)
    for bad in (0, "-1", "0"):
        with pytest.raises(ValueError):
            convert(usd(1), "EUR", bad)
    with pytest.raises(UnknownCurrency):
        convert(usd(1), "XXX", "1")
    table = RateTable({("USD", "EUR"): "0.9", ("USD", "JPY"): "150"})
    assert table.convert(usd(10000), "EUR") == eur(9000)
    assert table.convert(eur(9000), "USD") == usd(10000)
    assert table.convert(jpy(15000), "USD") == usd(10000)
    assert table.convert(usd(5), "USD") == usd(5)
    with pytest.raises(KeyError):
        table.convert(eur(1), "JPY")
    with pytest.raises(ValueError):
        RateTable({("USD", "EUR"): 0})


def test_hidden_ledger_balances_per_currency_and_is_atomic():
    led = Ledger()
    e1 = led.post([Posting("cash", usd(-500)), Posting("sales", usd(500))], "sale")
    assert (e1.id, e1.memo) == (1, "sale")
    # each currency has to balance on its own
    with pytest.raises(UnbalancedEntry):
        led.post([Posting("a", usd(100)), Posting("b", eur(-100))])
    with pytest.raises(UnbalancedEntry):
        led.post([Posting("a", usd(-100)), Posting("b", usd(60)), Posting("c", eur(40))])
    with pytest.raises(UnbalancedEntry):
        led.post([Posting("a", jpy(100)), Posting("b", usd(-100)), Posting("c", usd(100)), Posting("d", jpy(-99))])
    # and a failed post changes nothing
    with pytest.raises(UnbalancedEntry):
        led.post([Posting("a", usd(-100)), Posting("b", usd(60))])
    with pytest.raises(UnbalancedEntry):
        led.post([Posting("only", usd(0))])
    assert len(led.entries) == 1
    assert led.balances("a") == {} and led.balances("b") == {} and led.balances("c") == {}
    assert led.balance("a", "USD") == usd(0)
    assert led.trial_balance() == {}
    # multi-currency entries that balance in every currency are fine
    e2 = led.post([Posting("a", usd(-100)), Posting("b", usd(100)), Posting("b", eur(-50)), Posting("c", eur(50))], "swap")
    assert e2.id == 2
    assert led.balances("b") == {"USD": usd(100), "EUR": eur(-50)}
    assert led.balance("c", "EUR") == eur(50)
    assert led.balance("c", "USD") == usd(0)
    assert led.trial_balance() == {}
    # zero amounts and repeated accounts inside one entry are fine
    led.post([Posting("z", usd(0)), Posting("z", usd(5)), Posting("y", usd(-5))])
    assert led.balance("z", "USD") == usd(5)
    assert len(led.entries) == 3


def test_hidden_zero_balances_reversals_and_comparisons():
    led = Ledger()
    led.transfer("a", "b", usd(100))
    led.transfer("b", "a", usd(100))
    assert led.balances("a") == {} and led.balances("b") == {}
    assert led.balance("a", "USD") == usd(0)
    led.transfer("a", "b", jpy(7))
    assert led.balances("b") == {"JPY": jpy(7)}
    # reversals
    e = led.transfer("b", "c", jpy(7), "pay")
    r = led.reverse(e.id)
    assert r.memo == "reversal of #%d" % e.id and [(p.account, p.money) for p in r.postings] == [("b", jpy(7)), ("c", jpy(-7))]
    assert led.balances("c") == {} and led.balances("b") == {"JPY": jpy(7)}
    n = len(led.entries)
    with pytest.raises(AlreadyReversed):
        led.reverse(e.id)
    with pytest.raises(AlreadyReversed):
        led.reverse(r.id)
    custom = led.reverse(1, memo="oops")
    assert custom.memo == "oops"
    with pytest.raises(AlreadyReversed):
        led.reverse(custom.id)
    with pytest.raises(AlreadyReversed):
        led.reverse(1)
    for bad in (0, -1, 99):
        with pytest.raises(KeyError):
            led.reverse(bad)
    assert len(led.entries) == n + 1
    assert led.balances("a") == {"JPY": jpy(-7), "USD": usd(100)}  # the first transfer was reversed by custom
    assert led.trial_balance() == {}
    # comparisons and arithmetic across currencies
    for op in (lambda x, y: x < y, lambda x, y: x <= y, lambda x, y: x > y, lambda x, y: x >= y,
               lambda x, y: x + y, lambda x, y: x - y):
        with pytest.raises(CurrencyMismatch):
            op(usd(5), eur(3))
    assert usd(5) != eur(5) and not (usd(5) == eur(5))
    assert usd(5) == usd(5) and hash(usd(5)) == hash(usd(5)) and len({usd(5), usd(5), eur(5)}) == 2
    assert sorted([usd(3), usd(-1), usd(2)]) == [usd(-1), usd(2), usd(3)]
    assert usd(3) >= usd(3) and usd(3) <= usd(3) and usd(4) > usd(3)
    assert abs(usd(-4)) == usd(4) and -usd(4) == usd(-4) and not usd(0) and usd(1)
    with pytest.raises(AttributeError):
        usd(1).amount = 2
