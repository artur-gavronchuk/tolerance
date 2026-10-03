# Benchmark generator (platform code, runs in the sandbox): python3 bench_gen.py OUTDIR writes plan.json plus the
# input and expected-output files. Deterministic: the same files every run.
import json
import os
import random
import sys

out = sys.argv[1]
rnd = random.Random(20261003)
WORDS = ["alpha", "beta", "gamma", "delta", "kappa", "omega", "id", "name", "tags", "meta", "items", "value", "ß", "é", "日本", "😀"]
ESC = {'"': '\\"', "\\": "\\\\", "\n": "\\n", "\t": "\\t"}


class Num:
    def __init__(self, text):
        self.text = text


class Raw:  # a string or key: decoded text and the way it is written in the input
    def __init__(self, decoded, written):
        self.decoded, self.written = decoded, written


def rand_string():
    s = "".join(rnd.choice(WORDS) + rnd.choice([" ", "-", "_", ""]) for _ in range(rnd.randint(1, 4)))
    if rnd.random() < 0.15:
        s += rnd.choice(['"', "\\", "\n", "\t"])
    written = "".join(ESC.get(c, c) for c in s)
    if rnd.random() < 0.1:
        written = written.replace("é", "\\u00e9")
        written = written.replace("/", "\\/")
    return Raw(json.loads('"%s"' % written), '"%s"' % written)


def rand_num():
    r = rnd.random()
    if r < 0.4:
        return Num(str(rnd.randint(-10**6, 10**6)))
    if r < 0.7:
        return Num("%d.%02d" % (rnd.randint(0, 9999), rnd.randint(0, 99)))
    if r < 0.85:
        return Num("%dE%d" % (rnd.randint(1, 99), rnd.randint(-20, 20)))
    return Num(str(rnd.randint(10**18, 10**24)))


def gen(depth):
    r = rnd.random()
    if depth >= 7 or r < 0.55:
        c = rnd.random()
        if c < 0.4:
            return rand_num()
        if c < 0.8:
            return rand_string()
        return rnd.choice([True, False, None])
    if r < 0.8:
        return [gen(depth + 1) for _ in range(rnd.randint(0, 6))]
    keys, seen, members = [], set(), []
    for _ in range(rnd.randint(0, 6)):
        k = rand_string()
        if k.decoded in seen:
            continue
        seen.add(k.decoded)
        members.append((k, gen(depth + 1)))
    return {"members": members}


def ws(rich):
    return rnd.choice(["", " ", "\n", "\t", "  \r\n"]) if rich else ""


def ser(v, indent=None, sort=False, rich=False, level=0):
    if isinstance(v, Num):
        return v.text
    if isinstance(v, Raw):
        return v.written
    if v is True or v is False or v is None:
        return {True: "true", False: "false", None: "null"}[v]
    if isinstance(v, list):
        items = [ser(x, indent, sort, rich, level + 1) for x in v]
        sep_open, sep_close = "[", "]"
    else:
        members = v["members"]
        if sort:
            members = sorted(members, key=lambda m: m[0].decoded)
        items = [m[0].written + (": " if indent else ":") + ser(m[1], indent, sort, rich, level + 1) for m in members]
        sep_open, sep_close = "{", "}"
    if not items:
        return sep_open + sep_close
    if indent:
        pad = "\n" + " " * (indent * (level + 1))
        return sep_open + pad + ("," + pad).join(items) + "\n" + " " * (indent * level) + sep_close
    return sep_open + ",".join(items) + sep_close


def messy(v):  # the same value with arbitrary whitespace between tokens
    if isinstance(v, list):
        return "[" + ws(1) + ("," + ws(1)).join(ws(1) + messy(x) + ws(1) for x in v) + "]" if v else "[" + ws(1) + "]"
    if isinstance(v, dict):
        return "{" + ws(1) + ("," + ws(1)).join(ws(1) + m[0].written + ws(1) + ":" + ws(1) + messy(m[1]) + ws(1) for m in v["members"]) + "}" if v["members"] else "{" + ws(1) + "}"
    return ser(v)


doc = {"members": [(Raw("rows", '"rows"'), [gen(1) for _ in range(9000)])]}
text = messy(doc) + "\n"
open(os.path.join(out, "doc.in"), "w", encoding="utf-8").write(text)
open(os.path.join(out, "pretty.out"), "w", encoding="utf-8").write(ser(doc, indent=2, sort=True) + "\n")
open(os.path.join(out, "compact.out"), "w", encoding="utf-8").write(ser(doc) + "\n")
plan = [{"name": "pretty and sorted", "invocations": [{"args": ["--sort-keys"], "stdin": "doc.in", "stdout": "pretty.out", "exit_code": 0}]},
        {"name": "compact", "invocations": [{"args": ["--compact"], "stdin": "doc.in", "stdout": "compact.out", "exit_code": 0}]}]
json.dump(plan, open(os.path.join(out, "plan.json"), "w"))
