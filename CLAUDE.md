# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Prototype mode (since 2026-10-03) — read first

The product concept is still changing week to week (tanks, skills, challenges
may all be redone). The goal is a working skeleton, fast. This section
overrides the superpowers skills and any default habits:

- **Write code directly.** No brainstorming sessions, no specs or task-by-task
  plans in `docs/`, no subagent-driven-development, no TDD, no per-task or
  whole-branch reviews — unless the user asks for one. If a change needs a
  plan, a short list of steps in chat is enough.
- **No new tests.** Most tests were deleted on 2026-10-03; what's left guards
  only what you can't see by clicking: proof verdicts and diff-apply safety
  (`internal/proofs/*_test.go`), pytest output parsing
  (`internal/proofs/sandbox`), rating math (`internal/skillrating`), the job
  queue (`internal/platform/jobs`). Add a test only for a bug found in one of
  those. If a change breaks one of them, the change is probably wrong.
  `*_test.go` under `backend/fixtures/**` are task content (hidden tests), not
  our tests — never delete them.
- **"Done" = it compiles and works locally**: `make test-fast`, then `make dev`
  (or `make up`) and the flow clicked through. Full `make test` is optional.
  CI (`ci.yml`) is manual-only (`workflow_dispatch`); nothing runs on push.
- **Throwing code away is fine.** When the concept changes, delete the old
  feature (code, routes, pages, tests) rather than keeping it compatible.
- **No ops work** unless asked. The whole production stack (Caddy, canary
  releases, monitoring, backups, `deploy/`, `deploy.yml`) was cut from `main`;
  it lives at git tag `ops-snapshot-2026-10-03` — restore from there when the
  product is ready to ship. tolerance.cc keeps running the last deployed build.
- **Docs**: don't update `README.md` or `docs/` specs. Update this file only
  with what would trip up the next session.
- **Git**: commit to `main` directly (`git pull --rebase` first — other
  sessions push too), no PRs needed.

## What this is

**tolerance** (site https://tolerance.cc; formerly Agent Arena — the connector
command, `~/.arena`, the `ARENA_` env prefix and the `arena` database keep that
name). An agent owner signs up, creates an agent, runs the `arena` connector on
their machine; the platform hands it a task, the agent solves it locally, the
connector returns a diff, and the platform replays the diff against hidden
tests in a Docker sandbox. Model keys and agent code never leave the owner's
machine.

This repository is public: `backend/fixtures/*` tasks are practice tasks. The
real hidden rating tasks live in the private repo `artur-gavronchuk/arena-tasks`.
Never copy those tasks here.

Monorepo: `backend/` (Go API + connector CLI, module `tolerance`) and
`frontend/` (Next.js, talks to the backend over HTTP only). The code is the
only description of the current design; `docs/superpowers/specs/…-platform-roadmap.md`
is a history of ideas, and older specs/plans live in git history.

## Commands

```sh
make dev         # postgres in Docker + migrate, then api and web natively (Ctrl-C stops both)
make up          # whole stack in Docker: postgres, migrate, api (HTTP + all workers), web
make down        # make reset also wipes the DB volume
make migrate     # goose migrations + catalog sync
make run-api     # native API alone;  ARENA_SANDBOX=fake runs without Docker
make run-web     # pnpm dev, proxying /api to the native API
make images      # sandbox + bot runtime images, built only when missing
make test-fast   # go vet + go test + pnpm typecheck
make test        # everything: -race, ARENA_TEST_REQUIRE_DOCKER=1, frontend build

cd backend && go test ./internal/proofs/...   # one package
```

Ports come from `.env` (`WEB_PORT`, `API_PORT`, `PG_PORT`; Superset's
`.superset/setup.sh` assigns free ones per workspace). Frontend uses pnpm.
Sign in locally through the dev login (`ARENA_DEV_LOGIN=true`).
`ARENA_NO_LIMITS=true` (the local default) turns off daily/hourly quotas via
`internal/platform/limits`; route any new quota through `limits.Cap`.

Integration tests (`*_integration_test.go`) use testcontainers Postgres and
skip without Docker. With Colima, if testcontainers can't find the socket:
`export DOCKER_HOST=unix://$HOME/.colima/default/docker.sock TESTCONTAINERS_RYUK_DISABLED=true`.
`internal/games/match` tests need `python3` and `node` on `PATH`.

There is no API contract file: the Go handlers and `frontend/lib/types.ts`
are the contract.

## Map

```
backend/cmd/api            config, handler.go (ALL routing), main (server + workers), e2e test
backend/cmd/migrate        goose up + catalog sync (proofs, skills, tanks house bots)
backend/cmd/arena          connector CLI: login, init, connect, status, tanks …
backend/internal/identity  OAuth (GitHub/Google), dev login, sessions, RequireSession/RequireAgent/RequireAdmin
backend/internal/agents    agents, API keys, presence, derived stage
backend/internal/proofs    proof lifecycle, worker, sandbox/ (docker | fake)
backend/internal/games     tanks: bots, versions, check, ladder, match runner, rating
backend/internal/skills, skillrating, qualifications, arena, challenges, admin
backend/internal/platform  db, dbtest, httpx, jobs queue, auth, sanitize, …
backend/fixtures           practice proof and skill tasks (`_hidden/` = hidden tests)
frontend/app, frontend/lib types.ts (API types), api.ts (fetching)
```

Owner routes use a session cookie (`RequireSession`), connector routes
`/api/v1/connector/*` use `Authorization: Bearer <api key>` (`RequireAgent`),
public routes (leaderboard, agent profiles, challenges, `/tanks/*`) need
neither.

Proof states: `queued → claimed → running_agent → diff_submitted →
running_sandbox → passed | failed | infra_error | expired`. The worker runs
inside `cmd/api` and dispatches on `proofs.kind` (`proof | game_bot |
qualification | challenge`). Agent stage is derived, never stored.

## Rules that stay even in prototype mode

- **Verdict integrity**: `passed` requires every hidden test, by name, to have
  run and passed. `infra_error` is the platform's fault and never counts
  against the agent. A diff touching `*_test.go` fails.
- **Sandbox safety**: the worker applies diffs with plain `git apply` (no
  `--unsafe-paths`); the sandbox runs with `--network none` and dropped caps.
- **Migrations**: local data is disposable and the frozen production database
  will be recreated when the product ships, so editing an existing migration
  is fine — run `make reset` afterwards (goose won't re-run an applied file).
  Other sessions' databases need the same reset; say so in the commit message.
- Agent-written logs are sanitized server-side (`internal/platform/sanitize`).
- Errors go through `httpx.WriteError` (`{code, message, request_id}`);
  timestamps are UTC (`.UTC()` after scanning — pgx returns local time).
- Frontend: no mocks, data from the API, browser calls same-origin `/api/v1/*`.
- Commit subjects in English, imperative, sentence case, no `feat:` prefixes.
  Docs in `docs/` are in Russian, code and comments in English.
