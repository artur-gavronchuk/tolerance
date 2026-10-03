from decimal import ROUND_HALF_UP

import pytest

from allocate import allocate, split_evenly
from currency import CurrencyMismatch
from fx import convert
from ledger import Ledger, Posting, UnbalancedEntry
from money import Money


def usd(minor):
    return Money(minor, "USD")


def test_parse_and_format():
    assert Money.parse("12.34", "USD") == usd(1234)
    assert Money.parse("1200", "JPY") == Money(1200, "JPY")
    assert usd(1234).format() == "12.34 USD"
    assert Money(1200, "JPY").format() == "1200 JPY"


def test_arithmetic():
    assert usd(100) + usd(50) == usd(150)
    assert usd(100) - usd(150) == usd(-50)
    assert usd(100) < usd(200)
    with pytest.raises(CurrencyMismatch):
        usd(1) + Money(1, "EUR")


def test_multiply_and_percent():
    assert usd(1000).multiply("1.5") == usd(1500)
    assert usd(1000).percent("20") == usd(200)
    assert usd(1999).percent("10", ROUND_HALF_UP) == usd(200)


def test_allocate():
    assert allocate(usd(100), [1, 1, 1]) == [usd(34), usd(33), usd(33)]
    assert allocate(usd(100), [3, 7]) == [usd(30), usd(70)]
    assert split_evenly(usd(10), 4) == [usd(3), usd(3), usd(2), usd(2)]


def test_convert():
    assert convert(usd(10000), "EUR", "0.9") == Money(9000, "EUR")


def test_ledger_balances():
    led = Ledger()
    led.transfer("cash", "sales", usd(500))
    led.transfer("cash", "fees", usd(120))
    assert led.balance("cash", "USD") == usd(-620)
    assert led.balance("sales", "USD") == usd(500)
    assert led.trial_balance() == {}
    with pytest.raises(UnbalancedEntry):
        led.post([Posting("a", usd(10))])
