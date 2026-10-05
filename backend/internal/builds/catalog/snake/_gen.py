"""Generates scenarios.json for the snake challenge. Expected states come from the Python model below, an
implementation independent of _reference/index.html; the reference must pass every scenario.

    python3 _gen.py > scenarios.json
"""
import json

N = 20
D = {"up": (0, -1), "down": (0, 1), "left": (-1, 0), "right": (1, 0)}
OPP = {"up": "down", "down": "up", "left": "right", "right": "left"}
INIT = [[10, 10], [9, 10], [8, 10]]


def mulberry32(a):
    a &= 0xFFFFFFFF

    def imul(x, y):
        return ((x & 0xFFFFFFFF) * (y & 0xFFFFFFFF)) & 0xFFFFFFFF

    def rnd():
        nonlocal a
        a = (a + 0x6D2B79F5) & 0xFFFFFFFF
        t = imul(a ^ (a >> 15), 1 | a)
        t = ((t + imul(t ^ (t >> 7), 61 | t)) & 0xFFFFFFFF) ^ t
        return ((t ^ (t >> 14)) & 0xFFFFFFFF) / 4294967296

    return rnd


class Game:
    def __init__(self, seed, snake=None, dir="right", food=None):
        self.rng = mulberry32(seed)
        self.snake = [list(c) for c in (snake or INIT)]
        self.dir, self.queue, self.score, self.over = dir, [], 0, False
        self.food = list(food) if food else self.place()
        if self.food is None:
            self.over = True

    def place(self):
        occ = {(x, y) for x, y in self.snake}
        free = [[x, y] for y in range(N) for x in range(N) if (x, y) not in occ]
        return free[int(self.rng() * len(free))] if free else None

    def turn(self, d):
        last = self.queue[-1] if self.queue else self.dir
        if self.over or len(self.queue) >= 2 or d in (last, OPP[last]):
            return
        self.queue.append(d)

    def step(self, n=1):
        for _ in range(n):
            if self.over:
                return
            if self.queue:
                self.dir = self.queue.pop(0)
            dx, dy = D[self.dir]
            head = [self.snake[0][0] + dx, self.snake[0][1] + dy]
            if not (0 <= head[0] < N and 0 <= head[1] < N):
                self.over = True
                return
            eat = self.food == head
            body = self.snake if eat else self.snake[:-1]
            if head in body:
                self.over = True
                return
            self.snake.insert(0, head)
            if eat:
                self.score += 1
                self.food = self.place()
                if self.food is None:
                    self.over = True
            else:
                self.snake.pop()


def goto(path="/?test=1"):
    return {"op": "goto", "path": path}


def js(expr):
    return {"op": "js", "expr": expr}


def reset(seed, **opts):
    return js("window.snake.reset(%d%s)" % (seed, ", " + json.dumps(opts) if opts else ""))


def steps(n):
    return js("(() => { for (let i = 0; i < %d; i++) window.snake.step() })()" % n)


def expect(expr, value):
    return {"op": "expect_js", "expr": expr, "value": value}


def state_is(g, *fields):
    out = []
    for f in fields or ("snake", "dir", "score", "over", "food"):
        out.append(expect("window.snake.state().%s" % f, getattr(g, f)))
    return out


def press(key, selector=None):
    s = {"op": "press", "key": key}
    if selector:
        s["selector"] = selector
    return s


def tid(t):
    return "[data-testid=%s]" % t


KEYMAP = {"ArrowUp": "up", "ArrowDown": "down", "ArrowLeft": "left", "ArrowRight": "right", "w": "up", "s": "down", "a": "left", "d": "right"}


def play(g, *moves):
    """Steps for a sequence of key presses ("ArrowUp") and step counts (3), applied to the model too."""
    out = []
    for m in moves:
        if isinstance(m, int):
            out.append(steps(m))
            g.step(m)
        else:
            out.append(press(m))
            g.turn(KEYMAP[m])
    return out


sc = []


def scenario(name, steps_, **kw):
    sc.append({"name": name, "steps": steps_, **kw})


g = Game(1)
scenario("a new game starts with the documented snake, direction and score", [goto(), reset(1), *state_is(g, "snake", "dir", "score", "over")])
scenario("the first food is drawn from the seed", [goto(), reset(1), *state_is(Game(1), "food"), reset(42), *state_is(Game(42), "food"),
                                                   reset(123456789), *state_is(Game(123456789), "food")])
row = [[x, 0] for x in range(19, -1, -1)]
scenario("food is drawn from the free cells in row-major order", [goto(), reset(7, snake=row), *state_is(Game(7, snake=row), "food"),
                                                                  reset(99, snake=row), *state_is(Game(99, snake=row), "food")])

g = Game(1)
scenario("the snake moves one cell per tick", [goto(), reset(1), *play(g, 3), *state_is(g, "snake", "score")])
g = Game(1)
scenario("arrow keys turn the snake", [goto(), reset(1), *play(g, "ArrowUp", 1, "ArrowLeft", 2, "ArrowDown", 1), *state_is(g, "snake", "dir")])
g = Game(1)
scenario("WASD turns the snake", [goto(), reset(1), *play(g, "s", 1, "a", 1, "w", 1, "d", 1), *state_is(g, "snake", "dir")])
g = Game(1)
scenario("a turn into the opposite direction is ignored", [goto(), reset(1), *play(g, "ArrowLeft", 1), *state_is(g, "snake", "dir")])
g = Game(1)
scenario("two quick turns between ticks are both kept", [goto(), reset(1), *play(g, "ArrowUp", "ArrowLeft", 2), *state_is(g, "snake", "dir")])
g = Game(1)
scenario("a quick turn back is dropped from the buffer", [goto(), reset(1), *play(g, "ArrowUp", "ArrowDown", 2), *state_is(g, "snake", "dir")])
g = Game(1)
scenario("at most two turns are pending", [goto(), reset(1), *play(g, "ArrowUp", "ArrowLeft", "ArrowDown", 3), *state_is(g, "snake", "dir")])
g = Game(1)
scenario("a turn the snake is already making is not queued twice", [goto(), reset(1), *play(g, "ArrowUp", "ArrowUp", "ArrowRight", 3), *state_is(g, "snake")])

g = Game(3, food=[12, 10])
scenario("eating grows the snake, scores and draws new food from the seed",
         [goto(), reset(3, food=[12, 10]), *play(g, 2), *state_is(g, "snake", "score", "food"), {"op": "expect_text", "selector": tid("score"), "text": "1"}])
def chase(g, eats):
    moves = []
    while g.score < eats and not g.over:
        hx, hy = g.snake[0]
        fx, fy = g.food
        want = "right" if fx > hx else "left" if fx < hx else "down" if fy > hy else "up"
        if want == OPP[g.dir]:
            want = "down" if hy < N - 1 else "up"
        if want != g.dir:
            key = {v: k for k, v in KEYMAP.items() if k.startswith("Arrow")}[want]
            moves.append(key)
            g.turn(want)
        moves.append(1)
        g.step()
    return moves


planner = Game(3, food=[11, 10])
planner.step()
moves = chase(planner, 4)
g = Game(3, food=[11, 10])
scenario("the snake keeps eating and growing", [goto(), reset(3, food=[11, 10]), *play(g, 1, *moves), *state_is(g, "snake", "score", "food", "over")])

g = Game(5, food=[0, 0])
g.step(9)
s1 = state_is(g, "snake", "over")
g.step(1)
s2 = state_is(g, "over")
g.step(1)
scenario("leaving the field ends the game (no wrap-around), and ticks after it do nothing",
         [goto(), reset(5, food=[0, 0]), steps(9), *s1, steps(1), *s2, steps(1), *state_is(g, "snake", "over")])

body = [[5, 5], [5, 6], [6, 6], [6, 5], [7, 5]]
g = Game(1, snake=body, dir="up", food=[0, 0])
scenario("running into the body ends the game", [goto(), reset(1, snake=body, dir="up", food=[0, 0]), *play(g, "ArrowRight", 1), *state_is(g, "over")])
loop = [[5, 5], [5, 6], [6, 6], [6, 5]]
g = Game(1, snake=loop, dir="up", food=[0, 0])
scenario("moving into the cell the tail is leaving is allowed", [goto(), reset(1, snake=loop, dir="up", food=[0, 0]), *play(g, "ArrowRight", 1, "ArrowDown", 1), *state_is(g, "snake", "over")])

g = Game(1)
scenario("game over is hidden while playing", [goto(), reset(1), {"op": "expect_hidden", "selector": tid("game-over")}])
fresh = Game(5)
scenario("game over shows a restart button that starts the same seed again",
         [goto(), reset(5, food=[0, 0]), steps(10), {"op": "expect_visible", "selector": tid("game-over")},
          {"op": "click", "selector": tid("restart")}, *state_is(fresh, "snake", "dir", "score", "over", "food"),
          {"op": "expect_hidden", "selector": tid("game-over")}])
fresh = Game(11)
scenario("R restarts with the same seed", [goto(), reset(11), steps(12), expect("window.snake.state().over", True), press("r"),
                                           *state_is(fresh, "snake", "score", "over", "food")])

g = Game(1)
scenario("Space pauses and resumes", [goto(), reset(1), press(" "), steps(2), *state_is(Game(1), "snake"), press(" "), *play(g, 2), *state_is(g, "snake")])

scenario("the best score survives a reload",
         [goto(), reset(2, food=[11, 10]), steps(1), expect("window.snake.state().best", 1), {"op": "expect_text", "selector": tid("best"), "text": "1"},
          goto(), reset(2), expect("window.snake.state().best", 1), {"op": "expect_text", "selector": tid("best"), "text": "1"}])

g = Game(1)
g.turn("up"), g.step(), g.turn("left"), g.step()
scenario("on-screen buttons turn the snake on a phone",
         [goto(), reset(1), *[{"op": "expect_visible", "selector": tid("ctl-" + d)} for d in ("up", "down", "left", "right")],
          {"op": "click", "selector": tid("ctl-up")}, steps(1), {"op": "click", "selector": tid("ctl-left")}, steps(1), *state_is(g, "snake")],
         viewport=375)
scenario("the game fits a phone screen", [goto("/"), {"op": "wait", "ms": 300}, {"op": "expect_no_hscroll"}, goto("/?test=1"), {"op": "expect_no_hscroll"}], viewport=375)

scenario("outside test mode the game runs on its own after the first key",
         [goto("/"), {"op": "wait", "ms": 300}, press("ArrowUp"), expect("window.snake.state().snake[0][1] <= 8", True)])
scenario("Space pauses the running game",
         [goto("/"), {"op": "wait", "ms": 300}, press("ArrowUp"), expect("window.snake.state().snake[0][1] < 10", True), press(" "),
          js("window.__held = JSON.stringify(window.snake.state().snake)"), {"op": "wait", "ms": 800},
          expect("JSON.stringify(window.snake.state().snake) === window.__held", True)])

print(json.dumps(sc, indent=1))
