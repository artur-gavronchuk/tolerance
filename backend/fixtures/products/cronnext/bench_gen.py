# Benchmark generator (platform code, runs in the sandbox): python3 bench_gen.py OUTDIR writes plan.json plus the
# input and expected-output files. Deterministic: the same files every run. One pass runs the tool once per
# generated expression, so the time covers start-up plus the search, and sparse expressions punish stepping minute
# by minute.
import datetime as dt
import json
import os
import random
import sys

out = sys.argv[1]
rnd = random.Random(20261003)
MONTHS = ["jan", "feb", "mar", "apr", "may", "jun", "jul", "aug", "sep", "oct", "nov", "dec"]
DOWS = ["sun", "mon", "tue", "wed", "thu", "fri", "sat"]


def field(lo, hi, names=None, dow=False):
    """A random field: (text, set of values)."""
    items, vals = [], set()
    for _ in range(rnd.choice([1, 1, 2, 3])):
        kind = rnd.choice(["star", "step", "value", "range", "rstep"])
        top = 6 if dow else hi
        if kind == "star":
            if len(items) == 0 and rnd.random() < 0.5:
                return "*", set(range(lo, top + 1))
            kind = "step"
        if kind == "step":
            s = rnd.randint(2, max(2, (top - lo) // 2))
            items.append("*/%d" % s)
            vals.update(range(lo, top + 1, s))
        elif kind == "value":
            v = rnd.randint(lo, top)
            if names and rnd.random() < 0.5:
                items.append(names[v - (1 if not dow else 0)].upper() if rnd.random() < 0.5 else names[v - (1 if not dow else 0)])
            else:
                items.append(str(v) if not (dow and v == 0 and rnd.random() < 0.3) else "7")
            vals.add(v)
        else:
            a = rnd.randint(lo, top)
            b = rnd.randint(a, top)
            if kind == "range":
                items.append("%d-%d" % (a, b))
                vals.update(range(a, b + 1))
            else:
                s = rnd.randint(1, 5)
                items.append("%d-%d/%d" % (a, b, s))
                vals.update(range(a, b + 1, s))
    return ",".join(items), vals


def run(frm, n, text, mi, ho, dom, mo, dow, dom_r, dow_r):
    end = dt.date(frm.year + 10, 12, 31)
    day, res = frm.date(), []
    while day <= end and len(res) < n:
        if day.month in mo:
            a, b = day.day in dom, day.isoweekday() % 7 in dow
            if (a or b) if (dom_r and dow_r) else (a and b):
                for h in sorted(ho):
                    for m in sorted(mi):
                        t = dt.datetime(day.year, day.month, day.day, h, m)
                        if t > frm and len(res) < n:
                            res.append(t.strftime("%Y-%m-%d %H:%M"))
        day += dt.timedelta(days=1)
    return res


fixed = [  # sparse expressions that make a minute-by-minute search crawl
    ("0 0 29 2 *", 1000, {0}, {0}, {29}, {2}, set(range(7)), True, False),
    ("30 4 29 2 mon", 1000, {30}, {4}, {29}, {2}, {1}, True, True),
    ("59 23 31 12 *", 20, {59}, {23}, {31}, {12}, set(range(7)), True, False),
    ("0 12 31 4,6,9,11 *", 5, {0}, {12}, {31}, {4, 6, 9, 11}, set(range(7)), True, False),
]
invs, k = [], 0
for text, n, mi, ho, dom, mo, dow, dr, wr in fixed:
    frm = dt.datetime(rnd.randint(1970, 2090), rnd.randint(1, 12), rnd.randint(1, 28), rnd.randint(0, 23), rnd.randint(0, 59))
    invs.append((text, frm, n, run(frm, n, text, mi, ho, dom, mo, dow, dr, wr)))
while len(invs) < 36:
    f = [field(0, 59), field(0, 23), field(1, 31), field(1, 12, MONTHS), field(0, 7, DOWS, dow=True)]
    text = " ".join(x[0] for x in f)
    frm = dt.datetime(rnd.randint(1970, 2090), rnd.randint(1, 12), rnd.randint(1, 28), rnd.randint(0, 23), rnd.randint(0, 59))
    n = rnd.choice([1, 5, 5, 20, 100, 1000])
    dow_vals = {v % 7 for v in f[4][1]}
    invs.append((text, frm, n, run(frm, n, text, f[0][1], f[1][1], f[2][1], f[3][1], dow_vals, f[2][0] != "*", f[4][0] != "*")))

invocations = []
for i, (text, frm, n, res) in enumerate(invs):
    name = "r%d.out" % i
    open(os.path.join(out, name), "w").write("".join(r + "\n" for r in res))
    invocations.append({"args": ["--from", frm.strftime("%Y-%m-%dT%H:%M"), "-n", str(n), text], "stdout": name, "exit_code": 0 if res else 1})
json.dump([{"name": "36 expressions", "invocations": invocations}], open(os.path.join(out, "plan.json"), "w"))
