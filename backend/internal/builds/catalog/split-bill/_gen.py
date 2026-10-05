"""Generates scenarios.json for the split-bill challenge. Expected values come from the Python model below
(exact fractions), an implementation independent of _reference/index.html; the reference must pass every
scenario. The wrong models at the bottom (float dollars, Math.round per person, leftover to the first
people) are asserted to differ from the model on the rounding scenarios, so those scenarios do discriminate.

    python3 _gen.py > scenarios.json
"""
import json
import math
import re
import sys
from fractions import Fraction

# ---------------------------------------------------------------- model


def parse_money(text):
    m = re.fullmatch(r"(\d{1,6})(?:[.,](\d{1,2}))?", text.strip())
    if not m:
        return None
    cents = int(m.group(1)) * 100 + (int((m.group(2) or "").ljust(2, "0")) if m.group(2) else 0)
    return cents if cents >= 1 else None


def parse_pct(text):
    """Percent text -> basis points (hundredths of a percent), 0..10000, or None."""
    m = re.fullmatch(r"(\d{1,3})(?:[.,](\d{1,2}))?", text.strip())
    if not m:
        return None
    bp = int(m.group(1)) * 100 + (int(m.group(2).ljust(2, "0")) if m.group(2) else 0)
    return bp if bp <= 10000 else None


def fmt(cents):
    return "${:,}.{:02d}".format(cents // 100, cents % 100)


def half_up(x):
    return math.floor(Fraction(x) + Fraction(1, 2))


def exact_shares(n, items, bp):
    """items: [(cents, [person index, ...])]. Returns (subtotal, tip, [exact share in cents as Fraction])."""
    inc = [(c, s) for c, s in items if s]
    sub = sum(c for c, _ in inc)
    tip = half_up(Fraction(sub * bp, 10000))
    own = [Fraction(0)] * n
    for c, s in inc:
        for p in s:
            own[p] += Fraction(c, len(s))
    ex = [o + (Fraction(tip) * o / sub if sub else 0) for o in own]
    return sub, tip, ex


def shares(n, items, bp):
    """The rule from TASK.md: floor each exact share, hand out the leftover cents by largest remainder, ties to the earlier person."""
    sub, tip, ex = exact_shares(n, items, bp)
    fl = [math.floor(e) for e in ex]
    left = sub + tip - sum(fl)
    order = sorted(range(n), key=lambda i: (-(ex[i] - fl[i]), i))
    for i in order[:left]:
        fl[i] += 1
    assert sum(fl) == sub + tip
    return sub, tip, fl


def shares_round(n, items, bp):  # wrong: Math.round per person
    sub, tip, ex = exact_shares(n, items, bp)
    return [half_up(e) for e in ex]


def shares_first(n, items, bp):  # wrong: leftover cents to the first people
    sub, tip, ex = exact_shares(n, items, bp)
    fl = [math.floor(e) for e in ex]
    for i in range(sub + tip - sum(fl)):
        fl[i] += 1
    return fl


def shares_dollars(n, items, bp):  # wrong: floats in dollars, toFixed(2) per person
    inc = [(c, s) for c, s in items if s]
    sub = sum(c for c, _ in inc) / 100
    tip = math.floor(sub * (bp / 10000) * 100 + 0.5) / 100
    own = [0.0] * n
    for c, s in inc:
        for p in s:
            own[p] += c / 100 / len(s)
    return [int(round(float("%.2f" % (o + (tip * o / sub if sub else 0))) * 100)) for o in own]


# ---------------------------------------------------------------- step builders


def tid(t):
    return "[data-testid=%s]" % t


def sel(t):
    """A bare testid name, or an already built selector."""
    return t if t.startswith("[") else tid(t)


def goto(path="/?test=1"):
    return {"op": "goto", "path": path}


def click(t):
    return {"op": "click", "selector": sel(t)}


def fill(t, v):
    return {"op": "fill", "selector": tid(t), "value": v}


def expect_js(expr, value):
    return {"op": "expect_js", "expr": expr, "value": value}


def text_of(t, value):
    return expect_js("(document.querySelector('[data-testid=%s]') || {textContent: null}).textContent?.trim()" % t, value)


def list_of(t, value):
    return expect_js("[...document.querySelectorAll('[data-testid=%s]')].map(e => e.textContent.trim())" % t, value)


def visible(t):
    return {"op": "expect_visible", "selector": sel(t)}


def hidden(t):
    return {"op": "expect_hidden", "selector": sel(t)}


def count(s, n):
    return {"op": "expect_count", "selector": sel(s), "count": n}


def pressed(preset):
    return expect_js("['0','10','15','20'].map(p => document.querySelector('[data-testid=tip-' + p + ']').getAttribute('aria-pressed'))",
                     ["true" if p == preset else "false" for p in (0, 10, 15, 20)])


SUM_OK = expect_js("""(() => {
  const c = t => Math.round(parseFloat(t.replace(/[$,]/g, '')) * 100)
  const parts = [...document.querySelectorAll('[data-testid=person-total]')].map(e => c(e.textContent.trim()))
  return parts.reduce((a, b) => a + b, 0) === c(document.querySelector('[data-testid=grand-total]').textContent.trim())
})()""", True)

TARGETS_OK = expect_js("""[...document.querySelectorAll('button, input, select, textarea, a[href]')].filter(e => {
  const r = e.getBoundingClientRect(), s = getComputedStyle(e)
  return r.width > 0 && r.height > 0 && s.visibility !== 'hidden' && (Math.round(r.width) < 44 || Math.round(r.height) < 44)
}).length""", 0)


class Bill:
    """Drives the UI and the model together; check() asserts everything the model says is on screen."""

    def __init__(self):
        self.steps = []
        self.people = []   # names
        self.items = []    # [name, cents, set(names)]
        self.bp = 0

    # --- actions
    def person(self, name, key=False):
        self.steps.append(fill("new-person", name))
        self.steps.append({"op": "press", "key": "Enter", "selector": tid("new-person")} if key else click("add-person"))
        n = name.strip()
        if n and len(n) <= 24 and n.lower() not in [p.lower() for p in self.people]:
            self.people.append(n)
        return self

    def item(self, name, price, key=False):
        self.steps += [fill("new-item-name", name), fill("new-item-price", price)]
        self.steps.append({"op": "press", "key": "Enter", "selector": tid("new-item-price")} if key else click("add-item"))
        cents = parse_money(price)
        if name.strip() and len(name.strip()) <= 40 and cents:
            self.items.append([name.strip(), cents, set(self.people)])
        return self

    def toggle(self, item, person):
        self.steps.append(click("%s >> nth=%d >> %s >> nth=%d" % (tid("item"), item, tid("sharer"), person)))
        p = self.people[person]
        self.items[item][2] ^= {p}
        return self

    def only(self, item, *persons):
        """Toggle so that exactly these people (indexes) share the item."""
        want = {self.people[i] for i in persons}
        for i, p in enumerate(self.people):
            if (p in self.items[item][2]) != (p in want):
                self.toggle(item, i)
        return self

    def remove_person(self, i):
        self.steps.append(click("%s >> nth=%d >> %s" % (tid("person"), i, tid("remove-person"))))
        gone = self.people.pop(i)
        for it in self.items:
            it[2].discard(gone)
        return self

    def remove_item(self, i):
        self.steps.append(click("%s >> nth=%d >> %s" % (tid("item"), i, tid("remove-item"))))
        self.items.pop(i)
        return self

    def preset(self, p):
        self.steps.append(click("tip-%d" % p))
        self.bp = p * 100
        return self

    def custom(self, text):
        self.steps.append(fill("tip-custom", text))
        bp = parse_pct(text)
        if bp is not None:
            self.bp = bp
        return self

    def new_bill(self):
        self.steps.append(click("new-bill"))
        self.people, self.items, self.bp = [], [], 0
        return self

    # --- model
    def idx_items(self):
        return [(c, [self.people.index(p) for p in s]) for _, c, s in self.items]

    def calc(self):
        return shares(len(self.people), self.idx_items(), self.bp)

    def state(self):
        sub, tip, sh = self.calc()
        return {"people": [{"name": p, "share": sh[i]} for i, p in enumerate(self.people)],
                "items": [{"name": n, "cents": c, "sharers": [p for p in self.people if p in s]} for n, c, s in self.items],
                "tipPercent": self.bp / 100, "subtotal": sub, "tip": tip, "total": sub + tip}

    def check(self, hook=False, ui_items=True):
        sub, tip, sh = self.calc()
        st = self.steps
        st.append(list_of("person-name", list(self.people)))
        st.append(list_of("person-total", [fmt(x) for x in sh]))
        st.append(text_of("subtotal", fmt(sub)))
        st.append(text_of("tip-amount", fmt(tip)))
        st.append(text_of("grand-total", fmt(sub + tip)))
        st.append(SUM_OK)
        if ui_items:
            st.append(list_of("item-title", [n for n, _, _ in self.items]))
            st.append(list_of("item-price", [fmt(c) for _, c, _ in self.items]))
        if hook:
            st.append(expect_js("window.bill.state()", self.state()))
        return self


sc = []


def scenario(name, steps_, **kw):
    sc.append({"name": name, "steps": steps_, **kw})


# ---------------------------------------------------------------- scenarios

b = Bill()
b.steps += [goto(), count("person", 0), count("item", 0), pressed(0)]
b.check(hook=True)
scenario("a new bill is empty: nobody, no items, every amount $0.00, tip 0%", b.steps)

b = Bill()
b.steps.append(goto())
b.person("  Ann  ").person("Ben", key=True).person("mIA").check()
b.steps += [expect_js("document.querySelector('[data-testid=new-person]').value", ""), count("person", 3)]
scenario("people are added trimmed and in order, by button or Enter, and the field is cleared", b.steps)

b = Bill()
b.steps += [goto(), click("add-person"), visible("person-error"), count("person", 0), fill("new-person", "   "), click("add-person"),
            visible("person-error"), count("person", 0), fill("new-person", "x" * 25), click("add-person"), visible("person-error"), count("person", 0)]
b.person("y" * 24)
b.steps += [hidden("person-error"), count("person", 1), list_of("person-name", ["y" * 24])]
scenario("empty, blank and 25-character names are rejected with an error; 24 characters is fine", b.steps)

b = Bill()
b.steps.append(goto())
b.person("Ann")
b.steps += [fill("new-person", "ann"), click("add-person"), visible("person-error"), count("person", 1)]
b.steps += [fill("new-person", " ANN "), click("add-person"), visible("person-error"), count("person", 1)]
b.person("Anna")
b.steps += [hidden("person-error"), list_of("person-name", ["Ann", "Anna"])]
scenario("a name that is already at the table is rejected, ignoring case and surrounding spaces", b.steps)

b = Bill()
b.steps.append(goto())
b.person("Ann").person("Ben").person("Cy").item("Pizza", "30.00").item("Wine", "21.00")
b.remove_person(1).check()
b.remove_item(0).check()
b.remove_person(0).remove_person(0).check()
scenario("removing a person or an item removes the row and recalculates everything", b.steps)

b = Bill()
b.steps.append(goto())
b.person("Ann")
prices = [("12.50", "$12.50"), ("12,50", "$12.50"), ("7", "$7.00"), ("0.5", "$0.50"), ("1,5", "$1.50"), ("  8.25 ", "$8.25"),
          ("0,05", "$0.05"), ("0.01", "$0.01"), ("007.10", "$7.10"), ("999999.99", "$999,999.99")]
for i, (txt, _) in enumerate(prices):
    b.item("Item %d" % (i + 1), txt)
assert [fmt(c) for _, c, _ in b.items] == [f for _, f in prices]
b.steps += [list_of("item-price", [f for _, f in prices]), list_of("item-title", ["Item %d" % (i + 1) for i in range(len(prices))])]
scenario("prices accept a dot or a comma as the decimal separator, with one or two decimals", b.steps)

bad = ["", "abc", "1.234", "1,2,3", "-5", "0", "0,00", "$5", "1 000", ".5", "5.", "1e3", "1000000", "12.5.0", "1,000.00", "+3"]
steps_ = []
for v in bad:
    assert parse_money(v) is None, v
    steps_ += [goto(), fill("new-item-name", "Soup"), fill("new-item-price", v), click("add-item"), visible("item-error"), count("item", 0),
               expect_js("window.bill.state().items.length", 0)]
steps_ += [goto(), fill("new-item-name", "   "), fill("new-item-price", "5.00"), click("add-item"), visible("item-error"), count("item", 0),
           goto(), fill("new-item-name", "z" * 41), fill("new-item-price", "5.00"), click("add-item"), visible("item-error"), count("item", 0)]
scenario("invalid prices and item names are rejected with an error and no item is added", steps_)

b = Bill()
b.steps += [goto(), fill("new-item-name", "Soup"), fill("new-item-price", "nope"), click("add-item"), visible("item-error")]
b.item("Beer", "6,50", key=True).item("Beer", "6.50")
b.steps += [hidden("item-error"), expect_js("document.querySelector('[data-testid=new-item-name]').value", ""),
            expect_js("document.querySelector('[data-testid=new-item-price]').value", ""), count("item", 2),
            list_of("item-title", ["Beer", "Beer"]), list_of("item-price", ["$6.50", "$6.50"])]
scenario("a valid item clears the error and the fields (Enter works too); the same name can be added twice", b.steps)

b = Bill()
b.steps.append(goto())
b.person("Ann").person("Ben").person("Cy").item("Dinner", "10.00").check(hook=True)
assert [fmt(x) for x in b.calc()[2]] == ["$3.34", "$3.33", "$3.33"]
scenario("$10.00 split three ways: $3.34, $3.33, $3.33", b.steps)

b = Bill()
b.steps.append(goto())
b.person("Ann").person("Ben").item("Mint", "0.01").check(hook=True)
assert [fmt(x) for x in b.calc()[2]] == ["$0.01", "$0.00"]
scenario("one cent between two people goes to the first", b.steps)

b = Bill()
b.steps.append(goto())
b.person("Ann").person("Ben").person("Cy").item("Pizza", "30.00").item("Cake", "9.00")
b.steps.append(expect_js("[...document.querySelectorAll('[data-testid=sharer]')].every(e => e.checked !== undefined ? e.checked : e.getAttribute('aria-checked') === 'true')", True))
b.steps += [{"op": "expect_checked", "selector": "%s >> nth=1 >> %s >> nth=2" % (tid("item"), tid("sharer")), "checked": True}]
b.toggle(1, 1)
b.steps += [{"op": "expect_checked", "selector": "%s >> nth=1 >> %s >> nth=1" % (tid("item"), tid("sharer")), "checked": False},
            {"op": "expect_checked", "selector": "%s >> nth=0 >> %s >> nth=1" % (tid("item"), tid("sharer")), "checked": True}]
b.check(hook=True)
assert [fmt(x) for x in b.calc()[2]] == ["$14.50", "$10.00", "$14.50"]
b.toggle(0, 0).check()
scenario("everyone shares a new item by default, and each item's sharers toggle independently", b.steps)

b = Bill()
b.steps.append(goto())
b.person("Ann").person("Ben").item("Soup", "8.00").person("Cy")
b.steps += [{"op": "expect_count", "selector": "%s >> nth=0 >> %s" % (tid("item"), tid("sharer")), "count": 3},
            {"op": "expect_checked", "selector": "%s >> nth=0 >> %s >> nth=2" % (tid("item"), tid("sharer")), "checked": False},
            {"op": "expect_checked", "selector": "%s >> nth=0 >> %s >> nth=0" % (tid("item"), tid("sharer")), "checked": True}]
b.items[0][2] = {"Ann", "Ben"}
b.check(hook=True)
b.item("Tea", "3.00").check()
scenario("a person added later is not on the items that already exist, but is on the next new item", b.steps)

b = Bill()
b.steps.append(goto())
b.person("Ann").person("Ben").item("Feast", "85.00")
b.steps.append(pressed(0))
for p in (10, 15, 20, 0):
    b.preset(p)
    b.steps.append(pressed(p))
    b.check()
    assert fmt(b.calc()[1]) == {10: "$8.50", 15: "$12.75", 20: "$17.00", 0: "$0.00"}[p]
scenario("tip presets 0 / 10 / 15 / 20 set the tip and exactly one is pressed", b.steps)

cases = [("10.10", 15, "$1.52"), ("12.30", 15, "$1.85"), ("0.05", 10, "$0.01"), ("10.05", 10, "$1.01"), ("3.30", 15, "$0.50"), ("0.04", 10, "$0.00")]
b = Bill()
b.steps.append(goto())
for price, pct, want in cases:
    b.new_bill().person("Ann").item("Meal", price).preset(pct).check()
    assert fmt(b.calc()[1]) == want, (price, pct)
b.steps.append(expect_js("window.bill.state().tip", 0))
scenario("the tip is rounded half up to the cent", b.steps)

b = Bill()
b.steps.append(goto())
b.person("Ann").person("Ben").item("Feast", "80.00").custom("12.5")
b.steps.append(pressed(None))
b.check(hook=True)
assert fmt(b.calc()[1]) == "$10.00"
for txt in ("7,5", "0", "100", "33,33", "15"):
    b.custom(txt).check()
b.steps.append(pressed(15))
b.custom("12.5")
b.steps.append(click("tip-20"))
b.bp = 2000
b.steps += [expect_js("document.querySelector('[data-testid=tip-custom]').value", ""), pressed(20)]
b.check()
scenario("a custom tip percent takes a dot or a comma; choosing a preset empties the custom field", b.steps)

b = Bill()
b.steps.append(goto())
b.person("Ann").item("Meal", "50.00").preset(10)
for v in ("abc", "101", "-5", "1.234", "12%", "100.01"):
    assert parse_pct(v) is None
    b.steps += [fill("tip-custom", v), visible("tip-error")]
    b.check()
b.steps += [fill("tip-custom", ""), hidden("tip-error")]
b.check()
b.custom("20")
b.steps.append(hidden("tip-error"))
b.check()
scenario("an invalid custom tip shows an error and keeps the tip; a valid one hides it", b.steps)

b = Bill()
b.steps.append(goto())
b.person("Ann").person("Ben").item("Steak", "20.00").item("Salad", "10.00").only(0, 0).only(1, 1).preset(10).check(hook=True)
assert [fmt(x) for x in b.calc()[2]] == ["$22.00", "$11.00"]
scenario("the tip is split in proportion to what each person ordered", b.steps)

# leftover cents by largest remainder: a hand-built case, checked against the wrong models
b = Bill()
b.steps.append(goto())
b.person("Ann").person("Ben").person("Cy").person("Dee")
b.item("Nachos", "4.99")                  # all four: 124.75 cents each
b.item("Mints", "0.03").only(1, 0, 1, 3)  # Ann, Ben, Dee: 1 cent each
b.item("Gum", "0.05").only(2, 0, 2)       # Ann, Cy: 2.5 cents each
b.preset(10).check(hook=True)
mine = b.calc()[2]
args = (len(b.people), b.idx_items(), b.bp)
assert mine != shares_round(*args) and mine != shares_first(*args), (mine, shares_round(*args), shares_first(*args))
assert sum(shares_round(*args)) != sum(mine)
scenario("leftover cents go to the largest remainders, ties to the earlier person (shares add up exactly)", b.steps)

b = Bill()
b.steps.append(goto())
for n in ("Ann", "Ben", "Cy", "Dee", "Eli", "Flo"):
    b.person(n)
b.item("Ribs", "47.30").only(0, 0, 1, 2)
b.item("Pasta", "18.45").only(1, 3, 4)
b.item("Wine", "62.00")
b.item("Fries", "9.99").only(3, 0, 1, 2, 3, 4)
b.item("Tiramisu", "13,33").only(4, 2, 5)
b.item("Water", "2.75").only(5, 0, 1, 2, 3, 4, 5)
b.item("Beer", "5.10").only(6, 1, 4)
b.custom("18,5").check(hook=True)
args = (len(b.people), b.idx_items(), b.bp)
assert b.calc()[2] != shares_round(*args) or b.calc()[2] != shares_first(*args)
scenario("a messy dinner of six: every share exact and the shares add up to the grand total", b.steps)

b = Bill()
b.steps.append(goto())
b.person("Ann").person("Ben").item("Tasting menu", "1234.56").only(0, 0).item("Cellar wine", "999999.99").only(1, 1).item("Bread", "1000").only(2, 0).preset(10)
b.check(hook=True)
assert fmt(b.calc()[0]) == "$1,002,234.55"
scenario("amounts are formatted like $1,234.56 with thousands separators", b.steps)

b = Bill()
b.steps.append(goto())
b.person("Ann").person("Ben").item("Soup", "20.00").item("Cake", "10.00").only(1)  # nobody
b.steps += [hidden("%s >> nth=0 >> %s" % (tid("item"), tid("item-flag")))]
b.steps += [visible("%s >> nth=1 >> %s" % (tid("item"), tid("item-flag")))]
b.check(hook=True)
b.toggle(1, 1)
b.steps += [hidden("%s >> nth=1 >> %s" % (tid("item"), tid("item-flag")))]
b.check()
scenario("an item nobody shares is flagged and left out of every total until someone shares it", b.steps)

b = Bill()
b.steps.append(goto())
b.item("Soup", "9.00").person("Ann").person("Ben").preset(20)
b.steps += [visible("%s >> nth=0 >> %s" % (tid("item"), tid("item-flag"))),
            {"op": "expect_checked", "selector": "%s >> nth=0 >> %s >> nth=0" % (tid("item"), tid("sharer")), "checked": False}]
b.check(hook=True)
b.toggle(0, 0).toggle(0, 1)
b.steps.append(hidden("%s >> nth=0 >> %s" % (tid("item"), tid("item-flag"))))
b.check()
scenario("an item added before anyone is at the table is flagged, and fixed by choosing its sharers", b.steps)

b = Bill()
b.steps.append(goto())
b.person("Ann").person("Ben").person("Cy").item("Pizza", "30.00").item("Soup", "6.00").only(1, 1).preset(10)
b.remove_person(1)
b.steps += [count("%s >> nth=0 >> %s" % (tid("item"), tid("sharer")), 2), count("%s >> nth=1 >> %s" % (tid("item"), tid("sharer")), 2),
            visible("%s >> nth=1 >> %s" % (tid("item"), tid("item-flag"))), hidden("%s >> nth=0 >> %s" % (tid("item"), tid("item-flag")))]
b.check(hook=True)
assert fmt(b.calc()[0]) == "$30.00"
b.toggle(1, 1).check()
scenario("removing a person takes them off every item; an item left with nobody is flagged and left out", b.steps)

b = Bill()
b.steps.append(goto())
b.person("Ann").person("Ben").person("Cy").item("Pizza", "29.99").item("Soup", "6.50").only(1, 0, 2).custom("12,5").toggle(0, 2)
b.check(hook=True)
b.steps += [{"op": "reload"}]
b.check(hook=True)
b.steps += [pressed(None), expect_js("document.querySelector('[data-testid=tip-custom]').value.replace(',', '.')", "12.5"),
            {"op": "expect_checked", "selector": "%s >> nth=0 >> %s >> nth=2" % (tid("item"), tid("sharer")), "checked": False},
            {"op": "expect_checked", "selector": "%s >> nth=1 >> %s >> nth=2" % (tid("item"), tid("sharer")), "checked": True}]
b.preset(10).person("Dee").check()
b.steps.append({"op": "reload"})
b.check(hook=True)
b.steps.append(pressed(10))
scenario("the whole bill survives a reload: people, items, sharers and tip", b.steps)

b = Bill()
b.steps.append(goto())
b.person("Ann").person("Ben").item("Pizza", "30.00").preset(20).check()
b.new_bill()
b.steps += [count("person", 0), count("item", 0), pressed(0)]
b.check(hook=True)
b.steps.append({"op": "reload"})
b.steps += [count("person", 0), count("item", 0), pressed(0)]
b.check(hook=True)
b.person("Zed").item("Tea", "2.00").check()
scenario("New bill clears everything, stays cleared after a reload, and starts a fresh bill", b.steps)

names = ["Alexandra-Maria", "Bartholomew", "Christopher J.", "Dmitriy Petrov-Sidorov", "Eleonora", "Ferdinand", "Gwendolyn", "Hubert"]
b = Bill()
b.steps.append(goto())
for n in names:
    b.person(n)
b.item("Seafood platter with all the extras", "2345.67").item("Sparkling mineral water, large bottle", "4,50").only(1, 0, 1, 2, 3)
b.preset(20)
b.check()
b.steps += [{"op": "expect_no_hscroll"}, TARGETS_OK, {"op": "reload"}, {"op": "expect_no_hscroll"}, TARGETS_OK]
scenario("a crowded bill fits a 375px phone: no sideways scrolling, every control at least 44px", b.steps, viewport=375)

scenario("outside test mode the app loads on a phone without sideways scrolling and has the test hook",
         [goto("/"), {"op": "wait", "ms": 300}, {"op": "expect_no_hscroll"}, TARGETS_OK,
          expect_js("typeof window.bill.state", "function"), expect_js("typeof window.bill.state().total", "number")])

# ---------------------------------------------------------------- do the rounding scenarios discriminate?
assert any(shares_dollars(1, [(int(round(float(p) * 100)), [0])], pct * 100) != shares(1, [(parse_money(p), [0])], pct * 100)[2]
           for p, pct, _ in cases), "no tip case separates float dollars from exact cents"
print(json.dumps(sc, indent=1))
