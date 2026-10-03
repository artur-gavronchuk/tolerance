# Scenario runner, executed inside the sandbox next to the participant's code. It is trusted platform code:
# the participant's process only ever gets stdin/args, and its stdout is captured here, never forwarded.
import json
import shlex
import subprocess

spec = json.load(open("_scenarios/spec.json"))
cmd = shlex.split(spec["command"])
results = []
for sc in spec["scenarios"]:
    try:
        p = subprocess.run(cmd + sc.get("args", []), input=sc.get("stdin", ""), capture_output=True,
                           text=True, timeout=10, cwd="solution")
        ok = p.returncode == sc.get("exit_code", 0) and p.stdout.rstrip() == sc.get("stdout", "").rstrip()
    except Exception:
        ok = False
    results.append({"name": sc["name"], "passed": ok})
print("@@RESULTS@@" + json.dumps(results))


# Benchmark: the task ships a generator (_scenarios/bench_gen.py OUTDIR) that writes plan.json and the input/expected
# files. Every invocation's output must be right, otherwise there is no bench score. The score is the sum, over the
# plan's cases, of the trimmed median wall time (ms) of one pass over the case's invocations: the slowest and the fastest
# sample are dropped (when there are at least 3) and the median of the rest taken. The spread is half the range of what is left,
# summed the same way, so the score reads "ms ± spread".
import os
import statistics
import sys
import tempfile
import time

BENCH_BUDGET_S = 30.0  # the whole benchmark; later samples are skipped once it is spent
BENCH_RUN_TIMEOUT_S = 10


def bench(spec, cmd):
    runs = int(spec.get("runs", 5))
    out = tempfile.mkdtemp(prefix="bench-")
    subprocess.run([sys.executable, "_scenarios/bench_gen.py", out], check=True, timeout=60)
    plan = json.load(open(os.path.join(out, "plan.json")))
    deadline = time.monotonic() + BENCH_BUDGET_S
    total = spread = 0.0
    for case in plan:
        samples = []
        for i in range(runs + 1):  # the first pass is a warm-up and the correctness check
            took = 0.0
            for inv in case["invocations"]:
                want = open(os.path.join(out, inv["stdout"]), "rb").read() if inv.get("stdout") else b""
                with open(os.path.join(out, inv["stdin"]), "rb") if inv.get("stdin") else open(os.devnull, "rb") as fin:
                    t0 = time.perf_counter()
                    try:
                        p = subprocess.run(cmd + inv.get("args", []), stdin=fin, capture_output=True,
                                           timeout=BENCH_RUN_TIMEOUT_S, cwd="solution")
                    except subprocess.TimeoutExpired:
                        return None, "case %s: timed out" % case["name"]
                    took += time.perf_counter() - t0
                if p.returncode != inv.get("exit_code", 0) or p.stdout.rstrip() != want.rstrip():
                    return None, "case %s: wrong output" % case["name"]
            if i > 0:
                samples.append(took * 1000)
            if time.monotonic() > deadline and (i > 0 or took > BENCH_BUDGET_S / 2):
                break
        if not samples:
            return None, "case %s: too slow to measure" % case["name"]
        samples.sort()
        if len(samples) >= 3:
            samples = samples[1:-1]
        total += statistics.median(samples)
        spread += (samples[-1] - samples[0]) / 2
    return (total, spread), ""


if spec.get("bench"):
    try:
        score, err = bench(spec["bench"], cmd)
    except Exception as e:
        score, err = None, "benchmark could not run: %s" % e
    if err:
        print("bench: " + err)
    print("@@BENCH@@" + json.dumps({"ms": score[0] if score else None, "spread_ms": score[1] if score else None}))
