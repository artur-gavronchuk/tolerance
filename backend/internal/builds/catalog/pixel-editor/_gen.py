"""Generates scenarios.json for the pixel-editor challenge. Expected grids come from the Python model below,
an implementation independent of _reference/index.html; the reference must pass every scenario.

    python3 _gen.py > scenarios.json
"""
import json
from collections import deque

N = 16
PAL = ["#000000", "#1d2b53", "#7e2553", "#008751", "#ab5236", "#5f574f", "#c2c3c7", "#fff1e8",
       "#ff004d", "#ffa300", "#ffec27", "#00e436", "#29adff", "#83769c", "#ff77a8", "#ffccaa"]
HEX = "0123456789abcdef"
PAL_JS = json.dumps(PAL)


# ---- the model: what the editor must do, written from the rules in TASK.md

class Model:
    def __init__(self):
        self.grid = [[None] * N for _ in range(N)]  # grid[y][x] = palette index or None
        self.color, self.tool = 0, "pencil"
        self.undo, self.redo = [], []

    def snap(self):
        return [row[:] for row in self.grid]

    def commit(self, before):
        if before != self.grid:  # an action that changes nothing leaves no trace at all
            self.undo.append(before)
            self.redo = []

    def set_tool(self, t):
        self.tool = t

    def set_color(self, i):
        self.color = i
        if self.tool == "eraser":
            self.tool = "pencil"

    def paint(self, x, y):
        self.grid[y][x] = None if self.tool == "eraser" else self.color

    def fill(self, x, y):
        target = self.grid[y][x]
        if target == self.color:
            return
        seen, q = {(x, y)}, deque([(x, y)])
        while q:
            cx, cy = q.popleft()
            self.grid[cy][cx] = self.color
            for nx, ny in ((cx + 1, cy), (cx - 1, cy), (cx, cy + 1), (cx, cy - 1)):
                if 0 <= nx < N and 0 <= ny < N and (nx, ny) not in seen and self.grid[ny][nx] == target:
                    seen.add((nx, ny))
                    q.append((nx, ny))

    def click(self, x, y):
        before = self.snap()
        self.fill(x, y) if self.tool == "fill" else self.paint(x, y)
        self.commit(before)

    def drag(self, a, b):
        """One press on cell a, one jump of the pointer to cell b, release. Only axis-aligned and exactly
        diagonal strokes are used, where the straight line between the two cells is unambiguous."""
        (x0, y0), (x1, y1) = a, b
        dx, dy = x1 - x0, y1 - y0
        assert dx == 0 or dy == 0 or abs(dx) == abs(dy), "ambiguous line"
        before = self.snap()
        n = max(abs(dx), abs(dy))
        sx, sy = (dx > 0) - (dx < 0), (dy > 0) - (dy < 0)
        for i in range(n + 1):
            self.paint(x0 + sx * i, y0 + sy * i)
        self.commit(before)

    def do_undo(self):
        if self.undo:
            self.redo.append(self.snap())
            self.grid = self.undo.pop()

    def do_redo(self):
        if self.redo:
            self.undo.append(self.snap())
            self.grid = self.redo.pop()

    def clear(self):
        before = self.snap()
        self.grid = [[None] * N for _ in range(N)]
        self.commit(before)

    def rows(self):
        return ["".join("." if c is None else HEX[c] for c in row) for row in self.grid]

    def meta(self):
        return {"color": self.color, "tool": self.tool, "canUndo": bool(self.undo), "canRedo": bool(self.redo)}


# ---- step builders

def tid(t):
    return "[data-testid=%s]" % t


def cell(x, y):
    return tid("cell-%d-%d" % (x, y))


def expect(expr, value):
    return {"op": "expect_js", "expr": expr, "value": value}


GRID_EXPR = ("(() => { const P = %s; return window.pixel.state().grid.map(r => r.map(c => c === null ? '.' : "
             "'0123456789abcdef'[P.indexOf(c)]).join('')) })()" % PAL_JS)
DOM_EXPR = ("(() => { const P = %s; const rows = []; for (let y = 0; y < 16; y++) { let s = ''; "
            "for (let x = 0; x < 16; x++) { const c = document.querySelector('[data-testid=cell-' + x + '-' + y + ']')"
            ".getAttribute('data-color'); s += c === null ? '.' : '0123456789abcdef'[P.indexOf(c)] } rows.push(s) } return rows })()" % PAL_JS)
EXPORT_EXPR = ("(async () => { const P = %s; const u = await window.pixel.exportPNG(); const img = new Image(); img.src = u; "
               "await img.decode(); const c = document.createElement('canvas'); c.width = img.width; c.height = img.height; "
               "const x = c.getContext('2d'); x.drawImage(img, 0, 0); const d = x.getImageData(0, 0, c.width, c.height).data; "
               "const rows = []; for (let y = 0; y < c.height; y++) { let s = ''; for (let i = 0; i < c.width; i++) { "
               "const o = (y * c.width + i) * 4; if (d[o + 3] === 0) { s += '.'; continue } "
               "const hex = '#' + [d[o], d[o + 1], d[o + 2]].map(v => v.toString(16).padStart(2, '0')).join(''); "
               "s += d[o + 3] === 255 ? '0123456789abcdef'[P.indexOf(hex)] : '?' } rows.push(s) } "
               "return { head: u.slice(0, 22), size: [img.width, img.height], rows } })()" % PAL_JS)


def attr(testid, name):
    return "document.querySelector('[data-testid=%s]').%s" % (testid, name)


class Scenario:
    """Records steps and applies each of them to the model as well."""

    def __init__(self, name, test=True, **kw):
        self.name, self.kw, self.m, self.steps = name, kw, Model(), []
        self.steps.append({"op": "goto", "path": "/?test=1" if test else "/"})

    def add(self, *s):
        self.steps.extend(s)
        return self

    def color(self, i):
        self.m.set_color(i)
        return self.add({"op": "click", "selector": tid("color-%d" % i)})

    def tool(self, t):
        self.m.set_tool(t)
        return self.add({"op": "click", "selector": tid("tool-" + t)})

    def key(self, k):
        if k in ("b", "e", "f", "B", "E", "F") or k in ("Shift+b", "Shift+e", "Shift+f"):
            self.m.set_tool({"b": "pencil", "e": "eraser", "f": "fill"}[k[-1].lower()])
        elif k == "Control+z":
            self.m.do_undo()
        elif k in ("Control+Shift+z", "Control+y"):
            self.m.do_redo()
        return self.add({"op": "press", "key": k})

    def click(self, x, y):
        self.m.click(x, y)
        return self.add({"op": "click", "selector": cell(x, y)})

    def clicks(self, cells):
        for x, y in cells:
            self.click(x, y)
        return self

    def drag(self, a, b):
        self.m.drag(a, b)
        return self.add({"op": "drag", "selector": cell(*a), "target": cell(*b)})

    def reload(self):
        self.m.undo, self.m.redo, self.m.tool = [], [], "pencil"  # the history is not persisted; the tool is not specified
        return self.add({"op": "reload"})

    def btn(self, name):
        assert name == "clear" or (self.m.undo if name == "undo" else self.m.redo), "a disabled button cannot be clicked"
        {"undo": self.m.do_undo, "redo": self.m.do_redo, "clear": self.m.clear}[name]()
        return self.add({"op": "click", "selector": tid(name)})

    def grid(self):
        return self.add(expect(GRID_EXPR, self.m.rows()))

    def meta(self, *fields):
        for f in fields or ("color", "tool", "canUndo", "canRedo"):
            self.add(expect("window.pixel.state().%s" % f, self.m.meta()[f]))
        return self

    def check(self):
        return self.grid().meta()

    def done(self):
        return {"name": self.name, "steps": self.steps, **self.kw}


sc = []


def add(s):
    sc.append(s.done())


def rect(x0, y0, x1, y1):
    """The border cells of a rectangle, as a click list."""
    out = []
    for x in range(x0, x1 + 1):
        out += [(x, y0), (x, y1)]
    for y in range(y0 + 1, y1):
        out += [(x0, y), (x1, y)]
    return out


# 1
s = Scenario("the canvas is 16 by 16 and starts empty and ready")
s.add({"op": "expect_count", "selector": "[data-testid^=cell-]", "count": 256},
      {"op": "expect_visible", "selector": cell(0, 0)}, {"op": "expect_visible", "selector": cell(15, 15)},
      {"op": "expect_count", "selector": cell(16, 0), "count": 0}, {"op": "expect_count", "selector": cell(0, 16), "count": 0}).check()
add(s)

# 2
s = Scenario("the palette has the 16 fixed colors, reported as lowercase hex")
s.add({"op": "expect_count", "selector": "[data-testid^=color-]", "count": 16})
for i in range(16):
    s.color(i).click(i, 0)
s.grid().add(expect("window.pixel.state().grid[0]", PAL), expect("window.pixel.state().grid[1]", [None] * 16)).meta()
add(s)

# 3
s = Scenario("the selected color is marked with aria-pressed")
s.add(expect(attr("color-0", "getAttribute('aria-pressed')"), "true"), expect(attr("color-5", "getAttribute('aria-pressed')"), "false"))
s.color(5).add(expect(attr("color-5", "getAttribute('aria-pressed')"), "true"), expect(attr("color-0", "getAttribute('aria-pressed')"), "false"),
               expect("document.querySelectorAll('[data-testid^=color-][aria-pressed=true]').length", 1))
s.color(12).add(expect(attr("color-12", "getAttribute('aria-pressed')"), "true"), expect(attr("color-5", "getAttribute('aria-pressed')"), "false"),
                expect("document.querySelectorAll('[data-testid^=color-][aria-pressed=true]').length", 1)).meta("color")
add(s)

# 4
s = Scenario("the pencil paints the clicked cell and x runs left to right, y top to bottom")
s.color(8).click(3, 4).color(12).click(15, 0).click(0, 15).color(11).click(15, 15).click(0, 0).color(14).click(3, 4)
s.check()
s.add(expect("window.pixel.state().grid[4][3]", PAL[14]), expect("window.pixel.state().grid[0][15]", PAL[12]),
      expect("window.pixel.state().grid[15][0]", PAL[12]))
add(s)

# 5
s = Scenario("the cells show what the hook reports")
s.color(8).click(2, 2).click(3, 2).color(10).click(7, 11).color(3).click(15, 14).color(14).click(2, 2).tool("eraser").click(3, 2)
s.add(expect(DOM_EXPR, s.m.rows()))
s.grid()
add(s)

# 6
s = Scenario("a stroke paints a straight line even when the pointer jumps over cells")
s.color(8).drag((2, 3), (9, 3)).color(12).drag((12, 1), (12, 9)).color(11).drag((1, 14), (6, 9)).color(14).drag((8, 6), (14, 12)).check()
add(s)

# 7
s = Scenario("strokes work in every direction")
s.color(9).drag((13, 2), (4, 2)).color(10).drag((2, 14), (2, 5)).color(3).drag((14, 14), (8, 8)).color(13).drag((14, 6), (9, 11)).check()
add(s)

# 8
s = Scenario("one press-drag-release is one undo step, however many cells it touched")
s.color(8).drag((1, 1), (14, 1)).drag((1, 3), (1, 12)).meta("canUndo")
s.btn("undo").grid().meta("canUndo", "canRedo")
s.btn("undo").check()
add(s)

# 9
s = Scenario("a press that changes nothing creates no undo step")
s.color(8).click(5, 5).click(5, 5).tool("eraser").click(6, 6).drag((7, 7), (7, 12)).meta("canUndo")
s.tool("pencil").color(10).drag((0, 8), (7, 8)).drag((2, 8), (6, 8)).click(3, 8).color(10).drag((0, 8), (7, 8)).meta("canUndo")
s.btn("undo").btn("undo").check()
add(s)

# 11
s = Scenario("undo and redo walk through the history")
s.color(8).click(1, 1).color(12).click(2, 2).color(11).click(3, 3)
s.btn("undo").check().btn("undo").check().btn("redo").check().btn("redo").check()
s.btn("undo").btn("undo").btn("undo").check()
add(s)

# 12
s = Scenario("undo and redo are disabled when there is nothing to undo or redo")
dis = lambda b: attr(b, "disabled")
s.add(expect(dis("undo"), True), expect(dis("redo"), True))
s.color(8).click(4, 4)
s.add(expect(dis("undo"), False), expect(dis("redo"), True))
s.btn("undo").add(expect(dis("undo"), True), expect(dis("redo"), False))
s.btn("redo").add(expect(dis("undo"), False), expect(dis("redo"), True)).meta()
add(s)

# 13
s = Scenario("a new action after undo clears the redo stack")
s.color(8).click(1, 1).click(2, 2).click(3, 3).btn("undo").btn("undo").meta("canRedo")
s.color(12).click(9, 9).check()
s.add(expect(dis("redo"), True))
s.key("Control+Shift+z").key("Control+y").check()
add(s)

# 14
s = Scenario("Ctrl+Z, Ctrl+Shift+Z and Ctrl+Y undo and redo")
s.color(8).click(1, 1).click(2, 2).click(3, 3).click(4, 4)
s.key("Control+z").check().key("Control+z").check().key("Control+Shift+z").check().key("Control+y").check().key("Control+z").key("Control+z").key("Control+z").check()
s.key("Control+z").check()
add(s)

# 15
s = Scenario("the history is at least 100 steps deep")
cells100 = [(i % 16, i // 16) for i in range(100)]
s.color(2).clicks(cells100).meta("canUndo")
for _ in range(100):
    s.key("Control+z")
s.check()
for _ in range(100):
    s.key("Control+y")
s.check()
add(s)

# 16
s = Scenario("the eraser empties cells, one stroke is one step, erasing nothing is no step")
s.color(8).drag((1, 2), (12, 2)).drag((1, 4), (12, 4))
s.tool("eraser").add(expect(attr("tool-eraser", "getAttribute('aria-pressed')"), "true"), expect(attr("tool-pencil", "getAttribute('aria-pressed')"), "false"))
s.drag((4, 2), (9, 2)).click(6, 4).click(6, 4).click(14, 14).drag((2, 10), (2, 15))
s.check().btn("undo").check().btn("undo").check().btn("undo").check().meta("canUndo")
add(s)

# 18
s = Scenario("the eraser keeps the color, and picking a color goes back to the pencil")
s.color(8).tool("eraser").meta().color(9).meta("tool", "color")
s.add(expect(attr("tool-pencil", "getAttribute('aria-pressed')"), "true")).click(5, 5)
s.tool("fill").color(10).meta("tool", "color").tool("eraser").key("b").meta("tool").click(0, 0).check()
add(s)

# 19
s = Scenario("fill floods an empty canvas, in one undo step")
s.color(14).tool("fill").click(7, 7).check().btn("undo").check().btn("redo").check()
add(s)

# 20
s = Scenario("fill stops at a closed wall and fills inside or outside only")
s.color(7).clicks(rect(3, 3, 9, 8))
s.tool("fill").color(10).click(5, 5)
s.check()
s.color(12).click(0, 0).check()
s.btn("undo").btn("undo").check()
add(s)

# 21
s = Scenario("fill is 4-connected: it does not leak through a diagonal wall")
s.color(6).clicks([(i, i) for i in range(16)])
s.tool("fill").color(8).click(15, 0).check()
s.color(11).click(0, 15).check()
s.btn("undo").check()
add(s)

# 22
s = Scenario("fill follows a winding corridor but not past a sealed wall")
walls = [(x, 3) for x in range(0, 14)] + [(x, 7) for x in range(2, 16)] + [(x, 11) for x in range(16)]
s.color(1).clicks(walls)
s.tool("fill").color(9).click(0, 0).check()
s.color(3).click(0, 15).check()
add(s)

# 23
s = Scenario("fill recolors only the clicked color region, other regions of that color stay")
s.tool("fill").color(1).click(0, 0)
s.tool("pencil").color(2).clicks([(5, y) for y in range(16)])
s.tool("fill").color(3).click(0, 0).check()
s.color(4).click(5, 9).check()
s.color(13).click(10, 10).check()
s.btn("undo").btn("undo").check()
add(s)

# 24
s = Scenario("filling with the color the region already has changes nothing and is not an undo step")
s.color(8).click(2, 2).tool("fill").click(2, 2).meta("canUndo")
s.color(12).click(9, 9).color(12).click(9, 9).click(0, 0).check()
s.btn("undo").check().btn("undo").check()
add(s)

# 25
s = Scenario("actions that change nothing keep the redo stack")
s.color(8).click(1, 1).click(2, 2).btn("undo")
s.tool("fill").click(1, 1).tool("eraser").click(10, 10).meta("canRedo")
s.btn("redo").check().btn("undo").btn("undo").btn("clear").meta("canUndo", "canRedo")
s.btn("redo").check()
add(s)

# 26
s = Scenario("B, E and F pick the tool and the buttons show it")
for k, t in (("e", "eraser"), ("f", "fill"), ("b", "pencil"), ("E", "eraser"), ("Shift+f", "fill"), ("Shift+b", "pencil")):
    s.key(k)
    s.add(expect(attr("tool-" + t, "getAttribute('aria-pressed')"), "true"),
          expect("document.querySelectorAll('[data-testid^=tool-][aria-pressed=true]').length", 1)).meta("tool")
s.add({"op": "press", "key": "Control+e"}).add({"op": "press", "key": "Control+f"}).meta("tool")
s.key("e").color(8).click(0, 0).meta("tool")
add(s)

# 27
s = Scenario("clear empties the canvas as one undoable step, and clearing nothing is no step")
s.btn("clear").meta("canUndo")
s.color(8).click(1, 1).color(12).click(2, 2).drag((4, 4), (4, 9)).btn("clear").check()
s.btn("undo").check().btn("redo").check().btn("undo").btn("undo").btn("undo").check()
add(s)

# 29
s = Scenario("the drawing and the selected color survive a reload")
s.color(8).click(1, 1).click(2, 1).color(12).click(8, 8).click(9, 9).btn("undo").color(9)
s.add({"op": "reload"})
s.grid().meta("color")
s.add(expect(attr("color-9", "getAttribute('aria-pressed')"), "true")).click(10, 10).grid()
s.btn("clear").reload().grid()
add(s)

# 30
s = Scenario("the picture is exported as a 16 by 16 PNG, transparent where empty")
s.add(expect(EXPORT_EXPR, {"head": "data:image/png;base64,", "size": [16, 16], "rows": ["." * 16] * 16}))
s.color(8).click(0, 0).color(11).click(15, 0).color(12).click(0, 15).color(14).click(15, 15).color(10).drag((5, 7), (10, 7)).tool("eraser").click(7, 7)
s.add(expect(EXPORT_EXPR, {"head": "data:image/png;base64,", "size": [16, 16], "rows": s.m.rows()}))
s.btn("undo").btn("undo")
s.add(expect(EXPORT_EXPR, {"head": "data:image/png;base64,", "size": [16, 16], "rows": s.m.rows()}))
add(s)

# 31
s = Scenario("the editor fits a phone screen and drawing works there", viewport=375)
s.add({"op": "wait", "ms": 300}, {"op": "expect_no_hscroll"}, {"op": "expect_visible", "selector": cell(0, 0)}, {"op": "expect_visible", "selector": cell(15, 15)},
      {"op": "expect_visible", "selector": tid("tool-fill")}, {"op": "expect_visible", "selector": tid("undo")}, {"op": "expect_visible", "selector": tid("clear")})
s.color(15).click(0, 0).click(15, 15).color(8).drag((3, 3), (8, 3)).tool("fill").color(12).click(10, 10).check()
s.add({"op": "expect_no_hscroll"})
s.btn("undo").btn("undo").check()
add(s)

print(json.dumps(sc, indent=1, ensure_ascii=False))
