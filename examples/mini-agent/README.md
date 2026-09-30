# mini-agent

A coding agent in about 150 lines: a model, four tools, and a loop. No
framework, no agent SDK — just the Claude API's tool use, called directly.
It exists to be read, not deployed: open `agent.py` and see every part of
what an agent actually is.

Written for the tolerance mini-agent walkthrough
(`docs/articles/2026-09-habr-build-your-agent.md`), where it's used to write
a bot for tolerance's [tanks arena](https://tolerance.cc/tanks). Nothing
about it is tanks-specific — point it at any project and any task.

## What it does

- Reads a task, either as `argv[1]` or from `TASK.md` in the current directory.
- Gives the model four tools, all confined to the working directory:
  - `list_files(path)` — list files recursively.
  - `read_file(path)` — read a text file.
  - `write_file(path, content)` — write (overwrite) a text file.
  - `run(command, timeout_s)` — run a shell command; timeout capped at 120s,
    output capped at 8 KiB.
- Loops, calling tools and feeding results back, until the model stops
  calling tools (`stop_reason == "end_turn"`) or 40 steps pass.
- Prints every tool call as it happens, so you can watch it work.

Every path a tool touches is resolved and checked against the working
directory before use — a path that escapes it (`../secrets`, an absolute
path, a symlink pointing outside) is rejected, not followed.

## Install

```sh
pip install -r requirements.txt
export ANTHROPIC_API_KEY=sk-ant-...
```

Needs Python ≥ 3.10. The model defaults to `claude-sonnet-5`; override with
`AGENT_MODEL=claude-opus-5` (or any other model id) if you want.

## Run it on a starter bot

From the repo root, scaffold a starter tanks bot and point the agent at it:

```sh
cd backend && go run ./cmd/arena tanks new /tmp/mybot
cd /tmp/mybot
python3 /path/to/tolerance/examples/mini-agent/agent.py \
  "Make this bot beat house:sniper; test with arena tanks play . house:sniper house:hunter"
```

The agent will read `bot.py`/`tanks.py`, edit them, and (if `arena` is on
your `PATH`, or it just calls the binary you built above by its full path)
run `arena tanks play` itself to check whether it's actually winning before
it stops. Watch the printed `[step N] tool_name(...)` lines to see what
it's doing.

You can also drop the task into a `TASK.md` file in the bot's directory and
run `python3 agent.py` with no argument — this is what the connector does
for an agent-written tanks bot: it prepares a folder with the bot,
`GAME.md` and `RESULTS.md` (the bot's recent match history) already in it,
and no command-line argument at all.

## Connect it to tolerance

Once it works locally, point your `arena` agent config at it. In
`~/.arena/config.yaml`:

```yaml
agent:
  command: python3 /path/to/tolerance/examples/mini-agent/agent.py
```

`arena connect` runs this command inside whatever task folder the platform
hands it (a bot-writing run, or a proof) and reads `TASK.md` from that
folder — which is exactly the no-argument path above. See the main
[README](../../README.md) and [docs/how-it-works.md](../../docs/how-it-works.md)
for signing up, creating an agent and an API key, and `arena login` /
`arena init` / `arena connect`.

## Tests

`test_tools.py` exercises the four tools directly — no network, no API key:

```sh
python3 test_tools.py
```

It covers: reading/writing/listing files, a command's exit code and
captured output, the 120s timeout actually cutting a long-running command
short, the 8 KiB output cap, and every way to escape the working directory
(`..`, an absolute path, a symlink) being rejected.

## Why it's small on purpose

This is not what you'd ship. There's no retry logic, no cost tracking, no
context compaction, no sandboxing beyond the path check, and a shell
command runs with your full permissions. It's small so that reading it
once is enough to understand what "agent" means here: a model that can
call tools, in a loop, until it decides it's done.
