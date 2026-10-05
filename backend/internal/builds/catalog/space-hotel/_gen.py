"""Generates scenarios.json for the space-hotel challenge. Expected totals come from the Python model below
(exact integer/Fraction arithmetic), an implementation independent of _reference/index.html; the reference
must pass every scenario.

    python3 _gen.py > scenarios.json
"""
import json
from fractions import Fraction
import math

ROOMS = {"pod": 895, "suite": 1999, "dome": 6495}
FEE = 2500
DASH = "—"


def discount_pct(n):
    return 20 if n >= 14 else 10 if n >= 7 else 0


def total(room, n):
    """Whole dollars: round-half-up of the discounted stay (exact fractions), plus the flat launch fee."""
    stay = Fraction(ROOMS[room] * n * (100 - discount_pct(n)), 100)
    return math.floor(stay + Fraction(1, 2)) + FEE


def money(v):
    return "${:,}".format(v)


def goto(path="/"):
    return {"op": "goto", "path": path}


def js(expr):
    return {"op": "js", "expr": expr}


def expect(expr, value):
    return {"op": "expect_js", "expr": expr, "value": value}


def tid(t):
    return "[data-testid=%s]" % t


def q(t):
    return "document.querySelector('[data-testid=%s]')" % t


def qa(t):
    return "document.querySelectorAll('[data-testid=%s]')" % t


def visible(t):
    return {"op": "expect_visible", "selector": tid(t)}


def hidden(t):
    return {"op": "expect_hidden", "selector": tid(t)}


def click(t):
    return {"op": "click", "selector": tid(t)}


def fill(t, v):
    return {"op": "fill", "selector": tid(t), "value": v}


def total_is(s):
    return expect(q("total") + ".textContent.trim()", s)


def book(room, n):
    """Steps that choose a room and a number of nights and check the exact total."""
    return [{"op": "select", "selector": tid("room"), "value": room}, fill("nights", str(n)), total_is(money(total(room, n))), hidden("nights-error")]


def invalid(v):
    return [fill("nights", v), visible("nights-error"), total_is(DASH)]


def aria(sel_js):
    return "%s.getAttribute('aria-expanded')" % sel_js


def in_view(sec):
    return expect("(() => { const r = document.getElementById('%s').getBoundingClientRect(); return r.top < innerHeight && r.bottom > 0 })()" % sec, True)


def theme(v):
    return expect("document.documentElement.getAttribute('data-theme')", v)


BG = "(() => { for (const el of [document.body, document.documentElement]) { const c = getComputedStyle(el).backgroundColor; if (c !== 'rgba(0, 0, 0, 0)' && c !== 'transparent') return c } return 'none' })()"

sc = []


def scenario(name, steps, **kw):
    sc.append({"name": name, "steps": steps, **kw})


# ---- header and menu -------------------------------------------------------------------------------------------
scenario("desktop: the nav links are visible and there is no burger",
         [goto(), *[visible("nav-" + s) for s in ("rooms", "booking", "faq")], hidden("menu-toggle"), hidden("mobile-menu")])
scenario("desktop: the nav links scroll to their sections",
         [goto(), *[s for sec in ("booking", "faq", "rooms") for s in (click("nav-" + sec), expect("location.hash", "#" + sec), in_view(sec))]])
scenario("phone: the links collapse behind a closed burger",
         [goto(), visible("menu-toggle"), expect(aria(q("menu-toggle")), "false"), hidden("mobile-menu"), *[hidden("nav-" + s) for s in ("rooms", "booking", "faq")]],
         viewport=375)
scenario("phone: the burger opens and closes the menu, Escape closes it too",
         [goto(), click("menu-toggle"), expect(aria(q("menu-toggle")), "true"), visible("mobile-menu"), *[visible("menu-" + s) for s in ("rooms", "booking", "faq")],
          click("menu-toggle"), expect(aria(q("menu-toggle")), "false"), hidden("mobile-menu"),
          click("menu-toggle"), visible("mobile-menu"), {"op": "press", "key": "Escape"}, expect(aria(q("menu-toggle")), "false"), hidden("mobile-menu")],
         viewport=375)
scenario("phone: choosing a menu link closes the menu and scrolls to the section",
         [goto(), click("menu-toggle"), click("menu-booking"), hidden("mobile-menu"), expect(aria(q("menu-toggle")), "false"), expect("location.hash", "#booking"), in_view("booking"),
          click("menu-toggle"), visible("mobile-menu"), click("menu-faq"), hidden("mobile-menu"), expect(aria(q("menu-toggle")), "false"), expect("location.hash", "#faq"), in_view("faq")],
         viewport=375)
scenario("phone: nothing scrolls sideways, even with the menu, a FAQ answer and the biggest total open",
         [goto(), {"op": "expect_no_hscroll"}, click("menu-toggle"), {"op": "expect_no_hscroll"}, click("menu-toggle"),
          {"op": "select", "selector": tid("room"), "value": "dome"}, fill("nights", "30"), total_is("$158,380"),
          {"op": "click", "selector": "[data-testid=faq-question] >> nth=0"}, {"op": "expect_no_hscroll"},
          fill("email", "a-rather-long-address-for-a-narrow-screen@subdomain.example-company.com"), click("subscribe"), visible("subscribe-success"), {"op": "expect_no_hscroll"}],
         viewport=375)

# ---- rooms -----------------------------------------------------------------------------------------------------
scenario("the rooms section has three cards with the nightly prices, and the sections exist",
         [goto(), *[expect("!!document.getElementById('%s')" % s, True) for s in ("rooms", "booking", "faq")],
          {"op": "expect_count", "selector": tid("room-card"), "count": 3},
          *[{"op": "expect_text", "selector": "[data-testid=room-card] >> nth=%d" % i, "text": money(p)} for i, p in enumerate(ROOMS.values())],
          expect("[...document.querySelectorAll('[data-testid=room] option')].map(o => o.value)", list(ROOMS))])

# ---- booking calculator ----------------------------------------------------------------------------------------
scenario("the calculator starts on the first room and 3 nights, and the total follows the chosen room",
         [goto(), expect(q("room") + ".value", "pod"), {"op": "expect_value", "selector": tid("nights"), "value": "3"}, total_is(money(total("pod", 3))), hidden("nights-error"),
          *book("suite", 3), *book("dome", 3), *book("pod", 3)])
scenario("6 nights are full price, 7 nights get 10% off",
         [goto(), *book("suite", 6), *book("suite", 7), *book("pod", 6), *book("pod", 7)])
scenario("13 nights get 10% off, 14 nights get 20% off",
         [goto(), *book("suite", 13), *book("suite", 14), *book("dome", 13), *book("dome", 14)])
scenario("half dollars round up, other fractions to the nearest dollar",
         [goto(), *book("pod", 7), *book("pod", 9), *book("pod", 11), *book("pod", 13),  # .5 -> up
          *book("suite", 7), *book("suite", 11), *book("suite", 14)])  # .7 up, .1 down, .8 up
scenario("the launch fee is added once and never discounted; 1 and 30 nights are valid",
         [goto(), *book("pod", 1), *book("dome", 1), *book("pod", 30), *book("dome", 30), *book("dome", 29), *book("dome", 15)])
scenario("totals are formatted like $3,395, $21,985 and $158,380",
         [goto(), *book("pod", 1), expect(q("total") + ".textContent.trim()", "$3,395"), *book("dome", 3), total_is("$21,985"), *book("dome", 30), total_is("$158,380")])
scenario("empty nights show the error and a dash",
         [goto(), *invalid(""), expect(q("nights-error") + ".textContent.trim().length > 0", True)])
scenario("0 and 31 nights are out of range",
         [goto(), *invalid("0"), *invalid("31"), *invalid("100"), *invalid("300")])
scenario("fractions, negatives and typed letters are not valid nights",
         [goto(), *invalid("2.5"), *invalid("-3"), *invalid("1.0001"), fill("nights", ""), {"op": "type", "selector": tid("nights"), "value": "abc", "delay": 20},
          visible("nights-error"), total_is(DASH)])
scenario("fixing the nights clears the error and brings the total back",
         [goto(), *invalid("0"), *book("pod", 5), *invalid("31"), {"op": "select", "selector": tid("room"), "value": "dome"}, total_is(DASH), visible("nights-error"),
          *book("dome", 8)])
scenario("the total updates while typing, with no blur or button",
         [goto(), fill("nights", ""), total_is(DASH), {"op": "type", "selector": tid("nights"), "value": "1", "delay": 30}, total_is(money(total("pod", 1))),
          {"op": "type", "selector": tid("nights"), "value": "4", "delay": 30}, total_is(money(total("pod", 14))),
          {"op": "press", "key": "Backspace", "selector": tid("nights")}, total_is(money(total("pod", 1)))])

# ---- newsletter ------------------------------------------------------------------------------------------------
BAD = ["hello", "a@b", "a b@c.de", "@x.io", "a@b.", "a@@b.co", "two words@x.io"]
scenario("newsletter: invalid emails show the error and no success; nothing is shown before submitting",
         [goto(), hidden("email-error"), hidden("subscribe-success"), *[s for e in BAD for s in (fill("email", e), click("subscribe"), visible("email-error"), hidden("subscribe-success"))]])
scenario("newsletter: a valid email shows the success with the address and does not leave the page",
         [goto(), js("window.__alive = 41"), fill("email", "pilot+1@mail.example.com"), click("subscribe"), visible("subscribe-success"),
          expect(q("subscribe-success") + ".textContent.includes('pilot+1@mail.example.com')", True), hidden("email-error"),
          expect("window.__alive", 41), expect("location.pathname + location.search", "/"),
          fill("email", "a@b.c"), click("subscribe"), expect(q("subscribe-success") + ".textContent.includes('a@b.c')", True), expect("window.__alive", 41)])
scenario("newsletter: Enter submits, and the newest result replaces the old one",
         [goto(), fill("email", "nova@orbit.io"), {"op": "press", "key": "Enter", "selector": tid("email")}, visible("subscribe-success"), hidden("email-error"),
          fill("email", "nova@orbit"), {"op": "press", "key": "Enter", "selector": tid("email")}, visible("email-error"), hidden("subscribe-success"),
          fill("email", "nova@orbit.io"), click("subscribe"), visible("subscribe-success"), hidden("email-error"), expect("location.search", "")])

# ---- FAQ -------------------------------------------------------------------------------------------------------
Q = lambda i: "[data-testid=faq-item] >> nth=%d >> [data-testid=faq-question]" % i
A = lambda i: "[data-testid=faq-item] >> nth=%d >> [data-testid=faq-answer]" % i
states = "[...document.querySelectorAll('[data-testid=faq-question]')].map(b => b.getAttribute('aria-expanded')).join()"
scenario("FAQ: at least five questions, all closed at first",
         [goto(), expect(qa("faq-item") + ".length >= 5", True), expect(qa("faq-question") + ".length === " + qa("faq-item") + ".length", True),
          expect("[...document.querySelectorAll('[data-testid=faq-question]')].every(b => b.tagName === 'BUTTON' && b.getAttribute('aria-expanded') === 'false')", True),
          {"op": "expect_hidden", "selector": A(0)}, {"op": "expect_hidden", "selector": A(4)}])
scenario("FAQ: opening a question shows its answer, opening another closes the first",
         [goto(), {"op": "click", "selector": Q(1)}, expect("document.querySelectorAll('[data-testid=faq-question]')[1].getAttribute('aria-expanded')", "true"), {"op": "expect_visible", "selector": A(1)},
          expect(states + ".split(',').filter(s => s === 'true').length", 1),
          {"op": "click", "selector": Q(3)}, expect("document.querySelectorAll('[data-testid=faq-question]')[3].getAttribute('aria-expanded')", "true"), {"op": "expect_visible", "selector": A(3)},
          expect("document.querySelectorAll('[data-testid=faq-question]')[1].getAttribute('aria-expanded')", "false"), {"op": "expect_hidden", "selector": A(1)},
          expect(states + ".split(',').filter(s => s === 'true').length", 1),
          {"op": "click", "selector": Q(3)}, expect("document.querySelectorAll('[data-testid=faq-question]')[3].getAttribute('aria-expanded')", "false"), {"op": "expect_hidden", "selector": A(3)},
          expect(states + ".split(',').filter(s => s === 'true').length", 0)])
scenario("FAQ: Enter and Space work from the keyboard",
         [goto(), {"op": "press", "key": "Enter", "selector": Q(0)}, {"op": "expect_visible", "selector": A(0)},
          {"op": "press", "key": " ", "selector": Q(2)}, {"op": "expect_visible", "selector": A(2)}, {"op": "expect_hidden", "selector": A(0)},
          expect("document.querySelectorAll('[data-testid=faq-question]')[0].getAttribute('aria-expanded')", "false"),
          {"op": "press", "key": " ", "selector": Q(2)}, {"op": "expect_hidden", "selector": A(2)}])

# ---- theme -----------------------------------------------------------------------------------------------------
scenario("theme: the page follows the system colour scheme at first",
         [goto(), theme("light"), expect("matchMedia('(prefers-color-scheme: dark)').matches", False)])
scenario("theme: the toggle flips data-theme and saves the choice",
         [goto(), click("theme-toggle"), theme("dark"), expect("localStorage.getItem('theme')", "dark"), click("theme-toggle"), theme("light"), expect("localStorage.getItem('theme')", "light"),
          click("theme-toggle"), theme("dark")])
scenario("theme: the choice survives a reload, a saved choice wins over the system",
         [goto(), click("theme-toggle"), theme("dark"), {"op": "reload"}, theme("dark"),
          click("theme-toggle"), theme("light"), {"op": "reload"}, theme("light"),
          js("localStorage.setItem('theme', 'dark')"), goto(), theme("dark"), click("theme-toggle"), theme("light")])
scenario("theme: dark and light really look different",
         [goto(), js("window.__light = " + BG), click("theme-toggle"), theme("dark"), expect("(%s) !== window.__light && (%s) !== 'none'" % (BG, BG), True),
          click("theme-toggle"), theme("light"), expect("(%s) === window.__light" % BG, True)])

print(json.dumps(sc, indent=1, ensure_ascii=False))
