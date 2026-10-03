# Money and ledger

`money.py` (`Money`), `allocate.py` (`allocate`, `split_evenly`), `fx.py`
(`convert`, `RateTable`) and `ledger.py` (`Ledger`) handle money as integers of
the currency's minor unit (`currency.py` lists the digits: USD 2, JPY 0, KWD 3).
The visible tests pass, but amounts are off by a unit here and there, splits
give the leftovers to the wrong parts, and the ledger accepts entries that do
not balance. Fix the code without editing `test_*.py` files. Hidden tests
exercise the contract below.

## Money

- `Money(amount, currency)`: `amount` is an `int` of minor units (a `bool` or a
  float is a `TypeError`); an unknown currency code is `UnknownCurrency`.
  Immutable, hashable, `==` is false across currencies without raising.
- `+`, `-` and the ordering operators (`<`, `<=`, `>`, `>=`) between different
  currencies raise `CurrencyMismatch`.
- `Money.parse(text, currency, rounding=None)`: `text` is a decimal in major
  units (str, int or Decimal; a float is a `TypeError`; garbage, `NaN` and
  infinities are `ValueError`). It is parsed **exactly**, never through a float.
  With `rounding=None` the text has to fit the minor unit (`"12.345"` for USD,
  `"12.5"` for JPY: `ValueError`); with a `decimal` rounding mode it is rounded
  to the minor unit with that mode.
- `format()`: `"12.34 USD"`, `"-0.05 USD"`, `"1200 JPY"`, `"0.500 KWD"`: always
  all the minor digits of the currency, the sign in front of everything.
- `multiply(factor, rounding=ROUND_HALF_EVEN)` multiplies the exact amount by
  `factor` (str, int or Decimal) and rounds **once**, at the end, with the given
  mode. `percent(pct, rounding=ROUND_HALF_EVEN)` is `pct` percent of the amount
  (`percent("7.5")`). Banker's rounding applies to negative amounts too:
  -2.5 minor units round to -2 and -3.5 to -4.

## Allocation

`allocate(money, ratios)`: see its docstring for the algorithm: floors of exact
shares, leftover units to the largest fractional remainders (equal remainders:
lower index; remainders are compared **exactly**, so 1/3 and 1/3 are equal
however they were computed), zero ratios get nothing, negative amounts are allocated as their
absolute value and negated. The parts always add up to the amount exactly.
`ValueError` for no ratios, a negative ratio or all-zero ratios.
`split_evenly(money, n)` is `allocate` with `n` equal ratios (`n < 1`: `ValueError`).

## Conversion

`convert(money, to_currency, rate, rounding=ROUND_HALF_EVEN)`: the exact value is
`amount in source major units * rate`, rounded once to the **target** currency's
minor unit (so USD to JPY has no cents, USD to KWD has three decimals, and
JPY to USD multiplies by 100 less than the yen amount suggests). `rate` must be
positive (`ValueError`). `RateTable` quotes pairs and uses the reciprocal for
the opposite direction.

## Ledger

`Ledger.post(postings, memo)`: at least two postings, and the postings **of each
currency** add up to zero by themselves, otherwise `UnbalancedEntry` is raised
**and nothing at all has changed** (no balance, no entry). Entry ids are
1, 2, 3, ... in posting order. `balances(account)` leaves out currencies whose
balance is zero; `balance(account, currency)` gives a (possibly zero) `Money`;
`trial_balance()` is the total per currency over all accounts, zeros left out.
`reverse(entry_id, memo=None)` posts the negated postings of an entry. Each
entry can be reversed once, and a reversal cannot be reversed itself: both
`AlreadyReversed` (a custom memo does not change that). An unknown id is `KeyError`.
A failed `reverse` changes nothing.
