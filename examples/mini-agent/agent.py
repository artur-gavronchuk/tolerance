#!/usr/bin/env python3
"""A minimal coding agent: a model, four tools, and a loop.

Usage:
    python3 agent.py "<task>"     # task given on the command line
    python3 agent.py              # task read from TASK.md in the working directory

The working directory is wherever you run this from (the connector runs it
inside the task folder it prepared, e.g. `agent.command: python /path/agent.py`).
Every tool call is confined to that directory: a path that resolves outside
it — including through a symlink — is rejected, not sandboxed by anything
external. This script has no other safety net, so only run it somewhere
you're fine having a model write files and shell commands.

Model: $AGENT_MODEL, or claude-sonnet-5 if unset. Needs ANTHROPIC_API_KEY.
"""

import json
import os
import subprocess
import sys
from pathlib import Path

DEFAULT_MODEL = "claude-sonnet-5"
MODEL = os.environ.get("AGENT_MODEL", DEFAULT_MODEL)
MAX_STEPS = 40
MAX_RUN_TIMEOUT_S = 120
OUTPUT_CAP_BYTES = 8 * 1024

WORK_DIR = Path.cwd().resolve()

SYSTEM_PROMPT = (
    "You are a coding agent working alone in a local project directory. "
    "Use list_files, read_file and write_file to inspect and edit the project, "
    "and run to execute commands in it — tests, a linter, or the project's own "
    "checks. Everything you do is confined to the working directory; you cannot "
    "read or write anywhere else. Prefer verifying your work by running it over "
    "guessing whether it works. When you are done, stop calling tools and briefly "
    "summarize what you changed."
)


def safe_path(rel: str) -> Path:
    """Resolve rel against the working directory and reject anything that
    escapes it, including via a symlink (resolve() follows symlinks, so a
    link pointing outside is caught by the containment check below)."""
    candidate = (WORK_DIR / rel).resolve()
    if candidate != WORK_DIR and WORK_DIR not in candidate.parents:
        raise ValueError(f"path escapes the working directory: {rel}")
    return candidate


def cap_output(text: str) -> str:
    data = text.encode("utf-8", errors="replace")
    if len(data) <= OUTPUT_CAP_BYTES:
        return text
    return data[:OUTPUT_CAP_BYTES].decode("utf-8", errors="ignore") + "\n... (truncated)"


def tool_list_files(path: str = ".") -> str:
    root = safe_path(path)
    if not root.exists():
        return f"error: {path} does not exist"
    if root.is_file():
        return str(root.relative_to(WORK_DIR))
    names = sorted(
        str(p.relative_to(WORK_DIR)) + ("/" if p.is_dir() else "")
        for p in root.rglob("*")
        if ".git" not in p.parts
    )
    return "\n".join(names) if names else "(empty)"


def tool_read_file(path: str) -> str:
    target = safe_path(path)
    return target.read_text(encoding="utf-8", errors="replace")


def tool_write_file(path: str, content: str) -> str:
    target = safe_path(path)
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(content, encoding="utf-8")
    return f"wrote {len(content.encode('utf-8'))} bytes to {path}"


def tool_run(command: str, timeout_s: int = 30) -> str:
    timeout_s = max(1, min(int(timeout_s), MAX_RUN_TIMEOUT_S))
    try:
        proc = subprocess.run(
            command,
            shell=True,
            cwd=WORK_DIR,
            capture_output=True,
            text=True,
            timeout=timeout_s,
        )
    except subprocess.TimeoutExpired:
        return f"error: command timed out after {timeout_s}s"
    output = cap_output((proc.stdout or "") + (proc.stderr or ""))
    return f"exit code: {proc.returncode}\n{output}"


TOOLS = [
    {
        "name": "list_files",
        "description": "List files and directories under a path inside the working directory, recursively.",
        "input_schema": {
            "type": "object",
            "properties": {"path": {"type": "string", "description": 'Path relative to the working directory. Defaults to ".".'}},
        },
    },
    {
        "name": "read_file",
        "description": "Read a text file's contents.",
        "input_schema": {
            "type": "object",
            "properties": {"path": {"type": "string", "description": "Path relative to the working directory."}},
            "required": ["path"],
        },
    },
    {
        "name": "write_file",
        "description": "Write (overwrite) a text file, creating parent directories if needed.",
        "input_schema": {
            "type": "object",
            "properties": {
                "path": {"type": "string", "description": "Path relative to the working directory."},
                "content": {"type": "string", "description": "Full new contents of the file."},
            },
            "required": ["path", "content"],
        },
    },
    {
        "name": "run",
        "description": "Run a shell command in the working directory and return its exit code and output.",
        "input_schema": {
            "type": "object",
            "properties": {
                "command": {"type": "string", "description": "Shell command to run."},
                "timeout_s": {"type": "integer", "description": "Timeout in seconds, capped at 120. Defaults to 30."},
            },
            "required": ["command"],
        },
    },
]

HANDLERS = {
    "list_files": tool_list_files,
    "read_file": tool_read_file,
    "write_file": tool_write_file,
    "run": tool_run,
}


def read_task() -> str:
    if len(sys.argv) > 1:
        return sys.argv[1]
    task_file = WORK_DIR / "TASK.md"
    if task_file.exists():
        return task_file.read_text(encoding="utf-8")
    sys.exit('Usage: agent.py "<task>"  (or put the task in TASK.md)')


def main() -> None:
    import anthropic  # imported here so the tool functions above stay testable without the SDK installed

    task = read_task()
    client = anthropic.Anthropic()
    messages = [{"role": "user", "content": task}]

    for step in range(1, MAX_STEPS + 1):
        response = client.messages.create(
            model=MODEL,
            max_tokens=4096,
            system=SYSTEM_PROMPT,
            tools=TOOLS,
            messages=messages,
        )
        messages.append({"role": "assistant", "content": response.content})

        if response.stop_reason != "tool_use":
            for block in response.content:
                if block.type == "text":
                    print(block.text)
            if response.stop_reason != "end_turn":
                print(f"(stopped: {response.stop_reason})", file=sys.stderr)
            return

        tool_results = []
        for block in response.content:
            if block.type != "tool_use":
                continue
            print(f"[step {step}] {block.name}({json.dumps(block.input)})")
            try:
                result = HANDLERS[block.name](**block.input)
                is_error = False
            except Exception as e:  # a bad path, a bad arg, a failed command — report it, don't crash the loop
                result = f"error: {e}"
                is_error = True
            tool_results.append(
                {"type": "tool_result", "tool_use_id": block.id, "content": str(result), "is_error": is_error}
            )
        messages.append({"role": "user", "content": tool_results})

    print(f"stopped after {MAX_STEPS} steps without finishing", file=sys.stderr)


if __name__ == "__main__":
    main()
