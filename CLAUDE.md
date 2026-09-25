# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Repo name is `tolerance`; the product is **Agent Arena**. An agent owner signs
up, creates an agent, runs the `arena` connector on their own machine, and the
platform hands that connector a proof task. The agent solves it locally, the
connector returns a diff, and the platform replays the diff against hidden tests
in a Docker sandbox. Model keys and agent code never leave the owner's machine.

Monorepo, two separately deployable apps: `backend/` (Go API + connector CLI)
and `frontend/` (Next.js owner dashboard, talks to the backend over HTTP only).

**Source of truth for the current design** (all in Russian, all under `docs/superpowers/`):

- `specs/2026-09-23-platform-roadmap.md` — the whole platform in slices and the
  decisions taken up front. Read this first for where a change fits. Revised
  2026-09-25: the product is an arena (connect → prove → rating → competitions);
  the labor market (jobs, money, autopilot) is deferred and its specs are not executed.
- Slice 1, complete: spec `specs/2026-09-23-agent-connect-and-proof-design.md`,
  task-by-task plan `plans/2026-09-23-agent-connect-and-proof.md`.
- Slice 2 (qualification and rating) is designed but not started:
  `specs/2026-09-23-qualification-and-rating-design.md` + its plan.
  Tanks (a public bot tournament whose bots are written by agents) is in progress:
  `specs/2026-09-25-tanks-arena-design.md` + `plans/2026-09-25-tanks-arena.md`.
  Slice 3 (competitions) needs a new spec; slice 4 (versions, several agents) comes
  from the old challenges-and-versions spec. Jobs, money and autopilot specs are
  kept as deferred hypotheses.

The product direction is revised often. Check a doc's date before trusting it, and
when a doc and the code disagree, trust the code (`backend/internal/*`, `frontend/app/*`).

**`README.md` and `backend/README.md` are current** — Task 14 rewrote both for
slice 1 (no more competitions, submissions, leaderboards, LLM judge, OIDC/Dex).
`backend/docs/agent-arena-research-and-product-design.md` is background research,
not a description of what is built.

Implementation status: slice 1 (plan Tasks 1–14) is complete, including the
dashboard, the proof page, and ops (Caddy, backups, CI). A whole-branch review
after Task 14 found and fixed critical issues in proof verdict integrity, the
sandbox's file-apply safety, and the deploy stack; see that plan's own
"Самопроверка" section and its SDD workspace ledger
(`.superpowers/sdd/2026-09-23-agent-connect-and-proof/progress.md`, if still
present) for what was fixed vs. deliberately deferred as follow-up work
(connector retry/backoff, a read-only `arena status` endpoint, the full
read-only/tmpfs sandbox redesign).

## Commands

Whole stack (Docker, from the repo root):

```sh
make up        # postgres, api, web; site on http://localhost:3000, api on :8080
make logs
make down      # make reset also wipes the DB volume
```
Reads `.env` (created from `.env.example` on first `make up`). No seeded login —
sign in through GitHub, Google, or (locally) the dev login. `make up` auto-detects the docker.sock group
(`DOCKER_GID`) so the API container's sandbox can reach the daemon.

Native/backend dev (`postgres` from compose, `api`/`web` on the host):

```sh
make migrate                  # goose migrations + sync fixtures/proofs into proof_tasks
make run-api                  # go run ./cmd/api
make run-web                  # pnpm dev, proxying /api to the native API
make test                     # backend go test -race + frontend typecheck/build
make check                    # go vet ./... && gofmt -l . (backend only)

cd backend && go test -race ./internal/proofs/...            # one package
cd backend && go test -race -run TestExpireStale ./internal/proofs   # one test
```

Frontend (`cd frontend`, pnpm — there is no npm lockfile):

```sh
pnpm install
pnpm dev        # next dev on :3000, proxies /api/* to $API_URL (default http://127.0.0.1:8080)
pnpm typecheck  # tsc --noEmit
pnpm build
```

CI (`.github/workflows/ci.yml`) runs `go vet`/`gofmt`/`go test -race` with
`ARENA_TEST_REQUIRE_DOCKER=1` for backend, `pnpm typecheck && pnpm build` for
frontend. Match that locally before claiming work is done.

### Docker-dependent tests

Integration tests (`*_integration_test.go`) start a real Postgres via
testcontainers, and `internal/proofs/sandbox` builds and runs a real container.
They **skip** when Docker is unreachable — set `ARENA_TEST_REQUIRE_DOCKER=1` to
turn those skips into failures. A skipped integration test is not a passing one;
don't report it as green. Under some Colima setups, testcontainers-go's
provider auto-detection doesn't resolve Colima's non-default socket path —
if you hit "rootless Docker not found" with the strict flag set, try:

```sh
export DOCKER_HOST=unix://$HOME/.colima/default/docker.sock
export TESTCONTAINERS_RYUK_DISABLED=true   # tests Terminate() in t.Cleanup themselves
```

The `internal/proofs/sandbox` package's own tests invoke the `docker` CLI
directly (not testcontainers) and are unaffected by that quirk.

## Architecture

Go module is `tolerance`; all env vars are prefixed `ARENA_`; Postgres database
`arena` with two roles, `arena_migrate` (owns the schema) and `arena_app` (what
the API connects as). `cmd/api` never migrates; `cmd/migrate` does, and also
syncs the on-disk proof catalog into `proof_tasks`.

```
backend/cmd/api          config, handler (all routing), main (server + worker goroutine), main_test (e2e)
backend/cmd/migrate      goose up + catalog sync
backend/cmd/arena        the owner-side connector CLI: login, init, connect, status
backend/internal/identity  GitHub/Google OAuth (state + PKCE), dev login, sessions, RequireSession/RequireAgent, /auth/*, /me
backend/internal/agents    agent, API keys, presence, derived stage, /agent/*, /connector/heartbeat
backend/internal/proofs    proof lifecycle, catalog, owner + connector HTTP, worker, sandbox/
backend/internal/platform  db, dbtest, httpx, jobs, auth (API keys), audit, idgen, ratelimit, sanitize
backend/fixtures/proofs    proof task definitions
backend/contracts/openapi  openapi.yaml + validator used by the e2e test
```

**Two auth schemes, two route groups.** `cmd/api/handler.go` is the single place
routing is declared. Owner routes (`/api/v1/me`, `/agent*`, `/proof-tasks`,
`/proofs*`) sit behind `identity.RequireSession` — an HttpOnly session cookie,
30 days, only the hash stored. Connector routes (`/api/v1/connector/*`) sit
behind `identity.RequireAgent` — `Authorization: Bearer <api key>`, SHA-256 in
the database, plaintext shown once at creation. Adding a route means adding it
to the right mux *and* to `contracts/openapi/openapi.yaml`.

Owner sign-in is OAuth only (GitHub, Google); `ARENA_DEV_LOGIN=true` adds
`POST /auth/dev` for local runs and CI and is refused next to
`ARENA_SECURE_COOKIES=true`.

**Proof lifecycle** (the spine of the product):

```
queued → claimed → running_agent → diff_submitted → running_sandbox
       → passed | failed | infra_error | expired
```

`POST /proofs` enqueues. The connector long-polls `GET /connector/tasks/next`
(25s), which moves `queued → claimed` atomically — concurrent pollers must yield
the task to exactly one. It downloads the repo tarball, runs `agent.command` via
`sh -c`, and posts `{diff, log_tail, duration_ms, exit_code}`, which enqueues a
`run_proof` job (dedupe key is intentionally empty — a retried proof reuses its
id, so a fixed dedupe key would silently swallow the resubmission's job).
`internal/proofs/worker.go` runs inside `cmd/api` (a goroutine, not a separate
process): it drains the `jobs` queue (`FOR UPDATE SKIP LOCKED` with leases and
backoff, `internal/platform/jobs`) and ticks stale proofs to `expired` (or
`infra_error`, for proofs stuck mid-run) every 30s.

`infra_error` means the platform failed (no image, docker error, a job stuck
past its bound) and is never counted against the agent — it is retryable.
`failed` means the diff didn't apply, touched a `*_test.go` file, or a hidden
test didn't run or didn't pass — a `passed` verdict requires every hidden test
by name to have actually run and passed, not just "no failures reported."
`worker.go`'s `applyDiff` uses plain `git apply` (no `--unsafe-paths`) so a
malicious diff can't write outside the sandbox work directory. Keep both
properties when touching the worker.

**Agent stage** (`registered, offline, connected, checking, operational,
check_failed`) is never stored — it is derived in `internal/agents/stage.go` from
presence freshness (2 min) plus proof history. Don't add a column for it.

**Sandbox.** `internal/proofs/sandbox` is a `Runner` interface with `docker` and
`fake` implementations; `ARENA_SANDBOX=fake` selects the fake for local runs
without Docker (its `PassAll` mode still checks hidden-test names, so it stays
honest about the same verdict rule real runs enforce). The docker runner copies
the work dir into a container with `--network none`, cpu/memory/pids limits,
`--cap-drop=ALL --security-opt=no-new-privileges`, a capped output reader, and
parses `go test -json`. `--read-only` is deliberately not set — see the comment
above `Docker.Run` for why, and the plan file's Task 7 for a sketched
tar-over-stdin alternative that hasn't been built yet.

**Proof task fixtures** live in `backend/fixtures/proofs/<slug>/`: `manifest.json`,
`TASK.md`, `repo/` (what the agent sees), `_hidden/` (copied over `repo/` before
the sandbox run). The directory is `_hidden`, not `hidden` — the underscore keeps
the Go toolchain from compiling it. The catalog is tarred and stored in
`proof_tasks`, idempotent by slug, on every `cmd/migrate` run.

Nothing builds the sandbox image outside tests or `make up`:
`internal/proofs/sandbox/docker_integration_test.go` does
`docker build -t arena-proof-go:1` itself, and the root `Makefile`'s
`proof-image` target does the same for `make up`. A real proof run needs that
image present on the host already.

## Conventions

- Errors only via `httpx.Problem`; response body is `{code, message, request_id, fields?}`.
  Handlers call `httpx.WriteError`, never `http.Error`.
- All timestamps serialized in UTC. pgx returns `time.Local`, so normalize with
  `.UTC()` when scanning, as existing scanners do.
- Every response in the `cmd/api/main_test.go` e2e test is validated against
  `contracts/openapi/openapi.yaml`, error bodies included. A contract change
  without a spec change fails there.
- Integration tests get their database from `dbtest.New(t)`, which gives both an
  admin pool (arrange fixtures) and the `arena_app` pool the service really uses.
  Assert through the app pool so role grants are exercised.
- Anything an agent writes to its log is sanitized server-side by
  `internal/platform/sanitize` independently of the connector — never trust the
  connector to have done it.
- Frontend: no mocks, all data from the API. Types mirroring the API live in
  `lib/types.ts`, fetching in `lib/api.ts` (`credentials: 'include'`). The browser
  always calls same-origin `/api/v1/*`; the Next rewrite forwards to the Go API so
  the session cookie stays first-party. Must work at 375px with no horizontal scroll.
  The rewrite's target is baked in at `next build` time (`API_URL` build arg in
  `frontend/Dockerfile`), not read at container runtime.
- Commit subjects: English, imperative, sentence case, no `feat:`-style prefixes.
- Prose docs and specs in `docs/` are in Russian; code, comments and commit
  messages are in English. Match whichever you are editing.
