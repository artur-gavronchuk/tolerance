#!/usr/bin/env python3
"""Runs a site through a challenge's scenarios in the arena-site sandbox image, the way the worker does, and
prints what passed. For authoring challenges; needs Docker and `make images`.

    python3 backend/internal/builds/try.py backend/internal/builds/catalog/snake backend/internal/builds/catalog/snake/_reference
    python3 backend/internal/builds/try.py <challenge-dir> <site-dir | page.html> [scenario-name-substring]
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile

here = os.path.dirname(os.path.abspath(__file__))
challenge, site = sys.argv[1], sys.argv[2]
only = sys.argv[3] if len(sys.argv) > 3 else ""

scenarios = json.load(open(os.path.join(challenge, "scenarios.json")))
if only:
    scenarios = [s for s in scenarios if only in s["name"]]
manifest = json.load(open(os.path.join(challenge, "manifest.json")))

# Under the repo, so Docker Desktop / Colima can mount it.
work = tempfile.mkdtemp(prefix=".try-", dir=os.path.dirname(os.path.dirname(os.path.dirname(here))))
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
    if manifest.get("mobile"):  # mirrors worker.go
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
    print("quality", json.dumps(report["quality"]))
    if os.path.exists(os.path.join(work, "_out", "shot.jpg")):
        dst = os.path.join(challenge, "..", "..", ".try-shot-%s.jpg" % os.path.basename(os.path.normpath(challenge)))
        shutil.copy(os.path.join(work, "_out", "shot.jpg"), dst)
        print("screenshot", os.path.normpath(dst))
finally:
    shutil.rmtree(work, ignore_errors=True)
