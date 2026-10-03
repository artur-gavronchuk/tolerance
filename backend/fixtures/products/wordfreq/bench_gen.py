# Benchmark generator (platform code, runs in the sandbox): python3 bench_gen.py OUTDIR writes plan.json plus the
# input and expected-output files. Deterministic: the same files every run.
import json
import os
import random
import re
import sys
from collections import Counter

out = sys.argv[1]
rnd = random.Random(20261003)
vocab = []
for i in range(30000):
    n = rnd.randint(2, 9)
    w = "".join(rnd.choice("abcdefghijklmnopqrstuvwxyz") for _ in range(n))
    if i % 7 == 0:
        w += "'" + rnd.choice(["s", "t", "re"])
    if i % 11 == 0:
        w += str(rnd.randint(0, 99))
    vocab.append(w)
weights = [1.0 / (i + 1) for i in range(len(vocab))]
picks = rnd.choices(vocab, weights, k=2400000)
seps = [" ", " ", " ", ", ", ". ", "\n", " - ", "! ", "; ", "\t"]
parts = []
for w in picks:
    r = rnd.random()
    parts.append(w.upper() if r < 0.05 else w.capitalize() if r < 0.25 else w)
    parts.append(rnd.choice(seps))
text = "".join(parts) + "\n"
open(os.path.join(out, "words.in"), "w").write(text)

counts = Counter(re.findall(r"[a-z0-9']+", text.lower()))
ranked = sorted(counts.items(), key=lambda kv: (-kv[1], kv[0]))[:25]
open(os.path.join(out, "words.out"), "w").write("".join("%s %d\n" % kv for kv in ranked))
plan = [{"name": "large text", "invocations": [{"args": ["--top", "25"], "stdin": "words.in", "stdout": "words.out", "exit_code": 0}]}]
json.dump(plan, open(os.path.join(out, "plan.json"), "w"))
