# Build challenge runner, executed inside the sandbox (no network, dropped capabilities). It is trusted
# platform code: it serves the participant's static files on 127.0.0.1, drives them in Chromium through the
# challenge's scenarios, measures quality signals and saves a screenshot to _out/shot.jpg. The participant's
# JavaScript only ever runs inside the page. The verdict is one line: <spec marker>{results, quality}; the
# marker is a per-run secret, so page text echoed into the log (errors, failed expectations) cannot forge it.
import functools
import http.server
import json
import os
import statistics
import threading

from playwright.sync_api import sync_playwright, expect

STEP_MS = 3000
spec = json.load(open("_run/spec.json"))


class Quiet(http.server.SimpleHTTPRequestHandler):
    # UTF-8 like the platform's own preview, so a page without <meta charset> looks the same in both.
    extensions_map = {**http.server.SimpleHTTPRequestHandler.extensions_map, ".html": "text/html; charset=utf-8",
                      ".htm": "text/html; charset=utf-8", ".js": "text/javascript; charset=utf-8",
                      ".mjs": "text/javascript; charset=utf-8", ".css": "text/css; charset=utf-8"}

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
        if loc is None:
            page.keyboard.press(s["key"])  # the focused element
        else:
            loc.first.press(s["key"], timeout=STEP_MS)
    elif op == "type":
        loc.first.press_sequentially(s["value"], delay=s.get("delay", 0), timeout=STEP_MS)
    elif op == "dblclick":
        loc.first.dblclick(timeout=STEP_MS)
    elif op == "drag":
        loc.first.drag_to(page.locator(s["target"]).first, timeout=STEP_MS)
    elif op == "set_time":
        # Date.now() and new Date() return this instant (an ISO string with Z or an offset); timers keep running.
        page.clock.set_fixed_time(s["time"])
    elif op == "js":
        # Trusted platform expression evaluated once in the page (drives a test hook); its result is ignored.
        page.evaluate(s["expr"])
    elif op == "wait":
        page.wait_for_timeout(s["ms"])
    elif op == "expect_no_errors":
        # Uncaught exceptions and console.error since the scenario started (favicon misses aside).
        page.wait_for_timeout(s.get("ms", 300))
        errs = [e for e in page.arena_errors if "favicon" not in e]
        if errs:
            raise AssertionError("page errors: " + " | ".join(errs[:3])[:300])
    elif op == "expect_js":
        # Trusted platform expression evaluated in the page; its JSON result must equal "value".
        got = None
        for _ in range(STEP_MS // 100):
            got = page.evaluate(s["expr"])
            if got == s["value"]:
                break
            page.wait_for_timeout(100)
        else:
            raise AssertionError("expected %s, got %s" % (json.dumps(s["value"]), json.dumps(got)))
    elif op == "expect_hidden":
        expect(loc.first).to_be_hidden(timeout=STEP_MS)
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


AXE = "/opt/axe.min.js"
IMPACTS = ("critical", "serious", "moderate", "minor")
TARGETS_JS = """() => {
  const sel = 'a[href],button,input:not([type=hidden]),select,textarea,summary,[role=button],[role=link],[tabindex]:not([tabindex="-1"])';
  let n = 0;
  for (const el of document.querySelectorAll(sel)) {
    const cs = getComputedStyle(el);
    if (cs.visibility === 'hidden' || cs.display === 'none' || cs.display === 'contents') continue;
    if (cs.display === 'inline' && el.tagName === 'A') continue; // inline links in text are exempt (WCAG 2.5.8)
    const r = el.getBoundingClientRect();
    if (r.width === 0 || r.height === 0) continue;
    if (r.width < 24 || r.height < 24) n++;
  }
  return n;
}"""


def axe_scan(browser, width):
    """Violations of one page load at a viewport width: counts by impact and a rule -> affected nodes map."""
    ctx = browser.new_context(viewport={"width": width, "height": 768})
    try:
        page = ctx.new_page()
        page.goto(base + "/", timeout=STEP_MS * 2, wait_until="load")
        page.wait_for_timeout(300)
        page.add_script_tag(path=AXE)
        res = page.evaluate("axe.run(document).then(r => r.violations.map(v => ({id: v.id, impact: v.impact, n: v.nodes.length})))")
        counts = {k: 0 for k in IMPACTS}
        rules = {}
        for v in res:
            counts[v["impact"] if v["impact"] in counts else "minor"] += 1
            rules[v["id"]] = rules.get(v["id"], 0) + v["n"]
        out = {"counts": counts, "rules": rules}
        if width <= 480:
            out["overflow"] = bool(page.evaluate("document.documentElement.scrollWidth > document.documentElement.clientWidth"))
            out["small_targets"] = page.evaluate(TARGETS_JS)
        return out
    finally:
        ctx.close()


def perf_load(browser):
    ctx = browser.new_context(viewport={"width": 1024, "height": 768})
    try:
        page = ctx.new_page()
        size = [0, 0]
        errors = []

        def on_response(r):
            size[1] += 1
            try:
                size[0] += len(r.body())
            except Exception:
                pass

        page.on("response", on_response)
        page.on("pageerror", lambda e: errors.append(str(e)))
        page.on("console", lambda m: errors.append(m.text) if m.type == "error" else None)
        page.goto(base + "/", timeout=STEP_MS * 2, wait_until="load")
        page.wait_for_timeout(500)
        nav = page.evaluate("(() => { const n = performance.getEntriesByType('navigation')[0]; return [n.domContentLoadedEventEnd, n.loadEventEnd]; })()")
        return {"bytes": size[0], "requests": size[1], "dcl_ms": nav[0], "load_ms": nav[1], "errors": len(errors)}
    finally:
        ctx.close()


def quality(browser):
    """Quality signals (the Go side turns them into points): accessibility, load performance, mobile fit."""
    q = {}
    try:
        desk, mob = axe_scan(browser, 1024), axe_scan(browser, 375)
        rules = {}
        for scan in (desk, mob):
            for k, n in scan["rules"].items():
                rules[k] = rules.get(k, 0) + n
        top = [k for k, _ in sorted(rules.items(), key=lambda kv: (-kv[1], kv[0]))[:5]]
        q["a11y"] = {"desktop": desk["counts"], "mobile": mob["counts"], "top_rules": top}
        q["mobile"] = {"overflow": mob["overflow"], "small_targets": mob["small_targets"]}
    except Exception as e:
        print("quality a11y:", str(e).strip().splitlines()[0][:200])
    try:
        loads = [perf_load(browser) for _ in range(3)]
        med = lambda k: round(statistics.median(l[k] for l in loads))
        q["perf"] = {"bytes": med("bytes"), "requests": med("requests"), "dcl_ms": med("dcl_ms"),
                     "load_ms": med("load_ms"), "errors": loads[0]["errors"]}
    except Exception as e:
        print("quality perf:", str(e).strip().splitlines()[0][:200])
    return q


def screenshot(browser):
    shot = spec.get("shot") or {}
    ctx = browser.new_context(viewport={"width": shot.get("width", 1280), "height": shot.get("height", 800)},
                              device_scale_factor=shot.get("scale", 1), has_touch=bool(shot.get("touch")))
    try:
        page = ctx.new_page()
        page.goto(base + "/", timeout=STEP_MS * 2, wait_until="load")
        page.wait_for_timeout(800)
        os.makedirs("_out", exist_ok=True)
        page.screenshot(path="_out/shot.jpg", type="jpeg", quality=72)
    except Exception as e:
        print("screenshot:", str(e).strip().splitlines()[0][:200])
    finally:
        ctx.close()


results = []
q = {}
with sync_playwright() as p:
    browser = p.chromium.launch(args=["--no-sandbox", "--disable-dev-shm-usage", "--disable-gpu"])
    screenshot(browser)
    for sc in spec["scenarios"]:
        ok = False
        width = sc.get("viewport") or spec.get("viewport") or 1024
        opts = {"viewport": {"width": width, "height": 844 if width < 600 else 768}}
        if width < 600:
            opts["has_touch"] = True
        if sc.get("timezone"):
            opts["timezone_id"] = sc["timezone"]
        ctx = browser.new_context(**opts)
        try:
            page = ctx.new_page()
            page.arena_errors = []
            page.on("pageerror", lambda e, p=page: (p.arena_errors.append(str(e)[:200]), print("  pageerror:", str(e)[:200])))
            page.on("console", lambda m, p=page: p.arena_errors.append(m.text[:200]) if m.type == "error" else None)
            for s in sc.get("steps", []):
                step(page, s)
            ok = True
        except Exception as e:
            msg = str(e).strip()
            print("FAIL %s: %s" % (sc["name"], msg.splitlines()[0][:300] if msg else type(e).__name__))
        finally:
            ctx.close()
        results.append({"name": sc["name"], "passed": ok})
    q = quality(browser)
    browser.close()
print(spec["marker"] + json.dumps({"results": results, "quality": q}))
