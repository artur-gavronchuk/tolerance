# Scenario runner for "site" products, executed inside the sandbox (no network, dropped capabilities). It is
# trusted platform code: it serves the participant's static files on 127.0.0.1 and drives them in Chromium.
# The participant's JavaScript only ever runs inside the page.
import functools
import http.server
import json
import threading

from playwright.sync_api import sync_playwright, expect

STEP_MS = 3000
spec = json.load(open("_scenarios/spec.json"))


class Quiet(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *a):
        pass


server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), functools.partial(Quiet, directory="solution"))
threading.Thread(target=server.serve_forever, daemon=True).start()
base = "http://127.0.0.1:%d" % server.server_address[1]


def step(page, s):
    op = s["op"]
    loc = page.locator(s["selector"]) if "selector" in s else None
    if op == "goto":
        resp = page.goto(base + s.get("path", "/"), timeout=STEP_MS * 2)
        if resp is None or resp.status >= 400:
            raise AssertionError("goto %s: status %s" % (s.get("path", "/"), resp and resp.status))
    elif op == "reload":
        page.reload(timeout=STEP_MS * 2)
    elif op == "fill":
        loc.first.fill(s["value"], timeout=STEP_MS)
    elif op == "click":
        loc.first.click(timeout=STEP_MS)
    elif op == "select":
        loc.first.select_option(s["value"], timeout=STEP_MS)
    elif op == "expect_number":
        got = None
        for _ in range(STEP_MS // 100):
            try:
                got = float(loc.first.input_value(timeout=STEP_MS).replace(",", "."))
                if abs(got - s["value"]) <= s.get("tolerance", 0.0):
                    break
            except ValueError:
                got = None
            page.wait_for_timeout(100)
        else:
            raise AssertionError("expected %s (+-%s), got %s" % (s["value"], s.get("tolerance", 0.0), got))
    elif op == "press":
        loc.first.press(s["key"], timeout=STEP_MS)
    elif op == "expect_text":
        expect(loc.first).to_contain_text(s["text"], timeout=STEP_MS)
    elif op == "expect_visible":
        expect(loc.first).to_be_visible(timeout=STEP_MS)
    elif op == "expect_count":
        expect(loc).to_have_count(s["count"], timeout=STEP_MS)
    elif op == "expect_checked":
        expect(loc.first).to_be_checked(checked=s.get("checked", True), timeout=STEP_MS)
    elif op == "expect_value":
        expect(loc.first).to_have_value(s["value"], timeout=STEP_MS)
    elif op == "expect_no_hscroll":
        w = page.evaluate("[document.documentElement.scrollWidth, document.documentElement.clientWidth]")
        if w[0] > w[1]:
            raise AssertionError("horizontal scroll: content %dpx in a %dpx viewport" % (w[0], w[1]))
    else:
        raise AssertionError("unknown op " + op)


results = []
with sync_playwright() as p:
    browser = p.chromium.launch(args=["--no-sandbox", "--disable-dev-shm-usage", "--disable-gpu"])
    for sc in spec["scenarios"]:
        ok = False
        ctx = browser.new_context(viewport={"width": sc.get("viewport") or 1024, "height": 768})
        try:
            page = ctx.new_page()
            page.on("pageerror", lambda e: print("  pageerror:", str(e)[:200]))
            for s in sc.get("steps", []):
                step(page, s)
            ok = True
        except Exception as e:
            msg = str(e).strip()
            print("FAIL %s: %s" % (sc["name"], msg.splitlines()[0][:300] if msg else type(e).__name__))
        finally:
            ctx.close()
        results.append({"name": sc["name"], "passed": ok})
    browser.close()
print("@@RESULTS@@" + json.dumps(results))
