# Срез 1: подключение агента и базовая проверка. План реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Владелец регистрируется, создаёт агента, подключает его коннектором со своей машины, запускает проверочную задачу; агент решает её локально, платформа прогоняет скрытые тесты в Docker-песочнице и показывает результат в кабинете.

**Architecture:** Go-бэкенд (`net/http`, pgx, goose) с сессиями в cookie и API-ключами для коннектора; очередь `jobs` на SKIP LOCKED уже есть и получает воркер внутри `cmd/api`; песочница запускает `docker create/cp/start` через docker.sock; коннектор `cmd/arena` на той же кодовой базе; фронт Next.js 16 на API через rewrite `/api/*`, cookie same-origin.

**Tech Stack:** Go 1.26+, PostgreSQL 16, pgx/v5, goose, argon2id (`golang.org/x/crypto/argon2`), kin-openapi (валидация ответов в e2e), testcontainers-go, Docker CLI, Next.js 16 / React 19 / Tailwind 4 / shadcn (база из `proofwork-frontend-development.zip`), Caddy.

**Spec:** `docs/superpowers/specs/2026-09-23-agent-connect-and-proof-design.md`

## Global Constraints

- Go-модуль остаётся `tolerance`; префикс переменных окружения `ARENA_`; роли Postgres `arena_app` / `arena_migrate`, БД `arena`.
- Все временные метки в JSON в UTC (pgx отдаёт time.Local; нормализовать `.UTC()` при сканировании, как в существующем коде).
- Ошибки только через `httpx.Problem`; тело ошибки `{code, message, request_id, fields?}`.
- Каждый ответ в e2e-тесте `cmd/api/main_test.go` валидируется по `contracts/openapi/openapi.yaml`, включая ошибки.
- Интеграционные тесты через `dbtest.New`; при `ARENA_TEST_REQUIRE_DOCKER=1` отсутствие Docker это провал, не skip.
- Никаких моков во фронте; все данные из API. Мобильная ширина 375px без горизонтального скролла.
- Пароль от 10 символов. Сессия 30 дней. Лимит логина 10 попыток в минуту на e-mail и на IP. Лимит проверок 10 в сутки на агента. Одна незавершённая проверка на агента. Diff до 256 KiB, хвост лога до 32 KiB. Presence считается свежей 2 минуты.
- Статусы проверки: `queued, claimed, running_agent, diff_submitted, running_sandbox, passed, failed, infra_error, expired`.
- Стадии агента: `registered, offline, connected, checking, operational, check_failed`.
- Коммиты завершаются строкой `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`.

## Review Focus

1. **Diff с CRLF или без завершающего перевода строки** от агента на Windows/macOS: `git apply` должен либо применить, либо дать `failed` с причиной `diff_not_applicable`, а не `infra_error`. Тест в задаче 8.
2. **Коннектор теряет сеть посреди задачи**: результат отправляется повторно с backoff, сервер отвечает 409 на дубликат, коннектор считает 409 успехом. Тест в задаче 9.
3. **Два коннектора с одним ключом** одновременно делают long-poll: задача достаётся ровно одному. Тест в задаче 6.
4. **Владелец отзывает ключ, пока коннектор online**: следующий heartbeat получает 401, presence протухает, стадия становится `offline`, `POST /proofs` отвечает 409. Тест в задаче 10.
5. **Регистрация с e-mail в разном регистре и с пробелами** (`" User@Example.com "`): нормализуется в `user@example.com`, второй signup даёт 409. Тест в задаче 2.

## Структура файлов

```
backend/
  migrations/00001_app_role.go           без изменений
  migrations/00002_schema.sql            НОВАЯ единая схема среза (старые 00002/00003 удаляются)
  internal/platform/httpx, db, dbtest, idgen, audit, jobs   остаются
  internal/platform/auth/apikey.go       остаётся; verifier.go удаляется
  internal/platform/ratelimit/           НОВЫЙ: скользящее окно в памяти
  internal/platform/sanitize/            НОВЫЙ: CleanText (переезд из attempts) + CleanLog
  internal/identity/                     пароли, сессии, RequireSession, /auth/*, /me
  internal/agents/                       агент, ключи, presence, стадия, /agent/*, /connector/heartbeat
  internal/proofs/catalog.go             каталог задач из fixtures/proofs, tar.gz, синхронизация в proof_tasks
  internal/proofs/model.go,service.go    proofs: создание, выдача, результат, истечение, факты для стадии
  internal/proofs/http_me.go             /proof-tasks, /proofs*
  internal/proofs/http_connector.go      /connector/tasks/next, /connector/proofs/*
  internal/proofs/worker.go              job run_proof + expire tick
  internal/proofs/sandbox/               Runner: docker, fake, парсер go test -json
  fixtures/proofs/go-fix-retry/          первая задача
  cmd/api/                               config, handler, main (+ worker), main_test (e2e)
  cmd/migrate/                           миграции + синхронизация каталога
  cmd/arena/                             коннектор
  contracts/openapi/openapi.yaml         контракт среза
frontend/                                кабинет владельца (пересоздаётся из proofwork)
docker-compose.yml, Makefile, Caddyfile, .github/workflows/ci.yml
```

---

### Task 1: Чистый лист: удалить домен арены, новая схема, сборка зелёная

**Files:**
- Delete: `backend/internal/{arena,attempts,checks,competitions,standings,submissions,tasks}/`, `backend/internal/platform/idempotency/`, `backend/internal/platform/auth/verifier.go`, `backend/internal/platform/auth/verifier_test.go`, `backend/internal/identity/handle.go`, `backend/internal/identity/*_test.go`, `backend/fixtures/`, `backend/cmd/seed/`, `backend/dev/dex/`, `backend/migrations/00002_schema.sql`, `backend/migrations/00003_mvp.sql`, `backend/docs/*` кроме `agent-arena-research-and-product-design.md` и `audit-2026-09-23.md`, `frontend/docs/`, `backend/internal/platform/db/db_integration_test.go`, `backend/internal/platform/jobs/jobs_integration_test.go` (переписывается в шаге 7)
- Create: `backend/migrations/00002_schema.sql`
- Modify: `backend/internal/identity/service.go`, `backend/internal/identity/middleware.go`, `backend/internal/identity/http.go`, `backend/internal/agents/model.go`, `backend/internal/agents/service.go`, `backend/internal/agents/http.go`, `backend/internal/platform/dbtest/dbtest.go`, `backend/cmd/api/config.go`, `backend/cmd/api/handler.go`, `backend/cmd/api/main.go`, `backend/cmd/api/main_test.go`, `backend/cmd/migrate/main.go`, `backend/contracts/openapi/openapi.yaml`, `backend/Makefile`, `backend/go.mod`

**Interfaces:**
- Produces: схема из 3.1 спеки; `identity.User{ID, Email, Role, CreatedAt}`; `agents.Agent{ID, OwnerUserID, Name, Description, CreatedAt, Version}`; `dbtest.New(t)` с флагом `ARENA_TEST_REQUIRE_DOCKER`.

- [ ] **Step 1: Удалить старый домен**

```bash
cd backend
git rm -rq internal/arena internal/attempts internal/checks internal/competitions internal/standings internal/submissions internal/tasks internal/platform/idempotency fixtures cmd/seed dev/dex
git rm -q internal/platform/auth/verifier.go internal/platform/auth/verifier_test.go internal/identity/handle.go internal/identity/*_test.go migrations/00002_schema.sql migrations/00003_mvp.sql internal/platform/db/db_integration_test.go internal/platform/jobs/jobs_integration_test.go
cd docs && git rm -rq $(ls | grep -v -e '^agent-arena-research-and-product-design.md$' -e '^audit-2026-09-23.md$') && cd ..
git rm -rq ../frontend/docs
```

- [ ] **Step 2: Новая схема**

`backend/migrations/00002_schema.sql`:

```sql
-- +goose Up

CREATE TABLE users (
    id text PRIMARY KEY,
    email text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    role text NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- id is the SHA-256 hex of the cookie token; the token itself is never stored.
CREATE TABLE sessions (
    id text PRIMARY KEY,
    user_id text NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_user_idx ON sessions (user_id);

CREATE TABLE agents (
    id text PRIMARY KEY,
    owner_user_id text NOT NULL REFERENCES users (id),
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    version int NOT NULL DEFAULT 1,
    UNIQUE (owner_user_id)
);
CREATE UNIQUE INDEX agents_name_ci_idx ON agents (lower(name));

CREATE TABLE api_keys (
    id text PRIMARY KEY,
    agent_id text NOT NULL REFERENCES agents (id),
    prefix text NOT NULL,
    key_hash text NOT NULL UNIQUE,
    name text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    revoked_at timestamptz
);
CREATE INDEX api_keys_agent_idx ON api_keys (agent_id) WHERE revoked_at IS NULL;

CREATE TABLE agent_presence (
    agent_id text PRIMARY KEY REFERENCES agents (id),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    connector_version text NOT NULL DEFAULT '',
    hostname text NOT NULL DEFAULT ''
);

CREATE TABLE proof_tasks (
    slug text PRIMARY KEY,
    title text NOT NULL,
    language text NOT NULL,
    image text NOT NULL,
    run_cmd text NOT NULL,
    agent_timeout_s int NOT NULL,
    sandbox_timeout_s int NOT NULL,
    visible_tests int NOT NULL,
    hidden_tests int NOT NULL,
    task_md text NOT NULL,
    repo_tar bytea NOT NULL,
    hidden_tar bytea NOT NULL,
    repo_sha256 text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE proofs (
    id text PRIMARY KEY,
    agent_id text NOT NULL REFERENCES agents (id),
    task_slug text NOT NULL REFERENCES proof_tasks (slug),
    status text NOT NULL DEFAULT 'queued' CHECK (status IN
        ('queued', 'claimed', 'running_agent', 'diff_submitted', 'running_sandbox',
         'passed', 'failed', 'infra_error', 'expired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    claimed_at timestamptz,
    diff_submitted_at timestamptz,
    finished_at timestamptz,
    diff text NOT NULL DEFAULT '',
    agent_log_tail text NOT NULL DEFAULT '',
    agent_duration_ms int,
    agent_exit_code int,
    sandbox_result jsonb,
    failure_reason text NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX proofs_one_open_idx ON proofs (agent_id)
    WHERE status IN ('queued', 'claimed', 'running_agent', 'diff_submitted', 'running_sandbox');
CREATE INDEX proofs_agent_created_idx ON proofs (agent_id, created_at DESC);

CREATE TABLE jobs (
    id text PRIMARY KEY,
    kind text NOT NULL CHECK (kind IN ('run_proof')),
    dedupe_key text UNIQUE,
    state text NOT NULL DEFAULT 'queued' CHECK (state IN ('queued', 'leased', 'done', 'failed')),
    run_after timestamptz NOT NULL DEFAULT now(),
    lease_owner text,
    lease_until timestamptz,
    attempts int NOT NULL DEFAULT 0,
    max_attempts int NOT NULL DEFAULT 3,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX jobs_claim_idx ON jobs (state, run_after);

CREATE TABLE audit_events (
    id text PRIMARY KEY,
    actor_id text NOT NULL,
    actor_kind text NOT NULL CHECK (actor_kind IN ('user', 'agent', 'system')),
    action text NOT NULL,
    aggregate_kind text NOT NULL,
    aggregate_id text NOT NULL,
    before_version int,
    after_version int,
    reason text,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    request_id text,
    at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_events_aggregate_idx ON audit_events (aggregate_kind, aggregate_id, at DESC);

GRANT USAGE ON SCHEMA public TO arena_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO arena_app;

-- +goose Down
DROP TABLE audit_events;
DROP TABLE jobs;
DROP TABLE proofs;
DROP TABLE proof_tasks;
DROP TABLE agent_presence;
DROP TABLE api_keys;
DROP TABLE agents;
DROP TABLE sessions;
DROP TABLE users;
```

- [ ] **Step 3: Обрезать identity до User и агентского middleware**

`backend/internal/identity/service.go` целиком:

```go
package identity

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
)

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type Service struct {
	pool        *db.Pool
	adminEmails map[string]bool
}

func NewService(pool *db.Pool, adminEmails []string) *Service {
	m := map[string]bool{}
	for _, e := range adminEmails {
		if e = NormalizeEmail(e); e != "" {
			m[e] = true
		}
	}
	return &Service{pool: pool, adminEmails: m}
}

// NormalizeEmail trims and lower-cases; the database stores this form only.
func NormalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

const userColumns = `id, email, role, created_at`

func scanUser(row interface{ Scan(...any) error }, u *User) error {
	if err := row.Scan(&u.ID, &u.Email, &u.Role, &u.CreatedAt); err != nil {
		return err
	}
	u.CreatedAt = u.CreatedAt.UTC()
	return nil
}

func (s *Service) Get(ctx context.Context, id string) (User, error) {
	var u User
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return scanUser(tx.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id), &u)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, httpx.NotFound()
	}
	return u, err
}
```

`backend/internal/identity/middleware.go`: удалить `TokenVerifier`, `RequireUser` и импорт `context`; оставить `AgentLookup`, `ErrNoAgent`, `bearer`, `RequireAdmin`, `RequireAgent` без изменений.

`backend/internal/identity/http.go`: заменить содержимое на пустой пакет-файл (маршруты появятся в задаче 2):

```go
package identity
```

- [ ] **Step 4: Обрезать agents**

`backend/internal/agents/model.go`:

```go
package agents

import (
	"regexp"
	"time"
)

var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{1,31}$`)

type Agent struct {
	ID          string
	OwnerUserID string
	Name        string
	Description string
	CreatedAt   time.Time
	Version     int
}

type KeyView struct {
	ID         string     `json:"id"`
	Prefix     string     `json:"prefix"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

type Private struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	APIKeys     []KeyView `json:"api_keys"`
}

type CreateInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type PatchInput struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}
```

`backend/internal/agents/service.go`: в существующем файле
- убрать импорт `tolerance/internal/standings`, поле `standings` и параметр `st` из `NewService` (сигнатура `NewService(pool *db.Pool) *Service`);
- `agentCols = "id, owner_user_id, name, description, created_at, version"`, `scanAgent` сканирует `&a.Description` вместо `&a.Model, &a.Bio`;
- `validateCreate`: проверка `name` по `nameRe`, `description` до 500 символов (`invalid_body`, поле `description`, код `too_long`); проверку `model` удалить;
- `Create`: `INSERT INTO agents (id, owner_user_id, name, description, created_at) VALUES ($1,$2,$3,$4,$5)`;
- `Patch`: `UPDATE agents SET name = coalesce($2, name), description = coalesce($3, description), version = version + 1 WHERE owner_user_id = $1 RETURNING ...`; валидация `name` по `nameRe`, `description` до 500; при `uniqueViolation(err, "agents_name_ci_idx")` вернуть `httpx.New(409, "name_taken", "This agent name is already taken")`;
- `private`: убрать запрос к `arena_queue`; `Private{ID, Name, Description, CreatedAt, APIKeys}`;
- удалить `ByName`, `ProfileByName`, `Profile`; оставить `byOwner`, `PrivateForUser`, `MeAgent`, `ByID`, `CreateKey`, `RevokeKey`, `AgentIDByKeyHash`.

`backend/internal/agents/http.go`: удалить `RegisterPublicRoutes` и `RegisterAgentRoutes`; в `RegisterMeRoutes` убрать обёртку `idempotency.Command` и импорт `idempotency`/`db`/`pgx`: `POST /api/v1/me/agent` читает тело через `httpx.ReadBody` + `httpx.Decode(raw, &in)` и отвечает `httpx.Respond(w, http.StatusCreated, p)`. Сигнатура `RegisterMeRoutes(mux *http.ServeMux, s *Service)`. Пути маршрутов пока оставить как есть, задача 3 переименует.

- [ ] **Step 5: dbtest: провал вместо skip по флагу**

В `backend/internal/platform/dbtest/dbtest.go` заменить блок после `tcpostgres.Run`:

```go
	if err != nil {
		if os.Getenv("ARENA_TEST_REQUIRE_DOCKER") == "1" {
			t.Fatalf("postgres testcontainer unavailable and ARENA_TEST_REQUIRE_DOCKER=1: %v", err)
		}
		t.Skipf("postgres testcontainer unavailable, skipping integration test: %v", err)
	}
```

и добавить `"os"` в импорты.

- [ ] **Step 6: cmd/api и cmd/migrate компилируются**

`backend/cmd/api/config.go` целиком:

```go
package main

import (
	"errors"
	"net"
	"os"
	"strings"
)

type config struct {
	addr             string
	databaseURL      string
	adminEmails      []string
	secureCookies    bool
	allowNonLoopback bool
	workDir          string
	sandbox          string // "docker" | "fake"
}

func loadConfig() (config, error) {
	cfg := config{
		addr:             env("ARENA_ADDR", "127.0.0.1:8080"),
		databaseURL:      os.Getenv("ARENA_APP_DATABASE_URL"),
		secureCookies:    os.Getenv("ARENA_SECURE_COOKIES") == "true",
		allowNonLoopback: os.Getenv("ARENA_ALLOW_NON_LOOPBACK") == "true",
		workDir:          env("ARENA_WORK_DIR", os.TempDir()),
		sandbox:          env("ARENA_SANDBOX", "docker"),
	}
	for _, e := range strings.Split(os.Getenv("ARENA_ADMIN_EMAILS"), ",") {
		if e = strings.TrimSpace(e); e != "" {
			cfg.adminEmails = append(cfg.adminEmails, e)
		}
	}
	host, _, err := net.SplitHostPort(cfg.addr)
	if err != nil || net.ParseIP(host) == nil {
		return config{}, errors.New("ARENA_ADDR must be an ip:port")
	}
	if !net.ParseIP(host).IsLoopback() && !cfg.allowNonLoopback {
		return config{}, errors.New("ARENA_ADDR must be a loopback address; a reverse proxy is expected in front (set ARENA_ALLOW_NON_LOOPBACK=true inside a container)")
	}
	if cfg.databaseURL == "" {
		return config{}, errors.New("ARENA_APP_DATABASE_URL is required")
	}
	if cfg.sandbox != "docker" && cfg.sandbox != "fake" {
		return config{}, errors.New("ARENA_SANDBOX must be docker or fake")
	}
	return cfg, nil
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
```

`backend/cmd/api/handler.go` целиком (минимум, расширяется в следующих задачах):

```go
package main

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/agents"
	"tolerance/internal/identity"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
)

type deps struct {
	pool   *db.Pool
	log    *slog.Logger
	users  *identity.Service
	agents *agents.Service
}

func newHandler(cfg config, d deps) http.Handler {
	api := http.NewServeMux()

	top := http.NewServeMux()
	top.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := d.pool.Tx(r.Context(), func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "SELECT 1")
			return err
		}); err != nil {
			httpx.Respond(w, http.StatusServiceUnavailable, map[string]string{"status": "db_unavailable"})
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	top.Handle("/api/", api)
	return withMiddleware(top, d.log)
}

func withMiddleware(next http.Handler, log *slog.Logger) http.Handler {
	withRequestID := httpx.WithRequestID(func() string { return idgen.New("req") })(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		withRequestID.ServeHTTP(w, r)
	})
}
```

`backend/cmd/api/main.go`: убрать импорты и конструкторы удалённых пакетов; `d := deps{pool: pool, log: log, users: identity.NewService(pool, cfg.adminEmails), agents: agents.NewService(pool)}`; убрать `go competitions.RunCloser(...)`; убрать `verifier`.

`backend/cmd/api/main_test.go` целиком (заново, e2e наполняется в задаче 10):

```go
package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"tolerance/internal/agents"
	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	d := dbtest.New(t)
	cfg := config{addr: "127.0.0.1:0", adminEmails: []string{"admin@arena.local"}, sandbox: "fake"}
	dp := deps{pool: d.AppPool, log: slog.New(slog.NewTextHandler(os.Stderr, nil)),
		users: identity.NewService(d.AppPool, cfg.adminEmails), agents: agents.NewService(d.AppPool)}
	srv := httptest.NewServer(newHandler(cfg, dp))
	t.Cleanup(srv.Close)
	return srv
}

func TestHealthz(t *testing.T) {
	srv := newTestServer(t)
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}
```

`backend/cmd/migrate/main.go`: без изменений в этой задаче.

`backend/contracts/openapi/openapi.yaml` временно минимальный (полный контракт в задаче 10):

```yaml
openapi: 3.0.3
info:
  title: Agent Arena API
  version: "0.3.0"
servers:
  - url: /api/v1
paths: {}
```

`backend/Makefile`: удалить цели `seed-demo`, `seed-task`; `run` без OIDC-переменных.

- [ ] **Step 7: Тест миграции**

`backend/internal/platform/db/db_integration_test.go`:

```go
package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/dbtest"
)

func TestMigrate_CreatesSliceTablesAndAppRoleCanUseThem(t *testing.T) {
	d := dbtest.New(t)
	for _, table := range []string{"users", "sessions", "agents", "api_keys", "agent_presence", "proof_tasks", "proofs", "jobs", "audit_events"} {
		err := d.AppPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "SELECT 1 FROM "+table+" LIMIT 1")
			return err
		})
		if err != nil {
			t.Fatalf("arena_app cannot read %s: %v", table, err)
		}
	}
}
```

Переписать `backend/internal/platform/jobs/jobs_integration_test.go`: оставить только тесты, использующие `kind = 'run_proof'` (заменить все `'judge_submission'`/`'recompute_badges'` на `'run_proof'`, убрать тесты, ссылающиеся на удалённые таблицы).

- [ ] **Step 8: Сборка, тесты, tidy**

```bash
cd backend && go mod tidy && gofmt -l . && go vet ./... && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./...
```

Expected: `gofmt -l .` пусто, vet без ошибок, все тесты PASS (в том числе `TestMigrate_CreatesSliceTablesAndAppRoleCanUseThem` и `TestHealthz`). `go mod tidy` убирает `jwx` и `kin-openapi` остаётся (используется в задаче 10; если tidy его убрал, вернётся там).

- [ ] **Step 9: Commit**

```bash
git add -A backend frontend/docs && git commit -m "Reset domain to slice 1: drop arena, new schema, minimal api

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 2: Пароли, сессии, регистрация и вход

**Files:**
- Create: `backend/internal/identity/password.go`, `backend/internal/identity/password_test.go`, `backend/internal/identity/session.go`, `backend/internal/identity/auth_integration_test.go`, `backend/internal/platform/ratelimit/ratelimit.go`, `backend/internal/platform/ratelimit/ratelimit_test.go`
- Modify: `backend/internal/identity/service.go`, `backend/internal/identity/middleware.go`, `backend/internal/identity/http.go`, `backend/cmd/api/handler.go`, `backend/go.mod`

**Interfaces:**
- Produces: `identity.HashPassword(pw) (string, error)`, `identity.VerifyPassword(hash, pw) bool`; `(*Service).Signup(ctx, email, pw) (User, token string, error)`, `Login(ctx, email, pw) (User, string, error)`, `Logout(ctx, token) error`, `UserBySession(ctx, token) (User, error)`; `identity.ErrNoSession`; `identity.RequireSession(s *Service) func(http.Handler) http.Handler`; `identity.SessionCookie = "arena_session"`; `identity.SetSessionCookie(w, token, secure)`, `identity.ClearSessionCookie(w, secure)`; `identity.RegisterAuthRoutes(mux, s, limiter, secure)`; `ratelimit.Limiter.Allow(key string, limit int, window time.Duration) bool`.

- [ ] **Step 1: Тест хэширования паролей**

`backend/internal/identity/password_test.go`:

```go
package identity

import (
	"strings"
	"testing"
)

func TestHashPassword_VerifiesAndRejects(t *testing.T) {
	h, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$") {
		t.Fatalf("unexpected encoding: %s", h)
	}
	if !VerifyPassword(h, "correct horse battery") {
		t.Fatalf("correct password rejected")
	}
	if VerifyPassword(h, "correct horse batter") {
		t.Fatalf("wrong password accepted")
	}
	if VerifyPassword("garbage", "x") {
		t.Fatalf("malformed hash accepted")
	}
	h2, _ := HashPassword("correct horse battery")
	if h2 == h {
		t.Fatalf("salt must differ between hashes")
	}
}
```

- [ ] **Step 2: Запустить, убедиться, что падает**

Run: `cd backend && go test ./internal/identity/ -run TestHashPassword -v`
Expected: FAIL, `undefined: HashPassword`.

- [ ] **Step 3: Реализация argon2id**

`backend/internal/identity/password.go`:

```go
package identity

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    = 1
	argonMemory  = 64 * 1024 // KiB
	argonThreads = 4
	argonKeyLen  = 32
)

// HashPassword returns a self-describing argon2id string:
// $argon2id$v=19$m=65536,t=1,p=4$<salt>$<hash> (base64 without padding).
func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMemory, argonTime, argonThreads,
		enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// VerifyPassword reports whether password matches encoded. Malformed input
// is simply "does not match"; the caller never learns why.
func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	enc := base64.RawStdEncoding
	salt, err := enc.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := enc.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}
```

Run: `go get golang.org/x/crypto@latest && go test ./internal/identity/ -run TestHashPassword -v`
Expected: PASS.

- [ ] **Step 4: Тест лимитера**

`backend/internal/platform/ratelimit/ratelimit_test.go`:

```go
package ratelimit

import (
	"testing"
	"time"
)

func TestLimiter_AllowsUpToLimitThenBlocksUntilWindowPasses(t *testing.T) {
	now := time.Unix(1000, 0)
	l := New(func() time.Time { return now })
	for i := 0; i < 3; i++ {
		if !l.Allow("k", 3, time.Minute) {
			t.Fatalf("call %d should be allowed", i)
		}
	}
	if l.Allow("k", 3, time.Minute) {
		t.Fatalf("4th call should be blocked")
	}
	if !l.Allow("other", 3, time.Minute) {
		t.Fatalf("different key must not be affected")
	}
	now = now.Add(61 * time.Second)
	if !l.Allow("k", 3, time.Minute) {
		t.Fatalf("after the window the key should be allowed again")
	}
}
```

- [ ] **Step 5: Реализация лимитера**

`backend/internal/platform/ratelimit/ratelimit.go`:

```go
// Package ratelimit is an in-memory sliding-window limiter for the few
// endpoints that need brute-force protection (login, signup). One process,
// no persistence: a restart forgets the counters, which is acceptable.
package ratelimit

import (
	"sync"
	"time"
)

type Limiter struct {
	mu   sync.Mutex
	now  func() time.Time
	hits map[string][]time.Time
}

func New(now func() time.Time) *Limiter {
	if now == nil {
		now = time.Now
	}
	return &Limiter{now: now, hits: map[string][]time.Time{}}
}

// Allow records a hit for key and reports whether it is within limit hits
// per window. Old hits are pruned on every call so memory stays bounded by
// limit per active key.
func (l *Limiter) Allow(key string, limit int, window time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	cutoff := now.Add(-window)
	kept := l.hits[key][:0]
	for _, h := range l.hits[key] {
		if h.After(cutoff) {
			kept = append(kept, h)
		}
	}
	if len(kept) >= limit {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, now)
	return true
}
```

Run: `go test ./internal/platform/ratelimit/ -v` → PASS.

- [ ] **Step 6: Интеграционный тест сессий**

`backend/internal/identity/auth_integration_test.go`:

```go
package identity_test

import (
	"context"
	"errors"
	"testing"

	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/httpx"
)

func TestSignupLoginLogout(t *testing.T) {
	d := dbtest.New(t)
	s := identity.NewService(d.AppPool, []string{"Admin@Arena.local"})
	ctx := context.Background()

	u, tok, err := s.Signup(ctx, " User@Example.com ", "longenough1")
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	if u.Email != "user@example.com" || u.Role != "user" {
		t.Fatalf("unexpected user %+v", u)
	}
	got, err := s.UserBySession(ctx, tok)
	if err != nil || got.ID != u.ID {
		t.Fatalf("session lookup: %v %+v", err, got)
	}

	_, _, err = s.Signup(ctx, "user@example.com", "longenough1")
	var p *httpx.Problem
	if !errors.As(err, &p) || p.Status != 409 || p.Code != "email_taken" {
		t.Fatalf("expected 409 email_taken, got %v", err)
	}
	_, _, err = s.Signup(ctx, "x@example.com", "short")
	if !errors.As(err, &p) || p.Status != 422 {
		t.Fatalf("expected 422 for short password, got %v", err)
	}

	_, _, err = s.Login(ctx, "user@example.com", "wrong-password")
	if !errors.As(err, &p) || p.Status != 401 || p.Code != "invalid_credentials" {
		t.Fatalf("expected 401 invalid_credentials, got %v", err)
	}
	_, _, err = s.Login(ctx, "nobody@example.com", "longenough1")
	if !errors.As(err, &p) || p.Status != 401 || p.Code != "invalid_credentials" {
		t.Fatalf("unknown email must look exactly like a wrong password, got %v", err)
	}
	_, tok2, err := s.Login(ctx, "USER@example.com", "longenough1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	if err := s.Logout(ctx, tok); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := s.UserBySession(ctx, tok); !errors.Is(err, identity.ErrNoSession) {
		t.Fatalf("logged-out session must be gone, got %v", err)
	}
	if _, err := s.UserBySession(ctx, tok2); err != nil {
		t.Fatalf("other session must survive: %v", err)
	}

	admin, _, err := s.Signup(ctx, "admin@arena.local", "longenough1")
	if err != nil || admin.Role != "admin" {
		t.Fatalf("admin email must get admin role: %v %+v", err, admin)
	}
}
```

- [ ] **Step 7: Сервис сессий**

`backend/internal/identity/session.go`:

```go
package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"time"
)

const (
	SessionCookie = "arena_session"
	sessionTTL    = 30 * 24 * time.Hour
)

var ErrNoSession = errors.New("identity: no such session")

func newSessionToken() (token, id string, err error) {
	var b [32]byte
	if _, err = rand.Read(b[:]); err != nil {
		return "", "", err
	}
	token = hex.EncodeToString(b[:])
	return token, sessionID(token), nil
}

func sessionID(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func SetSessionCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: token, Path: "/", HttpOnly: true,
		Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: int(sessionTTL.Seconds())})
}

func ClearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: "", Path: "/", HttpOnly: true,
		Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}
```

Добавить в `backend/internal/identity/service.go`:

```go
import (
	"net/http"
	"net/mail"

	"github.com/jackc/pgx/v5/pgconn"

	"tolerance/internal/platform/audit"
	"tolerance/internal/platform/idgen"
)

const minPasswordLen = 10

func validateCredentials(email, password string) error {
	if _, err := mail.ParseAddress(email); err != nil || len(email) > 254 {
		return httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "email must be a valid address", "email", "invalid")
	}
	if len(password) < minPasswordLen || len(password) > 1024 {
		return httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "password must be at least 10 characters", "password", "too_short")
	}
	return nil
}

func (s *Service) roleFor(email string) string {
	if s.adminEmails[email] {
		return "admin"
	}
	return "user"
}

// Signup creates the user and a first session. The token goes into the
// cookie; only its hash is stored.
func (s *Service) Signup(ctx context.Context, email, password string) (User, string, error) {
	email = NormalizeEmail(email)
	if err := validateCredentials(email, password); err != nil {
		return User{}, "", err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, "", err
	}
	token, sid, err := newSessionToken()
	if err != nil {
		return User{}, "", err
	}
	var u User
	err = s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := scanUser(tx.QueryRow(ctx, `INSERT INTO users (id, email, password_hash, role) VALUES ($1, $2, $3, $4) RETURNING `+userColumns,
			idgen.New("user"), email, hash, s.roleFor(email)), &u); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO sessions (id, user_id, expires_at) VALUES ($1, $2, now() + $3::interval)`, sid, u.ID, sessionTTL.String()); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: u.ID, ActorKind: KindUser, Action: "user.signed_up",
			AggregateKind: "user", AggregateID: u.ID, RequestID: httpx.RequestID(ctx)})
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return User{}, "", httpx.New(http.StatusConflict, "email_taken", "An account with this email already exists")
	}
	if err != nil {
		return User{}, "", err
	}
	return u, token, nil
}

var errInvalidCredentials = httpx.New(http.StatusUnauthorized, "invalid_credentials", "Email or password is incorrect")

func (s *Service) Login(ctx context.Context, email, password string) (User, string, error) {
	email = NormalizeEmail(email)
	var u User
	var hash string
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT `+userColumns+`, password_hash FROM users WHERE email = $1`, email).
			Scan(&u.ID, &u.Email, &u.Role, &u.CreatedAt, &hash)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// Burn the same time as a real verification so timing does not reveal
		// whether the email exists.
		VerifyPassword("$argon2id$v=19$m=65536,t=1,p=4$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", password)
		return User{}, "", errInvalidCredentials
	}
	if err != nil {
		return User{}, "", err
	}
	if !VerifyPassword(hash, password) {
		return User{}, "", errInvalidCredentials
	}
	u.CreatedAt = u.CreatedAt.UTC()
	token, sid, err := newSessionToken()
	if err != nil {
		return User{}, "", err
	}
	err = s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE users SET role = $2 WHERE id = $1`, u.ID, s.roleFor(email)); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO sessions (id, user_id, expires_at) VALUES ($1, $2, now() + $3::interval)`, sid, u.ID, sessionTTL.String())
		return err
	})
	if err != nil {
		return User{}, "", err
	}
	u.Role = s.roleFor(email)
	return u, token, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, sessionID(token))
		return err
	})
}

// UserBySession resolves a cookie token; expired sessions are deleted on
// sight. last_seen_at is bumped at most once an hour.
func (s *Service) UserBySession(ctx context.Context, token string) (User, error) {
	if token == "" {
		return User{}, ErrNoSession
	}
	var u User
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return scanUser(tx.QueryRow(ctx, `
			UPDATE sessions s SET last_seen_at = CASE WHEN s.last_seen_at < now() - interval '1 hour' THEN now() ELSE s.last_seen_at END
			FROM users u WHERE s.id = $1 AND s.user_id = u.id AND s.expires_at > now()
			RETURNING u.id, u.email, u.role, u.created_at`, sessionID(token)), &u)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNoSession
	}
	return u, err
}
```

Run: `ARENA_TEST_REQUIRE_DOCKER=1 go test ./internal/identity/ -run TestSignupLoginLogout -v` → PASS.

- [ ] **Step 8: Middleware и маршруты**

Добавить в `backend/internal/identity/middleware.go`:

```go
// RequireSession authenticates the arena_session cookie and attaches a user Actor.
func RequireSession(s *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(SessionCookie)
			if err != nil {
				httpx.WriteError(w, r, httpx.Unauthenticated("Sign in required"))
				return
			}
			u, err := s.UserBySession(r.Context(), c.Value)
			if errors.Is(err, ErrNoSession) {
				httpx.WriteError(w, r, httpx.Unauthenticated("Session expired, sign in again"))
				return
			}
			if err != nil {
				httpx.WriteError(w, r, err)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithActor(r.Context(),
				Actor{Kind: KindUser, ID: u.ID, UserID: u.ID, Role: u.Role})))
		})
	}
}
```

`backend/internal/identity/http.go` целиком:

```go
package identity

import (
	"net"
	"net/http"
	"time"

	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/ratelimit"
)

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// RegisterAuthRoutes mounts signup, login and logout. They are unauthenticated
// and rate limited per email and per IP.
func RegisterAuthRoutes(mux *http.ServeMux, s *Service, limiter *ratelimit.Limiter, secure bool) {
	handle := func(fn func(ctx *http.Request, in credentials) (User, string, error), status int) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			raw, err := httpx.ReadBody(w, r)
			if err != nil {
				httpx.WriteError(w, r, err)
				return
			}
			var in credentials
			if err := httpx.Decode(raw, &in); err != nil {
				httpx.WriteError(w, r, err)
				return
			}
			email := NormalizeEmail(in.Email)
			if !limiter.Allow("ip:"+clientIP(r), 10, time.Minute) || !limiter.Allow("email:"+email, 10, time.Minute) {
				httpx.WriteError(w, r, httpx.New(http.StatusTooManyRequests, "rate_limited", "Too many attempts, try again in a minute"))
				return
			}
			u, token, err := fn(r, in)
			if err != nil {
				httpx.WriteError(w, r, err)
				return
			}
			SetSessionCookie(w, token, secure)
			httpx.Respond(w, status, map[string]any{"user": u})
		}
	}
	mux.HandleFunc("POST /api/v1/auth/signup", handle(func(r *http.Request, in credentials) (User, string, error) {
		return s.Signup(r.Context(), in.Email, in.Password)
	}, http.StatusCreated))
	mux.HandleFunc("POST /api/v1/auth/login", handle(func(r *http.Request, in credentials) (User, string, error) {
		return s.Login(r.Context(), in.Email, in.Password)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(SessionCookie); err == nil {
			if err := s.Logout(r.Context(), c.Value); err != nil {
				httpx.WriteError(w, r, err)
				return
			}
		}
		ClearSessionCookie(w, secure)
		w.WriteHeader(http.StatusNoContent)
	})
}
```

В `backend/cmd/api/handler.go`: добавить в `deps` поле `limiter *ratelimit.Limiter`; в `newHandler` перед `top`:

```go
	identity.RegisterAuthRoutes(api, d.users, d.limiter, cfg.secureCookies)
```

В `main.go` и `main_test.go` создавать `limiter: ratelimit.New(nil)`.

- [ ] **Step 9: Проверка и коммит**

```bash
cd backend && gofmt -l . && go vet ./... && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./...
git add -A backend && git commit -m "Add email/password signup, login and cookie sessions

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 3: Агент, ключи, presence, стадия, `/me`

**Files:**
- Create: `backend/internal/agents/stage.go`, `backend/internal/agents/stage_test.go`, `backend/internal/agents/presence.go`, `backend/internal/agents/agents_integration_test.go`
- Modify: `backend/internal/agents/model.go`, `backend/internal/agents/service.go`, `backend/internal/agents/http.go`, `backend/internal/identity/http.go`, `backend/cmd/api/handler.go`

**Interfaces:**
- Consumes: `identity.RequireSession`, `identity.RequireAgent`, `identity.MustFromContext`.
- Produces: `agents.ProofFacts{HasPassed, HasOpen bool; LastFinishedStatus string}`; `agents.ProofFactsSource interface{ ProofFacts(ctx, agentID string) (ProofFacts, error) }`; `agents.NoProofFacts{}` (заглушка); `agents.ComputeStage(hasKey bool, lastSeen *time.Time, now time.Time, f ProofFacts) string`; `agents.Presence{LastSeenAt time.Time; ConnectorVersion, Hostname string}`; `(*Service).Heartbeat(ctx, agentID, version, hostname) error`; `(*Service).Overview(ctx, userID) (*Overview, error)` где `Overview{Private; Stage string; Presence *Presence}`; `(*Service).OverviewByID(ctx, agentID) (Overview, error)`; маршруты `GET /api/v1/me`, `POST|PATCH /api/v1/agent`, `POST /api/v1/agent/keys`, `DELETE /api/v1/agent/keys/{id}`, `POST /api/v1/connector/heartbeat`.

- [ ] **Step 1: Тест стадии**

`backend/internal/agents/stage_test.go`:

```go
package agents

import (
	"testing"
	"time"
)

func TestComputeStage(t *testing.T) {
	now := time.Unix(10_000, 0)
	fresh := now.Add(-30 * time.Second)
	stale := now.Add(-3 * time.Minute)
	cases := []struct {
		name    string
		hasKey  bool
		seen    *time.Time
		facts   ProofFacts
		want    string
	}{
		{"no key", false, nil, ProofFacts{}, StageRegistered},
		{"key, never seen", true, nil, ProofFacts{}, StageRegistered},
		{"stale presence", true, &stale, ProofFacts{}, StageOffline},
		{"fresh, nothing yet", true, &fresh, ProofFacts{}, StageConnected},
		{"open proof", true, &fresh, ProofFacts{HasOpen: true}, StageChecking},
		{"open proof while offline still checking", true, &stale, ProofFacts{HasOpen: true}, StageChecking},
		{"passed once", true, &fresh, ProofFacts{HasPassed: true, LastFinishedStatus: "failed"}, StageOperational},
		{"passed but offline", true, &stale, ProofFacts{HasPassed: true}, StageOffline},
		{"last failed, never passed", true, &fresh, ProofFacts{LastFinishedStatus: "failed"}, StageCheckFailed},
		{"last infra_error, never passed", true, &fresh, ProofFacts{LastFinishedStatus: "infra_error"}, StageConnected},
	}
	for _, c := range cases {
		if got := ComputeStage(c.hasKey, c.seen, now, c.facts); got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Реализация стадии**

`backend/internal/agents/stage.go`:

```go
package agents

import (
	"context"
	"time"
)

const (
	StageRegistered  = "registered"
	StageOffline     = "offline"
	StageConnected   = "connected"
	StageChecking    = "checking"
	StageOperational = "operational"
	StageCheckFailed = "check_failed"

	presenceTTL = 2 * time.Minute
)

// ProofFacts is what the stage needs to know about an agent's proofs. The
// proofs module implements ProofFactsSource; agents does not import it.
type ProofFacts struct {
	HasPassed          bool
	HasOpen            bool
	LastFinishedStatus string // "" | passed | failed | infra_error | expired
}

type ProofFactsSource interface {
	ProofFacts(ctx context.Context, agentID string) (ProofFacts, error)
}

// NoProofFacts is the source used before the proofs module is wired.
type NoProofFacts struct{}

func (NoProofFacts) ProofFacts(context.Context, string) (ProofFacts, error) { return ProofFacts{}, nil }

// ComputeStage is the single definition of an agent's stage (spec §3.3).
// A proof in flight keeps the agent in "checking" even if the connector
// went quiet: the proof will expire on its own and the stage will follow.
func ComputeStage(hasKey bool, lastSeen *time.Time, now time.Time, f ProofFacts) string {
	if f.HasOpen {
		return StageChecking
	}
	if !hasKey || lastSeen == nil {
		return StageRegistered
	}
	if now.Sub(*lastSeen) > presenceTTL {
		return StageOffline
	}
	if f.HasPassed {
		return StageOperational
	}
	if f.LastFinishedStatus == "failed" {
		return StageCheckFailed
	}
	return StageConnected
}
```

Run: `go test ./internal/agents/ -run TestComputeStage -v` → PASS.

- [ ] **Step 3: Presence и overview**

Добавить в `backend/internal/agents/model.go`:

```go
type Presence struct {
	LastSeenAt       time.Time `json:"last_seen_at"`
	ConnectorVersion string    `json:"connector_version"`
	Hostname         string    `json:"hostname"`
}

type Overview struct {
	Private
	Stage    string    `json:"stage"`
	Presence *Presence `json:"presence"`
}

type heartbeatInput struct {
	ConnectorVersion string `json:"connector_version"`
	Hostname         string `json:"hostname"`
}
```

`backend/internal/agents/presence.go`:

```go
package agents

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Heartbeat records that the connector is alive. The row is rewritten at
// most every 10 seconds so a chatty connector does not turn into writes.
func (s *Service) Heartbeat(ctx context.Context, agentID, version, hostname string) error {
	if len(version) > 40 {
		version = version[:40]
	}
	if len(hostname) > 80 {
		hostname = hostname[:80]
	}
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO agent_presence (agent_id, last_seen_at, connector_version, hostname) VALUES ($1, now(), $2, $3)
			ON CONFLICT (agent_id) DO UPDATE SET last_seen_at = now(), connector_version = $2, hostname = $3
			WHERE agent_presence.last_seen_at < now() - interval '10 seconds'`, agentID, version, hostname)
		return err
	})
}

func (s *Service) presence(ctx context.Context, tx pgx.Tx, agentID string) (*Presence, error) {
	var p Presence
	err := tx.QueryRow(ctx, `SELECT last_seen_at, connector_version, hostname FROM agent_presence WHERE agent_id = $1`, agentID).
		Scan(&p.LastSeenAt, &p.ConnectorVersion, &p.Hostname)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.LastSeenAt = p.LastSeenAt.UTC()
	return &p, nil
}

func (s *Service) overview(ctx context.Context, a Agent) (Overview, error) {
	p, err := s.private(ctx, a)
	if err != nil {
		return Overview{}, err
	}
	var pr *Presence
	err = s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		pr, err = s.presence(ctx, tx, a.ID)
		return err
	})
	if err != nil {
		return Overview{}, err
	}
	facts, err := s.proofs.ProofFacts(ctx, a.ID)
	if err != nil {
		return Overview{}, err
	}
	var seen *time.Time
	if pr != nil {
		seen = &pr.LastSeenAt
	}
	return Overview{Private: p, Presence: pr, Stage: ComputeStage(len(p.APIKeys) > 0, seen, time.Now(), facts)}, nil
}

// Overview returns nil, nil when the user has no agent yet.
func (s *Service) Overview(ctx context.Context, userID string) (*Overview, error) {
	a, err := s.byOwner(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	o, err := s.overview(ctx, a)
	return &o, err
}

func (s *Service) OverviewByID(ctx context.Context, agentID string) (Overview, error) {
	a, err := s.ByID(ctx, agentID)
	if err != nil {
		return Overview{}, err
	}
	return s.overview(ctx, a)
}
```

В `service.go`: `type Service struct { pool *db.Pool; proofs ProofFactsSource }`, `NewService(pool *db.Pool, proofs ProofFactsSource) *Service`; удалить `PrivateForUser` и `MeAgent`.

- [ ] **Step 4: Маршруты**

`backend/internal/agents/http.go` целиком:

```go
package agents

import (
	"net/http"

	"tolerance/internal/identity"
	"tolerance/internal/platform/httpx"
)

type createKeyInput struct {
	Name string `json:"name"`
}

func decode[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var in T
	raw, err := httpx.ReadBody(w, r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return in, false
	}
	if err := httpx.Decode(raw, &in); err != nil {
		httpx.WriteError(w, r, err)
		return in, false
	}
	return in, true
}

// RegisterOwnerRoutes mounts the cabinet's agent routes (cookie session).
func RegisterOwnerRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("POST /api/v1/agent", func(w http.ResponseWriter, r *http.Request) {
		in, ok := decode[CreateInput](w, r)
		if !ok {
			return
		}
		p, err := s.Create(r.Context(), identity.MustFromContext(r.Context()).UserID, in)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusCreated, p)
	})
	mux.HandleFunc("PATCH /api/v1/agent", func(w http.ResponseWriter, r *http.Request) {
		in, ok := decode[PatchInput](w, r)
		if !ok {
			return
		}
		p, err := s.Patch(r.Context(), identity.MustFromContext(r.Context()).UserID, in)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, p)
	})
	mux.HandleFunc("POST /api/v1/agent/keys", func(w http.ResponseWriter, r *http.Request) {
		in, ok := decode[createKeyInput](w, r)
		if !ok {
			return
		}
		kv, key, err := s.CreateKey(r.Context(), identity.MustFromContext(r.Context()).UserID, in.Name)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusCreated, map[string]any{"id": kv.ID, "prefix": kv.Prefix, "name": kv.Name, "created_at": kv.CreatedAt, "key": key})
	})
	mux.HandleFunc("DELETE /api/v1/agent/keys/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := s.RevokeKey(r.Context(), identity.MustFromContext(r.Context()).UserID, r.PathValue("id")); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// RegisterConnectorRoutes mounts the connector's heartbeat (API key).
func RegisterConnectorRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("POST /api/v1/connector/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		in, ok := decode[heartbeatInput](w, r)
		if !ok {
			return
		}
		agentID := identity.MustFromContext(r.Context()).AgentID
		if err := s.Heartbeat(r.Context(), agentID, in.ConnectorVersion, in.Hostname); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		o, err := s.OverviewByID(r.Context(), agentID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"agent": map[string]any{"id": o.ID, "name": o.Name, "stage": o.Stage}})
	})
}
```

В `backend/internal/identity/http.go` добавить:

```go
// RegisterMeRoute mounts GET /me. The agent part comes from the agents
// module as an opaque value so identity does not import it.
func RegisterMeRoute(mux *http.ServeMux, s *Service, agentFor func(ctx context.Context, userID string) (any, error)) {
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, r *http.Request) {
		actor := MustFromContext(r.Context())
		u, err := s.Get(r.Context(), actor.UserID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		agent, err := agentFor(r.Context(), actor.UserID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"user": u, "agent": agent})
	})
}
```

(добавить `"context"` в импорты.) В `agents/service.go` адаптер для JSON `null`:

```go
// MeAgent adapts Overview for identity.RegisterMeRoute: an untyped nil
// interface encodes as JSON null, a typed nil pointer would not.
func (s *Service) MeAgent(ctx context.Context, userID string) (any, error) {
	o, err := s.Overview(ctx, userID)
	if err != nil || o == nil {
		return nil, err
	}
	return o, nil
}
```

В `backend/cmd/api/handler.go` внутри `newHandler`:

```go
	owner := http.NewServeMux()
	identity.RegisterMeRoute(owner, d.users, d.agents.MeAgent)
	agents.RegisterOwnerRoutes(owner, d.agents)

	connector := http.NewServeMux()
	agents.RegisterConnectorRoutes(connector, d.agents)

	session := identity.RequireSession(d.users)
	api.Handle("/api/v1/me", session(owner))
	api.Handle("/api/v1/agent", session(owner))
	api.Handle("/api/v1/agent/", session(owner))
	api.Handle("/api/v1/connector/", identity.RequireAgent(d.agents)(connector))
```

В `main.go`/`main_test.go`: `agents.NewService(pool, agents.NoProofFacts{})`.

- [ ] **Step 5: Интеграционный тест**

`backend/internal/agents/agents_integration_test.go`:

```go
package agents_test

import (
	"context"
	"testing"

	"tolerance/internal/agents"
	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
)

func TestAgentLifecycle_StageFollowsKeysAndPresence(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	users := identity.NewService(d.AppPool, nil)
	u, _, err := users.Signup(ctx, "o@example.com", "longenough1")
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	s := agents.NewService(d.AppPool, agents.NoProofFacts{})

	if o, err := s.Overview(ctx, u.ID); err != nil || o != nil {
		t.Fatalf("expected no agent yet, got %+v %v", o, err)
	}
	p, err := s.Create(ctx, u.ID, agents.CreateInput{Name: "fixer-7", Description: "go backend"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.Create(ctx, u.ID, agents.CreateInput{Name: "second"}); err == nil {
		t.Fatalf("second agent must be rejected")
	}
	o, _ := s.Overview(ctx, u.ID)
	if o.Stage != agents.StageRegistered {
		t.Fatalf("no key: want registered, got %s", o.Stage)
	}
	kv, key, err := s.CreateKey(ctx, u.ID, "laptop")
	if err != nil || key == "" {
		t.Fatalf("create key: %v", err)
	}
	o, _ = s.Overview(ctx, u.ID)
	if o.Stage != agents.StageRegistered {
		t.Fatalf("key but no heartbeat: want registered, got %s", o.Stage)
	}
	if err := s.Heartbeat(ctx, p.ID, "0.1.0", "laptop.local"); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	o, _ = s.Overview(ctx, u.ID)
	if o.Stage != agents.StageConnected || o.Presence == nil || o.Presence.Hostname != "laptop.local" {
		t.Fatalf("after heartbeat: %+v", o)
	}
	if err := s.RevokeKey(ctx, u.ID, kv.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	o, _ = s.Overview(ctx, u.ID)
	if o.Stage != agents.StageRegistered {
		t.Fatalf("no active key: want registered, got %s", o.Stage)
	}
}
```

Run: `ARENA_TEST_REQUIRE_DOCKER=1 go test ./internal/agents/ -v` → PASS.

- [ ] **Step 6: Коммит**

```bash
cd backend && gofmt -l . && go vet ./... && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./...
git add -A backend && git commit -m "Add agent presence, stage computation and owner/connector routes

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 4: Каталог проверочных задач и первая задача

**Files:**
- Create: `backend/fixtures/proofs/go-fix-retry/manifest.json`, `.../TASK.md`, `.../Dockerfile`, `.../repo/go.mod`, `.../repo/retry.go`, `.../repo/retry_test.go`, `.../_hidden/retry_hidden_test.go`, `backend/internal/proofs/catalog.go`, `backend/internal/proofs/catalog_test.go`, `backend/internal/proofs/model.go`
- Modify: `backend/cmd/migrate/main.go`, `backend/Dockerfile`, `backend/dev/entrypoint.sh`

**Interfaces:**
- Produces: `proofs.Task{Slug, Title, Language, Image, RunCmd string; AgentTimeoutS, SandboxTimeoutS, VisibleTests, HiddenTests int; TaskMD string; RepoTar, HiddenTar []byte; RepoSHA256 string}`; `proofs.LoadCatalog(dir string) ([]Task, error)`; `proofs.SyncCatalog(ctx, pool *db.Pool, tasks []Task) error`; `proofs.TarDir(dir string) ([]byte, error)`; `proofs.Untar(data []byte, dst string) error`.

- [ ] **Step 1: Задача go-fix-retry**

`backend/fixtures/proofs/go-fix-retry/manifest.json`:

```json
{
  "slug": "go-fix-retry",
  "title": "Fix exponential backoff in a Go retry helper",
  "language": "go",
  "image": "arena-proof-go:1",
  "run_cmd": "go test ./... -json -count=1",
  "agent_timeout_s": 900,
  "sandbox_timeout_s": 120,
  "visible_tests": 3,
  "hidden_tests": 5
}
```

`backend/fixtures/proofs/go-fix-retry/TASK.md`:

```markdown
# Fix the retry helper

This repository contains a tiny Go package `retry` with two functions:

- `Backoff(attempt int) time.Duration` must return the delay before attempt `attempt`
  (1-based): `Base` for attempt 1, doubled for every further attempt, and never more
  than `Max`. Attempt 0 or negative returns 0.
- `Do(ctx, maxAttempts, fn)` calls `fn` until it returns nil, sleeping `Backoff(i)`
  between attempts, gives up after `maxAttempts` returning the last error, and
  returns `ctx.Err()` if the context is cancelled while waiting.

`go test ./...` currently fails. Make all tests pass without changing the tests.
Hidden tests check the same contract on more inputs. Do not add dependencies.
```

`backend/fixtures/proofs/go-fix-retry/Dockerfile`:

```dockerfile
FROM golang:1.27-alpine
ENV GOTOOLCHAIN=local GOFLAGS=-mod=mod CGO_ENABLED=0
WORKDIR /work
```

`backend/fixtures/proofs/go-fix-retry/repo/go.mod`:

```
module retry

go 1.22
```

`backend/fixtures/proofs/go-fix-retry/repo/retry.go`:

```go
// Package retry retries an operation with exponential backoff.
package retry

import (
	"context"
	"time"
)

const (
	Base = 100 * time.Millisecond
	Max  = 30 * time.Second
)

// after is a hook so tests can run without real sleeping.
var after = time.After

// Backoff returns the delay before attempt n (1-based).
func Backoff(attempt int) time.Duration {
	if attempt < 1 {
		return 0
	}
	return Base * time.Duration(attempt)
}

// Do calls fn until it succeeds or maxAttempts is used up.
func Do(ctx context.Context, maxAttempts int, fn func() error) error {
	var err error
	for i := 1; i <= maxAttempts; i++ {
		if err = fn(); err == nil {
			return nil
		}
		if i == maxAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-after(Backoff(i)):
		}
	}
	return err
}
```

`backend/fixtures/proofs/go-fix-retry/repo/retry_test.go`:

```go
package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func fast(t *testing.T) {
	t.Helper()
	old := after
	after = func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}
	t.Cleanup(func() { after = old })
}

func TestDo_SucceedsFirstTry(t *testing.T) {
	fast(t)
	calls := 0
	if err := Do(context.Background(), 3, func() error { calls++; return nil }); err != nil || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestDo_RetriesUntilSuccess(t *testing.T) {
	fast(t)
	calls := 0
	err := Do(context.Background(), 5, func() error {
		calls++
		if calls < 3 {
			return errors.New("not yet")
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestBackoff_Doubles(t *testing.T) {
	want := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond}
	for i, w := range want {
		if got := Backoff(i + 1); got != w {
			t.Fatalf("Backoff(%d) = %v, want %v", i+1, got, w)
		}
	}
}
```

`backend/fixtures/proofs/go-fix-retry/_hidden/retry_hidden_test.go`:

```go
package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestHidden_BackoffSequence(t *testing.T) {
	for i, want := range []time.Duration{100e6, 200e6, 400e6, 800e6, 1600e6, 3200e6} {
		if got := Backoff(i + 1); got != want {
			t.Fatalf("Backoff(%d) = %v, want %v", i+1, got, want)
		}
	}
}

func TestHidden_BackoffCapsAtMax(t *testing.T) {
	for _, n := range []int{10, 20, 40, 1000} {
		if got := Backoff(n); got != Max {
			t.Fatalf("Backoff(%d) = %v, want cap %v", n, got, Max)
		}
	}
}

func TestHidden_BackoffZeroAndNegative(t *testing.T) {
	if Backoff(0) != 0 || Backoff(-3) != 0 {
		t.Fatalf("non-positive attempts must return 0")
	}
}

func TestHidden_DoStopsAtMaxAttempts(t *testing.T) {
	fast(t)
	calls := 0
	boom := errors.New("boom")
	err := Do(context.Background(), 4, func() error { calls++; return boom })
	if !errors.Is(err, boom) || calls != 4 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestHidden_DoReturnsContextErrorWhileWaiting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := Do(ctx, 3, func() error {
		calls++
		cancel()
		return errors.New("fail")
	})
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}
```

Проверка фикстуры руками: `cd backend/fixtures/proofs/go-fix-retry && mkdir -p /tmp/rt && cp -r repo/. /tmp/rt && cp _hidden/*.go /tmp/rt && (cd /tmp/rt && go test ./... 2>&1 | tail -5)` → `FAIL` (падают `TestBackoff_Doubles`, `TestHidden_BackoffSequence`, `TestHidden_BackoffCapsAtMax`). Эталонное исправление (для теста песочницы в задаче 7): заменить тело `Backoff` на

```go
	if attempt < 1 {
		return 0
	}
	if attempt > 20 {
		return Max
	}
	d := Base << uint(attempt-1)
	if d > Max {
		return Max
	}
	return d
```

и убедиться, что с ним все 8 тестов зелёные.

- [ ] **Step 2: Тест каталога**

`backend/internal/proofs/catalog_test.go`:

```go
package proofs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCatalog_ReadsFixtureAndTarsAreRestorable(t *testing.T) {
	tasks, err := LoadCatalog(filepath.Join("..", "..", "fixtures", "proofs"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Slug != "go-fix-retry" {
		t.Fatalf("unexpected catalog: %+v", tasks)
	}
	task := tasks[0]
	if task.Image == "" || task.RunCmd == "" || task.AgentTimeoutS == 0 || task.TaskMD == "" || task.RepoSHA256 == "" {
		t.Fatalf("manifest fields missing: %+v", task)
	}
	dst := t.TempDir()
	if err := Untar(task.RepoTar, dst); err != nil {
		t.Fatalf("untar repo: %v", err)
	}
	if err := Untar(task.HiddenTar, dst); err != nil {
		t.Fatalf("untar hidden: %v", err)
	}
	for _, f := range []string{"go.mod", "retry.go", "retry_test.go", "retry_hidden_test.go"} {
		if _, err := os.Stat(filepath.Join(dst, f)); err != nil {
			t.Fatalf("missing %s after untar: %v", f, err)
		}
	}
}

func TestUntar_RejectsPathTraversal(t *testing.T) {
	evil := tarWithEntry(t, "../escape.txt", "x")
	if err := Untar(evil, t.TempDir()); err == nil {
		t.Fatalf("expected traversal to be rejected")
	}
}
```

Хелпер `tarWithEntry` в том же файле:

```go
func tarWithEntry(t *testing.T, name, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}
```

(импорты `archive/tar`, `bytes`, `compress/gzip`.)

- [ ] **Step 3: Модель и каталог**

`backend/internal/proofs/model.go`:

```go
package proofs

import "time"

const (
	StatusQueued         = "queued"
	StatusClaimed        = "claimed"
	StatusRunningAgent   = "running_agent"
	StatusDiffSubmitted  = "diff_submitted"
	StatusRunningSandbox = "running_sandbox"
	StatusPassed         = "passed"
	StatusFailed         = "failed"
	StatusInfraError     = "infra_error"
	StatusExpired        = "expired"

	maxDiffBytes    = 256 << 10
	maxLogTailBytes = 32 << 10
	dailyLimit      = 10
	claimTimeout    = 5 * time.Minute
)

var openStatuses = []string{StatusQueued, StatusClaimed, StatusRunningAgent, StatusDiffSubmitted, StatusRunningSandbox}

type Task struct {
	Slug            string `json:"slug"`
	Title           string `json:"title"`
	Language        string `json:"language"`
	Image           string `json:"-"`
	RunCmd          string `json:"-"`
	AgentTimeoutS   int    `json:"agent_timeout_s"`
	SandboxTimeoutS int    `json:"sandbox_timeout_s"`
	VisibleTests    int    `json:"visible_tests"`
	HiddenTests     int    `json:"hidden_tests"`
	TaskMD          string `json:"task_md"`
	RepoTar         []byte `json:"-"`
	HiddenTar       []byte `json:"-"`
	RepoSHA256      string `json:"repo_sha256"`
}

type TestResult struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
}

type SandboxResult struct {
	Tests    []TestResult `json:"tests"`
	ExitCode int          `json:"exit_code"`
	Output   string       `json:"output"`
	TimedOut bool         `json:"timed_out"`
}

type Proof struct {
	ID              string         `json:"id"`
	AgentID         string         `json:"agent_id"`
	TaskSlug        string         `json:"task_slug"`
	Status          string         `json:"status"`
	CreatedAt       time.Time      `json:"created_at"`
	ClaimedAt       *time.Time     `json:"claimed_at"`
	DiffSubmittedAt *time.Time     `json:"diff_submitted_at"`
	FinishedAt      *time.Time     `json:"finished_at"`
	Diff            string         `json:"diff"`
	AgentLogTail    string         `json:"agent_log_tail"`
	AgentDurationMS *int           `json:"agent_duration_ms"`
	AgentExitCode   *int           `json:"agent_exit_code"`
	SandboxResult   *SandboxResult `json:"sandbox_result"`
	FailureReason   string         `json:"failure_reason"`
}
```

`backend/internal/proofs/catalog.go`:

```go
package proofs

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
)

type manifest struct {
	Slug            string `json:"slug"`
	Title           string `json:"title"`
	Language        string `json:"language"`
	Image           string `json:"image"`
	RunCmd          string `json:"run_cmd"`
	AgentTimeoutS   int    `json:"agent_timeout_s"`
	SandboxTimeoutS int    `json:"sandbox_timeout_s"`
	VisibleTests    int    `json:"visible_tests"`
	HiddenTests     int    `json:"hidden_tests"`
}

// LoadCatalog reads every task directory under dir: manifest.json, TASK.md,
// repo/ (what the agent gets) and _hidden/ (copied over repo/ before the
// sandbox run). The underscore keeps the Go toolchain out of _hidden.
func LoadCatalog(dir string) ([]Task, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("proofs: read catalog %s: %w", dir, err)
	}
	var out []Task
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		t, err := loadTask(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

func loadTask(dir string) (Task, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return Task{}, fmt.Errorf("proofs: %s: %w", dir, err)
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return Task{}, fmt.Errorf("proofs: %s/manifest.json: %w", dir, err)
	}
	if m.Slug == "" || m.Image == "" || m.RunCmd == "" || m.AgentTimeoutS <= 0 || m.SandboxTimeoutS <= 0 {
		return Task{}, fmt.Errorf("proofs: %s/manifest.json: slug, image, run_cmd and timeouts are required", dir)
	}
	taskMD, err := os.ReadFile(filepath.Join(dir, "TASK.md"))
	if err != nil {
		return Task{}, fmt.Errorf("proofs: %s: %w", dir, err)
	}
	repoTar, err := TarDir(filepath.Join(dir, "repo"))
	if err != nil {
		return Task{}, err
	}
	hiddenTar, err := TarDir(filepath.Join(dir, "_hidden"))
	if err != nil {
		return Task{}, err
	}
	sum := sha256.Sum256(repoTar)
	return Task{Slug: m.Slug, Title: m.Title, Language: m.Language, Image: m.Image, RunCmd: m.RunCmd,
		AgentTimeoutS: m.AgentTimeoutS, SandboxTimeoutS: m.SandboxTimeoutS, VisibleTests: m.VisibleTests, HiddenTests: m.HiddenTests,
		TaskMD: string(taskMD), RepoTar: repoTar, HiddenTar: hiddenTar, RepoSHA256: hex.EncodeToString(sum[:])}, nil
}

// TarDir packs dir into a deterministic gzip tarball with paths relative to
// dir. Deterministic (sorted, zero mtimes) so the sha256 is stable across
// deploys and the connector can verify it.
func TarDir(dir string) ([]byte, error) {
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("proofs: walk %s: %w", dir, err)
	}
	sort.Strings(files)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, p := range files {
		rel, _ := filepath.Rel(dir, p)
		body, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		if err := tw.WriteHeader(&tar.Header{Name: filepath.ToSlash(rel), Mode: 0o644, Size: int64(len(body))}); err != nil {
			return nil, err
		}
		if _, err := tw.Write(body); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Untar extracts a tarball produced by TarDir into dst, refusing entries
// that would escape dst.
func Untar(data []byte, dst string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("proofs: untar: %w", err)
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("proofs: untar: %w", err)
		}
		clean := filepath.Clean(h.Name)
		if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
			return fmt.Errorf("proofs: untar: illegal path %q", h.Name)
		}
		target := filepath.Join(dst, clean)
		if h.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, io.LimitReader(tr, 16<<20)); err != nil {
			f.Close()
			return err
		}
		f.Close()
	}
}

// SyncCatalog upserts tasks into proof_tasks. Run by cmd/migrate.
func SyncCatalog(ctx context.Context, pool *db.Pool, tasks []Task) error {
	return pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		for _, t := range tasks {
			_, err := tx.Exec(ctx, `
				INSERT INTO proof_tasks (slug, title, language, image, run_cmd, agent_timeout_s, sandbox_timeout_s,
				    visible_tests, hidden_tests, task_md, repo_tar, hidden_tar, repo_sha256, updated_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13, now())
				ON CONFLICT (slug) DO UPDATE SET title = $2, language = $3, image = $4, run_cmd = $5, agent_timeout_s = $6,
				    sandbox_timeout_s = $7, visible_tests = $8, hidden_tests = $9, task_md = $10, repo_tar = $11,
				    hidden_tar = $12, repo_sha256 = $13, updated_at = now()`,
				t.Slug, t.Title, t.Language, t.Image, t.RunCmd, t.AgentTimeoutS, t.SandboxTimeoutS,
				t.VisibleTests, t.HiddenTests, t.TaskMD, t.RepoTar, t.HiddenTar, t.RepoSHA256)
			if err != nil {
				return fmt.Errorf("proofs: sync %s: %w", t.Slug, err)
			}
		}
		return nil
	})
}
```

Run: `go test ./internal/proofs/ -v` → PASS.

- [ ] **Step 4: Синхронизация в cmd/migrate и образ**

`backend/cmd/migrate/main.go` после `db.Migrate`:

```go
	if dir := os.Getenv("ARENA_PROOFS_DIR"); dir != "" {
		tasks, err := proofs.LoadCatalog(dir)
		if err != nil {
			log.Fatalf("catalog: %v", err)
		}
		pool, err := db.Open(context.Background(), dsn)
		if err != nil {
			log.Fatalf("open: %v", err)
		}
		defer pool.Close()
		if err := proofs.SyncCatalog(context.Background(), pool, tasks); err != nil {
			log.Fatalf("catalog: %v", err)
		}
		log.Printf("catalog: %d task(s) synced", len(tasks))
	}
```

`backend/Dockerfile`: в стадии `build` убрать сборку `seed`; в рантайм-стадии `RUN apk add --no-cache ca-certificates tzdata git docker-cli && adduser -D -u 10001 arena`, добавить `COPY fixtures/proofs /opt/arena/proofs` и `ENV ARENA_PROOFS_DIR=/opt/arena/proofs`. `backend/dev/entrypoint.sh`: убрать строку `seed -task city-day-planner`. `backend/Makefile` цель `migrate`: добавить `ARENA_PROOFS_DIR=./fixtures/proofs`.

- [ ] **Step 5: Коммит**

```bash
cd backend && gofmt -l . && go vet ./... && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./...
git add -A backend && git commit -m "Add proof task catalog and the go-fix-retry task

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 5: Сервис проверок: создание, список, лимиты, факты для стадии

**Files:**
- Create: `backend/internal/proofs/service.go`, `backend/internal/proofs/http_me.go`, `backend/internal/proofs/service_integration_test.go`
- Modify: `backend/cmd/api/handler.go`, `backend/cmd/api/main.go`, `backend/cmd/api/main_test.go`

**Interfaces:**
- Consumes: `agents.ProofFactsSource`, `agents.ComputeStage`, `agents.Presence`.
- Produces: `proofs.NewService(pool *db.Pool) *Service`; `(*Service).Tasks(ctx) ([]Task, error)` (без tar); `(*Service).Create(ctx, userID, slug) (Proof, error)`; `(*Service).List(ctx, userID) ([]Proof, error)`; `(*Service).Get(ctx, userID, id) (Proof, error)`; `(*Service).Retry(ctx, userID, id) (Proof, error)`; `(*Service).ProofFacts(ctx, agentID) (agents.ProofFacts, error)`; `proofs.RegisterOwnerRoutes(mux, s)`; ошибки `agent_offline` (409), `proof_in_progress` (409), `daily_limit` (429), `no_agent` (404).

- [ ] **Step 1: Интеграционный тест**

`backend/internal/proofs/service_integration_test.go`:

```go
package proofs_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/agents"
	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/proofs"
)

type fixture struct {
	d      *dbtest.DB
	users  *identity.Service
	agents *agents.Service
	proofs *proofs.Service
	userID string
	agent  string
}

func setup(t *testing.T) fixture {
	t.Helper()
	d := dbtest.New(t)
	ctx := context.Background()
	tasks, err := proofs.LoadCatalog(filepath.Join("..", "..", "fixtures", "proofs"))
	if err != nil {
		t.Fatal(err)
	}
	if err := proofs.SyncCatalog(ctx, d.AdminPool, tasks); err != nil {
		t.Fatal(err)
	}
	ps := proofs.NewService(d.AppPool)
	as := agents.NewService(d.AppPool, ps)
	us := identity.NewService(d.AppPool, nil)
	u, _, err := us.Signup(ctx, "o@example.com", "longenough1")
	if err != nil {
		t.Fatal(err)
	}
	a, err := as.Create(ctx, u.ID, agents.CreateInput{Name: "fixer"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := as.CreateKey(ctx, u.ID, "k"); err != nil {
		t.Fatal(err)
	}
	return fixture{d: d, users: us, agents: as, proofs: ps, userID: u.ID, agent: a.ID}
}

func problem(t *testing.T, err error, status int, code string) {
	t.Helper()
	var p *httpx.Problem
	if !errors.As(err, &p) || p.Status != status || p.Code != code {
		t.Fatalf("expected %d %s, got %v", status, code, err)
	}
}

func TestCreateProof_RequiresOnlineAgentAndOneAtATime(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	_, err := f.proofs.Create(ctx, f.userID, "go-fix-retry")
	problem(t, err, 409, "agent_offline")

	if err := f.agents.Heartbeat(ctx, f.agent, "0.1", "h"); err != nil {
		t.Fatal(err)
	}
	p, err := f.proofs.Create(ctx, f.userID, "go-fix-retry")
	if err != nil || p.Status != proofs.StatusQueued {
		t.Fatalf("create: %v %+v", err, p)
	}
	_, err = f.proofs.Create(ctx, f.userID, "go-fix-retry")
	problem(t, err, 409, "proof_in_progress")

	_, err = f.proofs.Create(ctx, f.userID, "nope")
	problem(t, err, 404, "not_found")

	facts, err := f.proofs.ProofFacts(ctx, f.agent)
	if err != nil || !facts.HasOpen || facts.HasPassed {
		t.Fatalf("facts: %v %+v", err, facts)
	}
	o, _ := f.agents.Overview(ctx, f.userID)
	if o.Stage != agents.StageChecking {
		t.Fatalf("stage: %s", o.Stage)
	}

	got, err := f.proofs.Get(ctx, f.userID, p.ID)
	if err != nil || got.ID != p.ID {
		t.Fatalf("get: %v", err)
	}
	list, _ := f.proofs.List(ctx, f.userID)
	if len(list) != 1 {
		t.Fatalf("list: %d", len(list))
	}
}

func TestCreateProof_DailyLimit(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_ = f.agents.Heartbeat(ctx, f.agent, "0.1", "h")
	for i := 0; i < 10; i++ {
		p, err := f.proofs.Create(ctx, f.userID, "go-fix-retry")
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		// finish it so the next one can start
		err = f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE proofs SET status = 'failed', finished_at = now() WHERE id = $1`, p.ID)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err := f.proofs.Create(ctx, f.userID, "go-fix-retry")
	problem(t, err, 429, "daily_limit")
	o, _ := f.agents.Overview(ctx, f.userID)
	if o.Stage != agents.StageCheckFailed {
		t.Fatalf("stage after failures: %s", o.Stage)
	}
}

func TestRetry_OnlyForInfraErrorOrExpired(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_ = f.agents.Heartbeat(ctx, f.agent, "0.1", "h")
	p, _ := f.proofs.Create(ctx, f.userID, "go-fix-retry")
	_, err := f.proofs.Retry(ctx, f.userID, p.ID)
	problem(t, err, 409, "state_conflict")
	_ = f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE proofs SET status = 'infra_error', finished_at = now(), failure_reason = 'x' WHERE id = $1`, p.ID)
		return err
	})
	r, err := f.proofs.Retry(ctx, f.userID, p.ID)
	if err != nil || r.Status != proofs.StatusQueued || r.FailureReason != "" || r.FinishedAt != nil {
		t.Fatalf("retry: %v %+v", err, r)
	}
}
```

- [ ] **Step 2: Сервис**

`backend/internal/proofs/service.go`:

```go
package proofs

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"tolerance/internal/agents"
	"tolerance/internal/platform/audit"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
)

type Service struct{ pool *db.Pool }

func NewService(pool *db.Pool) *Service { return &Service{pool: pool} }

const proofCols = `id, agent_id, task_slug, status, created_at, claimed_at, diff_submitted_at, finished_at,
	diff, agent_log_tail, agent_duration_ms, agent_exit_code, sandbox_result, failure_reason`

func utcp(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func scanProof(row interface{ Scan(...any) error }, p *Proof) error {
	if err := row.Scan(&p.ID, &p.AgentID, &p.TaskSlug, &p.Status, &p.CreatedAt, &p.ClaimedAt, &p.DiffSubmittedAt, &p.FinishedAt,
		&p.Diff, &p.AgentLogTail, &p.AgentDurationMS, &p.AgentExitCode, &p.SandboxResult, &p.FailureReason); err != nil {
		return err
	}
	p.CreatedAt = p.CreatedAt.UTC()
	p.ClaimedAt, p.DiffSubmittedAt, p.FinishedAt = utcp(p.ClaimedAt), utcp(p.DiffSubmittedAt), utcp(p.FinishedAt)
	return nil
}

func (s *Service) agentOf(ctx context.Context, tx pgx.Tx, userID string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT id FROM agents WHERE owner_user_id = $1`, userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", httpx.New(http.StatusNotFound, "no_agent", "Create an agent first")
	}
	return id, err
}

// Tasks lists the catalog without tarballs.
func (s *Service) Tasks(ctx context.Context) ([]Task, error) {
	out := []Task{}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT slug, title, language, agent_timeout_s, sandbox_timeout_s, visible_tests, hidden_tests, task_md, repo_sha256 FROM proof_tasks ORDER BY slug`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var t Task
			if err := rows.Scan(&t.Slug, &t.Title, &t.Language, &t.AgentTimeoutS, &t.SandboxTimeoutS, &t.VisibleTests, &t.HiddenTests, &t.TaskMD, &t.RepoSHA256); err != nil {
				return err
			}
			out = append(out, t)
		}
		return rows.Err()
	})
	return out, err
}

// Create queues a proof for the caller's agent. The agent must be online
// (presence within 2 minutes), have no proof in flight and be under the
// daily limit; the partial unique index is the last word on "in flight".
func (s *Service) Create(ctx context.Context, userID, slug string) (Proof, error) {
	var p Proof
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		agentID, err := s.agentOf(ctx, tx, userID)
		if err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM proof_tasks WHERE slug = $1)`, slug).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return httpx.NotFound()
		}
		var online bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM agent_presence WHERE agent_id = $1 AND last_seen_at > now() - interval '2 minutes')`, agentID).Scan(&online); err != nil {
			return err
		}
		if !online {
			return httpx.New(http.StatusConflict, "agent_offline", "The connector is not online; run `arena connect` first")
		}
		var today int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM proofs WHERE agent_id = $1 AND created_at > now() - interval '24 hours'`, agentID).Scan(&today); err != nil {
			return err
		}
		if today >= dailyLimit {
			return httpx.New(http.StatusTooManyRequests, "daily_limit", "At most 10 proofs per day per agent")
		}
		if err := scanProof(tx.QueryRow(ctx, `INSERT INTO proofs (id, agent_id, task_slug) VALUES ($1, $2, $3) RETURNING `+proofCols,
			idgen.New("proof"), agentID, slug), &p); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: userID, Action: "proof.created", AggregateKind: "proof", AggregateID: p.ID, RequestID: httpx.RequestID(ctx)})
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "proofs_one_open_idx") {
		return Proof{}, httpx.New(http.StatusConflict, "proof_in_progress", "A proof is already in progress")
	}
	return p, err
}

func (s *Service) List(ctx context.Context, userID string) ([]Proof, error) {
	out := []Proof{}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		agentID, err := s.agentOf(ctx, tx, userID)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT `+proofCols+` FROM proofs WHERE agent_id = $1 ORDER BY created_at DESC LIMIT 50`, agentID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p Proof
			if err := scanProof(rows, &p); err != nil {
				return err
			}
			p.Diff, p.AgentLogTail = "", "" // list view stays light
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Service) Get(ctx context.Context, userID, id string) (Proof, error) {
	var p Proof
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return scanProof(tx.QueryRow(ctx, `SELECT `+proofCols+` FROM proofs p WHERE p.id = $1
			AND p.agent_id = (SELECT id FROM agents WHERE owner_user_id = $2)`, id, userID), &p)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Proof{}, httpx.NotFound()
	}
	return p, err
}

// Retry re-queues a proof that ended in infra_error or expired, clearing
// everything the previous run produced.
func (s *Service) Retry(ctx context.Context, userID, id string) (Proof, error) {
	var p Proof
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var status string
		err := tx.QueryRow(ctx, `SELECT status FROM proofs WHERE id = $1 AND agent_id = (SELECT id FROM agents WHERE owner_user_id = $2) FOR UPDATE`, id, userID).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound()
		}
		if err != nil {
			return err
		}
		if status != StatusInfraError && status != StatusExpired {
			return httpx.StateConflict("Only proofs that ended in infra_error or expired can be retried")
		}
		return scanProof(tx.QueryRow(ctx, `UPDATE proofs SET status = 'queued', created_at = now(), claimed_at = NULL, diff_submitted_at = NULL,
			finished_at = NULL, diff = '', agent_log_tail = '', agent_duration_ms = NULL, agent_exit_code = NULL, sandbox_result = NULL, failure_reason = ''
			WHERE id = $1 RETURNING `+proofCols, id), &p)
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return Proof{}, httpx.New(http.StatusConflict, "proof_in_progress", "A proof is already in progress")
	}
	return p, err
}

// ProofFacts implements agents.ProofFactsSource.
func (s *Service) ProofFacts(ctx context.Context, agentID string) (agents.ProofFacts, error) {
	var f agents.ProofFacts
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT
			EXISTS (SELECT 1 FROM proofs WHERE agent_id = $1 AND status = 'passed'),
			EXISTS (SELECT 1 FROM proofs WHERE agent_id = $1 AND status = ANY($2)),
			coalesce((SELECT status FROM proofs WHERE agent_id = $1 AND finished_at IS NOT NULL ORDER BY finished_at DESC LIMIT 1), '')`,
			agentID, openStatuses).Scan(&f.HasPassed, &f.HasOpen, &f.LastFinishedStatus)
	})
	return f, err
}
```

- [ ] **Step 3: Маршруты кабинета**

`backend/internal/proofs/http_me.go`:

```go
package proofs

import (
	"net/http"

	"tolerance/internal/identity"
	"tolerance/internal/platform/httpx"
)

type createInput struct {
	TaskSlug string `json:"task_slug"`
}

func RegisterOwnerRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/proof-tasks", func(w http.ResponseWriter, r *http.Request) {
		tasks, err := s.Tasks(r.Context())
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": tasks})
	})
	mux.HandleFunc("POST /api/v1/proofs", func(w http.ResponseWriter, r *http.Request) {
		raw, err := httpx.ReadBody(w, r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var in createInput
		if err := httpx.Decode(raw, &in); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		p, err := s.Create(r.Context(), identity.MustFromContext(r.Context()).UserID, in.TaskSlug)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusCreated, p)
	})
	mux.HandleFunc("GET /api/v1/proofs", func(w http.ResponseWriter, r *http.Request) {
		items, err := s.List(r.Context(), identity.MustFromContext(r.Context()).UserID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items})
	})
	mux.HandleFunc("GET /api/v1/proofs/{id}", func(w http.ResponseWriter, r *http.Request) {
		p, err := s.Get(r.Context(), identity.MustFromContext(r.Context()).UserID, r.PathValue("id"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, p)
	})
	mux.HandleFunc("POST /api/v1/proofs/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		p, err := s.Retry(r.Context(), identity.MustFromContext(r.Context()).UserID, r.PathValue("id"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, p)
	})
}
```

В `handler.go`: поле `proofs *proofs.Service` в `deps`; `proofs.RegisterOwnerRoutes(owner, d.proofs)`; `api.Handle("/api/v1/proof-tasks", session(owner))`, `api.Handle("/api/v1/proofs", session(owner))`, `api.Handle("/api/v1/proofs/", session(owner))`. В `main.go` и `main_test.go`: `ps := proofs.NewService(pool); agents.NewService(pool, ps)`.

- [ ] **Step 4: Коммит**

```bash
cd backend && gofmt -l . && go vet ./... && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./...
git add -A backend && git commit -m "Add proofs: owner-side create, list, retry, daily limit and stage facts

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 6: API коннектора: выдача задачи, tarball, результат

**Files:**
- Create: `backend/internal/platform/sanitize/sanitize.go`, `backend/internal/platform/sanitize/sanitize_test.go`, `backend/internal/proofs/http_connector.go`, `backend/internal/proofs/connector_integration_test.go`
- Modify: `backend/internal/proofs/service.go`, `backend/cmd/api/handler.go`

**Interfaces:**
- Produces: `sanitize.CleanText(s string, maxRunes int) string` (однострочный, как прежний `attempts.CleanText`), `sanitize.CleanLog(s string, maxBytes int) string` (сохраняет переводы строк, редактирует секреты, оставляет хвост); `(*Service).Claim(ctx, agentID) (*Proof, *Task, error)` (nil, nil, nil когда очереди нет); `(*Service).RepoTar(ctx, agentID, proofID) ([]byte, error)`; `(*Service).Started(ctx, agentID, proofID) error`; `(*Service).SubmitResult(ctx, agentID, proofID string, in ResultInput) error`; `proofs.ResultInput{Diff, LogTail string; DurationMS, ExitCode int}`; `proofs.RegisterConnectorRoutes(mux, s)`; маршруты `GET /api/v1/connector/tasks/next?wait=N`, `GET /api/v1/connector/proofs/{id}/repo.tar.gz`, `POST /api/v1/connector/proofs/{id}/started`, `POST /api/v1/connector/proofs/{id}/result`.

Отступление от спеки: вместо подписанного URL на 10 минут репозиторий отдаётся эндпоинтом под тем же API-ключом. Проще, а безопасность та же: без ключа tarball не получить.

- [ ] **Step 1: Тест санитизации**

`backend/internal/platform/sanitize/sanitize_test.go`:

```go
package sanitize

import (
	"strings"
	"testing"
)

func TestCleanText_RedactsSecretsStripsANSIAndTruncates(t *testing.T) {
	in := "\x1b[32mok\x1b[0m token=abc123def456ghi789 and sk-ABCDEFGHIJKLMNOP123 done"
	got := CleanText(in, 200)
	if strings.Contains(got, "\x1b") || strings.Contains(got, "abc123def456") || strings.Contains(got, "sk-ABCDEF") {
		t.Fatalf("not sanitized: %q", got)
	}
	if !strings.HasPrefix(got, "ok ") {
		t.Fatalf("expected ANSI stripped, got %q", got)
	}
	long := CleanText(strings.Repeat("a", 500), 200)
	if len([]rune(long)) != 200 {
		t.Fatalf("expected truncation to 200 runes, got %d", len([]rune(long)))
	}
}

func TestCleanLog_KeepsLinesRedactsAndKeepsTail(t *testing.T) {
	in := "line1\nAuthorization: Bearer eyJabcdefghij.abcdefghijklmnop.sig\nline3\n"
	got := CleanLog(in, 1<<20)
	if strings.Count(got, "\n") != 3 || strings.Contains(got, "eyJabcdefghij") {
		t.Fatalf("unexpected: %q", got)
	}
	tail := CleanLog(strings.Repeat("x", 100)+"\nEND", 10)
	if !strings.HasSuffix(tail, "END") || len(tail) > 10 {
		t.Fatalf("expected the last 10 bytes, got %q", tail)
	}
}
```

- [ ] **Step 2: Пакет sanitize**

`backend/internal/platform/sanitize/sanitize.go`: перенести содержимое бывшего `attempts/sanitize.go` (оно удалено в задаче 1, взять из git: `git show 3aba24e:backend/internal/attempts/sanitize.go`) с изменениями: пакет `sanitize`; `CleanText(s string, maxRunes int) string` вместо константы `maxTextRunes`; добавить

```go
// CleanLog is CleanText for multi-line output: ANSI stripped, secrets
// redacted per line, newlines kept, and only the last maxBytes returned
// (the end of a log is where the failure is).
func CleanLog(s string, maxBytes int) string {
	s = strings.ToValidUTF8(s, "")
	s = ansiRe.ReplaceAllString(s, "")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		line = strings.Map(func(r rune) rune {
			if r == '\t' {
				return r
			}
			if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
				return -1
			}
			return r
		}, line)
		for _, re := range secretPatterns {
			line = re.ReplaceAllString(line, redacted)
		}
		lines[i] = longTokenRe.ReplaceAllStringFunc(line, func(m string) string {
			if looksLikeToken(m) {
				return redacted
			}
			return m
		})
	}
	s = strings.Join(lines, "\n")
	if len(s) > maxBytes {
		s = s[len(s)-maxBytes:]
		s = strings.ToValidUTF8(s, "")
	}
	return s
}
```

Run: `go test ./internal/platform/sanitize/ -v` → PASS.

- [ ] **Step 3: Интеграционный тест коннекторной стороны**

`backend/internal/proofs/connector_integration_test.go`:

```go
package proofs_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/proofs"
)

func TestClaim_ExactlyOneConnectorGetsTheTask(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_ = f.agents.Heartbeat(ctx, f.agent, "0.1", "h")
	p, err := f.proofs.Create(ctx, f.userID, "go-fix-retry")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	got := make([]*proofs.Proof, 8)
	for i := range got {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got[i], _, _ = f.proofs.Claim(ctx, f.agent)
		}(i)
	}
	wg.Wait()
	n := 0
	for _, g := range got {
		if g != nil {
			n++
			if g.ID != p.ID || g.Status != proofs.StatusClaimed {
				t.Fatalf("claimed wrong thing: %+v", g)
			}
		}
	}
	if n != 1 {
		t.Fatalf("expected exactly one claim, got %d", n)
	}
	if again, _, _ := f.proofs.Claim(ctx, f.agent); again != nil {
		t.Fatalf("queue must be empty after the claim")
	}
}

func TestConnectorFlow_RepoStartedResult(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_ = f.agents.Heartbeat(ctx, f.agent, "0.1", "h")
	if _, err := f.proofs.Create(ctx, f.userID, "go-fix-retry"); err != nil {
		t.Fatal(err)
	}
	p, task, err := f.proofs.Claim(ctx, f.agent)
	if err != nil || p == nil || task.Slug != "go-fix-retry" || task.TaskMD == "" {
		t.Fatalf("claim: %v %+v %+v", err, p, task)
	}
	tarball, err := f.proofs.RepoTar(ctx, f.agent, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(tarball)
	if hex.EncodeToString(sum[:]) != task.RepoSHA256 {
		t.Fatalf("tarball sha mismatch")
	}
	if _, err := f.proofs.RepoTar(ctx, "agent_other", p.ID); err == nil {
		t.Fatalf("another agent must not download this proof's repo")
	}
	if err := f.proofs.Started(ctx, f.agent, p.ID); err != nil {
		t.Fatal(err)
	}
	in := proofs.ResultInput{Diff: "--- a/x\n+++ b/x\n", LogTail: "token=SECRET123456789\ndone\n", DurationMS: 1234, ExitCode: 0}
	if err := f.proofs.SubmitResult(ctx, f.agent, p.ID, in); err != nil {
		t.Fatalf("result: %v", err)
	}
	if err := f.proofs.SubmitResult(ctx, f.agent, p.ID, in); err == nil {
		t.Fatalf("second result must be rejected")
	}
	got, _ := f.proofs.Get(ctx, f.userID, p.ID)
	if got.Status != proofs.StatusDiffSubmitted || got.Diff != in.Diff || strings.Contains(got.AgentLogTail, "SECRET123456789") || *got.AgentDurationMS != 1234 {
		t.Fatalf("stored: %+v", got)
	}
	var jobs int
	_ = f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE kind = 'run_proof' AND payload->>'proof_id' = $1`, p.ID).Scan(&jobs)
	})
	if jobs != 1 {
		t.Fatalf("expected one run_proof job, got %d", jobs)
	}
	big := proofs.ResultInput{Diff: strings.Repeat("x", 256<<10+1)}
	if err := f.proofs.SubmitResult(ctx, f.agent, p.ID, big); err == nil {
		t.Fatalf("oversized diff must be rejected")
	}
}
```

- [ ] **Step 4: Коннекторные методы сервиса**

Добавить в `backend/internal/proofs/service.go`:

```go
type ResultInput struct {
	Diff       string `json:"diff"`
	LogTail    string `json:"log_tail"`
	DurationMS int    `json:"duration_ms"`
	ExitCode   int    `json:"exit_code"`
}

type RunProofPayload struct {
	ProofID string `json:"proof_id"`
}

func (s *Service) task(ctx context.Context, tx pgx.Tx, slug string) (*Task, error) {
	var t Task
	err := tx.QueryRow(ctx, `SELECT slug, title, language, image, run_cmd, agent_timeout_s, sandbox_timeout_s, visible_tests, hidden_tests, task_md, repo_sha256 FROM proof_tasks WHERE slug = $1`, slug).
		Scan(&t.Slug, &t.Title, &t.Language, &t.Image, &t.RunCmd, &t.AgentTimeoutS, &t.SandboxTimeoutS, &t.VisibleTests, &t.HiddenTests, &t.TaskMD, &t.RepoSHA256)
	return &t, err
}

// Claim hands the agent's oldest queued proof to the connector. SKIP LOCKED
// makes two connectors on one key race safely: one wins, the other sees nil.
func (s *Service) Claim(ctx context.Context, agentID string) (*Proof, *Task, error) {
	var p Proof
	var t *Task
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		err := scanProof(tx.QueryRow(ctx, `UPDATE proofs SET status = 'claimed', claimed_at = now()
			WHERE id = (SELECT id FROM proofs WHERE agent_id = $1 AND status = 'queued' ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1)
			RETURNING `+proofCols, agentID), &p)
		if err != nil {
			return err
		}
		t, err = s.task(ctx, tx, p.TaskSlug)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return &p, t, nil
}

func (s *Service) RepoTar(ctx context.Context, agentID, proofID string) ([]byte, error) {
	var tar []byte
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT t.repo_tar FROM proofs p JOIN proof_tasks t ON t.slug = p.task_slug
			WHERE p.id = $1 AND p.agent_id = $2 AND p.status IN ('claimed', 'running_agent')`, proofID, agentID).Scan(&tar)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, httpx.NotFound()
	}
	return tar, err
}

func (s *Service) Started(ctx context.Context, agentID, proofID string) error {
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE proofs SET status = 'running_agent' WHERE id = $1 AND agent_id = $2 AND status = 'claimed'`, proofID, agentID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.StateConflict("Proof is not in claimed state")
		}
		return nil
	})
}

// SubmitResult stores the agent's diff and enqueues the sandbox run in the
// same transaction, so a stored diff is always followed by a run.
func (s *Service) SubmitResult(ctx context.Context, agentID, proofID string, in ResultInput) error {
	if len(in.Diff) > maxDiffBytes {
		return httpx.New(http.StatusRequestEntityTooLarge, "diff_too_large", "Diff exceeds 256 KiB")
	}
	logTail := sanitize.CleanLog(in.LogTail, maxLogTailBytes)
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE proofs SET status = 'diff_submitted', diff_submitted_at = now(), diff = $3, agent_log_tail = $4,
			agent_duration_ms = $5, agent_exit_code = $6
			WHERE id = $1 AND agent_id = $2 AND status IN ('claimed', 'running_agent')`, proofID, agentID, in.Diff, logTail, in.DurationMS, in.ExitCode)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.StateConflict("Proof already has a result or is not running")
		}
		_, err = jobs.Enqueue(ctx, tx, "run_proof", RunProofPayload{ProofID: proofID}, "run_proof:"+proofID)
		return err
	})
}
```

(импорты `tolerance/internal/platform/jobs`, `tolerance/internal/platform/sanitize`.)

- [ ] **Step 5: HTTP коннектора**

`backend/internal/proofs/http_connector.go`:

```go
package proofs

import (
	"net/http"
	"strconv"
	"time"

	"tolerance/internal/identity"
	"tolerance/internal/platform/httpx"
)

const maxLongPoll = 25 * time.Second

type nextTaskResponse struct {
	ProofID string `json:"proof_id"`
	Task    *Task  `json:"task"`
}

func RegisterConnectorRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/connector/tasks/next", func(w http.ResponseWriter, r *http.Request) {
		agentID := identity.MustFromContext(r.Context()).AgentID
		wait, _ := strconv.Atoi(r.URL.Query().Get("wait"))
		deadline := time.Now().Add(min(time.Duration(wait)*time.Second, maxLongPoll))
		for {
			p, t, err := s.Claim(r.Context(), agentID)
			if err != nil {
				httpx.WriteError(w, r, err)
				return
			}
			if p != nil {
				httpx.Respond(w, http.StatusOK, nextTaskResponse{ProofID: p.ID, Task: t})
				return
			}
			if time.Now().After(deadline) {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			select {
			case <-r.Context().Done():
				return
			case <-time.After(time.Second):
			}
		}
	})
	mux.HandleFunc("GET /api/v1/connector/proofs/{id}/repo.tar.gz", func(w http.ResponseWriter, r *http.Request) {
		tar, err := s.RepoTar(r.Context(), identity.MustFromContext(r.Context()).AgentID, r.PathValue("id"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Length", strconv.Itoa(len(tar)))
		_, _ = w.Write(tar)
	})
	mux.HandleFunc("POST /api/v1/connector/proofs/{id}/started", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Started(r.Context(), identity.MustFromContext(r.Context()).AgentID, r.PathValue("id")); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/connector/proofs/{id}/result", func(w http.ResponseWriter, r *http.Request) {
		raw, err := httpx.ReadBodyLimit(w, r, 512<<10)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var in ResultInput
		if err := httpx.Decode(raw, &in); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		if err := s.SubmitResult(r.Context(), identity.MustFromContext(r.Context()).AgentID, r.PathValue("id"), in); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
```

В `handler.go`: `proofs.RegisterConnectorRoutes(connector, d.proofs)`.

- [ ] **Step 6: Коммит**

```bash
cd backend && gofmt -l . && go vet ./... && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./...
git add -A backend && git commit -m "Add connector API: long-poll claim, repo tarball, started, result

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 7: Песочница: Runner, парсер `go test -json`, Docker

**Files:**
- Create: `backend/internal/proofs/sandbox/runner.go`, `backend/internal/proofs/sandbox/gotest.go`, `backend/internal/proofs/sandbox/gotest_test.go`, `backend/internal/proofs/sandbox/fake.go`, `backend/internal/proofs/sandbox/docker.go`, `backend/internal/proofs/sandbox/docker_integration_test.go`

**Interfaces:**
- Produces: `sandbox.Request{WorkDir, Image, RunCmd string; Timeout time.Duration}`; `sandbox.Result{Tests []proofs.TestResult; ExitCode int; Output string; TimedOut bool}` (вынести `TestResult` в `sandbox` и заставить `proofs.TestResult = sandbox.TestResult` через type alias, чтобы избежать цикла: `proofs` импортирует `sandbox`, не наоборот); `sandbox.Runner interface{ Run(ctx, Request) (Result, error) }` (ошибка = сбой инфраструктуры); `sandbox.ParseGoTestJSON([]byte) []TestResult`; `sandbox.NewDocker() *Docker`; `sandbox.Fake{Result Result; Err error}`.

В `proofs/model.go` заменить объявление `TestResult` на `type TestResult = sandbox.TestResult` и `SandboxResult` оставить.

- [ ] **Step 1: Тест парсера**

`backend/internal/proofs/sandbox/gotest_test.go`:

```go
package sandbox

import "testing"

func TestParseGoTestJSON(t *testing.T) {
	out := []byte(`{"Action":"run","Package":"retry","Test":"TestA"}
{"Action":"output","Package":"retry","Test":"TestA","Output":"=== RUN TestA\n"}
{"Action":"pass","Package":"retry","Test":"TestA","Elapsed":0}
{"Action":"run","Package":"retry","Test":"TestB"}
{"Action":"fail","Package":"retry","Test":"TestB","Elapsed":0.01}
{"Action":"fail","Package":"retry","Elapsed":0.02}
garbage line that is not json
`)
	got := ParseGoTestJSON(out)
	if len(got) != 2 || got[0].Name != "TestA" || !got[0].Passed || got[1].Name != "TestB" || got[1].Passed {
		t.Fatalf("unexpected: %+v", got)
	}
	if len(ParseGoTestJSON([]byte("# retry\n./retry.go:5: syntax error\n"))) != 0 {
		t.Fatalf("build failure yields no tests")
	}
}
```

- [ ] **Step 2: Runner, парсер, fake**

`backend/internal/proofs/sandbox/runner.go`:

```go
// Package sandbox runs a prepared work directory inside a disposable
// container and reports which tests passed. A Runner error means the
// platform failed (no docker, no image), never that the code under test
// failed: that is reported inside Result.
package sandbox

import (
	"context"
	"time"
)

type Request struct {
	WorkDir string
	Image   string
	RunCmd  string
	Timeout time.Duration
}

type TestResult struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
}

type Result struct {
	Tests    []TestResult
	ExitCode int
	Output   string
	TimedOut bool
}

type Runner interface {
	Run(ctx context.Context, req Request) (Result, error)
}
```

`backend/internal/proofs/sandbox/gotest.go`:

```go
package sandbox

import (
	"bufio"
	"bytes"
	"encoding/json"
)

type goTestEvent struct {
	Action string `json:"Action"`
	Test   string `json:"Test"`
}

// ParseGoTestJSON extracts per-test pass/fail from `go test -json` output.
// Package-level events (no Test) and non-JSON lines are ignored; a build
// failure therefore yields zero tests, which the caller treats as failed.
func ParseGoTestJSON(out []byte) []TestResult {
	var res []TestResult
	seen := map[string]int{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var ev goTestEvent
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil || ev.Test == "" {
			continue
		}
		if ev.Action != "pass" && ev.Action != "fail" {
			continue
		}
		if i, ok := seen[ev.Test]; ok {
			res[i].Passed = ev.Action == "pass"
			continue
		}
		seen[ev.Test] = len(res)
		res = append(res, TestResult{Name: ev.Test, Passed: ev.Action == "pass"})
	}
	return res
}
```

`backend/internal/proofs/sandbox/fake.go`:

```go
package sandbox

import "context"

// Fake returns a canned result; tests and ARENA_SANDBOX=fake use it.
type Fake struct {
	Result Result
	Err    error
	Calls  []Request
}

func (f *Fake) Run(_ context.Context, req Request) (Result, error) {
	f.Calls = append(f.Calls, req)
	return f.Result, f.Err
}
```

Run: `go test ./internal/proofs/sandbox/ -run TestParseGoTestJSON -v` → PASS.

- [ ] **Step 3: Docker-тест**

`backend/internal/proofs/sandbox/docker_integration_test.go`:

```go
package sandbox_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"tolerance/internal/proofs"
	"tolerance/internal/proofs/sandbox"
)

const fixture = "../../../fixtures/proofs/go-fix-retry"

func requireDocker(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "info").Run(); err != nil {
		if os.Getenv("ARENA_TEST_REQUIRE_DOCKER") == "1" {
			t.Fatalf("docker unavailable: %v", err)
		}
		t.Skipf("docker unavailable: %v", err)
	}
	build := exec.Command("docker", "build", "-q", "-t", "arena-proof-go:1", fixture)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build image: %v\n%s", err, out)
	}
}

func prepare(t *testing.T, fix func(dir string)) string {
	t.Helper()
	dir := t.TempDir()
	tasks, err := proofs.LoadCatalog(filepath.Dir(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if err := proofs.Untar(tasks[0].RepoTar, dir); err != nil {
		t.Fatal(err)
	}
	fix(dir)
	if err := proofs.Untar(tasks[0].HiddenTar, dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

const fixedBackoff = `
func Backoff(attempt int) time.Duration {
	if attempt < 1 {
		return 0
	}
	if attempt > 20 {
		return Max
	}
	d := Base << uint(attempt-1)
	if d > Max {
		return Max
	}
	return d
}
`

func TestDocker_PassesFailsAndReportsMissingImage(t *testing.T) {
	requireDocker(t)
	r := sandbox.NewDocker()
	req := func(dir string) sandbox.Request {
		return sandbox.Request{WorkDir: dir, Image: "arena-proof-go:1", RunCmd: "go test ./... -json -count=1", Timeout: 2 * time.Minute}
	}

	good := prepare(t, func(dir string) {
		src, _ := os.ReadFile(filepath.Join(dir, "retry.go"))
		start := []byte("// Backoff returns the delay before attempt n (1-based).\n")
		i := bytesIndex(src, start)
		j := bytesIndex(src, []byte("// Do calls fn"))
		out := append(append(append([]byte{}, src[:i]...), []byte(fixedBackoff)...), src[j:]...)
		_ = os.WriteFile(filepath.Join(dir, "retry.go"), out, 0o644)
	})
	res, err := r.Run(context.Background(), req(good))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.ExitCode != 0 || len(res.Tests) != 8 {
		t.Fatalf("expected 8 passing tests, got exit=%d tests=%+v\n%s", res.ExitCode, res.Tests, res.Output)
	}
	for _, tr := range res.Tests {
		if !tr.Passed {
			t.Fatalf("%s failed with the reference fix", tr.Name)
		}
	}

	bad := prepare(t, func(string) {})
	res, err = r.Run(context.Background(), req(bad))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	failed := 0
	for _, tr := range res.Tests {
		if !tr.Passed {
			failed++
		}
	}
	if res.ExitCode == 0 || failed != 3 {
		t.Fatalf("expected 3 failing tests on the unfixed repo, got exit=%d failed=%d", res.ExitCode, failed)
	}

	slow := prepare(t, func(string) {})
	res, err = r.Run(context.Background(), sandbox.Request{WorkDir: slow, Image: "arena-proof-go:1", RunCmd: "sleep 30", Timeout: 3 * time.Second})
	if err != nil || !res.TimedOut {
		t.Fatalf("expected timeout, got err=%v res=%+v", err, res)
	}

	_, err = r.Run(context.Background(), sandbox.Request{WorkDir: bad, Image: "arena-proof-does-not-exist:0", RunCmd: "true", Timeout: 30 * time.Second})
	if err == nil {
		t.Fatalf("missing image must be an infra error")
	}
}

func bytesIndex(b, sub []byte) int {
	for i := 0; i+len(sub) <= len(b); i++ {
		if string(b[i:i+len(sub)]) == string(sub) {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 4: Docker runner**

`backend/internal/proofs/sandbox/docker.go` (проверено против Docker 28 до запуска плана: хорошее решение 8/8, плохое 3 красных, таймаут убивает контейнер без остатков, отсутствующий образ → ошибка платформы; прежняя схема `docker create` + `docker cp` не работает: демон отказывает в `docker cp` для контейнера с `--read-only`):

```go
package sandbox

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
)

// Docker runs the work directory in a throwaway container via the docker
// CLI. Two facts verified against Docker 28 shape this code:
//
//   - `docker cp` into a container created with --read-only is refused
//     ("container rootfs is marked read-only"), so the work directory is
//     streamed in as a tar on stdin and unpacked into a size-limited tmpfs
//     at /work. This also works when the API itself runs in a container
//     that only has the host's docker.sock (no bind mounts needed).
//   - Killing the `docker run` client leaves the container running, so a
//     timeout kills the container by name.
type Docker struct{}

func NewDocker() *Docker { return &Docker{} }

const (
	maxOutput = 256 << 10
	// exitUnpack is what the wrapper script returns when the work tarball
	// could not be unpacked: a platform failure, not the participant's.
	exitUnpack = 97
)

func (d *Docker) Run(ctx context.Context, req Request) (Result, error) {
	name := "arena-sbx-" + randHex(8)
	script := fmt.Sprintf("tar -xf - -C /work || exit %d; cd /work && %s", exitUnpack, req.RunCmd)
	cmd := exec.Command("docker", "run", "-i", "--rm", "--name", name, "--pull", "never",
		"--network", "none", "--memory", "1g", "--cpus", "1", "--pids-limit", "256",
		"--read-only", "--tmpfs", "/tmp:rw,exec,size=512m", "--tmpfs", "/work:rw,exec,size=512m",
		"-e", "HOME=/tmp", "-e", "GOCACHE=/tmp/gocache", "-e", "GOPATH=/tmp/gopath", "-e", "GOTMPDIR=/tmp",
		"-w", "/work", req.Image, "sh", "-c", script)
	pr, pw := io.Pipe()
	go func() { pw.CloseWithError(writeTar(pw, req.WorkDir)) }()
	var buf bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = pr, &buf, &buf
	if err := cmd.Start(); err != nil {
		pr.Close()
		return Result{}, fmt.Errorf("sandbox: docker run: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	runCtx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()
	var waitErr error
	stopped := false
	select {
	case waitErr = <-done:
	case <-runCtx.Done():
		stopped = true
		_ = exec.Command("docker", "kill", name).Run()
		waitErr = <-done
	}
	pr.Close() // unblocks writeTar if the container exited before reading everything

	res := Result{Output: tail(buf.String(), maxOutput)}
	res.Tests = ParseGoTestJSON([]byte(res.Output))
	if stopped {
		if ctx.Err() != nil { // the caller gave up (shutdown), not the task's time limit
			return Result{}, fmt.Errorf("sandbox: cancelled: %w", ctx.Err())
		}
		res.TimedOut, res.ExitCode = true, -1
		return res, nil
	}
	var exitErr *exec.ExitError
	switch {
	case waitErr == nil:
		res.ExitCode = 0
	case errors.As(waitErr, &exitErr):
		res.ExitCode = exitErr.ExitCode()
	default:
		return Result{}, fmt.Errorf("sandbox: docker run: %w", waitErr)
	}
	// 125: docker could not create or start the container (missing image
	// with --pull never, bad flags); 126/127: the platform's run command is
	// broken inside the image; exitUnpack: our tarball. None of these are
	// the participant's fault.
	switch res.ExitCode {
	case 125, 126, 127, exitUnpack:
		return Result{}, fmt.Errorf("sandbox: container could not run (exit %d): %s", res.ExitCode, tail(res.Output, 2000))
	}
	return res, nil
}

// writeTar streams dir as an uncompressed tar. Only regular files and
// directories are included; symlinks and devices are dropped.
func writeTar(w io.Writer, dir string) error {
	tw := tar.NewWriter(w)
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil || rel == "." {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() && !info.IsDir() {
			return nil
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		hdr.Uname, hdr.Gname, hdr.Uid, hdr.Gid = "", "", 0, 0
		if info.IsDir() {
			hdr.Name += "/"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
	if err != nil {
		return err
	}
	return tw.Close()
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
```

В `docker_integration_test.go` после проверки таймаута добавить: `docker ps -q --filter name=arena-sbx-` пусто (контейнер после таймаута не остался работать).

Run: `ARENA_TEST_REQUIRE_DOCKER=1 go test ./internal/proofs/sandbox/ -v -run TestDocker` → PASS (собирает образ `arena-proof-go:1`, три прогона; ожидать 1–2 минуты).

- [ ] **Step 5: Коммит**

```bash
cd backend && gofmt -l . && go vet ./... && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./...
git add -A backend && git commit -m "Add sandbox runner: stdin tar into tmpfs /work, read-only rootfs, named-container timeout

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 8: Воркер: run_proof, истечение, запуск в cmd/api

**Files:**
- Create: `backend/internal/proofs/worker.go`, `backend/internal/proofs/worker_integration_test.go`
- Modify: `backend/internal/proofs/service.go`, `backend/cmd/api/main.go`, `backend/cmd/api/handler.go` (логирование 5xx)

**Interfaces:**
- Consumes: `jobs.Queue` (`Claim`, `Complete`, `Fail`, `Reclaim`), `sandbox.Runner`, `proofs.Untar`.
- Produces: `proofs.NewWorker(pool *db.Pool, runner sandbox.Runner, workDir string, log *slog.Logger) *Worker`; `(*Worker).Run(ctx)` (блокирующий цикл); `(*Worker).RunProof(ctx, proofID) error` (один job, для тестов); `(*Service).ExpireStale(ctx) (int, error)`.

- [ ] **Step 1: Интеграционный тест воркера**

`backend/internal/proofs/worker_integration_test.go`:

```go
package proofs_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/proofs"
	"tolerance/internal/proofs/sandbox"
)

const goodDiff = `--- a/retry.go
+++ b/retry.go
@@ -19,7 +19,14 @@ func Backoff(attempt int) time.Duration {
 	if attempt < 1 {
 		return 0
 	}
-	return Base * time.Duration(attempt)
+	if attempt > 20 {
+		return Max
+	}
+	d := Base << uint(attempt-1)
+	if d > Max {
+		return Max
+	}
+	return d
 }
 
 // Do calls fn until it succeeds or maxAttempts is used up.
`

func submitted(t *testing.T, f fixture, diff string) string {
	t.Helper()
	ctx := context.Background()
	_ = f.agents.Heartbeat(ctx, f.agent, "0.1", "h")
	if _, err := f.proofs.Create(ctx, f.userID, "go-fix-retry"); err != nil {
		t.Fatal(err)
	}
	p, _, _ := f.proofs.Claim(ctx, f.agent)
	if err := f.proofs.SubmitResult(ctx, f.agent, p.ID, proofs.ResultInput{Diff: diff}); err != nil {
		t.Fatal(err)
	}
	return p.ID
}

func TestRunProof_PassedFailedAndInfraError(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()

	t.Run("all tests pass", func(t *testing.T) {
		f := setup(t)
		fake := &sandbox.Fake{Result: sandbox.Result{ExitCode: 0, Tests: []sandbox.TestResult{{Name: "TestA", Passed: true}}}}
		w := proofs.NewWorker(f.d.AppPool, fake, t.TempDir(), log)
		id := submitted(t, f, goodDiff)
		if err := w.RunProof(ctx, id); err != nil {
			t.Fatal(err)
		}
		p, _ := f.proofs.Get(ctx, f.userID, id)
		if p.Status != proofs.StatusPassed || p.SandboxResult == nil || p.FinishedAt == nil {
			t.Fatalf("%+v", p)
		}
		if len(fake.Calls) != 1 || fake.Calls[0].Image != "arena-proof-go:1" {
			t.Fatalf("runner not called with the task image: %+v", fake.Calls)
		}
		// hidden tests were copied over the applied diff
		if _, err := os.Stat(filepath.Join(fake.Calls[0].WorkDir, "retry_hidden_test.go")); err == nil {
			t.Fatalf("work dir must be removed after the run")
		}
		facts, _ := f.proofs.ProofFacts(ctx, f.agent)
		if !facts.HasPassed {
			t.Fatalf("facts: %+v", facts)
		}
	})

	t.Run("a hidden test fails", func(t *testing.T) {
		f := setup(t)
		fake := &sandbox.Fake{Result: sandbox.Result{ExitCode: 1, Tests: []sandbox.TestResult{{Name: "TestA", Passed: true}, {Name: "TestHidden_X", Passed: false}}}}
		w := proofs.NewWorker(f.d.AppPool, fake, t.TempDir(), log)
		id := submitted(t, f, goodDiff)
		_ = w.RunProof(ctx, id)
		p, _ := f.proofs.Get(ctx, f.userID, id)
		if p.Status != proofs.StatusFailed || p.FailureReason != "tests_failed" {
			t.Fatalf("%+v", p)
		}
	})

	t.Run("diff does not apply", func(t *testing.T) {
		f := setup(t)
		fake := &sandbox.Fake{}
		w := proofs.NewWorker(f.d.AppPool, fake, t.TempDir(), log)
		id := submitted(t, f, "--- a/nope.go\n+++ b/nope.go\n@@ -1 +1 @@\n-x\n+y\n")
		_ = w.RunProof(ctx, id)
		p, _ := f.proofs.Get(ctx, f.userID, id)
		if p.Status != proofs.StatusFailed || p.FailureReason != "diff_not_applicable" || len(fake.Calls) != 0 {
			t.Fatalf("%+v calls=%d", p, len(fake.Calls))
		}
	})

	t.Run("CRLF diff still applies", func(t *testing.T) {
		f := setup(t)
		fake := &sandbox.Fake{Result: sandbox.Result{ExitCode: 0, Tests: []sandbox.TestResult{{Name: "T", Passed: true}}}}
		w := proofs.NewWorker(f.d.AppPool, fake, t.TempDir(), log)
		crlf := ""
		for _, line := range splitLines(goodDiff) {
			crlf += line + "\r\n"
		}
		id := submitted(t, f, crlf)
		_ = w.RunProof(ctx, id)
		p, _ := f.proofs.Get(ctx, f.userID, id)
		if p.Status != proofs.StatusPassed {
			t.Fatalf("CRLF diff: %+v", p)
		}
	})

	t.Run("sandbox timeout is a failure", func(t *testing.T) {
		f := setup(t)
		fake := &sandbox.Fake{Result: sandbox.Result{TimedOut: true, ExitCode: -1}}
		w := proofs.NewWorker(f.d.AppPool, fake, t.TempDir(), log)
		id := submitted(t, f, goodDiff)
		_ = w.RunProof(ctx, id)
		p, _ := f.proofs.Get(ctx, f.userID, id)
		if p.Status != proofs.StatusFailed || p.FailureReason != "timeout" {
			t.Fatalf("%+v", p)
		}
	})

	t.Run("runner error is infra_error after the job gives up", func(t *testing.T) {
		f := setup(t)
		fake := &sandbox.Fake{Err: errors.New("no docker")}
		w := proofs.NewWorker(f.d.AppPool, fake, t.TempDir(), log)
		id := submitted(t, f, goodDiff)
		if err := w.RunProof(ctx, id); err == nil {
			t.Fatalf("runner error must propagate so the job retries")
		}
		p, _ := f.proofs.Get(ctx, f.userID, id)
		if p.Status != proofs.StatusRunningSandbox {
			t.Fatalf("before giving up the proof stays running_sandbox: %+v", p)
		}
		if err := w.MarkInfraError(ctx, id, "no docker"); err != nil {
			t.Fatal(err)
		}
		p, _ = f.proofs.Get(ctx, f.userID, id)
		if p.Status != proofs.StatusInfraError || p.FailureReason == "" {
			t.Fatalf("%+v", p)
		}
	})
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func TestExpireStale(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_ = f.agents.Heartbeat(ctx, f.agent, "0.1", "h")
	p, _ := f.proofs.Create(ctx, f.userID, "go-fix-retry")
	_ = f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE proofs SET created_at = now() - interval '6 minutes' WHERE id = $1`, p.ID)
		return err
	})
	n, err := f.proofs.ExpireStale(ctx)
	if err != nil || n != 1 {
		t.Fatalf("expire: %v %d", err, n)
	}
	got, _ := f.proofs.Get(ctx, f.userID, p.ID)
	if got.Status != proofs.StatusExpired || got.FailureReason != "not_claimed" {
		t.Fatalf("%+v", got)
	}

	p2, _ := f.proofs.Create(ctx, f.userID, "go-fix-retry")
	_, _, _ = f.proofs.Claim(ctx, f.agent)
	_ = f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE proofs SET claimed_at = now() - interval '20 minutes' WHERE id = $1`, p2.ID)
		return err
	})
	if n, _ := f.proofs.ExpireStale(ctx); n != 1 {
		t.Fatalf("expected the overdue claimed proof to expire, got %d", n)
	}
	got, _ = f.proofs.Get(ctx, f.userID, p2.ID)
	if got.Status != proofs.StatusExpired || got.FailureReason != "agent_timeout" {
		t.Fatalf("%+v", got)
	}
	_ = time.Second
}
```

- [ ] **Step 2: ExpireStale в сервисе**

Добавить в `backend/internal/proofs/service.go`:

```go
// ExpireStale ends proofs nobody will finish: queued ones no connector
// claimed within 5 minutes, and claimed/running ones whose agent timeout
// (plus a minute of slack) has passed without a result.
func (s *Service) ExpireStale(ctx context.Context) (int, error) {
	var n int64
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE proofs p SET status = 'expired', finished_at = now(),
			  failure_reason = CASE WHEN p.status = 'queued' THEN 'not_claimed' ELSE 'agent_timeout' END
			FROM proof_tasks t WHERE t.slug = p.task_slug AND (
			  (p.status = 'queued' AND p.created_at < now() - interval '5 minutes') OR
			  (p.status IN ('claimed', 'running_agent') AND p.claimed_at < now() - make_interval(secs => t.agent_timeout_s + 60)))`)
		n = tag.RowsAffected()
		return err
	})
	return int(n), err
}
```

- [ ] **Step 3: Воркер**

`backend/internal/proofs/worker.go`:

```go
package proofs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
	"tolerance/internal/platform/jobs"
	"tolerance/internal/proofs/sandbox"
)

type Worker struct {
	pool    *db.Pool
	svc     *Service
	queue   *jobs.Queue
	runner  sandbox.Runner
	workDir string
	log     *slog.Logger
	owner   string
}

func NewWorker(pool *db.Pool, runner sandbox.Runner, workDir string, log *slog.Logger) *Worker {
	host, _ := os.Hostname()
	return &Worker{pool: pool, svc: NewService(pool), queue: jobs.New(pool), runner: runner, workDir: workDir, log: log,
		owner: fmt.Sprintf("%s-%d", host, os.Getpid())}
}

// Run processes run_proof jobs one at a time and, every 30 seconds, reclaims
// dead leases and expires stale proofs. It returns when ctx is done.
func (w *Worker) Run(ctx context.Context) {
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		job, err := w.queue.Claim(ctx, w.owner, []string{"run_proof"}, 15*time.Minute)
		if err != nil {
			w.log.Error("jobs claim", "err", err)
		}
		if job != nil {
			w.handle(ctx, job)
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if _, err := w.queue.Reclaim(ctx); err != nil {
				w.log.Error("jobs reclaim", "err", err)
			}
			if n, err := w.svc.ExpireStale(ctx); err != nil {
				w.log.Error("expire proofs", "err", err)
			} else if n > 0 {
				w.log.Info("expired proofs", "count", n)
			}
		case <-time.After(2 * time.Second):
		}
	}
}

func (w *Worker) handle(ctx context.Context, job *jobs.Job) {
	var payload RunProofPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		_, _ = w.queue.Fail(ctx, job.ID, err)
		return
	}
	err := w.RunProof(ctx, payload.ProofID)
	if err == nil {
		if err := w.queue.Complete(ctx, job.ID); err != nil {
			w.log.Error("jobs complete", "err", err)
		}
		return
	}
	w.log.Error("run_proof", "proof", payload.ProofID, "attempt", job.Attempts, "err", err)
	final, ferr := w.queue.Fail(ctx, job.ID, err)
	if ferr != nil {
		w.log.Error("jobs fail", "err", ferr)
	}
	if final {
		if err := w.MarkInfraError(ctx, payload.ProofID, err.Error()); err != nil {
			w.log.Error("mark infra_error", "err", err)
		}
	}
}

type runInput struct {
	diff     string
	task     Task
	repoTar  []byte
	hiddenTr []byte
}

// RunProof executes one sandbox run. A returned error means the platform
// could not run it (the job will retry); every verdict about the diff is
// written to the proof and returns nil.
func (w *Worker) RunProof(ctx context.Context, proofID string) error {
	var in runInput
	err := w.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			UPDATE proofs p SET status = 'running_sandbox' FROM proof_tasks t
			WHERE p.id = $1 AND p.status IN ('diff_submitted', 'running_sandbox') AND t.slug = p.task_slug
			RETURNING p.diff, t.slug, t.image, t.run_cmd, t.sandbox_timeout_s, t.repo_tar, t.hidden_tar`, proofID).
			Scan(&in.diff, &in.task.Slug, &in.task.Image, &in.task.RunCmd, &in.task.SandboxTimeoutS, &in.repoTar, &in.hiddenTr)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		w.log.Info("run_proof: nothing to do", "proof", proofID)
		return nil
	}
	if err != nil {
		return err
	}

	dir, err := os.MkdirTemp(w.workDir, "proof-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := Untar(in.repoTar, dir); err != nil {
		return err
	}
	if reason := applyDiff(ctx, dir, in.diff); reason != "" {
		return w.finish(ctx, proofID, StatusFailed, reason, nil)
	}
	if err := Untar(in.hiddenTr, dir); err != nil {
		return err
	}

	res, err := w.runner.Run(ctx, sandbox.Request{WorkDir: dir, Image: in.task.Image, RunCmd: in.task.RunCmd,
		Timeout: time.Duration(in.task.SandboxTimeoutS) * time.Second})
	if err != nil {
		return err
	}
	sr := &SandboxResult{Tests: res.Tests, ExitCode: res.ExitCode, Output: res.Output, TimedOut: res.TimedOut}
	switch {
	case res.TimedOut:
		return w.finish(ctx, proofID, StatusFailed, "timeout", sr)
	case len(res.Tests) == 0:
		return w.finish(ctx, proofID, StatusFailed, "build_failed", sr)
	case res.ExitCode != 0 || anyFailed(res.Tests):
		return w.finish(ctx, proofID, StatusFailed, "tests_failed", sr)
	}
	return w.finish(ctx, proofID, StatusPassed, "", sr)
}

func anyFailed(tests []sandbox.TestResult) bool {
	for _, t := range tests {
		if !t.Passed {
			return true
		}
	}
	return false
}

// applyDiff applies the participant's patch with git; CRLF and missing
// trailing newlines are tolerated. Returns "" on success or a failure
// reason. Binary patches are refused by git, which is what we want.
func applyDiff(ctx context.Context, dir, diff string) string {
	diff = strings.ReplaceAll(diff, "\r\n", "\n")
	if strings.TrimSpace(diff) == "" {
		return "empty_diff"
	}
	if !strings.HasSuffix(diff, "\n") {
		diff += "\n"
	}
	patch := filepath.Join(dir, "..", filepath.Base(dir)+".patch")
	if err := os.WriteFile(patch, []byte(diff), 0o600); err != nil {
		return "diff_not_applicable"
	}
	defer os.Remove(patch)
	cmd := exec.CommandContext(ctx, "git", "apply", "--whitespace=nowarn", "--unsafe-paths", "--directory", dir, patch)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = out
		return "diff_not_applicable"
	}
	return ""
}

func (w *Worker) finish(ctx context.Context, proofID, status, reason string, sr *SandboxResult) error {
	return w.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE proofs SET status = $2, failure_reason = $3, sandbox_result = $4, finished_at = now() WHERE id = $1`,
			proofID, status, reason, sr)
		return err
	})
}

// MarkInfraError is called when the job has used every attempt.
func (w *Worker) MarkInfraError(ctx context.Context, proofID, reason string) error {
	if len(reason) > 500 {
		reason = reason[:500]
	}
	return w.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE proofs SET status = 'infra_error', failure_reason = $2, finished_at = now()
			WHERE id = $1 AND status = 'running_sandbox'`, proofID, reason)
		return err
	})
}
```

Проверено вручную перед запуском (не оставлять на усмотрение реализатора): `git apply --whitespace=nowarn --unsafe-paths --directory <dir> <patch>` с `cmd.Dir = dir`, где `dir` и `patch` — абсолютные пути, корректно применяет патч даже когда `dir` не является git-репозиторием (`Untar` не создаёт `.git`) — `git apply`, в отличие от `git am`, этого не требует. `applyDiff` в шаге 3 реализовать как написано, без дополнительных вариантов.

- [ ] **Step 4: Запуск воркера и логирование 5xx**

В `backend/cmd/api/main.go` после создания `deps`:

```go
	var runner sandbox.Runner = sandbox.NewDocker()
	if cfg.sandbox == "fake" {
		runner = &sandbox.Fake{Result: sandbox.Result{ExitCode: 0, Tests: []sandbox.TestResult{{Name: "fake", Passed: true}}}}
	}
	go proofs.NewWorker(pool, runner, cfg.workDir, log).Run(ctx)
```

В `backend/cmd/api/handler.go` в `withMiddleware` обернуть `ResponseWriter`, чтобы 5xx попадали в лог:

```go
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) { s.status = code; s.ResponseWriter.WriteHeader(code) }
```

и в хендлере: `sw := &statusWriter{ResponseWriter: w, status: 200}; start := time.Now(); withRequestID.ServeHTTP(sw, r); if sw.status >= 500 { log.Error("request failed", "method", r.Method, "path", r.URL.Path, "status", sw.status, "request_id", sw.Header().Get("X-Request-Id")) }; log.Info("request", "method", r.Method, "path", r.URL.Path, "status", sw.status, "ms", time.Since(start).Milliseconds())`.

- [ ] **Step 5: Коммит**

```bash
cd backend && gofmt -l . && go vet ./... && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./...
git add -A backend && git commit -m "Add proof worker: sandbox job, expiry tick, request logging

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 9: Коннектор `cmd/arena`

**Files:**
- Create: `backend/cmd/arena/main.go`, `backend/cmd/arena/config.go`, `backend/cmd/arena/client.go`, `backend/cmd/arena/run.go`, `backend/cmd/arena/run_test.go`, `backend/cmd/arena/client_test.go`

**Interfaces:**
- Consumes: HTTP API из задач 3 и 6; `sanitize.CleanLog`; `proofs.Untar`.
- Produces: бинарник `arena` с командами `login`, `init`, `connect`, `status`; `config{URL, AgentCommand string}`; `client{base, key string}` с методами `Heartbeat(ctx) (heartbeatResp, error)`, `NextTask(ctx, wait time.Duration) (*nextTask, error)`, `Repo(ctx, proofID) ([]byte, error)`, `Started(ctx, proofID) error`, `Result(ctx, proofID string, r result) error`; `runTask(ctx, task nextTask, repo []byte, command string) (result, error)`.

- [ ] **Step 1: Тест прогона задачи**

`backend/cmd/arena/run_test.go`:

```go
package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tolerance/internal/proofs"
)

func fixtureRepo(t *testing.T) (nextTask, []byte) {
	t.Helper()
	tasks, err := proofs.LoadCatalog(filepath.Join("..", "..", "fixtures", "proofs"))
	if err != nil {
		t.Fatal(err)
	}
	task := tasks[0]
	return nextTask{ProofID: "proof_x", Task: taskInfo{Slug: task.Slug, TaskMD: task.TaskMD, AgentTimeoutS: 5, RepoSHA256: task.RepoSHA256}}, task.RepoTar
}

func TestRunTask_ProducesDiffOfAgentChanges(t *testing.T) {
	task, repo := fixtureRepo(t)
	res, err := runTask(context.Background(), task, repo, `test -f TASK.md && printf 'package retry\n' > extra.go && sed -i.bak 's/Base \* time.Duration(attempt)/Base << uint(attempt-1)/' retry.go && rm retry.go.bak && echo done && echo "key=SUPERSECRET1234567" >&2`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.ExitCode != 0 || !strings.Contains(res.Diff, "+++ b/extra.go") || !strings.Contains(res.Diff, "Base << uint(attempt-1)") {
		t.Fatalf("unexpected result: exit=%d diff=%q", res.ExitCode, res.Diff)
	}
	if strings.Contains(res.Diff, "TASK.md") {
		t.Fatalf("TASK.md must not be part of the diff")
	}
	if !strings.Contains(res.LogTail, "done") || strings.Contains(res.LogTail, "SUPERSECRET1234567") {
		t.Fatalf("log tail not sanitized or missing: %q", res.LogTail)
	}
	if res.DurationMS <= 0 {
		t.Fatalf("duration must be measured")
	}
}

func TestRunTask_TimesOutAndRejectsBadTarball(t *testing.T) {
	task, repo := fixtureRepo(t)
	task.Task.AgentTimeoutS = 1
	start := time.Now()
	res, err := runTask(context.Background(), task, repo, "sleep 10")
	if err != nil || !res.TimedOut || time.Since(start) > 5*time.Second {
		t.Fatalf("expected timeout, got err=%v res=%+v", err, res)
	}
	task.Task.RepoSHA256 = "deadbeef"
	if _, err := runTask(context.Background(), task, repo, "true"); err == nil {
		t.Fatalf("sha mismatch must be an error")
	}
}
```

- [ ] **Step 2: Конфиг и клиент**

`backend/cmd/arena/config.go`:

```go
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

type config struct {
	URL   string `yaml:"url"`
	Agent struct {
		Command string `yaml:"command"`
	} `yaml:"agent"`
}

func arenaDir() (string, error) {
	if d := os.Getenv("ARENA_HOME"); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".arena"), nil
}

func loadConfig() (config, error) {
	dir, err := arenaDir()
	if err != nil {
		return config{}, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		return config{}, fmt.Errorf("read %s/config.yaml (run `arena init`): %w", dir, err)
	}
	var c config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return config{}, err
	}
	if v := os.Getenv("ARENA_URL"); v != "" {
		c.URL = v
	}
	c.URL = strings.TrimRight(c.URL, "/")
	if c.URL == "" || c.Agent.Command == "" {
		return config{}, errors.New("config.yaml needs url and agent.command")
	}
	return c, nil
}

func loadKey() (string, error) {
	dir, err := arenaDir()
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "key"))
	if err != nil {
		return "", fmt.Errorf("read %s/key (run `arena login`): %w", dir, err)
	}
	return strings.TrimSpace(string(raw)), nil
}

const defaultConfig = `# Agent Arena connector configuration.
url: %s
agent:
  # Any shell command. It runs in the root of the task repository; TASK.md
  # describes the task. Everything the command prints stays on this machine
  # except a redacted 32 KiB tail sent with the result.
  command: claude -p "$(cat TASK.md)" --dangerously-skip-permissions
`
```

`backend/cmd/arena/client.go`:

```go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const version = "0.1.0"

type client struct {
	base string
	key  string
	http *http.Client
}

type taskInfo struct {
	Slug          string `json:"slug"`
	Title         string `json:"title"`
	TaskMD        string `json:"task_md"`
	AgentTimeoutS int    `json:"agent_timeout_s"`
	RepoSHA256    string `json:"repo_sha256"`
}

type nextTask struct {
	ProofID string   `json:"proof_id"`
	Task    taskInfo `json:"task"`
}

type heartbeatResp struct {
	Agent struct {
		Name  string `json:"name"`
		Stage string `json:"stage"`
	} `json:"agent"`
}

type result struct {
	Diff       string `json:"diff"`
	LogTail    string `json:"log_tail"`
	DurationMS int    `json:"duration_ms"`
	ExitCode   int    `json:"exit_code"`
	TimedOut   bool   `json:"-"`
}

type apiError struct {
	Status int
	Code   string
	Msg    string
}

func (e *apiError) Error() string { return fmt.Sprintf("%d %s: %s", e.Status, e.Code, e.Msg) }

func (c *client) do(ctx context.Context, method, path string, body any, out any) error {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "arena-connector/"+version)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 400 {
		var p struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &p)
		return &apiError{Status: resp.StatusCode, Code: p.Code, Msg: p.Message}
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		if b, ok := out.(*[]byte); ok {
			*b = raw
		}
		return nil
	}
	if b, ok := out.(*[]byte); ok {
		*b = raw
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (c *client) Heartbeat(ctx context.Context) (heartbeatResp, error) {
	host, _ := os.Hostname()
	var out heartbeatResp
	err := c.do(ctx, http.MethodPost, "/api/v1/connector/heartbeat", map[string]string{"connector_version": version, "hostname": host}, &out)
	return out, err
}

// NextTask long-polls; nil, nil means nothing yet.
func (c *client) NextTask(ctx context.Context, wait time.Duration) (*nextTask, error) {
	var raw []byte
	err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/connector/tasks/next?wait=%d", int(wait.Seconds())), nil, &raw)
	if err != nil || len(raw) == 0 {
		return nil, err
	}
	var t nextTask
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (c *client) Repo(ctx context.Context, proofID string) ([]byte, error) {
	var raw []byte
	err := c.do(ctx, http.MethodGet, "/api/v1/connector/proofs/"+proofID+"/repo.tar.gz", nil, &raw)
	return raw, err
}

func (c *client) Started(ctx context.Context, proofID string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/connector/proofs/"+proofID+"/started", map[string]string{}, nil)
}

// Result retries on network errors; a 409 means the server already has it.
func (c *client) Result(ctx context.Context, proofID string, r result) error {
	delay := 2 * time.Second
	for attempt := 1; ; attempt++ {
		err := c.do(ctx, http.MethodPost, "/api/v1/connector/proofs/"+proofID+"/result", r, nil)
		var ae *apiError
		if err == nil || (errors.As(err, &ae) && ae.Status == http.StatusConflict) {
			return nil
		}
		if errors.As(err, &ae) || attempt >= 6 {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
	}
}
```

- [ ] **Step 3: Прогон задачи**

`backend/cmd/arena/run.go`:

```go
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"tolerance/internal/platform/sanitize"
	"tolerance/internal/proofs"
)

const logTailBytes = 32 << 10

func git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=arena", "GIT_AUTHOR_EMAIL=arena@localhost",
		"GIT_COMMITTER_NAME=arena", "GIT_COMMITTER_EMAIL=arena@localhost", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("git %v: %w: %s", args, err, bytes.TrimSpace(out))
	}
	return out, nil
}

// runTask unpacks the repo, runs the owner's agent command against it and
// returns the diff of what the agent changed. Nothing the agent prints
// leaves this machine except the redacted tail in result.LogTail.
func runTask(ctx context.Context, task nextTask, repo []byte, command string) (result, error) {
	sum := sha256.Sum256(repo)
	if hex.EncodeToString(sum[:]) != task.Task.RepoSHA256 {
		return result{}, errors.New("repository tarball checksum mismatch")
	}
	dir, err := os.MkdirTemp("", "arena-"+task.Task.Slug+"-")
	if err != nil {
		return result{}, err
	}
	defer os.RemoveAll(dir)
	if err := proofs.Untar(repo, dir); err != nil {
		return result{}, err
	}
	if _, err := git(ctx, dir, "init", "-q"); err != nil {
		return result{}, err
	}
	if _, err := git(ctx, dir, "add", "-A"); err != nil {
		return result{}, err
	}
	if _, err := git(ctx, dir, "commit", "-q", "-m", "task"); err != nil {
		return result{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, "TASK.md"), []byte(task.Task.TaskMD), 0o644); err != nil {
		return result{}, err
	}

	logFile, err := os.Create(filepath.Join(os.TempDir(), "arena-"+task.ProofID+".log"))
	if err != nil {
		return result{}, err
	}
	defer logFile.Close()

	timeout := time.Duration(task.Task.AgentTimeoutS) * time.Second
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, "sh", "-c", command)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.Env = append(os.Environ(), "ARENA_TASK="+task.Task.Slug, "ARENA_PROOF="+task.ProofID)
	start := time.Now()
	runErr := cmd.Run()
	res := result{DurationMS: int(time.Since(start).Milliseconds())}
	var exitErr *exec.ExitError
	switch {
	case errors.Is(runCtx.Err(), context.DeadlineExceeded):
		res.TimedOut, res.ExitCode = true, -1
	case runErr == nil:
		res.ExitCode = 0
	case errors.As(runErr, &exitErr):
		res.ExitCode = exitErr.ExitCode()
	default:
		return result{}, fmt.Errorf("start agent command: %w", runErr)
	}

	raw, _ := os.ReadFile(logFile.Name())
	res.LogTail = sanitize.CleanLog(string(raw), logTailBytes)

	_ = os.Remove(filepath.Join(dir, "TASK.md"))
	if _, err := git(ctx, dir, "add", "-A"); err != nil {
		return result{}, err
	}
	diff, err := git(ctx, dir, "diff", "--cached", "--no-color")
	if err != nil {
		return result{}, err
	}
	res.Diff = string(diff)
	return res, nil
}
```

Run: `go test ./cmd/arena/ -run TestRunTask -v` → PASS (нужны `git` и `sh`).

- [ ] **Step 4: Тест клиента против httptest**

`backend/cmd/arena/client_test.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_ResultRetriesAndTreats409AsDone(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ak_test" {
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/api/v1/connector/proofs/p1/result":
			n := atomic.AddInt32(&calls, 1)
			if n == 1 {
				hj, _ := w.(http.Hijacker)
				conn, _, _ := hj.Hijack()
				conn.Close() // simulate a dropped connection
				return
			}
			w.WriteHeader(409)
			_ = json.NewEncoder(w).Encode(map[string]string{"code": "state_conflict", "message": "already"})
		case "/api/v1/connector/tasks/next":
			w.WriteHeader(204)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := &client{base: srv.URL, key: "ak_test", http: srv.Client()}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Result(ctx, "p1", result{Diff: "x"}); err != nil {
		t.Fatalf("expected success after retry + 409, got %v", err)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
	nt, err := c.NextTask(ctx, time.Second)
	if err != nil || nt != nil {
		t.Fatalf("204 must be nil, nil: %v %+v", err, nt)
	}
}
```

- [ ] **Step 5: main**

`backend/cmd/arena/main.go`:

```go
// Command arena is the connector an agent owner runs on their own machine.
// It keeps the agent online, receives proof tasks, runs the owner's agent
// command locally and sends back only the resulting diff.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "login":
		err = cmdLogin()
	case "init":
		err = cmdInit()
	case "connect":
		err = cmdConnect()
	case "status":
		err = cmdStatus()
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "arena:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: arena <login|init|connect|status>

  login    read an API key from stdin and store it in ~/.arena/key
  init     write ~/.arena/config.yaml with the agent command to edit
  connect  stay online and run proof tasks as they arrive
  status   show the agent's stage`)
}

func cmdLogin() error {
	dir, err := arenaDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	fmt.Fprint(os.Stderr, "Paste the API key: ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	key := strings.TrimSpace(line)
	if !strings.HasPrefix(key, "ak_") {
		return errors.New("that does not look like an API key (expected ak_…)")
	}
	if err := os.WriteFile(filepath.Join(dir, "key"), []byte(key+"\n"), 0o600); err != nil {
		return err
	}
	fmt.Println("Key saved to", filepath.Join(dir, "key"))
	return nil
}

func cmdInit() error {
	dir, err := arenaDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, "config.yaml")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists; edit it instead", path)
	}
	url := os.Getenv("ARENA_URL")
	if url == "" {
		url = "https://arena.example.com"
	}
	if err := os.WriteFile(path, []byte(fmt.Sprintf(defaultConfig, url)), 0o600); err != nil {
		return err
	}
	fmt.Println("Wrote", path, "— set agent.command to how your agent is started.")
	return nil
}

func newClient() (*client, config, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, cfg, err
	}
	key, err := loadKey()
	if err != nil {
		return nil, cfg, err
	}
	return &client{base: cfg.URL, key: key, http: &http.Client{Timeout: 60 * time.Second}}, cfg, nil
}

func cmdStatus() error {
	c, _, err := newClient()
	if err != nil {
		return err
	}
	hb, err := c.Heartbeat(context.Background())
	if err != nil {
		return err
	}
	fmt.Printf("%s: %s\n", hb.Agent.Name, hb.Agent.Stage)
	return nil
}

func cmdConnect() error {
	c, cfg, err := newClient()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	hb, err := c.Heartbeat(ctx)
	if err != nil {
		return fmt.Errorf("heartbeat: %w", err)
	}
	fmt.Printf("%s is online (%s). Waiting for tasks; Ctrl-C to stop.\n", hb.Agent.Name, hb.Agent.Stage)

	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if _, err := c.Heartbeat(ctx); err != nil {
					fmt.Fprintln(os.Stderr, "heartbeat:", err)
				}
			}
		}
	}()

	backoff := time.Second
	for ctx.Err() == nil {
		task, err := c.NextTask(ctx, 25*time.Second)
		if err != nil {
			var ae *apiError
			if errors.As(err, &ae) && ae.Status == http.StatusUnauthorized {
				return errors.New("API key rejected; run `arena login` with a fresh key")
			}
			fmt.Fprintln(os.Stderr, "poll:", err)
			select {
			case <-ctx.Done():
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, time.Minute)
			continue
		}
		backoff = time.Second
		if task == nil {
			continue
		}
		fmt.Printf("Task %s (%s): running your agent, up to %ds\n", task.Task.Slug, task.ProofID, task.Task.AgentTimeoutS)
		repo, err := c.Repo(ctx, task.ProofID)
		if err != nil {
			fmt.Fprintln(os.Stderr, "download repo:", err)
			continue
		}
		_ = c.Started(ctx, task.ProofID)
		res, err := runTask(ctx, *task, repo, cfg.Agent.Command)
		if err != nil {
			fmt.Fprintln(os.Stderr, "run:", err)
			res = result{LogTail: "connector error: " + err.Error(), ExitCode: -1}
		}
		if err := c.Result(ctx, task.ProofID, res); err != nil {
			fmt.Fprintln(os.Stderr, "send result:", err)
			continue
		}
		fmt.Printf("Result sent (exit %d, %d bytes of diff). Check the dashboard for the verdict.\n", res.ExitCode, len(res.Diff))
	}
	return nil
}
```

`go get go.yaml.in/yaml/v3` (переводит его из indirect в прямую зависимость).

- [ ] **Step 6: Коммит**

```bash
cd backend && gofmt -l . && go vet ./... && go test ./cmd/arena/ -v && go build -o /tmp/arena ./cmd/arena && /tmp/arena 2>&1 | head -3
git add -A backend && git commit -m "Add the arena connector CLI: login, init, connect, status

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 10: OpenAPI-контракт и сквозной тест

**Files:**
- Modify: `backend/contracts/openapi/openapi.yaml`, `backend/cmd/api/main_test.go`

**Interfaces:**
- Consumes: всё из задач 2–8. Тест поднимает `newHandler` с `sandbox: "fake"` и запускает воркер вручную через `proofs.NewWorker(...).RunProof`.

- [ ] **Step 1: Контракт**

`backend/contracts/openapi/openapi.yaml` целиком:

```yaml
openapi: 3.0.3
info:
  title: Agent Arena API
  version: "0.3.0"
  description: Slice 1 — sign up, connect an agent, run a proof.
servers:
  - url: /api/v1
security:
  - cookieAuth: []
  - apiKeyAuth: []
  - {}

paths:
  /auth/signup:
    post:
      operationId: signup
      security: []
      requestBody: { $ref: "#/components/requestBodies/Credentials" }
      responses:
        "201": { $ref: "#/components/responses/UserEnvelope" }
        "409": { $ref: "#/components/responses/Problem" }
        "422": { $ref: "#/components/responses/Problem" }
        "429": { $ref: "#/components/responses/Problem" }
  /auth/login:
    post:
      operationId: login
      security: []
      requestBody: { $ref: "#/components/requestBodies/Credentials" }
      responses:
        "200": { $ref: "#/components/responses/UserEnvelope" }
        "401": { $ref: "#/components/responses/Problem" }
        "422": { $ref: "#/components/responses/Problem" }
        "429": { $ref: "#/components/responses/Problem" }
  /auth/logout:
    post:
      operationId: logout
      security: []
      responses:
        "204": { description: Signed out }

  /me:
    get:
      operationId: getMe
      responses:
        "200":
          description: OK
          content:
            application/json:
              schema:
                type: object
                required: [user, agent]
                properties:
                  user: { $ref: "#/components/schemas/User" }
                  agent:
                    nullable: true
                    allOf: [{ $ref: "#/components/schemas/AgentOverview" }]
        "401": { $ref: "#/components/responses/Problem" }

  /agent:
    post:
      operationId: createAgent
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [name]
              properties:
                name: { type: string }
                description: { type: string }
      responses:
        "201": { $ref: "#/components/responses/AgentPrivate" }
        "401": { $ref: "#/components/responses/Problem" }
        "409": { $ref: "#/components/responses/Problem" }
        "422": { $ref: "#/components/responses/Problem" }
    patch:
      operationId: patchAgent
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                name: { type: string }
                description: { type: string }
      responses:
        "200": { $ref: "#/components/responses/AgentPrivate" }
        "401": { $ref: "#/components/responses/Problem" }
        "404": { $ref: "#/components/responses/Problem" }
        "409": { $ref: "#/components/responses/Problem" }
        "422": { $ref: "#/components/responses/Problem" }
  /agent/keys:
    post:
      operationId: createKey
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                name: { type: string }
      responses:
        "201":
          description: Created; `key` is shown only here.
          content:
            application/json:
              schema:
                type: object
                required: [id, prefix, name, created_at, key]
                properties:
                  id: { type: string }
                  prefix: { type: string }
                  name: { type: string }
                  created_at: { type: string, format: date-time }
                  key: { type: string }
        "401": { $ref: "#/components/responses/Problem" }
        "404": { $ref: "#/components/responses/Problem" }
        "409": { $ref: "#/components/responses/Problem" }
  /agent/keys/{id}:
    delete:
      operationId: revokeKey
      parameters: [{ name: id, in: path, required: true, schema: { type: string } }]
      responses:
        "204": { description: Revoked }
        "401": { $ref: "#/components/responses/Problem" }
        "404": { $ref: "#/components/responses/Problem" }

  /proof-tasks:
    get:
      operationId: listProofTasks
      responses:
        "200":
          description: OK
          content:
            application/json:
              schema:
                type: object
                required: [items]
                properties:
                  items:
                    type: array
                    items: { $ref: "#/components/schemas/ProofTask" }
        "401": { $ref: "#/components/responses/Problem" }
  /proofs:
    get:
      operationId: listProofs
      responses:
        "200":
          description: OK
          content:
            application/json:
              schema:
                type: object
                required: [items]
                properties:
                  items:
                    type: array
                    items: { $ref: "#/components/schemas/Proof" }
        "401": { $ref: "#/components/responses/Problem" }
        "404": { $ref: "#/components/responses/Problem" }
    post:
      operationId: createProof
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [task_slug]
              properties:
                task_slug: { type: string }
      responses:
        "201": { $ref: "#/components/responses/ProofEnvelope" }
        "401": { $ref: "#/components/responses/Problem" }
        "404": { $ref: "#/components/responses/Problem" }
        "409": { $ref: "#/components/responses/Problem" }
        "422": { $ref: "#/components/responses/Problem" }
        "429": { $ref: "#/components/responses/Problem" }
  /proofs/{id}:
    get:
      operationId: getProof
      parameters: [{ name: id, in: path, required: true, schema: { type: string } }]
      responses:
        "200": { $ref: "#/components/responses/ProofEnvelope" }
        "401": { $ref: "#/components/responses/Problem" }
        "404": { $ref: "#/components/responses/Problem" }
  /proofs/{id}/retry:
    post:
      operationId: retryProof
      parameters: [{ name: id, in: path, required: true, schema: { type: string } }]
      responses:
        "200": { $ref: "#/components/responses/ProofEnvelope" }
        "401": { $ref: "#/components/responses/Problem" }
        "404": { $ref: "#/components/responses/Problem" }
        "409": { $ref: "#/components/responses/Problem" }

  /connector/heartbeat:
    post:
      operationId: heartbeat
      security: [{ apiKeyAuth: [] }]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                connector_version: { type: string }
                hostname: { type: string }
      responses:
        "200":
          description: OK
          content:
            application/json:
              schema:
                type: object
                required: [agent]
                properties:
                  agent:
                    type: object
                    required: [id, name, stage]
                    properties:
                      id: { type: string }
                      name: { type: string }
                      stage: { type: string }
        "401": { $ref: "#/components/responses/Problem" }
  /connector/tasks/next:
    get:
      operationId: nextTask
      security: [{ apiKeyAuth: [] }]
      parameters: [{ name: wait, in: query, schema: { type: integer } }]
      responses:
        "200":
          description: A task was claimed
          content:
            application/json:
              schema:
                type: object
                required: [proof_id, task]
                properties:
                  proof_id: { type: string }
                  task: { $ref: "#/components/schemas/ProofTask" }
        "204": { description: Nothing queued }
        "401": { $ref: "#/components/responses/Problem" }
  /connector/proofs/{id}/repo.tar.gz:
    get:
      operationId: proofRepo
      security: [{ apiKeyAuth: [] }]
      parameters: [{ name: id, in: path, required: true, schema: { type: string } }]
      responses:
        "200":
          description: gzip tarball
          content:
            application/gzip:
              schema: { type: string, format: binary }
        "401": { $ref: "#/components/responses/Problem" }
        "404": { $ref: "#/components/responses/Problem" }
  /connector/proofs/{id}/started:
    post:
      operationId: proofStarted
      security: [{ apiKeyAuth: [] }]
      parameters: [{ name: id, in: path, required: true, schema: { type: string } }]
      responses:
        "204": { description: Recorded }
        "401": { $ref: "#/components/responses/Problem" }
        "409": { $ref: "#/components/responses/Problem" }
  /connector/proofs/{id}/result:
    post:
      operationId: proofResult
      security: [{ apiKeyAuth: [] }]
      parameters: [{ name: id, in: path, required: true, schema: { type: string } }]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [diff]
              properties:
                diff: { type: string }
                log_tail: { type: string }
                duration_ms: { type: integer }
                exit_code: { type: integer }
      responses:
        "204": { description: Stored; sandbox run queued }
        "401": { $ref: "#/components/responses/Problem" }
        "409": { $ref: "#/components/responses/Problem" }
        "413": { $ref: "#/components/responses/Problem" }
        "422": { $ref: "#/components/responses/Problem" }

components:
  securitySchemes:
    cookieAuth: { type: apiKey, in: cookie, name: arena_session }
    apiKeyAuth: { type: http, scheme: bearer }
  requestBodies:
    Credentials:
      required: true
      content:
        application/json:
          schema:
            type: object
            required: [email, password]
            properties:
              email: { type: string }
              password: { type: string }
  responses:
    Problem:
      description: Error
      content:
        application/json:
          schema: { $ref: "#/components/schemas/Problem" }
    UserEnvelope:
      description: OK
      content:
        application/json:
          schema:
            type: object
            required: [user]
            properties:
              user: { $ref: "#/components/schemas/User" }
    AgentPrivate:
      description: OK
      content:
        application/json:
          schema: { $ref: "#/components/schemas/AgentPrivate" }
    ProofEnvelope:
      description: OK
      content:
        application/json:
          schema: { $ref: "#/components/schemas/Proof" }
  schemas:
    Problem:
      type: object
      required: [code, message]
      properties:
        code: { type: string }
        message: { type: string }
        request_id: { type: string }
        fields:
          type: array
          items:
            type: object
            required: [path, code]
            properties:
              path: { type: string }
              code: { type: string }
    User:
      type: object
      required: [id, email, role, created_at]
      properties:
        id: { type: string }
        email: { type: string }
        role: { type: string, enum: [user, admin] }
        created_at: { type: string, format: date-time }
    ApiKey:
      type: object
      required: [id, prefix, name, created_at, last_used_at]
      properties:
        id: { type: string }
        prefix: { type: string }
        name: { type: string }
        created_at: { type: string, format: date-time }
        last_used_at: { type: string, format: date-time, nullable: true }
    AgentPrivate:
      type: object
      required: [id, name, description, created_at, api_keys]
      properties:
        id: { type: string }
        name: { type: string }
        description: { type: string }
        created_at: { type: string, format: date-time }
        api_keys:
          type: array
          items: { $ref: "#/components/schemas/ApiKey" }
    Presence:
      type: object
      required: [last_seen_at, connector_version, hostname]
      properties:
        last_seen_at: { type: string, format: date-time }
        connector_version: { type: string }
        hostname: { type: string }
    AgentOverview:
      allOf:
        - { $ref: "#/components/schemas/AgentPrivate" }
        - type: object
          required: [stage, presence]
          properties:
            stage: { type: string, enum: [registered, offline, connected, checking, operational, check_failed] }
            presence:
              nullable: true
              allOf: [{ $ref: "#/components/schemas/Presence" }]
    ProofTask:
      type: object
      required: [slug, title, language, agent_timeout_s, sandbox_timeout_s, visible_tests, hidden_tests, task_md, repo_sha256]
      properties:
        slug: { type: string }
        title: { type: string }
        language: { type: string }
        agent_timeout_s: { type: integer }
        sandbox_timeout_s: { type: integer }
        visible_tests: { type: integer }
        hidden_tests: { type: integer }
        task_md: { type: string }
        repo_sha256: { type: string }
    TestResult:
      type: object
      required: [name, passed]
      properties:
        name: { type: string }
        passed: { type: boolean }
    SandboxResult:
      type: object
      required: [tests, exit_code, output, timed_out]
      properties:
        tests:
          type: array
          items: { $ref: "#/components/schemas/TestResult" }
        exit_code: { type: integer }
        output: { type: string }
        timed_out: { type: boolean }
    Proof:
      type: object
      required: [id, agent_id, task_slug, status, created_at, claimed_at, diff_submitted_at, finished_at, diff, agent_log_tail, agent_duration_ms, agent_exit_code, sandbox_result, failure_reason]
      properties:
        id: { type: string }
        agent_id: { type: string }
        task_slug: { type: string }
        status: { type: string, enum: [queued, claimed, running_agent, diff_submitted, running_sandbox, passed, failed, infra_error, expired] }
        created_at: { type: string, format: date-time }
        claimed_at: { type: string, format: date-time, nullable: true }
        diff_submitted_at: { type: string, format: date-time, nullable: true }
        finished_at: { type: string, format: date-time, nullable: true }
        diff: { type: string }
        agent_log_tail: { type: string }
        agent_duration_ms: { type: integer, nullable: true }
        agent_exit_code: { type: integer, nullable: true }
        sandbox_result:
          nullable: true
          allOf: [{ $ref: "#/components/schemas/SandboxResult" }]
        failure_reason: { type: string }
```

Проверка: `go test ./contracts/openapi/` (если нет теста, добавить `openapi_test.go` с `func TestDocLoads(t *testing.T){ if _, err := Doc(); err != nil { t.Fatal(err) } }`).

- [ ] **Step 2: Сквозной тест**

Заменить `backend/cmd/api/main_test.go` целиком:

```go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/routers"

	"tolerance/contracts/openapi"
	"tolerance/internal/agents"
	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/ratelimit"
	"tolerance/internal/proofs"
	"tolerance/internal/proofs/sandbox"
)

type e2e struct {
	srv    *httptest.Server
	router routers.Router
	worker *proofs.Worker
	fake   *sandbox.Fake
}

func newE2E(t *testing.T) *e2e {
	t.Helper()
	d := dbtest.New(t)
	ctx := context.Background()
	tasks, err := proofs.LoadCatalog(filepath.Join("..", "..", "fixtures", "proofs"))
	if err != nil {
		t.Fatal(err)
	}
	if err := proofs.SyncCatalog(ctx, d.AdminPool, tasks); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cfg := config{addr: "127.0.0.1:0", adminEmails: []string{"admin@arena.local"}, sandbox: "fake"}
	ps := proofs.NewService(d.AppPool)
	dp := deps{pool: d.AppPool, log: log, limiter: ratelimit.New(nil), users: identity.NewService(d.AppPool, cfg.adminEmails),
		agents: agents.NewService(d.AppPool, ps), proofs: ps}
	srv := httptest.NewServer(newHandler(cfg, dp))
	t.Cleanup(srv.Close)
	router, err := openapi.Router()
	if err != nil {
		t.Fatal(err)
	}
	fake := &sandbox.Fake{Result: sandbox.Result{ExitCode: 0, Tests: []sandbox.TestResult{{Name: "TestHidden_All", Passed: true}}}}
	return &e2e{srv: srv, router: router, worker: proofs.NewWorker(d.AppPool, fake, t.TempDir(), log), fake: fake}
}

func (e *e2e) browser(t *testing.T) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

// call performs a request (cookie jar on the client, optional bearer key),
// validates the response against openapi.yaml and decodes into out.
func (e *e2e) call(t *testing.T, c *http.Client, method, path, key string, body any, out any) int {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	openapi.ValidateResponse(t, e.router, req, resp, raw)
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("decode %s %s: %v\n%s", method, path, err, raw)
		}
	}
	return resp.StatusCode
}

func TestEndToEnd_SignupConnectProve(t *testing.T) {
	e := newE2E(t)
	owner := e.browser(t)
	plain := &http.Client{}

	// anonymous
	if code := e.call(t, plain, "GET", "/api/v1/me", "", nil, nil); code != 401 {
		t.Fatalf("anonymous /me: %d", code)
	}

	// signup, me
	var me struct {
		User  identity.User    `json:"user"`
		Agent *agents.Overview `json:"agent"`
	}
	if code := e.call(t, owner, "POST", "/api/v1/auth/signup", "", map[string]string{"email": "Owner@Example.com", "password": "longenough1"}, nil); code != 201 {
		t.Fatalf("signup: %d", code)
	}
	if code := e.call(t, owner, "POST", "/api/v1/auth/signup", "", map[string]string{"email": "owner@example.com", "password": "longenough1"}, nil); code != 409 {
		t.Fatalf("duplicate signup: %d", code)
	}
	e.call(t, owner, "GET", "/api/v1/me", "", nil, &me)
	if me.User.Email != "owner@example.com" || me.Agent != nil {
		t.Fatalf("me after signup: %+v", me)
	}

	// agent + key
	var created agents.Private
	if code := e.call(t, owner, "POST", "/api/v1/agent", "", map[string]string{"name": "fixer-7", "description": "go"}, &created); code != 201 {
		t.Fatalf("create agent: %d", code)
	}
	var keyResp struct {
		Key string `json:"key"`
		ID  string `json:"id"`
	}
	if code := e.call(t, owner, "POST", "/api/v1/agent/keys", "", map[string]string{"name": "laptop"}, &keyResp); code != 201 {
		t.Fatalf("create key: %d", code)
	}
	e.call(t, owner, "GET", "/api/v1/me", "", nil, &me)
	if me.Agent == nil || me.Agent.Stage != agents.StageRegistered || len(me.Agent.APIKeys) != 1 {
		t.Fatalf("me after key: %+v", me.Agent)
	}

	// proof before the connector is online
	if code := e.call(t, owner, "POST", "/api/v1/proofs", "", map[string]string{"task_slug": "go-fix-retry"}, nil); code != 409 {
		t.Fatalf("proof while offline: %d", code)
	}

	// connector
	var hb struct {
		Agent struct{ Stage string } `json:"agent"`
	}
	if code := e.call(t, plain, "POST", "/api/v1/connector/heartbeat", keyResp.Key, map[string]string{"connector_version": "0.1.0", "hostname": "laptop"}, &hb); code != 200 || hb.Agent.Stage != agents.StageConnected {
		t.Fatalf("heartbeat: %d %+v", code, hb)
	}
	if code := e.call(t, plain, "POST", "/api/v1/connector/heartbeat", "ak_bogus", map[string]string{}, nil); code != 401 {
		t.Fatalf("bogus key: %d", code)
	}
	if code := e.call(t, plain, "GET", "/api/v1/connector/tasks/next?wait=0", keyResp.Key, nil, nil); code != 204 {
		t.Fatalf("empty queue: %d", code)
	}

	// proof lifecycle
	var proof proofs.Proof
	if code := e.call(t, owner, "POST", "/api/v1/proofs", "", map[string]string{"task_slug": "go-fix-retry"}, &proof); code != 201 || proof.Status != proofs.StatusQueued {
		t.Fatalf("create proof: %d %+v", code, proof)
	}
	e.call(t, owner, "GET", "/api/v1/me", "", nil, &me)
	if me.Agent.Stage != agents.StageChecking {
		t.Fatalf("stage while queued: %s", me.Agent.Stage)
	}
	var next struct {
		ProofID string      `json:"proof_id"`
		Task    proofs.Task `json:"task"`
	}
	if code := e.call(t, plain, "GET", "/api/v1/connector/tasks/next?wait=1", keyResp.Key, nil, &next); code != 200 || next.ProofID != proof.ID || next.Task.TaskMD == "" {
		t.Fatalf("next: %d %+v", code, next)
	}
	req, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/connector/proofs/"+proof.ID+"/repo.tar.gz", nil)
	req.Header.Set("Authorization", "Bearer "+keyResp.Key)
	resp, err := plain.Do(req)
	if err != nil || resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/gzip" {
		t.Fatalf("repo: %v %v", err, resp)
	}
	resp.Body.Close()
	if code := e.call(t, plain, "POST", "/api/v1/connector/proofs/"+proof.ID+"/started", keyResp.Key, map[string]string{}, nil); code != 204 {
		t.Fatalf("started: %d", code)
	}
	diff := "--- a/retry.go\n+++ b/retry.go\n@@ -19,7 +19,14 @@ func Backoff(attempt int) time.Duration {\n \tif attempt < 1 {\n \t\treturn 0\n \t}\n-\treturn Base * time.Duration(attempt)\n+\tif attempt > 20 {\n+\t\treturn Max\n+\t}\n+\td := Base << uint(attempt-1)\n+\tif d > Max {\n+\t\treturn Max\n+\t}\n+\treturn d\n }\n \n // Do calls fn until it succeeds or maxAttempts is used up.\n"
	res := map[string]any{"diff": diff, "log_tail": "fixed\n", "duration_ms": 4200, "exit_code": 0}
	if code := e.call(t, plain, "POST", "/api/v1/connector/proofs/"+proof.ID+"/result", keyResp.Key, res, nil); code != 204 {
		t.Fatalf("result: %d", code)
	}
	if code := e.call(t, plain, "POST", "/api/v1/connector/proofs/"+proof.ID+"/result", keyResp.Key, res, nil); code != 409 {
		t.Fatalf("duplicate result: %d", code)
	}
	e.call(t, owner, "GET", "/api/v1/proofs/"+proof.ID, "", nil, &proof)
	if proof.Status != proofs.StatusDiffSubmitted {
		t.Fatalf("after result: %s", proof.Status)
	}

	// sandbox (fake) via the worker
	if err := e.worker.RunProof(context.Background(), proof.ID); err != nil {
		t.Fatalf("run proof: %v", err)
	}
	e.call(t, owner, "GET", "/api/v1/proofs/"+proof.ID, "", nil, &proof)
	if proof.Status != proofs.StatusPassed || proof.SandboxResult == nil || len(proof.SandboxResult.Tests) != 1 {
		t.Fatalf("after sandbox: %+v", proof)
	}
	e.call(t, owner, "GET", "/api/v1/me", "", nil, &me)
	if me.Agent.Stage != agents.StageOperational {
		t.Fatalf("final stage: %s", me.Agent.Stage)
	}
	var list struct{ Items []proofs.Proof }
	e.call(t, owner, "GET", "/api/v1/proofs", "", nil, &list)
	if len(list.Items) != 1 || list.Items[0].Diff != "" {
		t.Fatalf("list: %+v", list)
	}

	// revoke key → connector dies, owner can no longer start a proof once presence goes stale
	if code := e.call(t, owner, "DELETE", "/api/v1/agent/keys/"+keyResp.ID, "", nil, nil); code != 204 {
		t.Fatalf("revoke: %d", code)
	}
	if code := e.call(t, plain, "POST", "/api/v1/connector/heartbeat", keyResp.Key, map[string]string{}, nil); code != 401 {
		t.Fatalf("revoked key heartbeat: %d", code)
	}
	e.call(t, owner, "GET", "/api/v1/me", "", nil, &me)
	if me.Agent.Stage != agents.StageRegistered {
		t.Fatalf("stage after revoke: %s", me.Agent.Stage)
	}

	// logout
	if code := e.call(t, owner, "POST", "/api/v1/auth/logout", "", nil, nil); code != 204 {
		t.Fatalf("logout: %d", code)
	}
	if code := e.call(t, owner, "GET", "/api/v1/me", "", nil, nil); code != 401 {
		t.Fatalf("me after logout: %d", code)
	}
	_ = time.Second
}

func TestLogin_RateLimited(t *testing.T) {
	e := newE2E(t)
	c := e.browser(t)
	for i := 0; i < 10; i++ {
		e.call(t, c, "POST", "/api/v1/auth/login", "", map[string]string{"email": "x@example.com", "password": "wrongwrongwrong"}, nil)
	}
	if code := e.call(t, c, "POST", "/api/v1/auth/login", "", map[string]string{"email": "x@example.com", "password": "wrongwrongwrong"}, nil); code != 429 {
		t.Fatalf("11th attempt: %d", code)
	}
}
```

Run: `cd backend && ARENA_TEST_REQUIRE_DOCKER=1 go test ./cmd/api/ -v` → PASS. Любое расхождение ответа с контрактом падает с текстом нарушения; править контракт или код, не тест.

- [ ] **Step 3: Коммит**

```bash
cd backend && gofmt -l . && go vet ./... && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./...
git add -A backend && git commit -m "Add slice 1 OpenAPI contract and end-to-end test

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 11: Фронт: каркас из proofwork, API-клиент, вход и регистрация

**Files:**
- Delete: всё содержимое `frontend/` кроме `Dockerfile`, `.dockerignore`
- Create (из архива `proofwork-frontend-development.zip`, распакованного в `/tmp/pw`): `frontend/app/globals.css`, `frontend/components/ui/{button,card,input,label,badge,separator,skeleton,table,dialog}.tsx`, `frontend/components/status-dot.tsx`, `frontend/lib/utils.ts`, `frontend/components.json`, `frontend/postcss.config.mjs`, `frontend/tsconfig.json`, `frontend/pnpm-workspace.yaml`
- Create: `frontend/package.json`, `frontend/next.config.mjs`, `frontend/lib/api.ts`, `frontend/lib/types.ts`, `frontend/lib/use-me.ts`, `frontend/app/layout.tsx`, `frontend/app/page.tsx`, `frontend/app/login/page.tsx`, `frontend/app/signup/page.tsx`, `frontend/components/auth-form.tsx`, `frontend/app/app/layout.tsx`, `frontend/components/app-shell.tsx`

**Interfaces:**
- Produces: `api<T>(path, init?) : Promise<T>` бросает `ApiError{status, code, message}`; типы `User, ApiKey, AgentOverview, Me, ProofTask, Proof, Stage`; хук `useMe()` → `{me, loading, error, refresh}`; `AppShell` с навигацией и выходом; страницы `/login`, `/signup`; `/` редиректит на `/app` или `/login`.

- [ ] **Step 1: Пересоздать каталог**

```bash
cd /Users/nualimov/IdeaProjects/tolerance
rm -rf /tmp/pw && mkdir -p /tmp/pw && unzip -q proofwork-frontend-development.zip -d /tmp/pw
git rm -rq frontend && mkdir -p frontend/app frontend/components/ui frontend/lib
git checkout HEAD -- frontend/Dockerfile frontend/.dockerignore
cp /tmp/pw/app/globals.css frontend/app/
for f in button card input label badge separator skeleton table dialog; do cp /tmp/pw/components/ui/$f.tsx frontend/components/ui/; done
cp /tmp/pw/components/status-dot.tsx frontend/components/
cp /tmp/pw/lib/utils.ts frontend/lib/
cp /tmp/pw/components.json /tmp/pw/postcss.config.mjs /tmp/pw/tsconfig.json /tmp/pw/pnpm-workspace.yaml frontend/
```

Проверить, что скопированные `ui/*` не импортируют ничего кроме `@base-ui/react`, `class-variance-authority`, `lucide-react`, `@/lib/utils`: `grep -h "^import" frontend/components/ui/*.tsx | sort -u`. Всё лишнее (например `sonner`, `next-themes`) удалить вместе с файлом, который его тянет.

- [ ] **Step 2: package.json и next.config**

`frontend/package.json`:

```json
{
  "name": "arena-web",
  "version": "0.1.0",
  "private": true,
  "packageManager": "pnpm@12.3.4",
  "scripts": {
    "dev": "next dev",
    "build": "next build",
    "start": "next start",
    "typecheck": "tsc --noEmit"
  },
  "dependencies": {
    "@base-ui/react": "^1.5.0",
    "class-variance-authority": "^0.7.1",
    "clsx": "^2.1.1",
    "lucide-react": "^1.16.0",
    "next": "16.3.3",
    "react": "^19",
    "react-dom": "^19",
    "tailwind-merge": "^3.3.1",
    "tw-animate-css": "^1.4.0"
  },
  "devDependencies": {
    "@tailwindcss/postcss": "^4.3.3",
    "@types/node": "^24",
    "@types/react": "^19",
    "@types/react-dom": "^19",
    "postcss": "^8.5",
    "tailwindcss": "^4.3.3",
    "typescript": "5.7.3"
  }
}
```

`frontend/next.config.mjs`:

```js
/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'standalone',
  // The browser always talks to /api on the site's own origin; Next proxies
  // it to the Go API so the session cookie is first-party everywhere.
  async rewrites() {
    const api = process.env.API_URL ?? 'http://127.0.0.1:8080'
    return [{ source: '/api/:path*', destination: `${api}/api/:path*` }]
  },
}

export default nextConfig
```

Run: `cd frontend && pnpm install` → lockfile создаётся заново.

- [ ] **Step 3: Типы и клиент**

`frontend/lib/types.ts`:

```ts
export type Stage = 'registered' | 'offline' | 'connected' | 'checking' | 'operational' | 'check_failed'

export type ProofStatus =
  | 'queued' | 'claimed' | 'running_agent' | 'diff_submitted' | 'running_sandbox'
  | 'passed' | 'failed' | 'infra_error' | 'expired'

export interface User { id: string; email: string; role: 'user' | 'admin'; created_at: string }
export interface ApiKey { id: string; prefix: string; name: string; created_at: string; last_used_at: string | null }
export interface Presence { last_seen_at: string; connector_version: string; hostname: string }
export interface AgentOverview {
  id: string; name: string; description: string; created_at: string; api_keys: ApiKey[]
  stage: Stage; presence: Presence | null
}
export interface Me { user: User; agent: AgentOverview | null }
export interface ProofTask {
  slug: string; title: string; language: string; agent_timeout_s: number; sandbox_timeout_s: number
  visible_tests: number; hidden_tests: number; task_md: string; repo_sha256: string
}
export interface TestResult { name: string; passed: boolean }
export interface SandboxResult { tests: TestResult[]; exit_code: number; output: string; timed_out: boolean }
export interface Proof {
  id: string; agent_id: string; task_slug: string; status: ProofStatus
  created_at: string; claimed_at: string | null; diff_submitted_at: string | null; finished_at: string | null
  diff: string; agent_log_tail: string; agent_duration_ms: number | null; agent_exit_code: number | null
  sandbox_result: SandboxResult | null; failure_reason: string
}
```

`frontend/lib/api.ts`:

```ts
export class ApiError extends Error {
  constructor(public status: number, public code: string, message: string) {
    super(message)
  }
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const res = await fetch(`/api/v1${path}`, {
    ...init,
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...(init.headers ?? {}) },
  })
  if (res.status === 204) return undefined as T
  const text = await res.text()
  const body = text ? JSON.parse(text) : null
  if (!res.ok) {
    throw new ApiError(res.status, body?.code ?? 'error', body?.message ?? res.statusText)
  }
  return body as T
}

export const post = <T,>(path: string, body?: unknown) =>
  api<T>(path, { method: 'POST', body: body === undefined ? undefined : JSON.stringify(body) })
```

`frontend/lib/use-me.ts`:

```ts
'use client'

import { useCallback, useEffect, useState } from 'react'
import { api, ApiError } from './api'
import type { Me } from './types'

export function useMe(pollMs = 0) {
  const [me, setMe] = useState<Me | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<ApiError | null>(null)
  const refresh = useCallback(async () => {
    try {
      setMe(await api<Me>('/me'))
      setError(null)
    } catch (e) {
      setError(e as ApiError)
    } finally {
      setLoading(false)
    }
  }, [])
  useEffect(() => {
    void refresh()
    if (!pollMs) return
    const t = setInterval(() => void refresh(), pollMs)
    return () => clearInterval(t)
  }, [refresh, pollMs])
  return { me, loading, error, refresh }
}
```

- [ ] **Step 4: Layout, редирект, формы входа**

`frontend/app/layout.tsx`:

```tsx
import type { Metadata } from 'next'
import './globals.css'

export const metadata: Metadata = { title: 'Agent Arena', description: 'Connect your agent and prove it works.' }

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body className="min-h-dvh bg-background text-foreground antialiased">{children}</body>
    </html>
  )
}
```

`frontend/app/page.tsx`:

```tsx
'use client'

import { useEffect } from 'react'
import { useRouter } from 'next/navigation'
import { useMe } from '@/lib/use-me'

export default function Home() {
  const { me, loading } = useMe()
  const router = useRouter()
  useEffect(() => {
    if (!loading) router.replace(me ? '/app' : '/login')
  }, [me, loading, router])
  return <p className="p-6 text-sm text-muted-foreground">Loading…</p>
}
```

`frontend/components/auth-form.tsx`:

```tsx
'use client'

import { useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { post, ApiError } from '@/lib/api'

export function AuthForm({ mode }: { mode: 'login' | 'signup' }) {
  const router = useRouter()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await post(`/auth/${mode}`, { email, password })
      router.replace('/app')
    } catch (err) {
      const a = err as ApiError
      setError(a.status === 429 ? 'Too many attempts, wait a minute.' : a.message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="mx-auto flex min-h-dvh max-w-sm flex-col justify-center px-4">
      <h1 className="text-2xl font-semibold">{mode === 'login' ? 'Sign in' : 'Create account'}</h1>
      <p className="mt-1 text-sm text-muted-foreground">
        {mode === 'login' ? 'Welcome back.' : 'Your agent will need an owner.'}
      </p>
      <form onSubmit={submit} className="mt-6 flex flex-col gap-4">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="email">Email</Label>
          <Input id="email" type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="password">Password</Label>
          <Input id="password" type="password" minLength={10} required autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
            value={password} onChange={(e) => setPassword(e.target.value)} />
          {mode === 'signup' && <p className="text-xs text-muted-foreground">At least 10 characters.</p>}
        </div>
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        <Button type="submit" disabled={busy}>{busy ? 'Please wait…' : mode === 'login' ? 'Sign in' : 'Create account'}</Button>
      </form>
      <p className="mt-6 text-sm text-muted-foreground">
        {mode === 'login' ? (
          <>No account? <Link className="underline" href="/signup">Create one</Link></>
        ) : (
          <>Already registered? <Link className="underline" href="/login">Sign in</Link></>
        )}
      </p>
    </main>
  )
}
```

`frontend/app/login/page.tsx`: `import { AuthForm } from '@/components/auth-form'; export default function Page() { return <AuthForm mode="login" /> }`. `frontend/app/signup/page.tsx`: то же с `mode="signup"`.

- [ ] **Step 5: Каркас кабинета**

`frontend/components/app-shell.tsx`:

```tsx
'use client'

import Link from 'next/link'
import { usePathname, useRouter } from 'next/navigation'
import { Button } from '@/components/ui/button'
import { post } from '@/lib/api'
import { cn } from '@/lib/utils'
import type { Me } from '@/lib/types'

const NAV = [
  { label: 'Home', href: '/app' },
  { label: 'Connect', href: '/app/agent/connect' },
]

export function AppShell({ me, children }: { me: Me; children: React.ReactNode }) {
  const pathname = usePathname()
  const router = useRouter()
  async function signOut() {
    await post('/auth/logout')
    router.replace('/login')
  }
  return (
    <div className="min-h-dvh bg-background">
      <header className="sticky top-0 z-40 border-b border-border bg-background/95 backdrop-blur-sm">
        <div className="mx-auto flex h-14 max-w-5xl items-center gap-4 px-4">
          <Link href="/app" className="text-sm font-semibold">{me.agent?.name ?? 'Agent Arena'}</Link>
          <nav className="flex items-center gap-1">
            {NAV.map((item) => (
              <Link key={item.href} href={item.href}
                className={cn('rounded-md px-3 py-1.5 text-sm text-muted-foreground hover:bg-muted hover:text-foreground',
                  (item.href === '/app' ? pathname === '/app' : pathname.startsWith(item.href)) && 'bg-muted text-foreground')}>
                {item.label}
              </Link>
            ))}
          </nav>
          <div className="ml-auto flex items-center gap-2">
            <span className="hidden text-xs text-muted-foreground sm:inline">{me.user.email}</span>
            <Button variant="outline" size="sm" onClick={signOut}>Sign out</Button>
          </div>
        </div>
      </header>
      <main className="mx-auto max-w-5xl px-4 py-6">{children}</main>
    </div>
  )
}
```

`frontend/app/app/layout.tsx`:

```tsx
'use client'

import { useEffect } from 'react'
import { useRouter } from 'next/navigation'
import { AppShell } from '@/components/app-shell'
import { useMe } from '@/lib/use-me'
import { Skeleton } from '@/components/ui/skeleton'

export default function OwnerLayout({ children }: { children: React.ReactNode }) {
  const { me, loading, error } = useMe()
  const router = useRouter()
  useEffect(() => {
    if (!loading && (error?.status === 401 || (!me && !error))) router.replace('/login')
  }, [me, loading, error, router])
  if (loading || !me) return <div className="p-6"><Skeleton className="h-8 w-48" /></div>
  return <AppShell me={me}>{children}</AppShell>
}
```

Домашняя страница `/app` появляется в задаче 12; чтобы сборка проходила, временно `frontend/app/app/page.tsx`: `export default function Page() { return null }`.

- [ ] **Step 6: Проверка и коммит**

```bash
cd frontend && pnpm typecheck && pnpm build
cd .. && git add -A frontend && git commit -m "Rebuild frontend shell on proofwork base with API client and auth pages

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

Ручная проверка: `make up` (API из задач 1–10 и `pnpm dev` с `API_URL=http://127.0.0.1:8080`), открыть `http://localhost:3000/signup`, зарегистрироваться, попасть на пустой `/app`, «Sign out» возвращает на `/login`.

---

### Task 12: Фронт: домашний экран по стадии, создание агента, подключение

**Files:**
- Create: `frontend/app/app/page.tsx`, `frontend/app/app/agent/new/page.tsx`, `frontend/app/app/agent/connect/page.tsx`, `frontend/components/stage-card.tsx`, `frontend/components/key-reveal.tsx`, `frontend/components/copy-block.tsx`, `frontend/components/proof-list.tsx`, `frontend/lib/format.ts`

**Interfaces:**
- Consumes: `useMe`, `api`, `post`, типы.
- Produces: страницы `/app`, `/app/agent/new`, `/app/agent/connect`; компонент `StageCard({me, proofs, onStart})`; `KeyReveal({keyValue})`; `CopyBlock({text})`; `ProofList({items})`.

- [ ] **Step 1: Форматирование и общие компоненты**

`frontend/lib/format.ts`:

```ts
export function ago(iso: string, now = Date.now()) {
  const s = Math.max(0, Math.round((now - new Date(iso).getTime()) / 1000))
  if (s < 60) return `${s}s ago`
  if (s < 3600) return `${Math.round(s / 60)}m ago`
  if (s < 86400) return `${Math.round(s / 3600)}h ago`
  return `${Math.round(s / 86400)}d ago`
}

export function duration(ms: number | null) {
  if (ms == null) return '—'
  const s = Math.round(ms / 1000)
  return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m ${s % 60}s`
}

export const STATUS_LABEL: Record<string, string> = {
  queued: 'Waiting for the connector',
  claimed: 'Connector picked it up',
  running_agent: 'Your agent is working',
  diff_submitted: 'Diff received',
  running_sandbox: 'Running hidden tests',
  passed: 'Passed',
  failed: 'Failed',
  infra_error: 'Platform error',
  expired: 'Expired',
}

export const REASON_LABEL: Record<string, string> = {
  diff_not_applicable: 'The diff did not apply to a clean copy of the repository.',
  empty_diff: 'The agent changed nothing.',
  build_failed: 'The code did not compile in the sandbox.',
  tests_failed: 'One or more hidden tests failed.',
  timeout: 'The tests ran out of time in the sandbox.',
  not_claimed: 'No connector picked the task up within 5 minutes. Is `arena connect` running?',
  agent_timeout: 'The agent did not return a result within the time limit.',
}
```

`frontend/components/copy-block.tsx`:

```tsx
'use client'

import { useState } from 'react'
import { Check, Copy } from 'lucide-react'

export function CopyBlock({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)
  async function copy() {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {}
  }
  return (
    <div className="flex items-start gap-2 rounded-md border border-border bg-muted/40 px-3 py-2">
      <pre className="flex-1 overflow-x-auto whitespace-pre-wrap break-all font-mono text-xs">{text}</pre>
      <button onClick={copy} aria-label="Copy" className="shrink-0 rounded p-1 text-muted-foreground hover:text-foreground">
        {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
      </button>
    </div>
  )
}
```

`frontend/components/key-reveal.tsx`:

```tsx
import { CopyBlock } from './copy-block'

export function KeyReveal({ keyValue }: { keyValue: string }) {
  return (
    <div className="rounded-md border border-warning/40 bg-warning/10 p-4">
      <p className="text-sm font-medium">Your API key. It is shown once; copy it now.</p>
      <div className="mt-2"><CopyBlock text={keyValue} /></div>
      <p className="mt-2 text-xs text-muted-foreground">On your machine: <code>arena login</code> and paste it.</p>
    </div>
  )
}
```

`frontend/components/proof-list.tsx`:

```tsx
import Link from 'next/link'
import type { Proof } from '@/lib/types'
import { STATUS_LABEL, ago } from '@/lib/format'
import { cn } from '@/lib/utils'

export function ProofList({ items }: { items: Proof[] }) {
  if (items.length === 0) return <p className="text-sm text-muted-foreground">No proofs yet.</p>
  return (
    <ul className="divide-y divide-border rounded-md border border-border">
      {items.map((p) => (
        <li key={p.id}>
          <Link href={`/app/proofs/${p.id}`} className="flex items-center justify-between gap-3 px-4 py-3 text-sm hover:bg-muted/40">
            <span className="font-mono text-xs">{p.task_slug}</span>
            <span className={cn('text-xs', p.status === 'passed' && 'text-success', p.status === 'failed' && 'text-destructive')}>{STATUS_LABEL[p.status]}</span>
            <span className="text-xs text-muted-foreground">{ago(p.created_at)}</span>
          </Link>
        </li>
      ))}
    </ul>
  )
}
```

Если в `globals.css` нет токенов `--success`/`--warning`, добавить в `:root` `--success: oklch(0.65 0.18 150); --warning: oklch(0.8 0.16 85);` и в `@theme inline` `--color-success: var(--success); --color-warning: var(--warning);`.

- [ ] **Step 2: Карточка стадии и домашний экран**

`frontend/components/stage-card.tsx`:

```tsx
import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import type { Me, Proof } from '@/lib/types'
import { ago } from '@/lib/format'

export function StageCard({ me, last, onStart, starting }: { me: Me; last: Proof | null; onStart: () => void; starting: boolean }) {
  const a = me.agent
  if (!a) {
    return (
      <Card className="p-6">
        <h2 className="text-lg font-semibold">Create your agent</h2>
        <p className="mt-1 text-sm text-muted-foreground">Give it a name. It will have to prove itself before anything else.</p>
        <Button render={<Link href="/app/agent/new" />} nativeButton={false} className="mt-4">Create agent</Button>
      </Card>
    )
  }
  const open = last && !['passed', 'failed', 'infra_error', 'expired'].includes(last.status)
  switch (a.stage) {
    case 'registered':
      return (
        <Card className="p-6">
          <h2 className="text-lg font-semibold">Connect {a.name}</h2>
          <p className="mt-1 text-sm text-muted-foreground">Install the connector on the machine where your agent runs and start it.</p>
          <Button render={<Link href="/app/agent/connect" />} nativeButton={false} className="mt-4">Show connect instructions</Button>
        </Card>
      )
    case 'offline':
      return (
        <Card className="p-6">
          <h2 className="text-lg font-semibold">{a.name} is offline</h2>
          <p className="mt-1 text-sm text-muted-foreground">Last seen {a.presence ? ago(a.presence.last_seen_at) : 'never'}. Run <code>arena connect</code> to bring it back.</p>
          <Button variant="outline" render={<Link href="/app/agent/connect" />} nativeButton={false} className="mt-4">Connect instructions</Button>
        </Card>
      )
    case 'checking':
      return (
        <Card className="p-6">
          <h2 className="text-lg font-semibold">Proof in progress</h2>
          <p className="mt-1 text-sm text-muted-foreground">{a.name} is working. You can watch, but you can’t help.</p>
          {open && <Button render={<Link href={`/app/proofs/${last.id}`} />} nativeButton={false} className="mt-4">Watch</Button>}
        </Card>
      )
    case 'check_failed':
      return (
        <Card className="p-6">
          <h2 className="text-lg font-semibold">Not verified yet</h2>
          <p className="mt-1 text-sm text-muted-foreground">The last proof failed. Read the breakdown, improve the agent, try again.</p>
          <div className="mt-4 flex gap-2">
            {last && <Button variant="outline" render={<Link href={`/app/proofs/${last.id}`} />} nativeButton={false}>See why</Button>}
            <Button onClick={onStart} disabled={starting}>Run proof again</Button>
          </div>
        </Card>
      )
    case 'operational':
      return (
        <Card className="p-6">
          <h2 className="text-lg font-semibold">{a.name} is operational</h2>
          <p className="mt-1 text-sm text-muted-foreground">It has proven it can take a task, change code and pass hidden tests on its own.</p>
          <Button variant="outline" onClick={onStart} disabled={starting} className="mt-4">Run the proof again</Button>
        </Card>
      )
    default:
      return (
        <Card className="p-6">
          <h2 className="text-lg font-semibold">{a.name} is online</h2>
          <p className="mt-1 text-sm text-muted-foreground">Run the basic proof: a small repository with a failing test. Your agent works alone; the platform runs hidden tests on its diff.</p>
          <Button onClick={onStart} disabled={starting} className="mt-4">{starting ? 'Starting…' : 'Run basic proof'}</Button>
        </Card>
      )
  }
}
```

`frontend/app/app/page.tsx`:

```tsx
'use client'

import { useCallback, useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'
import { StageCard } from '@/components/stage-card'
import { ProofList } from '@/components/proof-list'
import { api, post, ApiError } from '@/lib/api'
import { useMe } from '@/lib/use-me'
import type { Proof } from '@/lib/types'

export default function HomePage() {
  const { me, refresh } = useMe(5000)
  const router = useRouter()
  const [proofs, setProofs] = useState<Proof[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [starting, setStarting] = useState(false)

  const load = useCallback(async () => {
    if (!me?.agent) return setProofs([])
    try {
      setProofs((await api<{ items: Proof[] }>('/proofs')).items)
    } catch (e) {
      setError((e as ApiError).message)
    }
  }, [me?.agent])
  useEffect(() => { void load() }, [load, me?.agent?.stage])

  async function start() {
    setStarting(true)
    setError(null)
    try {
      const p = await post<Proof>('/proofs', { task_slug: 'go-fix-retry' })
      router.push(`/app/proofs/${p.id}`)
    } catch (e) {
      const a = e as ApiError
      setError(a.code === 'agent_offline' ? 'The connector is not online. Start `arena connect` first.' : a.message)
      await refresh()
    } finally {
      setStarting(false)
    }
  }

  if (!me) return null
  return (
    <div className="flex flex-col gap-6">
      <StageCard me={me} last={proofs?.[0] ?? null} onStart={start} starting={starting} />
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      <section>
        <h3 className="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">Proof history</h3>
        {proofs ? <ProofList items={proofs} /> : <p className="text-sm text-muted-foreground">Loading…</p>}
      </section>
    </div>
  )
}
```

- [ ] **Step 3: Создание агента**

`frontend/app/app/agent/new/page.tsx`:

```tsx
'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { KeyReveal } from '@/components/key-reveal'
import { post, ApiError } from '@/lib/api'

export default function NewAgentPage() {
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [key, setKey] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await post('/agent', { name, description })
      const k = await post<{ key: string }>('/agent/keys', { name: 'first' })
      setKey(k.key)
    } catch (err) {
      setError((err as ApiError).message)
    } finally {
      setBusy(false)
    }
  }

  if (key) {
    return (
      <div className="mx-auto max-w-xl space-y-4">
        <h1 className="text-2xl font-semibold">{name} is registered</h1>
        <KeyReveal keyValue={key} />
        <Button render={<Link href="/app/agent/connect" />} nativeButton={false}>Continue to connect</Button>
      </div>
    )
  }
  return (
    <form onSubmit={submit} className="mx-auto flex max-w-xl flex-col gap-4">
      <h1 className="text-2xl font-semibold">Create your agent</h1>
      <p className="text-sm text-muted-foreground">Anyone can claim a skill. Your agent has to prove it.</p>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="name">Name</Label>
        <Input id="name" required pattern="[A-Za-z0-9][A-Za-z0-9_-]{1,31}" value={name} onChange={(e) => setName(e.target.value)} className="font-mono" />
        <p className="text-xs text-muted-foreground">2–32 characters: letters, digits, - and _.</p>
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="description">Description</Label>
        <Input id="description" maxLength={500} value={description} onChange={(e) => setDescription(e.target.value)} />
      </div>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      <Button type="submit" disabled={busy || !name}>Create agent and get a key</Button>
    </form>
  )
}
```

- [ ] **Step 4: Страница подключения**

`frontend/app/app/agent/connect/page.tsx`:

```tsx
'use client'

import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { CopyBlock } from '@/components/copy-block'
import { KeyReveal } from '@/components/key-reveal'
import { StatusDot } from '@/components/status-dot'
import { api, post, ApiError } from '@/lib/api'
import { useMe } from '@/lib/use-me'
import { ago } from '@/lib/format'

export default function ConnectPage() {
  const { me, refresh } = useMe(5000)
  const [newKey, setNewKey] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  if (!me) return null
  const a = me.agent
  if (!a) return <p className="text-sm text-muted-foreground">Create an agent first.</p>
  const online = a.stage !== 'registered' && a.stage !== 'offline'
  const origin = typeof window === 'undefined' ? '' : window.location.origin

  async function issue() {
    setError(null)
    try {
      setNewKey((await post<{ key: string }>('/agent/keys', { name: 'key' })).key)
      await refresh()
    } catch (e) {
      setError((e as ApiError).message)
    }
  }
  async function revoke(id: string) {
    await api(`/agent/keys/${id}`, { method: 'DELETE' })
    await refresh()
  }

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div>
        <h1 className="text-2xl font-semibold">Connect {a.name}</h1>
        <p className="mt-1 flex items-center gap-2 text-sm text-muted-foreground">
          <StatusDot status={online ? 'online' : 'offline'} />
          {online ? `Online · ${a.presence?.hostname} · seen ${ago(a.presence!.last_seen_at)}` : a.presence ? `Offline · last seen ${ago(a.presence.last_seen_at)}` : 'Never connected'}
        </p>
      </div>
      <Card className="space-y-4 p-5 text-sm">
        <p>Run this on the machine where your agent lives. Your model keys, prompts and code never leave it; only a diff comes back.</p>
        <div><p className="mb-1 text-xs text-muted-foreground">1. Install</p><CopyBlock text="go install tolerance/cmd/arena@latest" /></div>
        <div><p className="mb-1 text-xs text-muted-foreground">2. Save your API key (paste it when asked)</p><CopyBlock text="arena login" /></div>
        <div><p className="mb-1 text-xs text-muted-foreground">3. Tell the connector how to start your agent</p><CopyBlock text={`ARENA_URL=${origin} arena init\n$EDITOR ~/.arena/config.yaml`} /></div>
        <div><p className="mb-1 text-xs text-muted-foreground">4. Go online</p><CopyBlock text="arena connect" /></div>
      </Card>
      <Card className="p-5">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-semibold">API keys</h2>
          <Button size="sm" variant="outline" onClick={issue} disabled={a.api_keys.length >= 5}>New key</Button>
        </div>
        {newKey && <div className="mt-3"><KeyReveal keyValue={newKey} /></div>}
        {error && <p role="alert" className="mt-2 text-sm text-destructive">{error}</p>}
        <ul className="mt-3 divide-y divide-border">
          {a.api_keys.map((k) => (
            <li key={k.id} className="flex items-center justify-between py-2 text-sm">
              <span className="font-mono text-xs">{k.prefix}… <span className="text-muted-foreground">{k.name}</span></span>
              <span className="text-xs text-muted-foreground">{k.last_used_at ? `used ${ago(k.last_used_at)}` : 'never used'}</span>
              <button onClick={() => revoke(k.id)} className="text-xs text-destructive hover:underline">Revoke</button>
            </li>
          ))}
          {a.api_keys.length === 0 && <li className="py-2 text-sm text-muted-foreground">No active keys. Issue one to connect.</li>}
        </ul>
      </Card>
    </div>
  )
}
```

Проверить сигнатуру скопированного `StatusDot` (`/tmp/pw/components/status-dot.tsx`); если его проп называется иначе, подстроить вызов, не компонент.

- [ ] **Step 5: Проверка и коммит**

```bash
cd frontend && pnpm typecheck && pnpm build
cd .. && git add -A frontend && git commit -m "Add owner home by agent stage, agent creation and connect page

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

Ручная проверка с запущенным API: создать агента → ключ показан один раз → страница Connect показывает «Never connected» → `arena login`, `arena init`, `arena connect` на своей машине → через 5 секунд «Online».

---

### Task 13: Фронт: страница проверки

**Files:**
- Create: `frontend/app/app/proofs/[id]/page.tsx`, `frontend/components/proof/timeline.tsx`, `frontend/components/proof/result.tsx`, `frontend/components/proof/diff-view.tsx`

**Interfaces:**
- Consumes: `api`, `post`, `Proof`, `STATUS_LABEL`, `REASON_LABEL`, `duration`.

- [ ] **Step 1: Таймлайн**

`frontend/components/proof/timeline.tsx`:

```tsx
import { Check, Loader2, X } from 'lucide-react'
import type { Proof } from '@/lib/types'
import { cn } from '@/lib/utils'

const STEPS: { label: string; reached: (p: Proof) => boolean }[] = [
  { label: 'Queued', reached: () => true },
  { label: 'Connector picked it up', reached: (p) => p.claimed_at != null },
  { label: 'Agent running', reached: (p) => ['running_agent', 'diff_submitted', 'running_sandbox', 'passed', 'failed'].includes(p.status) },
  { label: 'Diff received', reached: (p) => p.diff_submitted_at != null },
  { label: 'Hidden tests', reached: (p) => ['running_sandbox', 'passed', 'failed'].includes(p.status) },
  { label: 'Verdict', reached: (p) => p.finished_at != null },
]

export function Timeline({ proof }: { proof: Proof }) {
  const terminal = proof.finished_at != null
  const dead = proof.status === 'expired' || proof.status === 'infra_error'
  let activeShown = false
  return (
    <ol className="flex flex-col gap-0">
      {STEPS.map((s) => {
        const done = s.reached(proof)
        const active = !done && !activeShown && !terminal
        if (active) activeShown = true
        return (
          <li key={s.label} className="flex items-center gap-3 border-b border-border py-2.5 text-sm last:border-b-0">
            <span className={cn('flex size-5 items-center justify-center rounded-full border text-[10px]',
              done && !dead && 'border-success bg-success text-white', active && 'border-primary', dead && done && 'border-destructive text-destructive')}>
              {done && !dead && <Check className="size-3" strokeWidth={3} />}
              {done && dead && <X className="size-3" />}
              {active && <Loader2 className="size-3 animate-spin" />}
            </span>
            <span className={cn(done || active ? 'text-foreground' : 'text-muted-foreground')}>{s.label}</span>
          </li>
        )
      })}
    </ol>
  )
}
```

- [ ] **Step 2: Результат и diff**

`frontend/components/proof/diff-view.tsx`:

```tsx
import { cn } from '@/lib/utils'

export function DiffView({ diff }: { diff: string }) {
  if (!diff.trim()) return <p className="text-sm text-muted-foreground">Empty diff.</p>
  return (
    <pre className="max-h-96 overflow-auto rounded-md border border-border bg-muted/30 p-3 font-mono text-xs leading-5">
      {diff.split('\n').map((line, i) => (
        <div key={i} className={cn(line.startsWith('+') && !line.startsWith('+++') && 'bg-success/10 text-success',
          line.startsWith('-') && !line.startsWith('---') && 'bg-destructive/10 text-destructive',
          line.startsWith('@@') && 'text-muted-foreground')}>{line || ' '}</div>
      ))}
    </pre>
  )
}
```

`frontend/components/proof/result.tsx`:

```tsx
import { Card } from '@/components/ui/card'
import type { Proof } from '@/lib/types'
import { REASON_LABEL, duration } from '@/lib/format'
import { cn } from '@/lib/utils'

export function ProofResult({ proof }: { proof: Proof }) {
  const r = proof.sandbox_result
  const passed = proof.status === 'passed'
  return (
    <div className="space-y-4">
      <div>
        <h2 className={cn('text-xl font-semibold', passed ? 'text-success' : 'text-destructive')}>
          {passed ? 'Verified: your agent works on its own' : proof.status === 'failed' ? 'Not verified yet' : proof.status === 'expired' ? 'Expired' : 'Platform error'}
        </h2>
        {proof.failure_reason && <p className="mt-1 text-sm text-muted-foreground">{REASON_LABEL[proof.failure_reason] ?? proof.failure_reason}</p>}
        {proof.status === 'infra_error' && <p className="mt-1 text-sm text-muted-foreground">This one is on us, not on your agent. Retry costs you nothing.</p>}
      </div>
      <dl className="grid grid-cols-2 gap-2 text-sm sm:grid-cols-4">
        <div><dt className="text-xs text-muted-foreground">Agent time</dt><dd>{duration(proof.agent_duration_ms)}</dd></div>
        <div><dt className="text-xs text-muted-foreground">Agent exit code</dt><dd>{proof.agent_exit_code ?? '—'}</dd></div>
        <div><dt className="text-xs text-muted-foreground">Tests</dt><dd>{r ? `${r.tests.filter((t) => t.passed).length}/${r.tests.length}` : '—'}</dd></div>
        <div><dt className="text-xs text-muted-foreground">Diff</dt><dd>{proof.diff.length} bytes</dd></div>
      </dl>
      {r && r.tests.length > 0 && (
        <Card className="overflow-hidden p-0">
          <table className="w-full text-sm">
            <tbody className="divide-y divide-border">
              {r.tests.map((t) => (
                <tr key={t.name}>
                  <td className="px-4 py-2 font-mono text-xs">{t.name}</td>
                  <td className={cn('px-4 py-2 text-right text-xs', t.passed ? 'text-success' : 'text-destructive')}>{t.passed ? 'pass' : 'FAIL'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </Card>
      )}
      {r && !passed && r.output && (
        <details><summary className="cursor-pointer text-sm text-muted-foreground">Sandbox output</summary>
          <pre className="mt-2 max-h-72 overflow-auto rounded-md border border-border bg-muted/30 p-3 font-mono text-xs">{r.output}</pre></details>
      )}
      {proof.agent_log_tail && (
        <details><summary className="cursor-pointer text-sm text-muted-foreground">Agent log (redacted tail)</summary>
          <pre className="mt-2 max-h-72 overflow-auto rounded-md border border-border bg-muted/30 p-3 font-mono text-xs">{proof.agent_log_tail}</pre></details>
      )}
    </div>
  )
}
```

- [ ] **Step 3: Страница**

`frontend/app/app/proofs/[id]/page.tsx`:

```tsx
'use client'

import { use, useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Timeline } from '@/components/proof/timeline'
import { ProofResult } from '@/components/proof/result'
import { DiffView } from '@/components/proof/diff-view'
import { api, post, ApiError } from '@/lib/api'
import type { Proof } from '@/lib/types'
import { STATUS_LABEL } from '@/lib/format'

const TERMINAL = ['passed', 'failed', 'infra_error', 'expired']

export default function ProofPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params)
  const [proof, setProof] = useState<Proof | null>(null)
  const [error, setError] = useState<string | null>(null)
  const load = useCallback(async () => {
    try {
      setProof(await api<Proof>(`/proofs/${id}`))
    } catch (e) {
      setError((e as ApiError).status === 404 ? 'No such proof.' : (e as ApiError).message)
    }
  }, [id])
  useEffect(() => {
    void load()
    const t = setInterval(() => { if (!proof || !TERMINAL.includes(proof.status)) void load() }, 3000)
    return () => clearInterval(t)
  }, [load, proof?.status])

  async function retry() {
    try {
      setProof(await post<Proof>(`/proofs/${id}/retry`))
    } catch (e) {
      setError((e as ApiError).message)
    }
  }

  if (error) return <p role="alert" className="text-sm text-destructive">{error}</p>
  if (!proof) return <p className="text-sm text-muted-foreground">Loading…</p>
  const done = TERMINAL.includes(proof.status)
  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <p className="font-mono text-xs text-muted-foreground">{proof.task_slug}</p>
          <h1 className="text-2xl font-semibold">{done ? 'Result' : STATUS_LABEL[proof.status]}</h1>
        </div>
        <Link href="/app" className="text-sm text-muted-foreground hover:text-foreground">← Home</Link>
      </div>
      <Card className="p-4"><Timeline proof={proof} /></Card>
      {!done && <p className="text-sm text-muted-foreground">This page refreshes on its own. Your agent works alone; you can watch, but you can’t help.</p>}
      {done && <ProofResult proof={proof} />}
      {(proof.status === 'infra_error' || proof.status === 'expired') && <Button onClick={retry}>Retry</Button>}
      {proof.diff && (
        <section>
          <h3 className="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">Diff your agent produced</h3>
          <DiffView diff={proof.diff} />
        </section>
      )}
    </div>
  )
}
```

- [ ] **Step 4: Мобильная проверка и коммит**

```bash
cd frontend && pnpm typecheck && pnpm build && pnpm start &
sleep 5
for p in / /login /signup; do node -e "
const {chromium}=require('playwright');(async()=>{const b=await chromium.launch();const pg=await b.newPage({viewport:{width:375,height:800}});await pg.goto('http://localhost:3000$p');const w=await pg.evaluate(()=>document.documentElement.scrollWidth);console.log('$p',w);await b.close();process.exit(w>375?1:0)})()" 2>/dev/null || echo "no playwright: check manually in devtools at 375px"; done
kill %1
```

Если Playwright не установлен, проверить в DevTools на 375px: `document.documentElement.scrollWidth` равен 375 на `/login`, `/app`, `/app/agent/connect`, `/app/proofs/<id>`.

```bash
git add -A frontend && git commit -m "Add proof page: live timeline, verdict, tests and diff

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 14: Эксплуатация: compose с Caddy, бэкапы, CI, README, чистка

**Files:**
- Create: `Caddyfile`, `.github/workflows/ci.yml`, `backend/dev/backup.sh`
- Modify: `docker-compose.yml`, `.env.example`, `Makefile`, `backend/Dockerfile`, `backend/dev/entrypoint.sh`, `README.md`, `backend/README.md`, `.gitignore`
- Delete: `backend/docker-compose.yml`, `backend/Makefile` (цели переезжают в корневой), три zip-архива в корне

- [ ] **Step 1: Compose**

`docker-compose.yml` целиком:

```yaml
# Local: `make up` → web on :3000 (proxies /api to api), api on :8080.
# Server: same file with ARENA_DOMAIN set and `--profile prod` → Caddy
# terminates TLS and routes /api/* to api, everything else to web.
services:
  postgres:
    image: postgres:16-alpine
    restart: unless-stopped
    environment:
      POSTGRES_USER: arena_migrate
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: arena
    volumes:
      - arena-postgres-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U arena_migrate -d arena"]
      interval: 2s
      timeout: 2s
      retries: 30

  api:
    build: ./backend
    restart: unless-stopped
    environment:
      ARENA_ADDR: 0.0.0.0:8080
      ARENA_ALLOW_NON_LOOPBACK: "true"
      ARENA_APP_DATABASE_URL: postgres://arena_app:${ARENA_APP_ROLE_PASSWORD}@postgres:5432/arena?sslmode=disable
      ARENA_MIGRATE_DATABASE_URL: postgres://arena_migrate:${POSTGRES_PASSWORD}@postgres:5432/arena?sslmode=disable
      ARENA_APP_ROLE_PASSWORD: ${ARENA_APP_ROLE_PASSWORD}
      ARENA_ADMIN_EMAILS: ${ARENA_ADMIN_EMAILS:-}
      ARENA_SECURE_COOKIES: ${ARENA_SECURE_COOKIES:-false}
      ARENA_WORK_DIR: /var/lib/arena/work
      ARENA_SANDBOX: docker
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - arena-work:/var/lib/arena/work
    ports:
      - "127.0.0.1:${API_PORT:-8080}:8080"
    depends_on:
      postgres:
        condition: service_healthy

  web:
    build: ./frontend
    restart: unless-stopped
    environment:
      API_URL: http://api:8080
    ports:
      - "127.0.0.1:${WEB_PORT:-3000}:3000"
    depends_on:
      - api

  caddy:
    image: caddy:2-alpine
    profiles: [prod]
    restart: unless-stopped
    environment:
      ARENA_DOMAIN: ${ARENA_DOMAIN}
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - arena-caddy-data:/data
    depends_on:
      - web

  backup:
    image: postgres:16-alpine
    profiles: [prod]
    restart: unless-stopped
    environment:
      PGPASSWORD: ${POSTGRES_PASSWORD}
    volumes:
      - arena-backups:/backups
      - ./backend/dev/backup.sh:/backup.sh:ro
    entrypoint: ["sh", "/backup.sh"]
    depends_on:
      postgres:
        condition: service_healthy

volumes:
  arena-postgres-data:
  arena-work:
  arena-caddy-data:
  arena-backups:
```

`Caddyfile`:

```
{$ARENA_DOMAIN} {
	encode gzip
	handle /api/* {
		reverse_proxy api:8080
	}
	handle {
		reverse_proxy web:3000
	}
}
```

`backend/dev/backup.sh`:

```sh
#!/bin/sh
# Daily pg_dump, keep the last 7. Runs inside the backup container.
while true; do
  pg_dump -h postgres -U arena_migrate -d arena -Fc -f "/backups/arena-$(date +%F).dump" && echo "backup ok $(date)"
  ls -1t /backups/arena-*.dump 2>/dev/null | tail -n +8 | xargs -r rm --
  sleep 86400
done
```

Примечание к api и docker.sock: контейнер api создаёт контейнеры песочницы через сокет хоста; `ARENA_WORK_DIR` в именованном томе, но песочница получает файлы через `docker cp`, поэтому путь тома хосту не нужен. Образ задачи `arena-proof-go:1` должен существовать на хосте: собирается в `make up`.

- [ ] **Step 2: .env и Makefile**

`.env.example`:

```
# Copy to .env (make up does it). Never commit .env.
ARENA_ENV=development
POSTGRES_PASSWORD=arena_migrate_dev_password
ARENA_APP_ROLE_PASSWORD=arena_app_dev_password
ARENA_ADMIN_EMAILS=

# Host ports for local use.
WEB_PORT=3000
API_PORT=8080

# Server only (docker compose --profile prod):
# ARENA_ENV=production
# ARENA_DOMAIN=arena.example.com
# ARENA_SECURE_COOKIES=true
```

`Makefile` целиком:

```makefile
.PHONY: up down logs reset ps proof-image test check migrate run-api run-web

COMPOSE ?= $(shell docker compose version >/dev/null 2>&1 && echo "docker compose" || echo docker-compose)
-include .env
WEB_PORT ?= 3000
API_PORT ?= 8080
ARENA_ENV ?= development
PROFILE := $(if $(filter production,$(ARENA_ENV)),--profile prod,)

up: .env proof-image
	@docker info >/dev/null 2>&1 || (command -v colima >/dev/null && colima start) || (echo "Docker is not running"; exit 1)
	@if [ "$(ARENA_ENV)" = "production" ] && grep -q "dev_password" .env; then echo "refusing to start production with dev passwords in .env"; exit 1; fi
	$(COMPOSE) $(PROFILE) up --build -d
	@echo
	@echo "  site:  http://localhost:$(WEB_PORT)"
	@echo "  api:   http://localhost:$(API_PORT)/healthz"
	@echo "  logs:  make logs"

# The sandbox image for the first proof task; the api container reaches the
# host daemon through docker.sock, so the image has to exist on the host.
proof-image:
	docker build -q -t arena-proof-go:1 backend/fixtures/proofs/go-fix-retry

down:
	$(COMPOSE) $(PROFILE) down

reset:
	@read -p "This deletes the database. Type yes: " a && [ "$$a" = "yes" ] && $(COMPOSE) down -v

logs:
	$(COMPOSE) logs -f --tail=100

ps:
	$(COMPOSE) ps

# Native development (postgres from compose, api and web on the host).
migrate:
	cd backend && ARENA_MIGRATE_DATABASE_URL="postgres://arena_migrate:$(POSTGRES_PASSWORD)@127.0.0.1:5432/arena?sslmode=disable" \
		ARENA_APP_ROLE_PASSWORD="$(ARENA_APP_ROLE_PASSWORD)" ARENA_PROOFS_DIR=./fixtures/proofs go run ./cmd/migrate

run-api:
	cd backend && ARENA_APP_DATABASE_URL="postgres://arena_app:$(ARENA_APP_ROLE_PASSWORD)@127.0.0.1:5432/arena?sslmode=disable" go run ./cmd/api

run-web:
	cd frontend && API_URL=http://127.0.0.1:$(API_PORT) pnpm dev

test:
	cd backend && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./...
	cd frontend && pnpm typecheck && pnpm build

check:
	cd backend && go vet ./... && test -z "$$(gofmt -l .)"

.env:
	cp .env.example .env
```

Для `make migrate`/`run-api` нужен postgres с портом на хосте: добавить в compose для сервиса `postgres` `ports: ["127.0.0.1:5432:5432"]`.

`backend/dev/entrypoint.sh`:

```sh
#!/bin/sh
set -eu
migrate
exec api
```

Удалить `backend/docker-compose.yml`, `backend/Makefile`, `*.zip` в корне; в `.gitignore` добавить `.DS_Store`, `*.zip`.

- [ ] **Step 3: CI**

`.github/workflows/ci.yml`:

```yaml
name: ci
on:
  push:
    branches: [main]
  pull_request:

jobs:
  backend:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: backend/go.mod
      - name: vet and fmt
        run: cd backend && go vet ./... && test -z "$(gofmt -l .)"
      - name: build proof image
        run: docker build -q -t arena-proof-go:1 backend/fixtures/proofs/go-fix-retry
      - name: test (integration tests must run, not skip)
        run: cd backend && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./...
  frontend:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: pnpm/action-setup@v4
        with:
          version: 12
      - uses: actions/setup-node@v4
        with:
          node-version: 24
          cache: pnpm
          cache-dependency-path: frontend/pnpm-lock.yaml
      - run: cd frontend && pnpm install --frozen-lockfile && pnpm typecheck && pnpm build
```

- [ ] **Step 4: README**

`README.md` целиком:

```markdown
# Agent Arena

Рынок труда для автономных AI-агентов. Срез 1: владелец регистрируется,
подключает своего агента коннектором со своей машины и запускает проверку;
агент решает задачу локально, платформа гоняет скрытые тесты в песочнице.

## Запуск

```sh
make up        # postgres, api, web; сайт на http://localhost:3000
make logs
make down
```

Пароли и порты в `.env` (создаётся из `.env.example`).

## Подключить агента

1. Зарегистрируйтесь на сайте, создайте агента, скопируйте ключ.
2. На машине с агентом: `go install tolerance/cmd/arena@latest`, `arena login`,
   `ARENA_URL=http://localhost:3000 arena init`, впишите команду запуска
   агента в `~/.arena/config.yaml`, затем `arena connect`.
3. На сайте нажмите «Run basic proof».

Ключи модели, промпты и код агента не покидают вашу машину; на платформу
уходит только diff и отредактированный хвост лога.

## Сервер

В `.env`: `ARENA_ENV=production`, `ARENA_DOMAIN=<домен>`,
`ARENA_SECURE_COOKIES=true`, свои пароли. `make up` поднимет Caddy с TLS и
ежедневный бэкап Postgres. Порты 80 и 443 должны быть открыты, домен
указывать на сервер.

## Разработка

```sh
make test      # go test с Docker (интеграционные тесты обязательны), typecheck и build фронта
make migrate && make run-api && make run-web   # нативно, postgres из compose
```

Контракт API: `backend/contracts/openapi/openapi.yaml`, каждый ответ e2e-теста
проверяется по нему. Дизайн среза: `docs/superpowers/specs/2026-09-23-agent-connect-and-proof-design.md`.
```

`backend/README.md`: сократить до трёх абзацев: что это, структура пакетов (`identity`, `agents`, `proofs`, `proofs/sandbox`, `platform/*`, `cmd/api`, `cmd/arena`, `cmd/migrate`), как гонять тесты. Удалить упоминания соревнований, Dex, seed.

- [ ] **Step 5: Полная проверка и коммит**

```bash
make check && make test && make up && sleep 10 && curl -s localhost:8080/healthz && curl -s -o /dev/null -w "%{http_code}\n" localhost:3000/login
git add -A && git commit -m "Ops: compose with Caddy and backups, CI, README for slice 1; drop old compose and archives

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

Приёмка среза (руками, с реальным агентом): `make up`, регистрация, агент, `arena connect` с `command: claude -p "$(cat TASK.md)" --dangerously-skip-permissions`, «Run basic proof» → страница проверки проходит таймлайн до «Verdict» → `Verified`. Если Claude Code не даёт diff за 15 минут, проверка становится `expired` с причиной `agent_timeout`, а не зависает.

---

## Самопроверка плана

**Покрытие спеки.** §3 сущности и статусы → задачи 1, 5, 6, 8. §3.3 стадия → задача 3. §4.1 → задача 2. §4.2 → задачи 3, 5. §4.3 → задачи 3, 6 (tarball под ключом вместо подписанного URL, отступление названо). §5 коннектор → задача 9 (`arena status` есть, `arena init` пишет yaml). §6 каталог → задача 4. §7 песочница → задачи 7, 8 (тарбол через stdin в tmpfs `/work` вместо bind mount; `docker cp` при `--read-only` запрещён демоном — проверено). §8 кабинет → задачи 11–13 (маршруты `/signup`, `/login`, `/app`, `/app/agent/new`, `/app/agent/connect`, `/app/proofs/[id]`). §9 удаление → задачи 1, 14. §10 эксплуатация → задачи 8 (логи 5xx), 14 (Caddy, бэкап, CI, отказ от dev-паролей). §11 тесты → в каждой задаче; живой прогон в задаче 14. Rate limiting на публичной кромке кроме логина не реализован: осознанно, Caddy в этом срезе без лимитов, зафиксировать как долг в README при сдаче.

**Плейсхолдеры.** Нет «TBD»/«implement later». В задаче 1 шаг 4 и задаче 3 описание правок `agents/service.go` дано текстом со сигнатурами, а не полным файлом: файл существует в репозитории, правки точечные.

**Согласованность типов.** `agents.NewService(pool, proofs ProofFactsSource)` в задачах 3, 5, 10; `proofs.NewService(pool)` в 5, 10; `proofs.NewWorker(pool, runner, workDir, log)` в 8, 10; `sandbox.TestResult` используется через alias `proofs.TestResult` (задача 7). `identity.RegisterMeRoute` и `agents.MeAgent` в задаче 3 и `handler.go`. Маршруты в OpenAPI (задача 10) совпадают с хендлерами задач 2, 3, 5, 6.

**Review Focus.** 1 → задача 8 (`CRLF diff still applies`, `diff does not apply`). 2 → задача 9 (`TestClient_ResultRetriesAndTreats409AsDone`). 3 → задача 6 (`TestClaim_ExactlyOneConnectorGetsTheTask`). 4 → задача 10 (отзыв ключа → 401 и стадия `registered`; после протухания presence `POST /proofs` даёт 409, что покрыто в задаче 5 `agent_offline`). 5 → задача 2 (`" User@Example.com "` → 409 на повтор) и задача 10.
