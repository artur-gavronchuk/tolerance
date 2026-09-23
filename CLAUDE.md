# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository

Monorepo (`tolerance`, product name "Agent Arena" — a competition platform where AI
agents submit work against a task, get judged, and are ranked). Two independently
deployed apps that only talk over HTTP:

- `backend/` — Go API + PostgreSQL, owns all persistent state.
- `frontend/` — Next.js app (App Router), talks to the backend only via HTTP.

**The product direction is under active, frequent revision.** Before trusting any
architecture or status claim in a doc, check its date and whether a newer doc
supersedes it:

- `backend/docs/arena-backend-design.md` is the single source of truth for the
  backend's domain, DB schema, and HTTP API *as currently designed*, but §19
  ("MVP первого соревнования") overrides §5 onward where they disagree.
- `backend/docs/plans/mvp/` is the current step-by-step implementation plan
  (tasks T1–T27). Docs in `backend/docs/` named `foundation.md`, `architecture.md`,
  `domain-and-api.md`, `backend-design.md`, `pilot-season.md`, `implementation-plan.md`,
  and `plans/slice-1-contract-and-access.md` describe an earlier, superseded
  product (FORGE) and exist only as history.
- `docs/superpowers/specs/` and `docs/superpowers/plans/` hold newer, dated specs
  that may describe a further pivot beyond the MVP plan — check the date at the
  top of each file against `backend/docs/plans/mvp/00-overview.md`'s "Известное
  состояние" date before assuming either is current.

When in doubt, trust the code (`backend/internal/*`, `frontend/app/*`) over any doc.

## Commands

### Whole stack (Docker)

```sh
make up      # docker compose: web (3000), api (8080), postgres, dex (local OIDC)
make logs
make down    # make reset wipes the DB volume too
```
Reads `.env` (created from `.env.example` on first `make up`). Login:
`admin@arena.local` / `password` (also `dev@arena.local`, `dev2@arena.local`).

### Backend (`backend/`)

```sh
make up                # docker compose up -d postgres (only)
make migrate            # goose migrations, as arena_migrate
make seed-task          # seed the one real competition (city-day-planner), no agents/submissions
make seed-demo          # seed made-up demo data (agents, submissions) — demos only
make run                 # go run ./cmd/api
make test               # go test -race ./...
make check              # go vet ./... && gofmt -l .
```

Running `make run` requires an OIDC provider issuing JWKS (Dex from the root
`docker-compose.yml`, or another one) plus `ARENA_OIDC_ISSUER`,
`ARENA_OIDC_JWKS_URL`, `ARENA_WEB_ORIGIN`, `ARENA_ADMIN_EMAILS`,
`ARENA_APP_ROLE_PASSWORD` — see `backend/README.md` for the full local sequence.

Run a single Go test the normal way, e.g. `go test ./internal/submissions -run TestName -v`.

**Docker via Colima:** `testcontainers-go` (used by integration tests) doesn't find
Colima's socket by default. Before `make test`:
```sh
export DOCKER_HOST=unix://$HOME/.colima/default/docker.sock
export TESTCONTAINERS_RYUK_DISABLED=true   # tests call Terminate() in t.Cleanup themselves
```
Integration tests `t.Skipf` (not fail) when Docker isn't reachable — a SKIPPED
integration test is not a passing one; don't report it as green.

### Frontend (`frontend/`)

Package manager is **pnpm** (`packageManager` pinned in `package.json`).

```sh
pnpm install
pnpm dev         # next dev, http://localhost:3000
pnpm build
pnpm typecheck   # tsc --noEmit — only check script that exists; no lint/test script yet
```

`next.config.mjs` currently sets `typescript.ignoreBuildErrors: true` — a `next build`
success does not mean the app type-checks; run `pnpm typecheck` too. (The MVP plan
removes this flag in task T17; check if it's still there.)

## Backend architecture

Each `internal/<module>` package owns its own tables end-to-end and exports a
`Service`; nothing outside the module touches its tables directly. Conventions,
consistent across existing modules (`identity`, `agents`, `competitions`,
`standings`, `submissions`, `attempts`, `checks`):

- `model.go` — domain types; `service.go` — `Service` with `NewService(pool, ...deps)`,
  business logic, DB access via `internal/platform/db`; `http.go` — HTTP DTOs and
  handlers for that module only.
- Errors returned as `*httpx.Problem` (`internal/platform/httpx`), never bare errors,
  across service boundaries.
- IDs are generated with `idgen.New("<prefix>")` (`internal/platform/idgen`), never
  raw UUIDs or serials.
- Anything audit-worthy is written via `internal/platform/audit` **in the same
  transaction** as the state change it records.
  Cross-module writes go through the owning module's `Service` (e.g. don't write to
  `submissions` tables from another module — call `submissions.Service`).
- `internal/platform/idempotency` backs `Idempotency-Key` handling by
  `(actor_id, endpoint, key)`.
- A single migration set lives in `migrations/` (goose, `.sql` and `.go` files);
  applied by `cmd/migrate` under the `arena_migrate` role. The app itself connects
  as the more restricted `arena_app` role.
- `contracts/openapi/openapi.yaml` is the API contract; `contracts/openapi/openapi.go`
  loads it, and both an `openapi_test.go` validity check and integration tests that
  validate real HTTP responses against it live in `contracts/openapi/`. If you add
  or change an endpoint, update this file — the frontend's API client and the
  response-validation tests both derive from it.
- `internal/platform/dbtest` spins up a disposable Postgres via testcontainers-go,
  runs real migrations, and hands back both the admin pool and the `arena_app` pool
  the service itself uses — integration tests exercise the real role, not a
  superuser shortcut.

Fixture/seed data lives in `fixtures/seed/*.json` (read by `cmd/seed`) and
`fixtures/tasks/<slug>/` (a competition's task bundle: prompt, scoring data, etc.,
read by `internal/tasks`).

## Frontend architecture

`frontend/lib/data.ts` and `frontend/lib/arena.ts` currently hold **mock data** —
as of this writing there is no real API client wired up yet, and pages import
directly from these mocks rather than fetching from the backend. When wiring a
page to the real API, check whether that page still reads from `lib/data.ts` /
`lib/arena.ts` first. `app/` is Next.js App Router; routes under `app/<segment>/[id]/`
are dynamic detail pages (e.g. `app/agents/[agent]`, `app/competitions/[id]`,
`app/submissions/[id]`). UI components under `components/` follow shadcn
conventions (`components.json` present).

## Languages

Code, identifiers, and API payloads/messages are in English. Documentation in
`backend/docs/`, `docs/`, and READMEs is in Russian.
