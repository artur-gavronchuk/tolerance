"""A double-entry ledger."""
from collections import defaultdict
from dataclasses import dataclass

from money import Money


class UnbalancedEntry(ValueError):
    pass


class AlreadyReversed(ValueError):
    pass


@dataclass(frozen=True)
class Posting:
    account: str
    money: Money


@dataclass(frozen=True)
class Entry:
    id: int
    memo: str
    postings: tuple


class Ledger:
    def __init__(self):
        self.entries = []
        self._balances = defaultdict(lambda: defaultdict(int))  # account -> currency -> minor units
        self._reversed = set()
        self._reversals = set()  # ids of entries that are themselves reversals

    def post(self, postings, memo=""):
        """Record an entry and return it.

        Needs at least two postings, and the postings of **each currency** must
        add up to zero on their own (+100 USD with -100 EUR is not balanced).
        Otherwise UnbalancedEntry is raised and the ledger is left untouched.
        """
        postings = tuple(postings)
        if len(postings) < 2:
            raise UnbalancedEntry("an entry needs at least two postings")
        entry = Entry(len(self.entries) + 1, memo, postings)
        total = 0
        for p in postings:
            self._balances[p.account][p.money.currency] += p.money.amount
            total += p.money.amount
        if total != 0:
            raise UnbalancedEntry("does not balance")
        self.entries.append(entry)
        return entry

    def transfer(self, src, dst, money, memo=""):
        return self.post([Posting(src, -money), Posting(dst, money)], memo)

    def balances(self, account):
        """{currency: Money} for the account; currencies whose balance is zero are left out."""
        return {c: Money(a, c) for c, a in self._balances.get(account, {}).items()}

    def balance(self, account, currency):
        return Money(self._balances.get(account, {}).get(currency, 0), currency)

    def trial_balance(self):
        """Total per currency over all accounts, zeros left out. Empty for a sound ledger."""
        sums = defaultdict(int)
        for per in self._balances.values():
            for c, a in per.items():
                sums[c] += a
        return {c: Money(a, c) for c, a in sums.items() if a != 0}

    def reverse(self, entry_id, memo=None):
        """Post the mirror image of an entry. An entry can be reversed once,
        and a reversal cannot itself be reversed (both raise AlreadyReversed)."""
        if not 1 <= entry_id <= len(self.entries):
            raise KeyError(entry_id)
        orig = self.entries[entry_id - 1]
        out = self.post(
            [Posting(p.account, -p.money) for p in orig.postings],
            memo if memo is not None else "reversal of #%d" % entry_id,
        )
        self._reversed.add(entry_id)
        self._reversals.add(out.id)
        return out
