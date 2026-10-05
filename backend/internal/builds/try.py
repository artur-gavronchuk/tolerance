#!/usr/bin/env python3
"""Runs a site through a challenge's hidden tests in the arena-site sandbox image, the way the worker does,
and prints what passed. For authoring challenges; needs Docker and `make images`.

    python3 backend/internal/builds/try.py <slug> <site-dir | page.html> [scenario-name-substring]

The tests come from $ARENA_BUILDS_TESTS_DIR/<slug>/tests.json (the private catalog, by default
../arena-tasks/builds next to this repository), or catalog/<slug>/scenarios.json for retired challenges.
Exits 0 only when every selected scenario passed.
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile

here = os.path.dirname(os.path.abspath(__file__))
slug, site = sys.argv[1], sys.argv[2]
only = sys.argv[3] if len(sys.argv) > 3 else ""

repo = os.path.dirname(os.path.dirname(os.path.dirname(here)))
challenge = os.path.join(here, "catalog", slug)
tests_dir = os.environ.get("ARENA_BUILDS_TESTS_DIR") or os.path.join(os.path.dirname(repo), "arena-tasks", "builds")
tests = os.path.join(tests_dir, slug, "tests.json")
if not os.path.exists(tests):
    tests = os.path.join(challenge, "scenarios.json")
scenarios = json.load(open(tests))
if only:
    scenarios = [s for s in scenarios if only in s["name"]]
manifest = json.load(open(os.path.join(challenge, "manifest.json")))

# Under the repo, so Docker Desktop / Colima can mount it.
work = tempfile.mkdtemp(prefix=".try-", dir=repo)
try:
    sol = os.path.join(work, "solution")
    if os.path.isdir(site):
        shutil.copytree(site, sol)
    else:
        os.makedirs(sol)
        shutil.copy(site, os.path.join(sol, "index.html"))
    os.makedirs(os.path.join(work, "_run"))
    marker = "@@TRY@@"
    spec = {"scenarios": scenarios, "marker": marker}
    if manifest.get("format") == "mobile":  # mirrors worker.go
        spec["viewport"] = 390
        spec["shot"] = {"width": 390, "height": 844, "scale": 2, "touch": True}
    json.dump(spec, open(os.path.join(work, "_run", "spec.json"), "w"))
    shutil.copy(os.path.join(here, "runner.py"), os.path.join(work, "_run", "run.py"))
    out = subprocess.run(["docker", "run", "--rm", "--network", "none", "-v", work + ":/work", "-w", "/work", "arena-site:1",
                          "python3", "_run/run.py"], capture_output=True, text=True, timeout=manifest.get("timeout_s", 300) + 60)
    report = None
    for line in out.stdout.splitlines():
        if line.startswith(marker):
            report = json.loads(line[len(marker):])
        else:
            print(line)
    if report is None:
        print(out.stderr[-3000:])
        sys.exit("no verdict")
    passed = sum(r["passed"] for r in report["results"])
    print("\nscenarios %d/%d" % (passed, len(report["results"])))
    for r in report["results"]:
        if not r["passed"]:
            print("  FAIL", r["name"])
    failed = passed != len(report["results"])
    print("quality", json.dumps(report["quality"]))
    if os.path.exists(os.path.join(work, "_out", "shot.jpg")):
        dst = os.path.join(here, ".try-shot-%s.jpg" % slug)
        shutil.copy(os.path.join(work, "_out", "shot.jpg"), dst)
        print("screenshot", os.path.normpath(dst))
finally:
    shutil.rmtree(work, ignore_errors=True)
sys.exit(1 if failed else 0)
