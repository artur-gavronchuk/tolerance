# Benchmark generator (platform code, runs in the sandbox): python3 bench_gen.py OUTDIR writes plan.json plus the
# input and expected-output files. Deterministic: the same files every run.
import json
import math
import os
import random
import sys

out = sys.argv[1]
rnd = random.Random(20261003)
NAMES = ["v%d" % i for i in range(40)] + ["total", "x", "y", "rate", "_tmp"]
FUNCS = {"sqrt": math.sqrt, "abs": abs, "floor": lambda v: float(math.floor(v)), "ceil": lambda v: float(math.ceil(v))}
env = {"pi": math.pi, "e": math.e}


def fmt(x):
    if x == 0:
        return "0"
    if x == int(x) and abs(x) < 1e15:
        return str(int(x))
    return format(x, ".12g")


class Bad(Exception):
    pass


def expr(depth, defined):
    """Returns (text, value, precedence); precedence: 0 sum, 1 product, 2 unary, 3 power, 4 atom."""
    r = rnd.random()
    if depth <= 0 or r < 0.25:
        c = rnd.random()
        if c < 0.55 or not defined:
            n = rnd.choice(["%d" % rnd.randint(0, 999), "%d.%d" % (rnd.randint(0, 99), rnd.randint(0, 99)), ".%d" % rnd.randint(1, 99), "%de%d" % (rnd.randint(1, 9), rnd.randint(0, 3))])
            return n, float(n), 4
        if c < 0.9:
            k = rnd.choice(defined)
            return k, env[k], 4
        k = rnd.choice(["pi", "e"])
        return k, env[k], 4
    if r < 0.55:
        op = rnd.choice("+-")
        a, b = expr(depth - 1, defined), expr(depth - 1, defined)
        return "%s %s %s" % (paren(a, 0), op, paren(b, 1)), (a[1] + b[1] if op == "+" else a[1] - b[1]), 0
    if r < 0.8:
        op = rnd.choice("*/%")
        a, b = expr(depth - 1, defined), expr(depth - 1, defined)
        if op != "*" and b[1] == 0:
            raise Bad
        v = a[1] * b[1] if op == "*" else a[1] / b[1] if op == "/" else a[1] % b[1]
        return "%s %s %s" % (paren(a, 1), op, paren(b, 2)), v, 1
    if r < 0.88:
        a = expr(depth - 1, defined)
        return "-" + paren(a, 3), -a[1], 2
    if r < 0.93:
        a, b = expr(depth - 1, defined), expr(depth - 1, defined)
        if a[1] <= 0 or abs(b[1]) > 4:
            raise Bad
        return "%s ^ %s" % (paren(a, 4), paren(b, 4)), a[1] ** b[1], 3
    if r < 0.97:
        f = rnd.choice(list(FUNCS))
        a = expr(depth - 1, defined)
        if f == "sqrt" and a[1] < 0:
            raise Bad
        return "%s(%s)" % (f, a[0]), FUNCS[f](a[1]), 4
    f = rnd.choice(["min", "max"])
    args = [expr(depth - 1, defined) for _ in range(rnd.randint(1, 4))]
    return "%s(%s)" % (f, ", ".join(a[0] for a in args)), (min if f == "min" else max)(a[1] for a in args), 4


def paren(a, need):
    return "(%s)" % a[0] if a[2] < need else a[0]


def build(want_lines):
    lines, outs = [], []
    defined = []
    bad_lines = 0
    while len(lines) < want_lines:
        try:
            if rnd.random() < 0.6 and rnd.random() > 0.0:
                t, v, _ = expr(rnd.randint(2, 6), defined)
                if not math.isfinite(v) or abs(v) > 1e12:
                    continue
                kind = "assign"
                name = rnd.choice(NAMES)
                line = "%s = %s" % (name, t)
                v = v
            else:
                t, v, _ = expr(rnd.randint(2, 6), defined)
                if not math.isfinite(v) or abs(v) > 1e12:
                    continue
                kind, line = "print", t
        except (Bad, OverflowError, ZeroDivisionError):
            continue
        if rnd.random() < 0.1:
            line += "  # note"
        if rnd.random() < 0.05:
            lines.append("")
            lines.append("# comment")
        lineno = len(lines) + 1
        lines.append(line)
        if kind == "assign":
            env[name] = v
            if name not in defined:
                defined.append(name)
        else:
            outs.append(fmt(v))
        if rnd.random() < 0.01:  # an error line: division by zero
            lines.append("1 / (2 - 2)")
            outs.append("error line %d" % len(lines))
            bad_lines += 1
    return lines, outs, bad_lines


lines, outs, bad = build(60000)
open(os.path.join(out, "prog.in"), "w").write("\n".join(lines) + "\n")
open(os.path.join(out, "prog.out"), "w").write("\n".join(outs) + "\n")
plan = [{"name": "long program", "invocations": [{"args": [], "stdin": "prog.in", "stdout": "prog.out", "exit_code": 1 if bad else 0}]}]
json.dump(plan, open(os.path.join(out, "plan.json"), "w"))
