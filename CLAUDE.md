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
  only what you can't see by clicking: submission verdicts, diff-apply safety
  and zip-upload handling (`internal/submissions`), pytest output parsing
  (`internal/sandbox`), the task catalog (`internal/tasks`), the job queue
  (`internal/platform/jobs`). Add a test only for a bug found in one of
  those. If a change breaks one of them, the change is probably wrong.
  `*_test.go` under `backend/fixtures/**` are task content (hidden tests), not
  our tests — never delete them.
- **"Done" = it compiles and works locally**: `make test-fast`, then `make dev`
  (or `make up`) and the flow clicked through. Full `make test` is optional.
  CI (`ci.yml`) is manual-only (`workflow_dispatch`); nothing runs on push.
- **Throwing code away is fine.** When the concept changes, delete the old
  feature (code, routes, pages, tests) rather than keeping it compatible.
- **No ops work** unless asked. Deploy is one button: Actions → deploy → Run
  workflow (`gh workflow run deploy`; `ref` = older sha to roll back). It
  builds the images into GHCR and runs `deploy/server.sh` on the host over SSH
  (secrets in the `production` environment). `deploy/compose.yml` and
  `deploy/Caddyfile` are the server's source of truth — the Caddyfile also
  serves `mentor.nualimov.dev`, keep that block. Migrations run on every
  deploy, so once a migration has shipped to tolerance.cc, don't edit it.
  The old canary/monitoring stack is at git tag `ops-snapshot-2026-10-03`.
- **Docs**: don't update `README.md` or `docs/` specs. Update this file only
  with what would trip up the next session.
- **Git**: commit to `main` directly (`git pull --rebase` first — other
  sessions push too), no PRs needed.

## What this is

**tolerance** (site https://tolerance.cc; the CLI is still called `arena`, env
vars are `ARENA_*`, the database is `arena`). Since 2026-10-03 the product is
upload-based — the platform never talks to users' agents. Two modes:

1. **Task of the day** (built): one coding task per UTC day. The user downloads
   the repo, has their own agent fix it, uploads a zip or a patch; the platform
   runs the hidden tests in the Docker sandbox. Daily/overall leaderboards,
   streaks, 3 attempts a day.
2. **Tanks** (`internal/games`): users download a starter kit zip, have their
   agent improve it, upload a zip; check match, ladder, monthly seasons
   (per-season ratings), weekly single-elimination tournaments, plain-text
   match reports for pasting back to the agent.

Also: `/u/[handle]` (+ `internal/profiles` for tanks bots and the main `made_with`
stack; `made_with` itself is a free-text field on submissions, normalized for
display in `internal/profiles/stack.go` and `components/daily/stack-label.tsx`),
`/admin` (`internal/admin`, pulse; admin = `identity.CanAdmin`: admin role, or
anyone when `ARENA_DEV_LOGIN=true` — the same rule gates "start a tournament now").
Products ("product of the week") and the `/agents` stacks page were removed on
2026-10-03; they live in the commit history.

**Build challenges** (`internal/builds`, since 2026-10-05 the whole visible site; daily and tanks are hidden
behind redirects in `frontend/next.config.mjs`): the public half is `catalog/<slug>/` (manifest, contract
en/ru) and `catalog/season.json` (order, dates); hidden tests, references and broken versions live in the
private `arena-tasks/builds/<slug>/`. Locally set `ARENA_BUILDS_TESTS_SOURCE=<arena-tasks>/builds` in `.env`;
check a challenge with `python3 <arena-tasks>/builds/verify.py <slug> --tolerance <this repo>`.

This repository is public: `backend/fixtures/*` tasks are practice tasks. The
real hidden tasks live in the private repo `artur-gavronchuk/arena-tasks`.
Never copy those tasks here.

Monorepo: `backend/` (Go module `tolerance`) and `frontend/` (Next.js, talks
to the backend over HTTP only). The code is the only description of the
current design; `docs/superpowers/specs/…-platform-roadmap.md` is a history of
ideas, older specs/plans live in git history.

## Commands

```sh
make dev         # postgres in Docker + migrate, then api and web natively (Ctrl-C stops both)
make up          # whole stack in Docker: postgres, migrate, api (HTTP + all workers), web
make down        # make reset also wipes the DB volume
make migrate     # goose migrations + catalog sync
make seed        # fake 30 days of activity (500 people; --users N via go run ./cmd/seed --yes); local DB only, re-run replaces it
make run-api     # native API alone;  ARENA_SANDBOX=fake runs without Docker
make run-web     # pnpm dev, proxying /api to the native API
make images      # sandbox + bot runtime images, built only when missing
make test-fast   # go vet + go test + pnpm typecheck
make test        # everything: -race, ARENA_TEST_REQUIRE_DOCKER=1, frontend build

cd backend && go test ./internal/submissions/...   # one package
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
backend/cmd/api              config, handler.go (ALL routing), main (server + workers)
backend/cmd/migrate          goose up + task catalog sync + tanks house bots
backend/cmd/arena            CLI: tanks new | tanks play (local only)
backend/internal/tasks       task catalog (fixtures → tasks table), repo.zip
backend/internal/daily       today's task (assigned lazily), days archive, leaderboards, streaks
backend/internal/submissions upload (zip → diff via git diff --no-index, or patch), worker, verdict
backend/internal/sandbox     hidden tests in Docker (or fake)
backend/internal/games       tanks: bots, versions, check, ladder, match runner, rating
backend/internal/identity    OAuth (GitHub/Google), dev login, sessions, RequireSession
backend/internal/platform    db, dbtest, httpx, jobs queue, limits, sanitize, …
backend/fixtures             practice tasks (`_hidden/` = hidden tests — task content, never delete)
backend/internal/profiles    /users/{handle}/activity, made_with normalizer
backend/internal/admin       /admin/pulse, /admin/recent
frontend/app                 / (today), /day/[day], /days, /leaderboard, /u/*, /admin, /tanks/*, /app/tanks
frontend/lib                 types.ts (API types), api.ts (fetching)
```

Session cookie routes: `/api/v1/me`, `/submissions*`, `/me/tanks*`, `/admin/*`. Everything
else (`/daily*`, `/days`, `/leaderboard`, `/tasks/{slug}/repo.zip`,
`/tanks/*`, `/connector/download`) is public.

Submission states: `queued → running → passed | failed | infra_error`. The
worker runs inside `cmd/api` on the `jobs` queue (`run_submission`); a
submission stuck > 15 min becomes `infra_error`. `infra_error` never uses up
an attempt.

## Rules that stay even in prototype mode

- **Verdict integrity**: `passed` requires every hidden test, by name, to have
  run and passed. `infra_error` is the platform's fault and never counts
  against the user. A diff touching test files or the test harness fails.
- **Sandbox safety**: the worker applies diffs with plain `git apply` (no
  `--unsafe-paths`); the sandbox runs with `--network none` and dropped caps.
- **Migrations**: tolerance.cc runs them on every deploy, so a migration that
  has been deployed is append-only — add a new file. One that hasn't shipped
  yet may still be edited; run `make reset` afterwards (goose won't re-run an
  applied file) and say in the commit message that other sessions need it too.
- Agent-written logs are sanitized server-side (`internal/platform/sanitize`).
- Errors go through `httpx.WriteError` (`{code, message, request_id}`);
  timestamps are UTC (`.UTC()` after scanning — pgx returns local time).
- Frontend: no mocks, data from the API, browser calls same-origin `/api/v1/*`.
- UI copy is bilingual (en, ru): strings live in `frontend/lib/i18n/messages/<area>.ts`
  (`defineMessages`, ru must have every en key), read with `useT(m)` in client
  components or `await getT(m)` on the server; dates via `formatDate`/
  `formatDateTime` from `lib/i18n/core`. Language = `lang` cookie, else
  Accept-Language, else en. Task content (TASK.md, GAME.md) stays English.
- Colours are tokens from `frontend/app/globals.css`, never raw hex in classes:
  `bg-terminal`/`text-terminal-foreground` for copy blocks and dark panels (navy
  in both themes), `bg-arena` for the match viewer, `border-input` for control
  borders (3:1), `border-strong` for dashed empty states.
- Commit subjects in English, imperative, sentence case, no `feat:` prefixes.
  Docs in `docs/` are in Russian, code and comments in English.
