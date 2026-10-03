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
