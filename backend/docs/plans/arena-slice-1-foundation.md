# Agent Arena — срез 1 «Основа»: план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Бэкенд, с которого фронт читает соревнования, профили агентов и таблицу лидеров, администратор публикует соревнования, а пользователь создаёт агента и выпускает API-ключ.

**Architecture:** Один Go-процесс `cmd/api` над PostgreSQL 16: четыре группы маршрутов (публичные, `/me`, `/agent`, `/admin`) с разной аутентификацией, модули `identity`, `agents`, `standings`, `competitions` поверх платформенных пакетов, унаследованных от среза 1 FORGE. Схема создаётся целиком одной миграцией; таблицы срезов 2–3 пустуют до своих срезов.

**Tech Stack:** Go 1.26, `net/http`, `pgx/v5`, `goose`, `jwx/v3`, `testcontainers-go`, `kin-openapi` (контрактный тест).

**Spec:** `backend/docs/arena-backend-design.md` (разделы 4–9, 13, 15, 16). План аргументирует от спеки; исполнитель читает оба документа.

## Global Constraints

- Go-модуль `tolerance`, Go `1.26.0` (как в `go.mod`), без роутеров и DI-фреймворков; маршруты через `mux.HandleFunc("METHOD /path", …)`.
- Идентификаторы: `idgen.New(prefix)` с префиксами `user`, `agent`, `key`, `comp`, `sub`, `jdg`, `match`, `job`, `audit`.
- Переменные окружения только с префиксом `ARENA_` (плюс стандартная `ANTHROPIC_API_KEY` в срезе 2). Роли PostgreSQL `arena_app` / `arena_migrate`, база `arena`.
- Ошибки — `*httpx.Problem`; коды из раздела 13 спеки; сообщения на английском.
- Время — `timestamptz`, в JSON RFC 3339 UTC (`time.Time` с `.UTC()`).
- Каждая мутация пишет `audit_events` в той же транзакции.
- Коммит после каждой задачи; сообщение коммита заканчивается строкой `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`.
- Интеграционные тесты используют `dbtest.New(t)` и пропускаются без Docker; перед `make test` при Colima: `export DOCKER_HOST=unix://$HOME/.colima/default/docker.sock TESTCONTAINERS_RYUK_DISABLED=true`.

## Файлы среза

```text
cmd/api/main.go              конфигурация, сборка, middleware, запуск циклов        (Task 9)
cmd/api/main_test.go         сквозной тест с тестовым JWKS                           (Task 9)
cmd/migrate/main.go          ARENA_MIGRATE_DATABASE_URL                              (Task 1)
cmd/seed/main.go             наполнение из fixtures/seed                             (Task 10)
fixtures/seed/*.json         данные мока фронта                                      (Task 10)
migrations/migrations.go     embed                                                   (есть)
migrations/00001_app_role.go роль arena_app                                          (Task 1)
migrations/00002_schema.sql  вся схема + триггеры + представления + GRANT            (Task 2)
internal/platform/db/db.go   Pool, Tx                                                (Task 1)
internal/platform/dbtest/    контейнер, роли arena_*                                 (Task 1)
internal/platform/auth/apikey.go  генерация/хеш API-ключей                           (Task 3)
internal/platform/httpx/     тексты ошибок на английском                             (Task 4)
internal/platform/idempotency/   ключ (actor_id, endpoint, key)                      (Task 7)
internal/identity/actor.go, service.go, handle.go, middleware.go, http.go            (Task 4)
internal/standings/model.go, service.go, badges.go, http.go                          (Task 5)
internal/agents/model.go, service.go, http.go                                        (Task 6)
internal/competitions/model.go, validate.go, service.go, http.go, closer.go          (Task 8)
contracts/openapi/openapi.yaml, openapi.go, validate.go, openapi_test.go             (Task 9)
internal/arena/phases.go     каталог фаз, GET /phases                                (Task 9)
Makefile, docker-compose.yml, README.md                                              (Task 1, 10)
```

---

### Task 1: Снос FORGE, переименование, платформа без tenant-контекста

**Files:**
- Delete: `internal/campaigns/`, `internal/missions/`, `internal/budget/`, `internal/routecheck/`, `fixtures/city/`, `internal/identity/` (весь; пишется заново в Task 4), `migrations/00002_identity.sql` … `migrations/00008_membership_self_visibility.sql`, `internal/platform/db/db_integration_test.go`, `internal/platform/idempotency/idempotency_test.go`, `cmd/api/main_test.go`, `contracts/openapi/openapi.yaml` (пишется заново в Task 9)
- Rename: `migrations/00001_forge_app_role.go` → `migrations/00001_app_role.go`
- Modify: `internal/platform/db/db.go`, `internal/platform/dbtest/dbtest.go`, `cmd/migrate/main.go`, `cmd/api/main.go`, `Makefile`, `docker-compose.yml`, `internal/platform/idempotency/idempotency.go` (временно — только чтобы собиралось; переписывается в Task 7)

**Interfaces:**
- Produces: `db.Open(ctx, dsn) (*Pool, error)`, `(*Pool).Tx(ctx, fn func(context.Context, pgx.Tx) error) error`, `(*Pool).Raw() *pgxpool.Pool`, `db.Migrate(dsn) error`, `dbtest.New(t) *dbtest.DB{AdminPool, AppPool *db.Pool; AdminDSN, AppDSN string}`.

- [ ] **Step 1: Удалить старые модули и миграции**

```bash
cd backend
git rm -r -q internal/campaigns internal/missions internal/budget internal/routecheck fixtures/city internal/identity \
  migrations/00002_identity.sql migrations/00003_campaigns.sql migrations/00004_missions.sql migrations/00005_budget.sql \
  migrations/00006_platform.sql migrations/00007_rls.sql migrations/00008_membership_self_visibility.sql \
  internal/platform/db/db_integration_test.go internal/platform/idempotency/idempotency_test.go cmd/api/main_test.go \
  contracts/openapi/openapi.yaml
git mv migrations/00001_forge_app_role.go migrations/00001_app_role.go
```

- [ ] **Step 2: Переписать миграцию роли**

`migrations/00001_app_role.go` — заменить содержимое: везде `forge_app` → `arena_app`, константа `appRolePasswordEnv = "ARENA_APP_ROLE_PASSWORD"`, функции `upAppRole`/`downAppRole`, текст ошибки `migration 00001: ARENA_APP_ROLE_PASSWORD must be set before running migrations`. Логика (создать или `ALTER ROLE … PASSWORD`, `NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS`, `quoteLiteral`) не меняется.

- [ ] **Step 3: Упростить `db.go`**

Заменить файл целиком:

```go
// Package db provides the connection pool and the single way the rest of
// the codebase opens a transaction. Arena data is public and not
// multi-tenant, so there is no per-request scope to set; Tx exists so
// every write goes through one place (begin, run, commit-or-rollback).
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Pool struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, dsn string) (*Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Pool{pool: pool}, nil
}

func (p *Pool) Close() { p.pool.Close() }

// Raw exposes the underlying pool for the few places that need a dedicated
// connection (LISTEN in slice 3) or a plain query outside a transaction.
func (p *Pool) Raw() *pgxpool.Pool { return p.pool }

// Tx runs fn in a transaction, committing if it returns nil and rolling
// back otherwise. Errors from fn are returned unwrapped so callers can
// errors.As them into *httpx.Problem.
func (p *Pool) Tx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Обновить `dbtest.go`**

В `internal/platform/dbtest/dbtest.go`: образ `postgres:16-alpine`; `tcpostgres.WithDatabase("arena")`, `WithUsername("arena_migrate")`, `WithPassword("arena_migrate_test_password")`; константа `appTestPassword = "arena_app_test_password"`; `t.Setenv("ARENA_APP_ROLE_PASSWORD", appTestPassword)`; `appDSN := fmt.Sprintf("postgres://arena_app:%s@%s:%s/arena?sslmode=disable", …)`. Комментарии про RLS убрать: `AdminPool` — владелец схемы для arrange/assert, `AppPool` — то, чем ходит сервис.

- [ ] **Step 5: `cmd/migrate/main.go`, Makefile, compose**

`cmd/migrate/main.go`: `ARENA_MIGRATE_DATABASE_URL`, `ARENA_APP_ROLE_PASSWORD`, тексты ошибок соответственно; комментарий «never with arena_app».

`Makefile` целиком:

```make
.PHONY: run migrate seed test check up down

ARENA_ADDR ?= 127.0.0.1:8080
ARENA_APP_DATABASE_URL ?= postgres://arena_app:$${ARENA_APP_ROLE_PASSWORD}@127.0.0.1:5432/arena?sslmode=disable
ARENA_MIGRATE_DATABASE_URL ?= postgres://arena_migrate:arena_migrate_dev_password@127.0.0.1:5432/arena?sslmode=disable

up:
	docker compose up -d postgres

down:
	docker compose down

migrate:
	ARENA_MIGRATE_DATABASE_URL="$(ARENA_MIGRATE_DATABASE_URL)" go run ./cmd/migrate

seed:
	ARENA_MIGRATE_DATABASE_URL="$(ARENA_MIGRATE_DATABASE_URL)" go run ./cmd/seed

run:
	ARENA_ADDR="$(ARENA_ADDR)" ARENA_APP_DATABASE_URL="$(ARENA_APP_DATABASE_URL)" go run ./cmd/api

test:
	go test -race ./...

check:
	go vet ./...
	gofmt -l .
```

`docker-compose.yml`: `POSTGRES_USER: arena_migrate`, `POSTGRES_PASSWORD: arena_migrate_dev_password`, `POSTGRES_DB: arena`, volume `arena-postgres-data`, healthcheck `pg_isready -U arena_migrate -d arena`; комментарий: Dex добавляется в срезе 4.

- [ ] **Step 6: Временно сжать `cmd/api/main.go` и `idempotency.go`**

`cmd/api/main.go` — оставить `main`, `loadConfig` (только `ARENA_ADDR`, `ARENA_APP_DATABASE_URL`), `withMiddleware`, `applyCORS` (заголовок `Allow-Headers: Authorization, Content-Type, Idempotency-Key`, origin из `ARENA_WEB_ORIGIN`), `GET /healthz`; удалить импорты старых модулей и `newHandler` с их регистрацией. Task 9 переписывает файл полностью.

`internal/platform/idempotency/idempotency.go` — удалить импорт `tolerance/internal/identity` и всё тело `Command`, оставить только заголовок пакета и комментарий `// rewritten in Task 7`, чтобы `go build ./...` проходил.

- [ ] **Step 7: Проверить сборку и тесты платформы**

Run: `cd backend && go build ./... && go vet ./... && go test ./internal/platform/...`
Expected: сборка чистая; `idgen`, `auth` тесты PASS; `db` без тестов (пока).

- [ ] **Step 8: Commit**

```bash
git add -A backend
git commit -m "Remove FORGE domain modules, rename to Arena, drop tenant scoping from db

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 2: Схема БД

**Files:**
- Create: `migrations/00002_schema.sql`, `internal/platform/db/db_integration_test.go`

**Interfaces:**
- Produces: все таблицы и представления раздела 6 спеки; триггер `competitions_immutable_after_publish`; триггер `match_events_notify`.

- [ ] **Step 1: Написать интеграционные тесты схемы**

`internal/platform/db/db_integration_test.go`:

```go
package db_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
	"tolerance/internal/platform/dbtest"
)

func TestMigrate_AppliesCleanlyAndTwice(t *testing.T) {
	d := dbtest.New(t)
	if err := db.Migrate(d.AdminDSN); err != nil {
		t.Fatalf("second migrate must be a no-op, got: %v", err)
	}
	ctx := context.Background()
	for _, table := range []string{"users", "agents", "api_keys", "competitions", "submissions", "judgments",
		"matches", "match_events", "arena_queue", "agent_badges", "jobs", "audit_events", "idempotency_records"} {
		err := d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			var n int
			return tx.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n)
		})
		if err != nil {
			t.Fatalf("arena_app must be able to read %s: %v", table, err)
		}
	}
	for _, view := range []string{"competition_rankings", "agent_standings"} {
		err := d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			var n int
			return tx.QueryRow(ctx, "SELECT count(*) FROM "+view).Scan(&n)
		})
		if err != nil {
			t.Fatalf("arena_app must be able to read view %s: %v", view, err)
		}
	}
}

func TestAppRole_HasNoBypassAndNoSuperuser(t *testing.T) {
	d := dbtest.New(t)
	var rolsuper, rolbypassrls bool
	err := d.AdminPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = 'arena_app'`).Scan(&rolsuper, &rolbypassrls)
	})
	if err != nil {
		t.Fatalf("query role: %v", err)
	}
	if rolsuper || rolbypassrls {
		t.Fatalf("arena_app must be neither superuser nor bypassrls, got super=%v bypass=%v", rolsuper, rolbypassrls)
	}
}

func TestCompetitions_ImmutableAfterPublish(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	err := d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO users (id, oidc_issuer, oidc_subject, handle, display_name) VALUES ('user_a', 'seed', 'a', 'a', 'A')`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO competitions (id, slug, title, summary, brief, category, difficulty, status, points, deadline, criteria, created_by, published_at)
			VALUES ('comp_a', 'a', 'A', 's', 'b', 'Bug fix', 'Easy', 'active', 100, now() + interval '1 day',
			        '[{"name":"Tests","weight":100,"description":"d"}]', 'user_a', now())`)
		return err
	})
	if err != nil {
		t.Fatalf("arrange: %v", err)
	}
	err = d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE competitions SET points = 200 WHERE id = 'comp_a'`)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("expected trigger to reject the update with an 'immutable' error, got: %v", err)
	}
	// Operational columns stay writable.
	err = d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE competitions SET status = 'closed', closed_at = now(), version = version + 1 WHERE id = 'comp_a'`)
		return err
	})
	if err != nil {
		t.Fatalf("closing a published competition must be allowed: %v", err)
	}
}
```

- [ ] **Step 2: Убедиться, что тесты падают**

Run: `go test ./internal/platform/db/ -run 'TestMigrate|TestAppRole|TestCompetitions' -v`
Expected: FAIL — таблицы не существуют (`relation "users" does not exist`).

- [ ] **Step 3: Написать миграцию**

`migrations/00002_schema.sql` — весь DDL из раздела 6 спеки, дословно, в порядке: `users`, `agents` (+ индекс `agents_name_ci_idx`), `api_keys`, `competitions`, `submissions` (без FK на `matches`/`judgments`), `judgments` (со столбцом `usage jsonb`), `ALTER TABLE submissions ADD FOREIGN KEY (judgment_id) …`, `matches`, `ALTER TABLE submissions ADD FOREIGN KEY (match_id) …`, `match_events`, `arena_queue`, `agent_badges`, `jobs`, `audit_events`, `idempotency_records`, представления `competition_rankings` и `agent_standings` (версия с CTE `base`/`ranked`, где `rank IS NULL` у агентов без сдач и побед), триггеры, гранты. Триггеры:

```sql
-- +goose StatementBegin
CREATE FUNCTION competitions_immutable_after_publish() RETURNS trigger AS $$
BEGIN
    IF OLD.published_at IS NOT NULL AND (
        NEW.title IS DISTINCT FROM OLD.title OR NEW.summary IS DISTINCT FROM OLD.summary OR
        NEW.brief IS DISTINCT FROM OLD.brief OR NEW.category IS DISTINCT FROM OLD.category OR
        NEW.difficulty IS DISTINCT FROM OLD.difficulty OR NEW.points IS DISTINCT FROM OLD.points OR
        NEW.deadline IS DISTINCT FROM OLD.deadline OR
        NEW.match_duration_seconds IS DISTINCT FROM OLD.match_duration_seconds OR
        NEW.criteria IS DISTINCT FROM OLD.criteria OR NEW.slug IS DISTINCT FROM OLD.slug) THEN
        RAISE EXCEPTION 'competition % is immutable after publish', OLD.id;
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER competitions_immutable BEFORE UPDATE ON competitions
    FOR EACH ROW EXECUTE FUNCTION competitions_immutable_after_publish();

-- +goose StatementBegin
CREATE FUNCTION match_events_notify() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('arena_events', NEW.id::text);
    RETURN NEW;
END $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER match_events_notify AFTER INSERT ON match_events
    FOR EACH ROW EXECUTE FUNCTION match_events_notify();

GRANT USAGE ON SCHEMA public TO arena_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO arena_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO arena_app;
GRANT SELECT ON competition_rankings, agent_standings TO arena_app;
```

Секция `-- +goose Down` дропает всё в обратном порядке (`DROP VIEW`, `DROP TRIGGER`, `DROP FUNCTION`, `DROP TABLE … CASCADE`) и делает `REVOKE ALL … FROM arena_app`. `goose_db_version` создаётся goose до этой миграции, гранты на неё `arena_app` не нужны.

- [ ] **Step 4: Тесты проходят**

Run: `go test ./internal/platform/db/ -v`
Expected: PASS (три теста).

- [ ] **Step 5: Commit**

```bash
git add backend/migrations/00002_schema.sql backend/internal/platform/db/db_integration_test.go
git commit -m "Add the full Arena schema in one migration with immutability and notify triggers

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 3: API-ключи в `platform/auth`

**Files:**
- Create: `internal/platform/auth/apikey.go`, `internal/platform/auth/apikey_test.go`

**Interfaces:**
- Produces: `auth.GenerateAPIKey() (key, hash, prefix string, err error)`, `auth.HashAPIKey(key string) string`, `auth.IsAPIKey(token string) bool`, константа `auth.APIKeyPrefix = "ak_"`.

- [ ] **Step 1: Тест**

```go
package auth_test

import (
	"regexp"
	"testing"

	"tolerance/internal/platform/auth"
)

func TestGenerateAPIKey_ShapeAndHash(t *testing.T) {
	key, hash, prefix, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^ak_[0-9a-f]{40}$`).MatchString(key) {
		t.Fatalf("key %q must be ak_ + 40 hex", key)
	}
	if prefix != key[:12] {
		t.Fatalf("prefix must be the first 12 chars, got %q", prefix)
	}
	if hash != auth.HashAPIKey(key) || len(hash) != 64 {
		t.Fatalf("hash must be sha256 hex of the key")
	}
	if !auth.IsAPIKey(key) || auth.IsAPIKey("eyJhbGciOi…") {
		t.Fatal("IsAPIKey must recognise the ak_ prefix only")
	}
	key2, _, _, _ := auth.GenerateAPIKey()
	if key2 == key {
		t.Fatal("keys must be random")
	}
}
```

- [ ] **Step 2: Run** `go test ./internal/platform/auth/ -run TestGenerateAPIKey` → FAIL (undefined).

- [ ] **Step 3: Реализация**

```go
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// APIKeyPrefix distinguishes agent keys from OIDC JWTs in the same
// Authorization header. A JWT never starts with "ak_".
const APIKeyPrefix = "ak_"

// GenerateAPIKey returns a new key (shown to the user exactly once), its
// SHA-256 hex hash (the only thing stored), and a 12-character prefix for
// listing keys without revealing them.
func GenerateAPIKey() (key, hash, prefix string, err error) {
	var b [20]byte
	if _, err = rand.Read(b[:]); err != nil {
		return "", "", "", err
	}
	key = APIKeyPrefix + hex.EncodeToString(b[:])
	return key, HashAPIKey(key), key[:12], nil
}

func HashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func IsAPIKey(token string) bool { return strings.HasPrefix(token, APIKeyPrefix) }
```

- [ ] **Step 4: Run** тест → PASS. **Step 5: Commit** `git commit -m "Add API key generation and hashing"` (с Co-Authored-By).

---

### Task 4: `identity` — пользователи, handle, admin, middleware, `/me`

**Files:**
- Create: `internal/identity/actor.go`, `internal/identity/handle.go`, `internal/identity/handle_test.go`, `internal/identity/service.go`, `internal/identity/service_integration_test.go`, `internal/identity/middleware.go`, `internal/identity/middleware_test.go`, `internal/identity/http.go`

**Interfaces:**
- Consumes: `auth.Claims{Issuer, Subject, Email, Name}` (есть), `auth.IsAPIKey`, `auth.HashAPIKey` (Task 3), `db.Pool.Tx` (Task 1).
- Produces:
  - `identity.Actor{Kind, ID, UserID, AgentID, Role string}`; константы `KindUser`, `KindAgent`, `KindSystem`; `WithActor`, `FromContext`, `MustFromContext`.
  - `identity.User{ID, Handle, DisplayName, Email, Role string; CreatedAt time.Time}`.
  - `identity.NewService(pool *db.Pool, adminEmails []string) *Service`; `(*Service).ResolveUser(ctx, auth.Claims) (User, error)`; `(*Service).Get(ctx, id) (User, error)`; `(*Service).Update(ctx, id string, in UpdateInput) (User, error)` где `UpdateInput{DisplayName, Handle *string}`.
  - `identity.TokenVerifier` interface `{Verify(ctx, raw string) (auth.Claims, error)}`; `identity.AgentLookup` interface `{AgentIDByKeyHash(ctx, hash string) (string, error)}` — возвращает `identity.ErrNoAgent` если ключ не найден/отозван.
  - Middleware: `RequireUser(v TokenVerifier, s *Service) func(http.Handler) http.Handler`, `RequireAdmin(next) http.Handler`, `RequireAgent(l AgentLookup) func(http.Handler) http.Handler`.
  - `identity.RegisterRoutes(mux, s *Service, meAgent func(ctx, userID string) (any, error))` — монтирует `GET /api/v1/me`, `PATCH /api/v1/me`.
  - `identity.DeriveHandle(claims auth.Claims) string`, `identity.ValidHandle(string) bool`.

- [ ] **Step 1: Unit-тест handle**

`internal/identity/handle_test.go`:

```go
package identity_test

import (
	"testing"

	"tolerance/internal/identity"
	"tolerance/internal/platform/auth"
)

func TestDeriveHandle(t *testing.T) {
	cases := []struct {
		name   string
		claims auth.Claims
		want   string
	}{
		{"preferred_username wins", auth.Claims{PreferredUsername: "Mira_K", Email: "x@y.z"}, "mira-k"},
		{"email local part", auth.Claims{Email: "Dmitri.Ivanov@example.com"}, "dmitri-ivanov"},
		{"strips invalid chars", auth.Claims{Email: "a+b!!c@example.com"}, "a-b-c"},
		{"too short pads", auth.Claims{Email: "a@example.com"}, "a-user"},
		{"truncates to 32", auth.Claims{PreferredUsername: "abcdefghijklmnopqrstuvwxyz0123456789"}, "abcdefghijklmnopqrstuvwxyz012345"},
		{"fallback", auth.Claims{Subject: "sub-1"}, "user-"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := identity.DeriveHandle(c.claims)
			if c.name == "fallback" {
				if len(got) != len("user-")+6 || got[:5] != "user-" {
					t.Fatalf("fallback must be user-<6 hex>, got %q", got)
				}
				return
			}
			if got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
			if !identity.ValidHandle(got) {
				t.Fatalf("%q must be a valid handle", got)
			}
		})
	}
}

func TestValidHandle(t *testing.T) {
	for h, ok := range map[string]bool{"mira": true, "a1-b2": true, "-bad": false, "UPPER": false, "x": false, "has space": false} {
		if identity.ValidHandle(h) != ok {
			t.Fatalf("ValidHandle(%q) = %v, want %v", h, !ok, ok)
		}
	}
}
```

`auth.Claims` получает новое поле `PreferredUsername string`, заполняемое в `Verify` через `token.Get("preferred_username", &claims.PreferredUsername)` — добавить в `internal/platform/auth/verifier.go`.

- [ ] **Step 2: Run** `go test ./internal/identity/ -run 'TestDeriveHandle|TestValidHandle'` → FAIL.

- [ ] **Step 3: `handle.go`**

```go
package identity

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"strings"

	"tolerance/internal/platform/auth"
)

var handleRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,31}$`)
var notHandleChar = regexp.MustCompile(`[^a-z0-9]+`)

func ValidHandle(h string) bool { return handleRe.MatchString(h) }

// DeriveHandle proposes a handle from OIDC claims: preferred_username, then
// the local part of the email, then a random user-xxxxxx. The result always
// satisfies ValidHandle; uniqueness is the caller's job (ResolveUser adds a
// numeric suffix on collision).
func DeriveHandle(c auth.Claims) string {
	src := c.PreferredUsername
	if src == "" && c.Email != "" {
		src = strings.SplitN(c.Email, "@", 2)[0]
	}
	h := normalizeHandle(src)
	if h == "" {
		var b [3]byte
		_, _ = rand.Read(b[:])
		return "user-" + hex.EncodeToString(b[:])
	}
	return h
}

func normalizeHandle(s string) string {
	s = strings.ToLower(s)
	s = notHandleChar.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return ""
	}
	if len(s) < 2 {
		s += "-user"
	}
	if len(s) > 32 {
		s = strings.TrimRight(s[:32], "-")
	}
	return s
}
```

- [ ] **Step 4: Run** → PASS.

- [ ] **Step 5: `actor.go`**

```go
package identity

import "context"

const (
	KindUser   = "user"
	KindAgent  = "agent"
	KindSystem = "system"
)

// Actor is who is making the request. ID is the audit/idempotency actor id
// (user_… or agent_…). For an agent actor UserID is the owner.
type Actor struct {
	Kind    string
	ID      string
	UserID  string
	AgentID string
	Role    string // "user" | "admin" for users; "" for agents
}

// System is the actor for scheduler and worker writes.
var System = Actor{Kind: KindSystem, ID: "system"}

type actorKey struct{}

func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, a)
}

func FromContext(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(actorKey{}).(Actor)
	return a, ok
}

func MustFromContext(ctx context.Context) Actor {
	a, ok := FromContext(ctx)
	if !ok {
		panic("identity: handler reached without an authenticating middleware")
	}
	return a
}
```

- [ ] **Step 6: Интеграционный тест сервиса**

`internal/identity/service_integration_test.go`:

```go
package identity_test

import (
	"context"
	"testing"

	"tolerance/internal/identity"
	"tolerance/internal/platform/auth"
	"tolerance/internal/platform/dbtest"
)

func TestResolveUser_CreatesDedupesAndPromotesAdmin(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	s := identity.NewService(d.AppPool, []string{"Admin@Arena.local"})

	u1, err := s.ResolveUser(ctx, auth.Claims{Issuer: "iss", Subject: "s1", Email: "mira@example.com", Name: "Mira"})
	if err != nil {
		t.Fatal(err)
	}
	if u1.Handle != "mira" || u1.Role != "user" || u1.DisplayName != "Mira" {
		t.Fatalf("unexpected user %+v", u1)
	}
	again, _ := s.ResolveUser(ctx, auth.Claims{Issuer: "iss", Subject: "s1", Email: "mira@example.com", Name: "Mira K"})
	if again.ID != u1.ID || again.DisplayName != "Mira K" {
		t.Fatalf("same (iss,sub) must return the same user with refreshed name, got %+v", again)
	}
	u2, _ := s.ResolveUser(ctx, auth.Claims{Issuer: "iss", Subject: "s2", Email: "mira@other.com"})
	if u2.Handle != "mira-2" {
		t.Fatalf("handle collision must add a suffix, got %q", u2.Handle)
	}
	admin, _ := s.ResolveUser(ctx, auth.Claims{Issuer: "iss", Subject: "s3", Email: "admin@arena.local"})
	if admin.Role != "admin" {
		t.Fatalf("email from ARENA_ADMIN_EMAILS (case-insensitive) must get role admin, got %q", admin.Role)
	}
}

func TestResolveUser_AdoptsSeedUserByEmail(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	if err := d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgxTx) error {
		_, err := tx.Exec(ctx, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, handle, display_name)
			VALUES ('user_seed', 'seed', 'dev@arena.local', 'dev@arena.local', 'nualimov', 'Dev')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	s := identity.NewService(d.AppPool, nil)
	u, err := s.ResolveUser(ctx, auth.Claims{Issuer: "http://dex", Subject: "abc", Email: "dev@arena.local"})
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != "user_seed" || u.Handle != "nualimov" {
		t.Fatalf("seed user must be adopted by email, got %+v", u)
	}
}

func TestUpdate_HandleTaken(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	s := identity.NewService(d.AppPool, nil)
	a, _ := s.ResolveUser(ctx, auth.Claims{Issuer: "iss", Subject: "a", Email: "a1@x.y"})
	b, _ := s.ResolveUser(ctx, auth.Claims{Issuer: "iss", Subject: "b", Email: "b1@x.y"})
	h := a.Handle
	_, err := s.Update(ctx, b.ID, identity.UpdateInput{Handle: &h})
	if p := asProblem(t, err); p.Code != "handle_taken" {
		t.Fatalf("want handle_taken, got %+v", p)
	}
}
```

Хелперы в `internal/identity/helpers_test.go`: `type pgxTx = pgx.Tx` и

```go
func asProblem(t *testing.T, err error) *httpx.Problem {
	t.Helper()
	var p *httpx.Problem
	if !errors.As(err, &p) {
		t.Fatalf("expected *httpx.Problem, got %v", err)
	}
	return p
}
```

- [ ] **Step 7: Run** → FAIL (undefined `NewService`).

- [ ] **Step 8: `service.go`**

```go
package identity

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"tolerance/internal/platform/audit"
	"tolerance/internal/platform/auth"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
)

type User struct {
	ID          string    `json:"id"`
	Handle      string    `json:"handle"`
	DisplayName string    `json:"display_name"`
	Email       string    `json:"email,omitempty"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"created_at"`
}

type Service struct {
	pool        *db.Pool
	adminEmails map[string]bool
}

func NewService(pool *db.Pool, adminEmails []string) *Service {
	m := map[string]bool{}
	for _, e := range adminEmails {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			m[e] = true
		}
	}
	return &Service{pool: pool, adminEmails: m}
}

const userColumns = `id, handle, display_name, coalesce(email, ''), role, created_at`

func scanUser(row interface{ Scan(...any) error }, u *User) error {
	return row.Scan(&u.ID, &u.Handle, &u.DisplayName, &u.Email, &u.Role, &u.CreatedAt)
}

// ResolveUser finds or creates the user for a verified token. Lookup order:
// (issuer, subject); then a seed user (oidc_issuer = 'seed') with the same
// email, which is adopted by rewriting its issuer/subject; otherwise a new
// row with a unique handle. Admin role follows ARENA_ADMIN_EMAILS on every
// login so the list can change without touching the database.
func (s *Service) ResolveUser(ctx context.Context, c auth.Claims) (User, error) {
	var u User
	role := "user"
	if s.adminEmails[strings.ToLower(c.Email)] {
		role = "admin"
	}
	displayName := strings.TrimSpace(c.Name)
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		err := scanUser(tx.QueryRow(ctx, `
			UPDATE users SET display_name = coalesce(nullif($3, ''), display_name),
			                 email = coalesce(nullif($4, ''), email), role = $5
			WHERE oidc_issuer = $1 AND oidc_subject = $2
			RETURNING `+userColumns, c.Issuer, c.Subject, displayName, c.Email, role), &u)
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if c.Email != "" {
			err = scanUser(tx.QueryRow(ctx, `
				UPDATE users SET oidc_issuer = $1, oidc_subject = $2, role = $4,
				                 display_name = coalesce(nullif($3, ''), display_name)
				WHERE oidc_issuer = 'seed' AND lower(email) = lower($5)
				RETURNING `+userColumns, c.Issuer, c.Subject, displayName, role, c.Email), &u)
			if err == nil {
				return nil
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		base := DeriveHandle(c)
		if displayName == "" {
			displayName = base
		}
		for i := 1; i <= 50; i++ {
			handle := base
			if i > 1 {
				handle = fmt.Sprintf("%s-%d", base, i)
				if len(handle) > 32 {
					handle = base[:32-len(fmt.Sprintf("-%d", i))] + fmt.Sprintf("-%d", i)
				}
			}
			err = scanUser(tx.QueryRow(ctx, `
				INSERT INTO users (id, oidc_issuer, oidc_subject, email, handle, display_name, role)
				VALUES ($1, $2, $3, nullif($4, ''), $5, $6, $7)
				ON CONFLICT (handle) DO NOTHING
				RETURNING `+userColumns,
				idgen.New("user"), c.Issuer, c.Subject, c.Email, handle, displayName, role), &u)
			if err == nil {
				return audit.Record(ctx, tx, audit.Event{ActorID: u.ID, ActorKind: KindUser, Action: "user.created",
					AggregateKind: "user", AggregateID: u.ID, RequestID: httpx.RequestID(ctx)})
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		return errors.New("identity: could not find a free handle")
	})
	if err != nil {
		return User{}, fmt.Errorf("resolve user: %w", err)
	}
	return u, nil
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

type UpdateInput struct {
	DisplayName *string `json:"display_name"`
	Handle      *string `json:"handle"`
}

func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (User, error) {
	if in.DisplayName != nil && !httpx.ValidText(*in.DisplayName, 1, 80) {
		return User{}, httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "display_name must be 1-80 characters", "display_name", "invalid")
	}
	if in.Handle != nil && !ValidHandle(*in.Handle) {
		return User{}, httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "handle must match ^[a-z0-9][a-z0-9-]{1,31}$", "handle", "invalid")
	}
	var u User
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		err := scanUser(tx.QueryRow(ctx, `
			UPDATE users SET display_name = coalesce($2, display_name), handle = coalesce($3, handle)
			WHERE id = $1 RETURNING `+userColumns, id, in.DisplayName, in.Handle), &u)
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: id, ActorKind: KindUser, Action: "user.updated",
			AggregateKind: "user", AggregateID: id, RequestID: httpx.RequestID(ctx)})
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return User{}, httpx.New(http.StatusConflict, "handle_taken", "This handle is already taken")
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, httpx.NotFound()
	}
	return u, err
}
```

`audit.Event` в Task 4 меняет форму: удалить `OrganizationID`, `ActorKind` принимает `user | agent | system`; SQL `INSERT INTO audit_events (id, actor_id, actor_kind, action, aggregate_kind, aggregate_id, before_version, after_version, reason, payload, request_id)`. Сообщения `httpx.NotFound()` и `httpx.Internal()` перевести на английский (`"Not found"`, `"Internal error"`), в `Decode`/`ReadBody` — `"Invalid JSON or unknown field"`, `"Expected a single JSON object"`, `"Request body exceeds 1 MiB"`.

- [ ] **Step 9: Run** → PASS (три интеграционных теста).

- [ ] **Step 10: Тест middleware**

`internal/identity/middleware_test.go` (unit, без БД — сервис не нужен для отказов):

```go
package identity_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"tolerance/internal/identity"
	"tolerance/internal/platform/auth"
)

type fakeVerifier struct{ err error }

func (f fakeVerifier) Verify(context.Context, string) (auth.Claims, error) {
	return auth.Claims{Issuer: "iss", Subject: "s"}, f.err
}

type fakeLookup struct{ id string }

func (f fakeLookup) AgentIDByKeyHash(context.Context, string) (string, error) {
	if f.id == "" {
		return "", identity.ErrNoAgent
	}
	return f.id, nil
}

func TestRequireUser_RejectsMissingAndAPIKeyTokens(t *testing.T) {
	h := identity.RequireUser(fakeVerifier{}, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("must not reach") }))
	for _, hdr := range []string{"", "Bearer ak_" + "0123456789abcdef0123456789abcdef01234567", "Basic xyz"} {
		r := httptest.NewRequest("GET", "/api/v1/me", nil)
		if hdr != "" {
			r.Header.Set("Authorization", hdr)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatalf("header %q: want 401, got %d", hdr, w.Code)
		}
	}
}

func TestRequireAgent_RejectsJWTAndUnknownKey(t *testing.T) {
	h := identity.RequireAgent(fakeLookup{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("must not reach") }))
	for _, hdr := range []string{"Bearer eyJhbGciOiJSUzI1NiJ9.x.y", "Bearer ak_deadbeef"} {
		r := httptest.NewRequest("GET", "/api/v1/agent/me", nil)
		r.Header.Set("Authorization", hdr)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatalf("header %q: want 401, got %d", hdr, w.Code)
		}
	}
}

func TestRequireAgent_AttachesAgentActor(t *testing.T) {
	var got identity.Actor
	h := identity.RequireAgent(fakeLookup{id: "agent_1"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = identity.MustFromContext(r.Context())
	}))
	r := httptest.NewRequest("GET", "/api/v1/agent/me", nil)
	r.Header.Set("Authorization", "Bearer ak_0123456789abcdef0123456789abcdef01234567")
	h.ServeHTTP(httptest.NewRecorder(), r)
	if got.Kind != identity.KindAgent || got.ID != "agent_1" || got.AgentID != "agent_1" {
		t.Fatalf("unexpected actor %+v", got)
	}
}

func TestRequireAdmin(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	for role, want := range map[string]int{"user": 403, "admin": 204} {
		r := httptest.NewRequest("GET", "/api/v1/admin/competitions", nil)
		r = r.WithContext(identity.WithActor(r.Context(), identity.Actor{Kind: identity.KindUser, ID: "user_1", UserID: "user_1", Role: role}))
		w := httptest.NewRecorder()
		identity.RequireAdmin(next).ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("role %s: want %d got %d", role, want, w.Code)
		}
	}
}

var _ = errors.New
```

- [ ] **Step 11: Run** → FAIL. **Step 12: `middleware.go`**

```go
package identity

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"tolerance/internal/platform/auth"
	"tolerance/internal/platform/httpx"
)

type TokenVerifier interface {
	Verify(ctx context.Context, raw string) (auth.Claims, error)
}

// AgentLookup resolves an API key hash to an agent id; implemented by the
// agents module. ErrNoAgent means unknown or revoked.
type AgentLookup interface {
	AgentIDByKeyHash(ctx context.Context, hash string) (string, error)
}

var ErrNoAgent = errors.New("identity: no agent for key")

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
}

// RequireUser authenticates an OIDC JWT and attaches a user Actor.
func RequireUser(v TokenVerifier, s *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok := bearer(r)
			if tok == "" || auth.IsAPIKey(tok) {
				httpx.WriteError(w, r, httpx.Unauthenticated("A user bearer token is required"))
				return
			}
			claims, err := v.Verify(r.Context(), tok)
			if err != nil {
				httpx.WriteError(w, r, httpx.Unauthenticated("Token is invalid or expired"))
				return
			}
			u, err := s.ResolveUser(r.Context(), claims)
			if err != nil {
				httpx.WriteError(w, r, err)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithActor(r.Context(),
				Actor{Kind: KindUser, ID: u.ID, UserID: u.ID, Role: u.Role})))
		})
	}
}

func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if MustFromContext(r.Context()).Role != "admin" {
			httpx.WriteError(w, r, httpx.Forbidden("Admin role required"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAgent authenticates an ak_ API key and attaches an agent Actor.
func RequireAgent(l AgentLookup) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok := bearer(r)
			if !auth.IsAPIKey(tok) {
				httpx.WriteError(w, r, httpx.Unauthenticated("An agent API key is required"))
				return
			}
			id, err := l.AgentIDByKeyHash(r.Context(), auth.HashAPIKey(tok))
			if errors.Is(err, ErrNoAgent) {
				httpx.WriteError(w, r, httpx.Unauthenticated("API key is unknown or revoked"))
				return
			}
			if err != nil {
				httpx.WriteError(w, r, err)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithActor(r.Context(), Actor{Kind: KindAgent, ID: id, AgentID: id})))
		})
	}
}
```

- [ ] **Step 13: Run** → PASS.

- [ ] **Step 14: `http.go`**

```go
package identity

import (
	"context"
	"net/http"

	"tolerance/internal/platform/httpx"
)

// RegisterRoutes mounts /me. meAgent returns the caller's private agent
// view (or nil) — supplied by the agents module so identity does not
// import it.
func RegisterRoutes(mux *http.ServeMux, s *Service, meAgent func(ctx context.Context, userID string) (any, error)) {
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, r *http.Request) {
		actor := MustFromContext(r.Context())
		u, err := s.Get(r.Context(), actor.UserID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		agent, err := meAgent(r.Context(), actor.UserID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"user": u, "agent": agent})
	})
	mux.HandleFunc("PATCH /api/v1/me", func(w http.ResponseWriter, r *http.Request) {
		actor := MustFromContext(r.Context())
		raw, err := httpx.ReadBody(w, r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var in UpdateInput
		if err := httpx.Decode(raw, &in); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		u, err := s.Update(r.Context(), actor.UserID, in)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, u)
	})
}
```

`meAgent` должен возвращать типизированный `nil` как `any(nil)`: агентский модуль возвращает `(any, error)` и при отсутствии агента — `nil, nil`, чтобы в JSON было `"agent": null`.

- [ ] **Step 15: Run** `go test ./internal/identity/ ./internal/platform/...` → PASS; `go vet ./...`.

- [ ] **Step 16: Commit**

```bash
git add backend/internal/identity backend/internal/platform/auth backend/internal/platform/audit backend/internal/platform/httpx
git commit -m "Add identity: OIDC users with handles, admin by email, JWT/API-key middleware, /me

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 5: `standings` — read-модель таблицы лидеров и каталог бейджей

**Files:**
- Create: `internal/standings/model.go`, `internal/standings/badges.go`, `internal/standings/service.go`, `internal/standings/http.go`, `internal/standings/service_integration_test.go`

**Interfaces:**
- Consumes: представление `agent_standings`, таблица `agent_badges` (Task 2).
- Produces:
  - `standings.Standing{Rank *int; Agent, Author string; Points, Wins, CompetitionWins, MatchWins, Submissions int; Avg *int}` (JSON: `rank, agent, author, points, wins, competition_wins, match_wins, submissions, avg`).
  - `standings.Badge{Code, Label, Description string; AwardedAt time.Time}`; `standings.Catalog map[string]BadgeInfo{Label, Description}` с семью кодами из раздела 9.6 спеки.
  - `standings.NewService(pool) *Service`; `(*Service).ForAgent(ctx, agentID) (Standing, error)`; `(*Service).Leaderboard(ctx, limit int) ([]Standing, error)`; `(*Service).BadgesForAgent(ctx, agentID) ([]Badge, error)`.
  - `standings.RegisterPublicRoutes(mux, s)` — `GET /api/v1/leaderboard`.

- [ ] **Step 1: Интеграционный тест на представлении**

```go
package standings_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/dbtest"
	"tolerance/internal/standings"
)

// arrange: two users/agents, one closed competition (200 pts) where A scored 90 and B 80,
// one active competition (500 pts) where A scored 50; one finished match won by B.
func arrange(t *testing.T, d *dbtest.DB) {
	t.Helper()
	err := d.AdminPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
		INSERT INTO users (id, oidc_issuer, oidc_subject, handle, display_name) VALUES
		  ('user_a','seed','a','alice','A'), ('user_b','seed','b','bob','B'), ('user_c','seed','c','carol','C');
		INSERT INTO agents (id, owner_user_id, name, model) VALUES
		  ('agent_a','user_a','Atlas','m'), ('agent_b','user_b','Sable','m'), ('agent_c','user_c','Nova','m');
		INSERT INTO competitions (id, slug, title, summary, brief, category, difficulty, status, points, deadline, criteria, created_by, published_at, closed_at) VALUES
		  ('comp_1','one','1','s','b','Full build','Easy','closed',200, now() - interval '1 day', '[{"name":"Functionality","weight":100,"description":"d"}]','user_a', now() - interval '2 days', now() - interval '1 day'),
		  ('comp_2','two','2','s','b','Bug fix','Easy','active',500, now() + interval '1 day', '[{"name":"Tests","weight":100,"description":"d"}]','user_a', now() - interval '2 days', NULL);
		INSERT INTO submissions (id, competition_id, agent_id, source, artifact, summary, score_status, total, points_awarded, submitted_at) VALUES
		  ('sub_a1','comp_1','agent_a','manual','app','s','scored',90,180, now() - interval '3 days'),
		  ('sub_b1','comp_1','agent_b','manual','app','s','scored',80,160, now() - interval '3 days'),
		  ('sub_a2','comp_2','agent_a','manual','pr','s','scored',50,250, now() - interval '1 hour');
		INSERT INTO matches (id, competition_id, left_agent_id, right_agent_id, state, total_seconds, winner_agent_id, outcome, finished_at) VALUES
		  ('match_1','comp_2','agent_a','agent_b','finished',900,'agent_b','right', now());
		INSERT INTO agent_badges (agent_id, code) VALUES ('agent_a','first_entry');`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestStandings_ViewAndRanks(t *testing.T) {
	d := dbtest.New(t)
	arrange(t, d)
	s := standings.NewService(d.AppPool)
	ctx := context.Background()

	a, err := s.ForAgent(ctx, "agent_a")
	if err != nil {
		t.Fatal(err)
	}
	if a.Points != 430 || a.Submissions != 2 || *a.Avg != 70 || a.CompetitionWins != 1 || a.MatchWins != 0 || a.Wins != 1 || *a.Rank != 1 {
		t.Fatalf("unexpected standing for A: %+v", a)
	}
	b, _ := s.ForAgent(ctx, "agent_b")
	if b.Points != 160 || b.MatchWins != 1 || b.Wins != 1 || *b.Rank != 2 {
		t.Fatalf("unexpected standing for B: %+v", b)
	}
	c, _ := s.ForAgent(ctx, "agent_c")
	if c.Rank != nil || c.Avg != nil || c.Points != 0 {
		t.Fatalf("agent without submissions must have nil rank/avg, got %+v", c)
	}
	board, _ := s.Leaderboard(ctx, 10)
	if len(board) != 2 || board[0].Agent != "Atlas" || board[1].Agent != "Sable" {
		t.Fatalf("leaderboard must list ranked agents only, in order: %+v", board)
	}
	badges, _ := s.BadgesForAgent(ctx, "agent_a")
	if len(badges) != 1 || badges[0].Label != "First entry" {
		t.Fatalf("badge must be resolved through the catalog: %+v", badges)
	}
}
```

- [ ] **Step 2: Run** → FAIL. **Step 3: Реализация**

`model.go`:

```go
package standings

import "time"

type Standing struct {
	Rank            *int   `json:"rank"`
	Agent           string `json:"agent"`
	Author          string `json:"author"`
	Points          int    `json:"points"`
	Wins            int    `json:"wins"`
	CompetitionWins int    `json:"competition_wins"`
	MatchWins       int    `json:"match_wins"`
	Submissions     int    `json:"submissions"`
	Avg             *int   `json:"avg"`
}

type Badge struct {
	Code        string    `json:"code"`
	Label       string    `json:"label"`
	Description string    `json:"description"`
	AwardedAt   time.Time `json:"awarded_at"`
}
```

`badges.go`:

```go
package standings

type BadgeInfo struct{ Label, Description string }

// Catalog is the fixed set of badges. Award rules live in slice 2
// (recompute job); slice 1 only needs labels for the profile page.
var Catalog = map[string]BadgeInfo{
	"first_entry":   {"First entry", "Scored a first submission."},
	"top3_finisher": {"Top 3 finisher", "Placed top 3 in a competition."},
	"clean_coder":   {"Clean coder", "Average code quality score above 90."},
	"best_ux":       {"Best UX", "Highest average UX & polish score."},
	"bug_hunter":    {"Bug hunter", "Highest root-cause score across all bug-fix rounds."},
	"win_streak_5":  {"5-win streak", "Won 5 matches in a row in the live arena."},
	"season_leader": {"Season leader", "#1 on the leaderboard for a full season."},
}
```

`service.go`:

```go
package standings

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
)

type Service struct{ pool *db.Pool }

func NewService(pool *db.Pool) *Service { return &Service{pool: pool} }

const cols = `rank, name, author, points, wins, competition_wins, match_wins, submissions, avg`

func scan(row interface{ Scan(...any) error }, s *Standing) error {
	return row.Scan(&s.Rank, &s.Agent, &s.Author, &s.Points, &s.Wins, &s.CompetitionWins, &s.MatchWins, &s.Submissions, &s.Avg)
}

func (s *Service) ForAgent(ctx context.Context, agentID string) (Standing, error) {
	var st Standing
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return scan(tx.QueryRow(ctx, `SELECT `+cols+` FROM agent_standings WHERE agent_id = $1`, agentID), &st)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Standing{}, httpx.NotFound()
	}
	return st, err
}

func (s *Service) Leaderboard(ctx context.Context, limit int) ([]Standing, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	out := []Standing{}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+cols+` FROM agent_standings WHERE rank IS NOT NULL ORDER BY rank LIMIT $1`, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var st Standing
			if err := scan(rows, &st); err != nil {
				return err
			}
			out = append(out, st)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Service) BadgesForAgent(ctx context.Context, agentID string) ([]Badge, error) {
	out := []Badge{}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT code, awarded_at FROM agent_badges WHERE agent_id = $1 ORDER BY awarded_at`, agentID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var b Badge
			if err := rows.Scan(&b.Code, &b.AwardedAt); err != nil {
				return err
			}
			info := Catalog[b.Code]
			b.Label, b.Description = info.Label, info.Description
			out = append(out, b)
		}
		return rows.Err()
	})
	return out, err
}
```

`http.go`:

```go
package standings

import (
	"net/http"
	"strconv"

	"tolerance/internal/platform/httpx"
)

func RegisterPublicRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/leaderboard", func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		items, err := s.Leaderboard(r.Context(), limit)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items})
	})
}
```

- [ ] **Step 4: Run** → PASS. **Step 5: Commit** `git commit -m "Add standings read model over agent_standings and the badge catalog"` (с Co-Authored-By).

---

### Task 6: `agents` — агент, API-ключи, публичный профиль

**Files:**
- Create: `internal/agents/model.go`, `internal/agents/service.go`, `internal/agents/http.go`, `internal/agents/service_integration_test.go`

**Interfaces:**
- Consumes: `identity.Actor`, `identity.ErrNoAgent`, `auth.GenerateAPIKey`, `standings.Service`, `idempotency.Command` (Task 7 — маршруты регистрируются в Task 9, поэтому здесь только сервис и хендлеры-функции).
- Produces:
  - `agents.Agent{ID, OwnerUserID, Name, Model, Bio string; CreatedAt time.Time; Version int}`.
  - `agents.KeyView{ID, Prefix, Name string; CreatedAt time.Time; LastUsedAt *time.Time}`.
  - `agents.Private{ID, Name, Model, Bio string; CreatedAt time.Time; InArenaQueue bool; APIKeys []KeyView}` (JSON `id, name, model, bio, created_at, in_arena_queue, api_keys`).
  - `agents.Profile{Agent, Author, Model, Bio string; Joined time.Time; Badges []standings.Badge}`.
  - `agents.CreateInput{Name, Model, Bio string}`, `agents.PatchInput{Model, Bio *string}`.
  - `agents.NewService(pool, st *standings.Service) *Service`; методы `Create(ctx, ownerID string, in CreateInput) (Private, error)`, `Patch(ctx, ownerID string, in PatchInput) (Private, error)`, `PrivateForUser(ctx, userID) (*Private, error)` (nil, nil если агента нет), `MeAgent(ctx, userID) (any, error)` (обёртка для identity), `ByName(ctx, name) (Agent, error)`, `ByID(ctx, id) (Agent, error)`, `ProfileByName(ctx, name) (Profile, standings.Standing, error)`, `CreateKey(ctx, ownerID, name string) (KeyView, string /*key*/, error)`, `RevokeKey(ctx, ownerID, keyID string) error`, `AgentIDByKeyHash(ctx, hash) (string, error)` (реализует `identity.AgentLookup`).
  - `agents.RegisterMeRoutes(mux, pool, s)` (`POST/PATCH /api/v1/me/agent`, `POST /api/v1/me/agent/api-keys`, `DELETE /api/v1/me/agent/api-keys/{id}`), `agents.RegisterPublicRoutes(mux, s)` (`GET /api/v1/agents/{name}`), `agents.RegisterAgentRoutes(mux, s)` (`GET /api/v1/agent/me`).

- [ ] **Step 1: Интеграционный тест**

```go
package agents_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"tolerance/internal/agents"
	"tolerance/internal/identity"
	"tolerance/internal/platform/auth"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/standings"
)

func problem(t *testing.T, err error) *httpx.Problem {
	t.Helper()
	var p *httpx.Problem
	if !errors.As(err, &p) {
		t.Fatalf("expected problem, got %v", err)
	}
	return p
}

func TestAgents_CreateKeysProfile(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	users := identity.NewService(d.AppPool, nil)
	u, _ := users.ResolveUser(ctx, auth.Claims{Issuer: "iss", Subject: "s", Email: "mira@example.com"})
	other, _ := users.ResolveUser(ctx, auth.Claims{Issuer: "iss", Subject: "o", Email: "other@example.com"})
	s := agents.NewService(d.AppPool, standings.NewService(d.AppPool))

	if got, _ := s.PrivateForUser(ctx, u.ID); got != nil {
		t.Fatal("no agent yet")
	}
	if _, err := s.Create(ctx, u.ID, agents.CreateInput{Name: "bad name!", Model: "m"}); problem(t, err).Code != "invalid_body" {
		t.Fatal("name format must be validated")
	}
	p, err := s.Create(ctx, u.ID, agents.CreateInput{Name: "Atlas", Model: "Custom · GPT-based", Bio: "bio"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, u.ID, agents.CreateInput{Name: "Second", Model: "m"}); problem(t, err).Code != "agent_exists" {
		t.Fatal("second agent for the same user must be rejected")
	}
	if _, err := s.Create(ctx, other.ID, agents.CreateInput{Name: "atlas", Model: "m"}); problem(t, err).Code != "name_taken" {
		t.Fatal("name must be unique case-insensitively")
	}

	kv, key, err := s.CreateKey(ctx, u.ID, "laptop")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key, "ak_") || kv.Prefix != key[:12] {
		t.Fatalf("bad key %q %+v", key, kv)
	}
	id, err := s.AgentIDByKeyHash(ctx, auth.HashAPIKey(key))
	if err != nil || id != p.ID {
		t.Fatalf("key must resolve to the agent: %v %q", err, id)
	}
	if err := s.RevokeKey(ctx, other.ID, kv.ID); problem(t, err).Status != 404 {
		t.Fatal("another user must not revoke the key")
	}
	if err := s.RevokeKey(ctx, u.ID, kv.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AgentIDByKeyHash(ctx, auth.HashAPIKey(key)); !errors.Is(err, identity.ErrNoAgent) {
		t.Fatal("revoked key must not resolve")
	}

	prof, st, err := s.ProfileByName(ctx, "ATLAS")
	if err != nil {
		t.Fatal(err)
	}
	if prof.Agent != "Atlas" || prof.Author != "mira" || st.Rank != nil || len(prof.Badges) != 0 {
		t.Fatalf("unexpected profile %+v %+v", prof, st)
	}
}
```

- [ ] **Step 2: Run** → FAIL. **Step 3: Реализация**

`model.go`:

```go
package agents

import (
	"regexp"
	"time"

	"tolerance/internal/standings"
)

var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{1,31}$`)

type Agent struct {
	ID          string
	OwnerUserID string
	Name        string
	Model       string
	Bio         string
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
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Model        string    `json:"model"`
	Bio          string    `json:"bio"`
	CreatedAt    time.Time `json:"created_at"`
	InArenaQueue bool      `json:"in_arena_queue"`
	APIKeys      []KeyView `json:"api_keys"`
}

type Profile struct {
	Agent  string           `json:"agent"`
	Author string           `json:"author"`
	Model  string           `json:"model"`
	Bio    string           `json:"bio"`
	Joined time.Time        `json:"joined"`
	Badges []standings.Badge `json:"badges"`
}

type CreateInput struct {
	Name  string `json:"name"`
	Model string `json:"model"`
	Bio   string `json:"bio"`
}

type PatchInput struct {
	Model *string `json:"model"`
	Bio   *string `json:"bio"`
}
```

`service.go`:

```go
package agents

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"tolerance/internal/identity"
	"tolerance/internal/platform/audit"
	"tolerance/internal/platform/auth"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/standings"
)

const maxActiveKeys = 5

type Service struct {
	pool      *db.Pool
	standings *standings.Service
}

func NewService(pool *db.Pool, st *standings.Service) *Service { return &Service{pool: pool, standings: st} }

const agentCols = `id, owner_user_id, name, model, bio, created_at, version`

func scanAgent(row interface{ Scan(...any) error }, a *Agent) error {
	return row.Scan(&a.ID, &a.OwnerUserID, &a.Name, &a.Model, &a.Bio, &a.CreatedAt, &a.Version)
}

func validateCreate(in CreateInput) error {
	if !nameRe.MatchString(in.Name) {
		return httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "name must match ^[A-Za-z0-9][A-Za-z0-9_-]{1,31}$", "name", "invalid")
	}
	if !httpx.ValidText(in.Model, 1, 80) {
		return httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "model must be 1-80 characters", "model", "invalid")
	}
	if len(in.Bio) > 500 {
		return httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "bio must be at most 500 characters", "bio", "too_long")
	}
	return nil
}

func uniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, constraint)
}

func (s *Service) Create(ctx context.Context, ownerID string, in CreateInput) (Private, error) {
	if err := validateCreate(in); err != nil {
		return Private{}, err
	}
	a := Agent{ID: idgen.New("agent"), OwnerUserID: ownerID, Name: in.Name, Model: strings.TrimSpace(in.Model), Bio: in.Bio, CreatedAt: time.Now().UTC(), Version: 1}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO agents (id, owner_user_id, name, model, bio, created_at) VALUES ($1,$2,$3,$4,$5,$6)`,
			a.ID, a.OwnerUserID, a.Name, a.Model, a.Bio, a.CreatedAt); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: ownerID, ActorKind: identity.KindUser, Action: "agent.created",
			AggregateKind: "agent", AggregateID: a.ID, RequestID: httpx.RequestID(ctx)})
	})
	switch {
	case uniqueViolation(err, "agents_owner_user_id_key"):
		return Private{}, httpx.New(http.StatusConflict, "agent_exists", "You already have an agent")
	case uniqueViolation(err, "agents_name_ci_idx"):
		return Private{}, httpx.New(http.StatusConflict, "name_taken", "This agent name is already taken")
	case err != nil:
		return Private{}, err
	}
	return s.private(ctx, a)
}

func (s *Service) Patch(ctx context.Context, ownerID string, in PatchInput) (Private, error) {
	if in.Model != nil && !httpx.ValidText(*in.Model, 1, 80) {
		return Private{}, httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "model must be 1-80 characters", "model", "invalid")
	}
	if in.Bio != nil && len(*in.Bio) > 500 {
		return Private{}, httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "bio must be at most 500 characters", "bio", "too_long")
	}
	var a Agent
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := scanAgent(tx.QueryRow(ctx, `UPDATE agents SET model = coalesce($2, model), bio = coalesce($3, bio), version = version + 1
			WHERE owner_user_id = $1 RETURNING `+agentCols, ownerID, in.Model, in.Bio), &a); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: ownerID, ActorKind: identity.KindUser, Action: "agent.updated",
			AggregateKind: "agent", AggregateID: a.ID, RequestID: httpx.RequestID(ctx)})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Private{}, httpx.NotFound()
	}
	if err != nil {
		return Private{}, err
	}
	return s.private(ctx, a)
}

func (s *Service) private(ctx context.Context, a Agent) (Private, error) {
	p := Private{ID: a.ID, Name: a.Name, Model: a.Model, Bio: a.Bio, CreatedAt: a.CreatedAt, APIKeys: []KeyView{}}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM arena_queue WHERE agent_id = $1)`, a.ID).Scan(&p.InArenaQueue); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id, prefix, name, created_at, last_used_at FROM api_keys WHERE agent_id = $1 AND revoked_at IS NULL ORDER BY created_at`, a.ID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var k KeyView
			if err := rows.Scan(&k.ID, &k.Prefix, &k.Name, &k.CreatedAt, &k.LastUsedAt); err != nil {
				return err
			}
			p.APIKeys = append(p.APIKeys, k)
		}
		return rows.Err()
	})
	return p, err
}

func (s *Service) byOwner(ctx context.Context, ownerID string) (Agent, error) {
	var a Agent
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return scanAgent(tx.QueryRow(ctx, `SELECT `+agentCols+` FROM agents WHERE owner_user_id = $1`, ownerID), &a)
	})
	return a, err
}

// PrivateForUser returns nil, nil when the user has no agent yet.
func (s *Service) PrivateForUser(ctx context.Context, userID string) (*Private, error) {
	a, err := s.byOwner(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p, err := s.private(ctx, a)
	return &p, err
}

// MeAgent adapts PrivateForUser for identity.RegisterRoutes: a missing
// agent must serialise as JSON null, which a typed nil pointer in an
// interface would not.
func (s *Service) MeAgent(ctx context.Context, userID string) (any, error) {
	p, err := s.PrivateForUser(ctx, userID)
	if err != nil || p == nil {
		return nil, err
	}
	return p, nil
}

func (s *Service) ByName(ctx context.Context, name string) (Agent, error) {
	var a Agent
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return scanAgent(tx.QueryRow(ctx, `SELECT `+agentCols+` FROM agents WHERE lower(name) = lower($1)`, name), &a)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Agent{}, httpx.NotFound()
	}
	return a, err
}

func (s *Service) ByID(ctx context.Context, id string) (Agent, error) {
	var a Agent
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return scanAgent(tx.QueryRow(ctx, `SELECT `+agentCols+` FROM agents WHERE id = $1`, id), &a)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Agent{}, httpx.NotFound()
	}
	return a, err
}

func (s *Service) ProfileByName(ctx context.Context, name string) (Profile, standings.Standing, error) {
	a, err := s.ByName(ctx, name)
	if err != nil {
		return Profile{}, standings.Standing{}, err
	}
	st, err := s.standings.ForAgent(ctx, a.ID)
	if err != nil {
		return Profile{}, standings.Standing{}, err
	}
	badges, err := s.standings.BadgesForAgent(ctx, a.ID)
	if err != nil {
		return Profile{}, standings.Standing{}, err
	}
	return Profile{Agent: a.Name, Author: st.Author, Model: a.Model, Bio: a.Bio, Joined: a.CreatedAt, Badges: badges}, st, nil
}

func (s *Service) CreateKey(ctx context.Context, ownerID, name string) (KeyView, string, error) {
	if len(name) > 40 {
		return KeyView{}, "", httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "name must be at most 40 characters", "name", "too_long")
	}
	a, err := s.byOwner(ctx, ownerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return KeyView{}, "", httpx.NotFound()
	}
	if err != nil {
		return KeyView{}, "", err
	}
	key, hash, prefix, err := auth.GenerateAPIKey()
	if err != nil {
		return KeyView{}, "", err
	}
	kv := KeyView{ID: idgen.New("key"), Prefix: prefix, Name: name, CreatedAt: time.Now().UTC()}
	err = s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM api_keys WHERE agent_id = $1 AND revoked_at IS NULL FOR UPDATE`, a.ID).Scan(&n); err != nil {
			return err
		}
		if n >= maxActiveKeys {
			return httpx.New(http.StatusConflict, "too_many_keys", "At most 5 active API keys per agent")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO api_keys (id, agent_id, prefix, key_hash, name, created_at) VALUES ($1,$2,$3,$4,$5,$6)`,
			kv.ID, a.ID, prefix, hash, name, kv.CreatedAt); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: ownerID, ActorKind: identity.KindUser, Action: "api_key.created",
			AggregateKind: "agent", AggregateID: a.ID, Payload: map[string]any{"key_id": kv.ID}, RequestID: httpx.RequestID(ctx)})
	})
	if err != nil {
		return KeyView{}, "", err
	}
	return kv, key, nil
}

func (s *Service) RevokeKey(ctx context.Context, ownerID, keyID string) error {
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE api_keys k SET revoked_at = now() FROM agents a
			WHERE k.id = $1 AND k.agent_id = a.id AND a.owner_user_id = $2 AND k.revoked_at IS NULL`, keyID, ownerID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.NotFound()
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: ownerID, ActorKind: identity.KindUser, Action: "api_key.revoked",
			AggregateKind: "api_key", AggregateID: keyID, RequestID: httpx.RequestID(ctx)})
	})
}

// AgentIDByKeyHash implements identity.AgentLookup. last_used_at is bumped
// at most once a minute to keep the hot path to a single indexed read.
func (s *Service) AgentIDByKeyHash(ctx context.Context, hash string) (string, error) {
	var id string
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `UPDATE api_keys SET last_used_at = CASE WHEN last_used_at IS NULL OR last_used_at < now() - interval '1 minute' THEN now() ELSE last_used_at END
			WHERE key_hash = $1 AND revoked_at IS NULL RETURNING agent_id`, hash).Scan(&id)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", identity.ErrNoAgent
	}
	return id, err
}
```

Имя ограничения `agents_owner_user_id_key` — то, что Postgres даёт `UNIQUE (owner_user_id)` в `CREATE TABLE agents`; если миграция назвала иначе, поправить строку в `uniqueViolation`.

`http.go`:

```go
package agents

import (
	"net/http"

	"tolerance/internal/identity"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idempotency"
)

type createKeyInput struct {
	Name string `json:"name"`
}

func RegisterMeRoutes(mux *http.ServeMux, pool *db.Pool, s *Service) {
	mux.HandleFunc("POST /api/v1/me/agent", idempotency.Command(pool, "POST /me/agent",
		func(r *http.Request, actor identity.Actor, raw []byte) (any, int, error) {
			var in CreateInput
			if err := httpx.Decode(raw, &in); err != nil {
				return nil, 0, err
			}
			p, err := s.Create(r.Context(), actor.UserID, in)
			return p, http.StatusCreated, err
		}))
	mux.HandleFunc("PATCH /api/v1/me/agent", func(w http.ResponseWriter, r *http.Request) {
		raw, err := httpx.ReadBody(w, r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var in PatchInput
		if err := httpx.Decode(raw, &in); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		p, err := s.Patch(r.Context(), identity.MustFromContext(r.Context()).UserID, in)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, p)
	})
	mux.HandleFunc("POST /api/v1/me/agent/api-keys", func(w http.ResponseWriter, r *http.Request) {
		raw, err := httpx.ReadBody(w, r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var in createKeyInput
		if err := httpx.Decode(raw, &in); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		kv, key, err := s.CreateKey(r.Context(), identity.MustFromContext(r.Context()).UserID, in.Name)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusCreated, map[string]any{"id": kv.ID, "prefix": kv.Prefix, "name": kv.Name, "created_at": kv.CreatedAt, "key": key})
	})
	mux.HandleFunc("DELETE /api/v1/me/agent/api-keys/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := s.RevokeKey(r.Context(), identity.MustFromContext(r.Context()).UserID, r.PathValue("id")); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func RegisterPublicRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/agents/{name}", func(w http.ResponseWriter, r *http.Request) {
		prof, st, err := s.ProfileByName(r.Context(), r.PathValue("name"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"profile": prof, "standing": st, "rank": st.Rank})
	})
}

func RegisterAgentRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/agent/me", func(w http.ResponseWriter, r *http.Request) {
		actor := identity.MustFromContext(r.Context())
		a, err := s.ByID(r.Context(), actor.AgentID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		st, err := s.standings.ForAgent(r.Context(), a.ID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var inQueue bool
		_ = s.pool.Tx(r.Context(), func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM arena_queue WHERE agent_id = $1)`, a.ID).Scan(&inQueue)
		})
		httpx.Respond(w, http.StatusOK, map[string]any{
			"agent": map[string]any{"id": a.ID, "name": a.Name, "model": a.Model},
			"owner": map[string]any{"handle": st.Author},
			"in_arena_queue": inQueue,
			"current_match":  nil, // slice 3
		})
	})
}
```

(В `RegisterAgentRoutes` нужны импорты `context` и `github.com/jackc/pgx/v5`.) `http.go` ссылается на `idempotency.Command` из Task 7; до него пакет не соберётся — выполняйте Task 7 сразу после, до `go build ./...`, либо временно закомментируйте регистрацию `POST /me/agent`.

- [ ] **Step 4: Run** `go test ./internal/agents/` → PASS (после Task 7). **Step 5: Commit** `git commit -m "Add agents: one agent per user, API keys, public profile"` (с Co-Authored-By).

---

### Task 7: `idempotency` по актору

**Files:**
- Modify: `internal/platform/idempotency/idempotency.go`
- Create: `internal/platform/idempotency/idempotency_integration_test.go`

**Interfaces:**
- Produces: `idempotency.Command(pool *db.Pool, endpoint string, fn Mutation) http.HandlerFunc`, `type Mutation func(r *http.Request, actor identity.Actor, raw []byte) (any, int, error)` — сигнатура как раньше; ключ записи `(actor.ID, endpoint, key)`.

- [ ] **Step 1: Тест**

```go
package idempotency_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/idempotency"
)

func TestCommand_ReplaysAndConflicts(t *testing.T) {
	d := dbtest.New(t)
	calls := 0
	h := idempotency.Command(d.AppPool, "POST /test", func(r *http.Request, actor identity.Actor, raw []byte) (any, int, error) {
		calls++
		return map[string]any{"n": calls, "actor": actor.ID}, http.StatusCreated, nil
	})
	do := func(actorID, key, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/test", bytes.NewBufferString(body))
		r.Header.Set("Idempotency-Key", key)
		r = r.WithContext(identity.WithActor(r.Context(), identity.Actor{Kind: identity.KindAgent, ID: actorID}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	first := do("agent_1", "key-00000001", `{"a":1}`)
	second := do("agent_1", "key-00000001", `{"a":1}`)
	if first.Code != 201 || second.Code != 201 || calls != 1 {
		t.Fatalf("replay must not rerun the mutation: %d %d calls=%d", first.Code, second.Code, calls)
	}
	var a, b map[string]any
	_ = json.Unmarshal(first.Body.Bytes(), &a)
	_ = json.Unmarshal(second.Body.Bytes(), &b)
	if a["n"] != b["n"] {
		t.Fatal("replayed body must equal the original")
	}
	if w := do("agent_1", "key-00000001", `{"a":2}`); w.Code != 409 {
		t.Fatalf("same key, different body must be 409, got %d", w.Code)
	}
	if w := do("agent_2", "key-00000001", `{"a":1}`); w.Code != 201 || calls != 2 {
		t.Fatalf("keys are scoped per actor: %d calls=%d", w.Code, calls)
	}
	if w := do("agent_2", "short", `{}`); w.Code != 422 {
		t.Fatalf("short key must be 422, got %d", w.Code)
	}
}
```

- [ ] **Step 2: Run** → FAIL. **Step 3: Переписать `idempotency.go`**

Восстановить прежнюю структуру (`Command`, `lookup`, `save`, `respondRaw`) с изменениями: `pool.Tx(ctx, func…)` вместо tenant-транзакций; SQL `SELECT payload_digest, status, body FROM idempotency_records WHERE actor_id = $1 AND endpoint = $2 AND key = $3` и `INSERT INTO idempotency_records (actor_id, endpoint, key, payload_digest, status, body, expires_at) VALUES ($1,$2,$3,$4,$5,$6, now() + interval '90 days') ON CONFLICT (actor_id, endpoint, key) DO NOTHING`; `actor := identity.MustFromContext(r.Context())`, ключ по `actor.ID`; сообщения `"Idempotency-Key must be 8-128 characters"`, `"This key was already used with a different body"`. Комментарий об известном ограничении (запись отдельной транзакцией) сохранить.

- [ ] **Step 4: Run** `go test ./internal/platform/idempotency/ ./internal/agents/` → PASS. **Step 5: Commit** `git commit -m "Rekey idempotency records by actor id"` (с Co-Authored-By).

---

### Task 8: `competitions` — admin CRUD, publish/close, автозакрытие, публичные GET

**Files:**
- Create: `internal/competitions/model.go`, `internal/competitions/validate.go`, `internal/competitions/validate_test.go`, `internal/competitions/service.go`, `internal/competitions/service_integration_test.go`, `internal/competitions/http.go`, `internal/competitions/closer.go`

**Interfaces:**
- Consumes: `identity.Actor`, `identity.System`, `audit.Record`, `idempotency.Command`.
- Produces:
  - `competitions.Criterion{Name string; Weight int; Description string}` (JSON `name, weight, description`).
  - `competitions.Competition{ID, Slug, Title, Summary, Brief, Category, Difficulty, Status string; Points int; Deadline time.Time; MatchDurationSeconds int; Criteria []Criterion; CreatedBy string; CreatedAt time.Time; PublishedAt, ClosedAt *time.Time; Version int; Participants, ScoredCount int}`; метод `(Competition).Public() PublicView` (JSON раздела 8.1: `status` → `"active"|"past"`).
  - `competitions.Input{Slug, Title, Summary, Brief, Category, Difficulty string; Points int; Deadline time.Time; MatchDurationSeconds int; Criteria []Criterion}`; `competitions.Validate(in Input, now time.Time) error`.
  - `competitions.NewService(pool) *Service`; `Create(ctx, actor, in) (Competition, error)`, `Update(ctx, actor, id, in) (Competition, error)`, `Delete(ctx, actor, id) error`, `Publish(ctx, actor, id, expectedVersion int) (Competition, error)`, `Close(ctx, actor, id, expectedVersion int, reason string) (Competition, error)`, `ListAdmin(ctx) ([]Competition, error)`, `ListPublic(ctx, status string) ([]Competition, error)`, `GetPublicBySlug(ctx, slug) (Competition, error)`, `GetByID(ctx, id) (Competition, error)`, `CloseExpired(ctx) (int, error)`.
  - `competitions.RegisterAdminRoutes(mux, pool, s)`, `competitions.RegisterPublicRoutes(mux, s)`, `competitions.RunCloser(ctx, s, every time.Duration, logger *slog.Logger)`.

- [ ] **Step 1: Unit-тест валидации**

```go
package competitions_test

import (
	"testing"
	"time"

	"tolerance/internal/competitions"
)

func valid() competitions.Input {
	return competitions.Input{Slug: "weekend-planner", Title: "T", Summary: "S", Brief: "B", Category: "Full build", Difficulty: "Hard",
		Points: 500, Deadline: time.Now().Add(48 * time.Hour), MatchDurationSeconds: 900,
		Criteria: []competitions.Criterion{{"Functionality", 60, "d"}, {"UX & polish", 40, "d"}}}
}

func TestValidate(t *testing.T) {
	now := time.Now()
	if err := competitions.Validate(valid(), now); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}
	cases := map[string]func(*competitions.Input){
		"slug":            func(i *competitions.Input) { i.Slug = "Bad Slug" },
		"category":        func(i *competitions.Input) { i.Category = "Other" },
		"difficulty":      func(i *competitions.Input) { i.Difficulty = "Insane" },
		"points":          func(i *competitions.Input) { i.Points = 0 },
		"deadline past":   func(i *competitions.Input) { i.Deadline = now.Add(-time.Hour) },
		"duration":        func(i *competitions.Input) { i.MatchDurationSeconds = 10 },
		"weights sum":     func(i *competitions.Input) { i.Criteria[0].Weight = 50 },
		"dup criterion":   func(i *competitions.Input) { i.Criteria[1].Name = "Functionality" },
		"no criteria":     func(i *competitions.Input) { i.Criteria = nil },
		"11 criteria":     func(i *competitions.Input) { i.Criteria = make11() },
		"zero weight":     func(i *competitions.Input) { i.Criteria = []competitions.Criterion{{"A", 100, "d"}, {"B", 0, "d"}} },
	}
	for name, mutate := range cases {
		in := valid()
		mutate(&in)
		if err := competitions.Validate(in, now); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func make11() []competitions.Criterion {
	out := make([]competitions.Criterion, 11)
	for i := range out {
		out[i] = competitions.Criterion{Name: string(rune('A' + i)), Weight: 9, Description: "d"}
	}
	out[10].Weight = 10
	return out
}
```

- [ ] **Step 2: Run** → FAIL. **Step 3: `model.go` и `validate.go`**

```go
package competitions

import "time"

type Criterion struct {
	Name        string `json:"name"`
	Weight      int    `json:"weight"`
	Description string `json:"description"`
}

const (
	StatusDraft  = "draft"
	StatusActive = "active"
	StatusClosed = "closed"
)

type Competition struct {
	ID                   string
	Slug                 string
	Title                string
	Summary              string
	Brief                string
	Category             string
	Difficulty           string
	Status               string
	Points               int
	Deadline             time.Time
	MatchDurationSeconds int
	Criteria             []Criterion
	CreatedBy            string
	CreatedAt            time.Time
	PublishedAt          *time.Time
	ClosedAt             *time.Time
	Version              int
	Participants         int
	ScoredCount          int
}

// PublicView is the shape the frontend's Competition type maps onto.
type PublicView struct {
	ID                   string      `json:"id"`
	Slug                 string      `json:"slug"`
	Title                string      `json:"title"`
	Summary              string      `json:"summary"`
	Brief                string      `json:"brief"`
	Category             string      `json:"category"`
	Difficulty           string      `json:"difficulty"`
	Status               string      `json:"status"` // active | past
	Points               int         `json:"points"`
	Deadline             time.Time   `json:"deadline"`
	Participants         int         `json:"participants"`
	ScoredCount          int         `json:"scored_count"`
	MatchDurationSeconds int         `json:"match_duration_seconds"`
	Criteria             []Criterion `json:"criteria"`
}

func (c Competition) Public() PublicView {
	status := "active"
	if c.Status == StatusClosed {
		status = "past"
	}
	return PublicView{ID: c.ID, Slug: c.Slug, Title: c.Title, Summary: c.Summary, Brief: c.Brief, Category: c.Category,
		Difficulty: c.Difficulty, Status: status, Points: c.Points, Deadline: c.Deadline.UTC(), Participants: c.Participants,
		ScoredCount: c.ScoredCount, MatchDurationSeconds: c.MatchDurationSeconds, Criteria: c.Criteria}
}

// AdminView adds lifecycle fields and the raw status.
type AdminView struct {
	PublicView
	RawStatus   string     `json:"raw_status"`
	CreatedBy   string     `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	PublishedAt *time.Time `json:"published_at"`
	ClosedAt    *time.Time `json:"closed_at"`
	Version     int        `json:"version"`
}

func (c Competition) Admin() AdminView {
	return AdminView{PublicView: c.Public(), RawStatus: c.Status, CreatedBy: c.CreatedBy, CreatedAt: c.CreatedAt.UTC(),
		PublishedAt: c.PublishedAt, ClosedAt: c.ClosedAt, Version: c.Version}
}

type Input struct {
	Slug                 string      `json:"slug"`
	Title                string      `json:"title"`
	Summary              string      `json:"summary"`
	Brief                string      `json:"brief"`
	Category             string      `json:"category"`
	Difficulty           string      `json:"difficulty"`
	Points               int         `json:"points"`
	Deadline             time.Time   `json:"deadline"`
	MatchDurationSeconds int         `json:"match_duration_seconds"`
	Criteria             []Criterion `json:"criteria"`
}
```

```go
package competitions

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"tolerance/internal/platform/httpx"
)

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,63}$`)

var Categories = map[string]bool{"Full build": true, "Bug fix": true, "DB design": true, "Refactor": true, "Integration": true}
var Difficulties = map[string]bool{"Easy": true, "Medium": true, "Hard": true}

func field(msg, path, code string) error {
	return httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", msg, path, code)
}

// Validate enforces invariant 3 of the design (criteria) plus field limits.
func Validate(in Input, now time.Time) error {
	switch {
	case !slugRe.MatchString(in.Slug):
		return field("slug must match ^[a-z0-9][a-z0-9-]{1,63}$", "slug", "invalid")
	case !httpx.ValidText(in.Title, 1, 200):
		return field("title must be 1-200 characters", "title", "invalid")
	case !httpx.ValidText(in.Summary, 1, 500):
		return field("summary must be 1-500 characters", "summary", "invalid")
	case !httpx.ValidText(in.Brief, 1, 10000):
		return field("brief must be 1-10000 characters", "brief", "invalid")
	case !Categories[in.Category]:
		return field("category must be one of Full build, Bug fix, DB design, Refactor, Integration", "category", "invalid")
	case !Difficulties[in.Difficulty]:
		return field("difficulty must be Easy, Medium or Hard", "difficulty", "invalid")
	case in.Points < 1 || in.Points > 10000:
		return field("points must be 1-10000", "points", "invalid")
	case !in.Deadline.After(now):
		return field("deadline must be in the future", "deadline", "past")
	case in.MatchDurationSeconds < 300 || in.MatchDurationSeconds > 3600:
		return field("match_duration_seconds must be 300-3600", "match_duration_seconds", "invalid")
	case len(in.Criteria) < 1 || len(in.Criteria) > 10:
		return field("criteria must have 1-10 items", "criteria", "invalid")
	}
	sum := 0
	seen := map[string]bool{}
	for i, c := range in.Criteria {
		name := strings.TrimSpace(c.Name)
		if !httpx.ValidText(name, 1, 40) || !httpx.ValidText(c.Description, 1, 300) {
			return field("criterion name (1-40) and description (1-300) are required", "criteria", "invalid")
		}
		if seen[name] {
			return field("criterion names must be unique", "criteria", "duplicate")
		}
		seen[name] = true
		if c.Weight < 1 {
			return field("criterion weight must be at least 1", "criteria", "invalid")
		}
		sum += c.Weight
		in.Criteria[i].Name = name
	}
	if sum != 100 {
		return field("criterion weights must sum to 100", "criteria", "weights_sum")
	}
	return nil
}
```

- [ ] **Step 4: Run** unit-тест → PASS.

- [ ] **Step 5: Интеграционный тест сервиса**

```go
package competitions_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/competitions"
	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/httpx"
)

var admin = identity.Actor{Kind: identity.KindUser, ID: "user_admin", UserID: "user_admin", Role: "admin"}

func seedAdmin(t *testing.T, d *dbtest.DB) {
	t.Helper()
	if err := d.AdminPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO users (id, oidc_issuer, oidc_subject, handle, display_name, role) VALUES ('user_admin','seed','adm','admin','Admin','admin')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func code(t *testing.T, err error) string {
	t.Helper()
	var p *httpx.Problem
	if !errors.As(err, &p) {
		t.Fatalf("expected problem, got %v", err)
	}
	return p.Code
}

func TestCompetitions_Lifecycle(t *testing.T) {
	d := dbtest.New(t)
	seedAdmin(t, d)
	ctx := context.Background()
	s := competitions.NewService(d.AppPool)

	c, err := s.Create(ctx, admin, valid())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, admin, valid()); code(t, err) != "slug_taken" {
		t.Fatal("duplicate slug must be slug_taken")
	}
	if _, err := s.GetPublicBySlug(ctx, c.Slug); code(t, err) != "not_found" {
		t.Fatal("drafts are not public")
	}
	if _, err := s.Publish(ctx, admin, c.ID, 999); code(t, err) != "state_conflict" {
		t.Fatal("stale expected_version must conflict")
	}
	pub, err := s.Publish(ctx, admin, c.ID, c.Version)
	if err != nil || pub.Status != competitions.StatusActive || pub.PublishedAt == nil {
		t.Fatalf("publish: %v %+v", err, pub)
	}
	in := valid()
	in.Points = 1
	if _, err := s.Update(ctx, admin, c.ID, in); code(t, err) != "state_conflict" {
		t.Fatal("published competitions are immutable via the service")
	}
	list, _ := s.ListPublic(ctx, "active")
	if len(list) != 1 || list[0].Public().Status != "active" || list[0].Participants != 0 {
		t.Fatalf("public list: %+v", list)
	}
	closed, err := s.Close(ctx, admin, c.ID, pub.Version, "done")
	if err != nil || closed.Status != competitions.StatusClosed || closed.Public().Status != "past" {
		t.Fatalf("close: %v %+v", err, closed)
	}
	if _, err := s.Close(ctx, admin, c.ID, closed.Version, "again"); code(t, err) != "state_conflict" {
		t.Fatal("closing twice must conflict")
	}
	if err := s.Delete(ctx, admin, c.ID); code(t, err) != "state_conflict" {
		t.Fatal("only drafts can be deleted")
	}
}

func TestCompetitions_ForbidsNonAdminAndCountsParticipants(t *testing.T) {
	d := dbtest.New(t)
	seedAdmin(t, d)
	ctx := context.Background()
	s := competitions.NewService(d.AppPool)
	user := identity.Actor{Kind: identity.KindUser, ID: "user_admin", UserID: "user_admin", Role: "user"}
	if _, err := s.Create(ctx, user, valid()); code(t, err) != "forbidden" {
		t.Fatal("non-admin must be forbidden")
	}
	c, _ := s.Create(ctx, admin, valid())
	c, _ = s.Publish(ctx, admin, c.ID, c.Version)
	err := d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO users (id, oidc_issuer, oidc_subject, handle, display_name) VALUES ('user_a','seed','a','a','A'), ('user_b','seed','b','b','B');
			INSERT INTO agents (id, owner_user_id, name, model) VALUES ('agent_a','user_a','A','m'), ('agent_b','user_b','B','m');
			INSERT INTO submissions (id, competition_id, agent_id, source, artifact, summary, score_status, total, points_awarded)
			VALUES ('sub_a', $1, 'agent_a', 'manual', 'app', 's', 'scored', 80, 400), ('sub_b', $1, 'agent_b', 'manual', 'app', 's', 'pending', NULL, NULL)`, c.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetPublicBySlug(ctx, c.Slug)
	if got.Participants != 2 || got.ScoredCount != 1 {
		t.Fatalf("participants=%d scored=%d", got.Participants, got.ScoredCount)
	}
}

func TestCloseExpired(t *testing.T) {
	d := dbtest.New(t)
	seedAdmin(t, d)
	ctx := context.Background()
	s := competitions.NewService(d.AppPool)
	c, _ := s.Create(ctx, admin, valid())
	c, _ = s.Publish(ctx, admin, c.ID, c.Version)
	// Move the deadline into the past directly (Validate forbids it via the API).
	if err := d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE competitions SET status = 'draft', published_at = NULL WHERE id = $1`, c.ID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE competitions SET deadline = now() - interval '1 second', status = 'active', published_at = now() - interval '1 day' WHERE id = $1`, c.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	n, err := s.CloseExpired(ctx)
	if err != nil || n != 1 {
		t.Fatalf("expected one closed, got %d %v", n, err)
	}
	got, _ := s.GetByID(ctx, c.ID)
	if got.Status != competitions.StatusClosed || got.ClosedAt == nil {
		t.Fatalf("not closed: %+v", got)
	}
	_ = time.Second
}
```

- [ ] **Step 6: Run** → FAIL. **Step 7: `service.go`**

```go
package competitions

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"tolerance/internal/identity"
	"tolerance/internal/platform/audit"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
)

type Service struct{ pool *db.Pool }

func NewService(pool *db.Pool) *Service { return &Service{pool: pool} }

const cols = `c.id, c.slug, c.title, c.summary, c.brief, c.category, c.difficulty, c.status, c.points, c.deadline,
	c.match_duration_seconds, c.criteria, c.created_by, c.created_at, c.published_at, c.closed_at, c.version,
	(SELECT count(DISTINCT agent_id) FROM submissions s WHERE s.competition_id = c.id)::int,
	(SELECT count(*) FROM submissions s WHERE s.competition_id = c.id AND s.score_status = 'scored')::int`

func scan(row interface{ Scan(...any) error }, c *Competition) error {
	var criteria []byte
	if err := row.Scan(&c.ID, &c.Slug, &c.Title, &c.Summary, &c.Brief, &c.Category, &c.Difficulty, &c.Status, &c.Points, &c.Deadline,
		&c.MatchDurationSeconds, &criteria, &c.CreatedBy, &c.CreatedAt, &c.PublishedAt, &c.ClosedAt, &c.Version, &c.Participants, &c.ScoredCount); err != nil {
		return err
	}
	return json.Unmarshal(criteria, &c.Criteria)
}

func requireAdmin(actor identity.Actor) error {
	if actor.Role != "admin" {
		return httpx.Forbidden("Admin role required")
	}
	return nil
}

func (s *Service) Create(ctx context.Context, actor identity.Actor, in Input) (Competition, error) {
	if err := requireAdmin(actor); err != nil {
		return Competition{}, err
	}
	if err := Validate(in, time.Now()); err != nil {
		return Competition{}, err
	}
	criteria, _ := json.Marshal(in.Criteria)
	id := idgen.New("comp")
	var c Competition
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO competitions (id, slug, title, summary, brief, category, difficulty, status, points, deadline, match_duration_seconds, criteria, created_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,'draft',$8,$9,$10,$11,$12)`,
			id, in.Slug, in.Title, in.Summary, in.Brief, in.Category, in.Difficulty, in.Points, in.Deadline.UTC(), in.MatchDurationSeconds, criteria, actor.UserID); err != nil {
			return err
		}
		if err := scan(tx.QueryRow(ctx, `SELECT `+cols+` FROM competitions c WHERE c.id = $1`, id), &c); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: actor.ID, ActorKind: actor.Kind, Action: "competition.created",
			AggregateKind: "competition", AggregateID: id, AfterVersion: &c.Version, RequestID: httpx.RequestID(ctx)})
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return Competition{}, httpx.New(http.StatusConflict, "slug_taken", "This slug is already used")
	}
	return c, err
}

func (s *Service) Update(ctx context.Context, actor identity.Actor, id string, in Input) (Competition, error) {
	if err := requireAdmin(actor); err != nil {
		return Competition{}, err
	}
	if err := Validate(in, time.Now()); err != nil {
		return Competition{}, err
	}
	criteria, _ := json.Marshal(in.Criteria)
	var c Competition
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := scan(tx.QueryRow(ctx, `SELECT `+cols+` FROM competitions c WHERE c.id = $1 FOR UPDATE`, id), &c); err != nil {
			return err
		}
		if c.Status != StatusDraft {
			return httpx.StateConflict("Only draft competitions can be edited")
		}
		before := c.Version
		if _, err := tx.Exec(ctx, `UPDATE competitions SET slug=$2, title=$3, summary=$4, brief=$5, category=$6, difficulty=$7, points=$8, deadline=$9,
			match_duration_seconds=$10, criteria=$11, version = version + 1 WHERE id = $1`,
			id, in.Slug, in.Title, in.Summary, in.Brief, in.Category, in.Difficulty, in.Points, in.Deadline.UTC(), in.MatchDurationSeconds, criteria); err != nil {
			return err
		}
		if err := scan(tx.QueryRow(ctx, `SELECT `+cols+` FROM competitions c WHERE c.id = $1`, id), &c); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: actor.ID, ActorKind: actor.Kind, Action: "competition.updated",
			AggregateKind: "competition", AggregateID: id, BeforeVersion: &before, AfterVersion: &c.Version, RequestID: httpx.RequestID(ctx)})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Competition{}, httpx.NotFound()
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return Competition{}, httpx.New(http.StatusConflict, "slug_taken", "This slug is already used")
	}
	return c, err
}

func (s *Service) Delete(ctx context.Context, actor identity.Actor, id string) error {
	if err := requireAdmin(actor); err != nil {
		return err
	}
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var status string
		if err := tx.QueryRow(ctx, `SELECT status FROM competitions WHERE id = $1 FOR UPDATE`, id).Scan(&status); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return httpx.NotFound()
			}
			return err
		}
		if status != StatusDraft {
			return httpx.StateConflict("Only draft competitions can be deleted")
		}
		if _, err := tx.Exec(ctx, `DELETE FROM competitions WHERE id = $1`, id); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: actor.ID, ActorKind: actor.Kind, Action: "competition.deleted",
			AggregateKind: "competition", AggregateID: id, RequestID: httpx.RequestID(ctx)})
	})
}

// transition moves a competition between statuses with optimistic
// concurrency; a stale expected_version yields 409 state_conflict.
func (s *Service) transition(ctx context.Context, actor identity.Actor, id string, expected int, from, to, set, action, reason string) (Competition, error) {
	var c Competition
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := scan(tx.QueryRow(ctx, `SELECT `+cols+` FROM competitions c WHERE c.id = $1 FOR UPDATE`, id), &c); err != nil {
			return err
		}
		if c.Status != from {
			return httpx.StateConflict("Competition is " + c.Status)
		}
		before := c.Version
		tag, err := tx.Exec(ctx, `UPDATE competitions SET status = $2, `+set+`, version = version + 1 WHERE id = $1 AND version = $3`, id, to, expected)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.StateConflict("Competition changed; reload and retry")
		}
		if err := scan(tx.QueryRow(ctx, `SELECT `+cols+` FROM competitions c WHERE c.id = $1`, id), &c); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: actor.ID, ActorKind: actor.Kind, Action: action, Reason: reason,
			AggregateKind: "competition", AggregateID: id, BeforeVersion: &before, AfterVersion: &c.Version, RequestID: httpx.RequestID(ctx)})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Competition{}, httpx.NotFound()
	}
	return c, err
}

func (s *Service) Publish(ctx context.Context, actor identity.Actor, id string, expected int) (Competition, error) {
	if err := requireAdmin(actor); err != nil {
		return Competition{}, err
	}
	c, err := s.GetByID(ctx, id)
	if err != nil {
		return Competition{}, err
	}
	if !c.Deadline.After(time.Now().Add(time.Hour)) {
		return Competition{}, httpx.StateConflict("Deadline must be at least one hour away to publish")
	}
	return s.transition(ctx, actor, id, expected, StatusDraft, StatusActive, "published_at = now()", "competition.published", "")
}

func (s *Service) Close(ctx context.Context, actor identity.Actor, id string, expected int, reason string) (Competition, error) {
	if err := requireAdmin(actor); err != nil {
		return Competition{}, err
	}
	if !httpx.ValidText(reason, 1, 2000) {
		return Competition{}, httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "reason is required", "reason", "required")
	}
	return s.transition(ctx, actor, id, expected, StatusActive, StatusClosed, "closed_at = now()", "competition.closed", reason)
}

func (s *Service) GetByID(ctx context.Context, id string) (Competition, error) {
	var c Competition
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return scan(tx.QueryRow(ctx, `SELECT `+cols+` FROM competitions c WHERE c.id = $1`, id), &c)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Competition{}, httpx.NotFound()
	}
	return c, err
}

func (s *Service) GetPublicBySlug(ctx context.Context, slug string) (Competition, error) {
	var c Competition
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return scan(tx.QueryRow(ctx, `SELECT `+cols+` FROM competitions c WHERE c.slug = $1 AND c.status <> 'draft'`, slug), &c)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Competition{}, httpx.NotFound()
	}
	return c, err
}

func (s *Service) list(ctx context.Context, where string, args ...any) ([]Competition, error) {
	out := []Competition{}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+cols+` FROM competitions c `+where, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c Competition
			if err := scan(rows, &c); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

// ListPublic: status "active" | "past" | "" (both). Active by deadline asc,
// past by deadline desc.
func (s *Service) ListPublic(ctx context.Context, status string) ([]Competition, error) {
	switch status {
	case "active":
		return s.list(ctx, `WHERE c.status = 'active' ORDER BY c.deadline ASC`)
	case "past":
		return s.list(ctx, `WHERE c.status = 'closed' ORDER BY c.deadline DESC`)
	case "":
		return s.list(ctx, `WHERE c.status <> 'draft' ORDER BY (c.status = 'active') DESC, CASE WHEN c.status = 'active' THEN c.deadline END ASC, c.deadline DESC`)
	}
	return nil, httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "status must be active or past", "status", "invalid")
}

func (s *Service) ListAdmin(ctx context.Context) ([]Competition, error) {
	return s.list(ctx, `ORDER BY c.created_at DESC`)
}

// CloseExpired is the scheduler's step: every active competition whose
// deadline has passed becomes closed, with a system audit event each.
func (s *Service) CloseExpired(ctx context.Context) (int, error) {
	n := 0
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `UPDATE competitions SET status = 'closed', closed_at = now(), version = version + 1
			WHERE status = 'active' AND deadline <= now() RETURNING id, version`)
		if err != nil {
			return err
		}
		type closed struct {
			id      string
			version int
		}
		var ids []closed
		for rows.Next() {
			var c closed
			if err := rows.Scan(&c.id, &c.version); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, c)
		}
		rows.Close()
		for _, c := range ids {
			before := c.version - 1
			if err := audit.Record(ctx, tx, audit.Event{ActorID: identity.System.ID, ActorKind: identity.KindSystem, Action: "competition.closed",
				Reason: "deadline", AggregateKind: "competition", AggregateID: c.id, BeforeVersion: &before, AfterVersion: &c.version}); err != nil {
				return err
			}
		}
		n = len(ids)
		return nil
	})
	return n, err
}
```

`closer.go`:

```go
package competitions

import (
	"context"
	"log/slog"
	"time"
)

// RunCloser closes expired competitions every `every` until ctx is done.
// The UPDATE is atomic, so several api instances may run this safely.
func RunCloser(ctx context.Context, s *Service, every time.Duration, log *slog.Logger) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n, err := s.CloseExpired(ctx)
			if err != nil {
				log.Error("close expired competitions", "err", err)
			} else if n > 0 {
				log.Info("closed expired competitions", "count", n)
			}
		}
	}
}
```

- [ ] **Step 8: Run** `go test ./internal/competitions/` → PASS.

- [ ] **Step 9: `http.go`**

```go
package competitions

import (
	"net/http"

	"tolerance/internal/identity"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idempotency"
)

type transitionInput struct {
	ExpectedVersion int    `json:"expected_version"`
	Reason          string `json:"reason,omitempty"`
}

func RegisterAdminRoutes(mux *http.ServeMux, pool *db.Pool, s *Service) {
	mux.HandleFunc("GET /api/v1/admin/competitions", func(w http.ResponseWriter, r *http.Request) {
		list, err := s.ListAdmin(r.Context())
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		items := make([]AdminView, 0, len(list))
		for _, c := range list {
			items = append(items, c.Admin())
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items})
	})
	mux.HandleFunc("POST /api/v1/admin/competitions", idempotency.Command(pool, "POST /admin/competitions",
		func(r *http.Request, actor identity.Actor, raw []byte) (any, int, error) {
			var in Input
			if err := httpx.Decode(raw, &in); err != nil {
				return nil, 0, err
			}
			c, err := s.Create(r.Context(), actor, in)
			if err != nil {
				return nil, 0, err
			}
			return c.Admin(), http.StatusCreated, nil
		}))
	mux.HandleFunc("PATCH /api/v1/admin/competitions/{id}", func(w http.ResponseWriter, r *http.Request) {
		raw, err := httpx.ReadBody(w, r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var in Input
		if err := httpx.Decode(raw, &in); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		c, err := s.Update(r.Context(), identity.MustFromContext(r.Context()), r.PathValue("id"), in)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, c.Admin())
	})
	mux.HandleFunc("DELETE /api/v1/admin/competitions/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Delete(r.Context(), identity.MustFromContext(r.Context()), r.PathValue("id")); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/admin/competitions/{id}/publish", func(w http.ResponseWriter, r *http.Request) {
		var in transitionInput
		if err := readInto(w, r, &in); err != nil {
			return
		}
		c, err := s.Publish(r.Context(), identity.MustFromContext(r.Context()), r.PathValue("id"), in.ExpectedVersion)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, c.Admin())
	})
	mux.HandleFunc("POST /api/v1/admin/competitions/{id}/close", func(w http.ResponseWriter, r *http.Request) {
		var in transitionInput
		if err := readInto(w, r, &in); err != nil {
			return
		}
		c, err := s.Close(r.Context(), identity.MustFromContext(r.Context()), r.PathValue("id"), in.ExpectedVersion, in.Reason)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, c.Admin())
	})
}

// readInto reads and decodes the body, writing the error response itself
// so handlers can `return` on a non-nil result.
func readInto(w http.ResponseWriter, r *http.Request, dst any) error {
	raw, err := httpx.ReadBody(w, r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return err
	}
	if err := httpx.Decode(raw, dst); err != nil {
		httpx.WriteError(w, r, err)
		return err
	}
	return nil
}

func RegisterPublicRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/competitions", func(w http.ResponseWriter, r *http.Request) {
		list, err := s.ListPublic(r.Context(), r.URL.Query().Get("status"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		items := make([]PublicView, 0, len(list))
		for _, c := range list {
			items = append(items, c.Public())
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items})
	})
	mux.HandleFunc("GET /api/v1/competitions/{slug}", func(w http.ResponseWriter, r *http.Request) {
		c, err := s.GetPublicBySlug(r.Context(), r.PathValue("slug"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, c.Public())
	})
}
```

- [ ] **Step 10: Run** `go build ./... && go vet ./... && go test ./internal/...` → PASS. **Step 11: Commit** `git commit -m "Add competitions: admin lifecycle, public reads, deadline auto-close"` (с Co-Authored-By).

---

### Task 9: `cmd/api` — сборка, конфигурация, CORS, `/stats`, `/phases`, OpenAPI, сквозной тест

**Files:**
- Create: `cmd/api/config.go`, `cmd/api/handler.go`, `cmd/api/main_test.go`, `internal/arena/phases.go`, `contracts/openapi/openapi.yaml`, `contracts/openapi/openapi.go`, `contracts/openapi/openapi_test.go`
- Modify: `cmd/api/main.go`, `go.mod` (`go get github.com/getkin/kin-openapi@latest`)

**Interfaces:**
- Consumes: всё из Task 4–8.
- Produces: `newHandler(cfg config, deps deps) http.Handler` (для теста); `openapi.Doc() (*openapi3.T, error)`; `openapi.ValidateResponse(t, router, req, resp)`; `arena.Phases [9]string`.

- [ ] **Step 1: `internal/arena/phases.go`**

```go
// Package arena (slice 3 adds the live arena itself) currently only
// publishes the phase catalog shared by the frontend and connectors.
package arena

import (
	"net/http"

	"tolerance/internal/platform/httpx"
)

var Phases = [9]string{
	"Reading the brief", "Planning approach", "Scaffolding project", "Writing core logic", "Building the UI",
	"Wiring data & state", "Testing the flow", "Polishing details", "Finalizing solution",
}

func RegisterPublicRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/phases", func(w http.ResponseWriter, r *http.Request) {
		httpx.Respond(w, http.StatusOK, map[string]any{"items": Phases[:]})
	})
}
```

- [ ] **Step 2: `cmd/api/config.go`**

```go
package main

import (
	"errors"
	"net"
	"os"
	"strings"
)

type config struct {
	addr         string
	webOrigin    string
	databaseURL  string
	oidcIssuer   string
	oidcAudience string
	oidcJWKSURL  string
	adminEmails  []string
}

func loadConfig() (config, error) {
	cfg := config{
		addr:         env("ARENA_ADDR", "127.0.0.1:8080"),
		webOrigin:    os.Getenv("ARENA_WEB_ORIGIN"),
		databaseURL:  os.Getenv("ARENA_APP_DATABASE_URL"),
		oidcIssuer:   os.Getenv("ARENA_OIDC_ISSUER"),
		oidcAudience: env("ARENA_OIDC_AUDIENCE", "arena-web"),
		oidcJWKSURL:  os.Getenv("ARENA_OIDC_JWKS_URL"),
	}
	for _, e := range strings.Split(os.Getenv("ARENA_ADMIN_EMAILS"), ",") {
		if e = strings.TrimSpace(e); e != "" {
			cfg.adminEmails = append(cfg.adminEmails, e)
		}
	}
	host, _, err := net.SplitHostPort(cfg.addr)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return config{}, errors.New("ARENA_ADDR must be a loopback address; a reverse proxy is expected in front")
	}
	if cfg.databaseURL == "" {
		return config{}, errors.New("ARENA_APP_DATABASE_URL is required")
	}
	if cfg.oidcIssuer == "" || cfg.oidcJWKSURL == "" {
		return config{}, errors.New("ARENA_OIDC_ISSUER and ARENA_OIDC_JWKS_URL are required")
	}
	if cfg.webOrigin == "" {
		return config{}, errors.New("ARENA_WEB_ORIGIN is required so CORS allows exactly one origin")
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

- [ ] **Step 3: `cmd/api/handler.go`**

```go
package main

import (
	"context"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/agents"
	"tolerance/internal/arena"
	"tolerance/internal/competitions"
	"tolerance/internal/identity"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/standings"
)

type deps struct {
	pool         *db.Pool
	verifier     identity.TokenVerifier
	users        *identity.Service
	agents       *agents.Service
	standings    *standings.Service
	competitions *competitions.Service
}

// newHandler wires four route groups with distinct authentication:
// public GETs (no auth), /me/* (user JWT), /agent/* (API key),
// /admin/* (user JWT + admin role).
func newHandler(cfg config, d deps) http.Handler {
	public := http.NewServeMux()
	competitions.RegisterPublicRoutes(public, d.competitions)
	agents.RegisterPublicRoutes(public, d.agents)
	standings.RegisterPublicRoutes(public, d.standings)
	arena.RegisterPublicRoutes(public)
	public.HandleFunc("GET /api/v1/stats", statsHandler(d.pool))

	me := http.NewServeMux()
	identity.RegisterRoutes(me, d.users, d.agents.MeAgent)
	agents.RegisterMeRoutes(me, d.pool, d.agents)

	agent := http.NewServeMux()
	agents.RegisterAgentRoutes(agent, d.agents)

	admin := http.NewServeMux()
	competitions.RegisterAdminRoutes(admin, d.pool, d.competitions)

	userAuth := identity.RequireUser(d.verifier, d.users)
	api := http.NewServeMux()
	api.Handle("/api/v1/me", userAuth(me))
	api.Handle("/api/v1/me/", userAuth(me))
	api.Handle("/api/v1/agent/", identity.RequireAgent(d.agents)(agent))
	api.Handle("/api/v1/admin/", userAuth(identity.RequireAdmin(admin)))
	api.Handle("/api/v1/", public)

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
	return withMiddleware(top, cfg)
}

func statsHandler(pool *db.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var active, agentsN, scored int
		err := pool.Tx(r.Context(), func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT
				(SELECT count(*) FROM competitions WHERE status = 'active'),
				(SELECT count(*) FROM agents),
				(SELECT count(*) FROM submissions WHERE score_status = 'scored')`).Scan(&active, &agentsN, &scored)
		})
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]int{"active_competitions": active, "agents": agentsN, "scored_submissions": scored})
	}
}

func withMiddleware(next http.Handler, cfg config) http.Handler {
	withRequestID := httpx.WithRequestID(func() string { return idgen.New("req") })(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		if !applyCORS(w, r, cfg.webOrigin) {
			return
		}
		withRequestID.ServeHTTP(w, r)
	})
}

func applyCORS(w http.ResponseWriter, r *http.Request, allowed string) bool {
	if origin := r.Header.Get("Origin"); origin != "" && origin == allowed {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, Last-Event-ID")
		w.Header().Set("Access-Control-Max-Age", "600")
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return false
	}
	return true
}
```

Порядок `api.Handle`: `net/http` выбирает самый специфичный шаблон, поэтому `/api/v1/me/` и `/api/v1/admin/` перекрывают `/api/v1/`; `GET /api/v1/agents/{name}` (публичный) не конфликтует с `/api/v1/agent/` — разные префиксы.

- [ ] **Step 4: `cmd/api/main.go`**

```go
// Command api is the whole Agent Arena backend: HTTP API plus the
// background loops (competition auto-close now; arena coordinator and
// judge worker in later slices).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tolerance/internal/agents"
	"tolerance/internal/competitions"
	"tolerance/internal/identity"
	"tolerance/internal/platform/auth"
	"tolerance/internal/platform/db"
	"tolerance/internal/standings"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := loadConfig()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.databaseURL)
	if err != nil {
		log.Error("open database", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	st := standings.NewService(pool)
	d := deps{
		pool:         pool,
		verifier:     auth.NewVerifier(cfg.oidcIssuer, cfg.oidcAudience, cfg.oidcJWKSURL, 10*time.Minute),
		users:        identity.NewService(pool, cfg.adminEmails),
		agents:       agents.NewService(pool, st),
		standings:    st,
		competitions: competitions.NewService(pool),
	}
	go competitions.RunCloser(ctx, d.competitions, 30*time.Second, log)

	server := &http.Server{
		Addr: cfg.addr, Handler: newHandler(cfg, d),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 0 /* SSE in slice 3 */, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	log.Info("arena api listening", "addr", cfg.addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("serve", "err", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 5: OpenAPI-документ и его загрузчик**

`go get github.com/getkin/kin-openapi@latest`. `contracts/openapi/openapi.go`:

```go
// Package openapi embeds the API contract and exposes helpers so handler
// tests can validate real responses against it.
package openapi

import (
	_ "embed"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
)

//go:embed openapi.yaml
var raw []byte

func Doc() (*openapi3.T, error) {
	doc, err := openapi3.NewLoader().LoadFromData(raw)
	if err != nil {
		return nil, err
	}
	return doc, doc.Validate(openapi3.NewLoader().Context)
}

func Router() (routers.Router, error) {
	doc, err := Doc()
	if err != nil {
		return nil, err
	}
	return gorillamux.NewRouter(doc)
}
```

`contracts/openapi/openapi_test.go`:

```go
package openapi_test

import (
	"testing"

	"tolerance/contracts/openapi"
)

func TestDoc_IsValid(t *testing.T) {
	if _, err := openapi.Doc(); err != nil {
		t.Fatalf("openapi.yaml must be a valid OpenAPI 3 document: %v", err)
	}
}
```

`contracts/openapi/openapi.yaml` — OpenAPI 3.0.3 (kin-openapi валидирует 3.0 строже, чем 3.1), `servers: [{url: /api/v1}]`, `securitySchemes: bearerAuth (JWT), apiKeyAuth (http bearer, описание «ak_ key»)`. Пути среза 1 с формами из раздела 8 спеки: `/stats`, `/phases`, `/competitions` (`status` query enum), `/competitions/{slug}`, `/leaderboard` (`limit`), `/agents/{name}`, `/me` (GET, PATCH), `/me/agent` (POST с `Idempotency-Key`, PATCH), `/me/agent/api-keys` (POST), `/me/agent/api-keys/{id}` (DELETE), `/agent/me`, `/admin/competitions` (GET, POST), `/admin/competitions/{id}` (PATCH, DELETE), `/admin/competitions/{id}/publish`, `/admin/competitions/{id}/close`. Схемы: `Problem`, `Criterion`, `Competition` (публичный вид, `status` enum `[active, past]`), `AdminCompetition` (allOf Competition + `raw_status`, `created_by`, `created_at`, `published_at`, `closed_at`, `version`), `CompetitionInput`, `Standing` (`rank`, `avg` nullable), `Badge`, `AgentProfileResponse` (`profile`, `standing`, `rank`), `User`, `AgentPrivate`, `KeyView`, `CreatedKey` (KeyView + `key`), `MeResponse` (`user`, `agent` nullable), `AgentMeResponse`, `ExpectedVersionInput`, `CloseInput`. Все поля из примеров спеки обязательны (`required`), `additionalProperties: false` у объектов ответов, чтобы тест ловил лишние и пропущенные поля.

- [ ] **Step 6: Сквозной тест**

`cmd/api/main_test.go` — тестовый JWKS-сервер как в прежнем `main_test.go` (функция `newTestIdP` с `jwk`/`jwt` из `jwx/v3`; перенести из истории git `git show 7502d5f:backend/cmd/api/main_test.go`), плюс:

```go
func newTestServer(t *testing.T) (*httptest.Server, *testIdP, *dbtest.DB) {
	t.Helper()
	d := dbtest.New(t)
	idp := newTestIdP(t)
	cfg := config{addr: "127.0.0.1:0", webOrigin: "http://localhost:3000", oidcIssuer: e2eIssuer, oidcAudience: e2eAudience, adminEmails: []string{"admin@arena.local"}}
	st := standings.NewService(d.AppPool)
	deps := deps{pool: d.AppPool, verifier: idp.verifier(), users: identity.NewService(d.AppPool, cfg.adminEmails),
		agents: agents.NewService(d.AppPool, st), standings: st, competitions: competitions.NewService(d.AppPool)}
	srv := httptest.NewServer(newHandler(cfg, deps))
	t.Cleanup(srv.Close)
	return srv, idp, d
}

// call performs a request, validates the response against openapi.yaml,
// and decodes the JSON body into out (if non-nil).
func call(t *testing.T, router routers.Router, srv *httptest.Server, method, path, token string, body any, out any) int {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, srv.URL+path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if method == "POST" {
		req.Header.Set("Idempotency-Key", idgen.New("idem"))
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	openapi.ValidateResponse(t, router, req, resp, raw)
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("decode %s %s: %v\n%s", method, path, err, raw)
		}
	}
	return resp.StatusCode
}

func TestEndToEnd_Slice1(t *testing.T) {
	srv, idp, _ := newTestServer(t)
	router, err := openapi.Router()
	if err != nil {
		t.Fatal(err)
	}
	adminTok := idp.token(t, "admin-sub", "admin@arena.local", "Admin")
	userTok := idp.token(t, "mira-sub", "mira@example.com", "Mira")

	// Admin publishes a competition; a plain user may not.
	input := map[string]any{"slug": "weekend-planner", "title": "Weekend planner", "summary": "s", "brief": "b",
		"category": "Full build", "difficulty": "Hard", "points": 500, "deadline": time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339),
		"match_duration_seconds": 900, "criteria": []map[string]any{{"name": "Functionality", "weight": 60, "description": "d"}, {"name": "UX & polish", "weight": 40, "description": "d"}}}
	if code := call(t, router, srv, "POST", "/api/v1/admin/competitions", userTok, input, nil); code != 403 {
		t.Fatalf("non-admin must get 403, got %d", code)
	}
	var created struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	}
	if code := call(t, router, srv, "POST", "/api/v1/admin/competitions", adminTok, input, &created); code != 201 {
		t.Fatalf("create: %d", code)
	}
	if code := call(t, router, srv, "GET", "/api/v1/competitions/weekend-planner", "", nil, nil); code != 404 {
		t.Fatalf("draft must be invisible, got %d", code)
	}
	if code := call(t, router, srv, "POST", "/api/v1/admin/competitions/"+created.ID+"/publish", adminTok, map[string]any{"expected_version": created.Version}, nil); code != 200 {
		t.Fatalf("publish: %d", code)
	}
	var list struct{ Items []struct{ Slug, Status string } }
	call(t, router, srv, "GET", "/api/v1/competitions?status=active", "", nil, &list)
	if len(list.Items) != 1 || list.Items[0].Slug != "weekend-planner" || list.Items[0].Status != "active" {
		t.Fatalf("public list: %+v", list)
	}

	// User: no agent → null; create; second → 409; key works on /agent/me, revoked → 401.
	var me struct{ Agent *json.RawMessage }
	call(t, router, srv, "GET", "/api/v1/me", userTok, nil, &me)
	if me.Agent != nil && string(*me.Agent) != "null" {
		t.Fatalf("agent must be null before creation: %s", *me.Agent)
	}
	if code := call(t, router, srv, "POST", "/api/v1/me/agent", userTok, map[string]any{"name": "Atlas", "model": "Custom", "bio": "b"}, nil); code != 201 {
		t.Fatalf("create agent: %d", code)
	}
	if code := call(t, router, srv, "POST", "/api/v1/me/agent", userTok, map[string]any{"name": "Other", "model": "Custom"}, nil); code != 409 {
		t.Fatalf("second agent: %d", code)
	}
	var key struct{ ID, Key string }
	if code := call(t, router, srv, "POST", "/api/v1/me/agent/api-keys", userTok, map[string]any{"name": "laptop"}, &key); code != 201 {
		t.Fatalf("create key: %d", code)
	}
	if code := call(t, router, srv, "GET", "/api/v1/agent/me", key.Key, nil, nil); code != 200 {
		t.Fatalf("agent/me with key: %d", code)
	}
	if code := call(t, router, srv, "GET", "/api/v1/me", key.Key, nil, nil); code != 401 {
		t.Fatalf("api key on /me must be 401, got %d", code)
	}
	if code := call(t, router, srv, "DELETE", "/api/v1/me/agent/api-keys/"+key.ID, userTok, nil, nil); code != 204 {
		t.Fatalf("revoke: %d", code)
	}
	if code := call(t, router, srv, "GET", "/api/v1/agent/me", key.Key, nil, nil); code != 401 {
		t.Fatalf("revoked key must be 401, got %d", code)
	}

	// Public profile and leaderboard.
	var prof struct {
		Profile struct{ Agent, Author string }
		Rank    *int
	}
	call(t, router, srv, "GET", "/api/v1/agents/atlas", "", nil, &prof)
	if prof.Profile.Agent != "Atlas" || prof.Profile.Author != "mira" || prof.Rank != nil {
		t.Fatalf("profile: %+v", prof)
	}
	var board struct{ Items []any }
	call(t, router, srv, "GET", "/api/v1/leaderboard", "", nil, &board)
	if len(board.Items) != 0 {
		t.Fatalf("no ranked agents yet: %+v", board)
	}
	var stats struct{ ActiveCompetitions int `json:"active_competitions"` }
	call(t, router, srv, "GET", "/api/v1/stats", "", nil, &stats)
	if stats.ActiveCompetitions != 1 {
		t.Fatalf("stats: %+v", stats)
	}
}
```

`idp.token(t, sub, email, name)` подписывает JWT с `iss = e2eIssuer`, `aud = e2eAudience`, claims `email`, `name`; `idp.verifier()` возвращает `*auth.Verifier` с `auth.SetFetcherForTest` на тестовый JWKS (файл `export_test.go` уже даёт `SetFetcherForTest`; так как тест в пакете `main`, добавить в `auth` экспортируемый конструктор `auth.NewVerifierWithFetcher(issuer, audience string, f Fetcher) *Verifier` и использовать его).

`openapi.ValidateResponse` в `contracts/openapi/validate.go`:

```go
package openapi

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
)

// ValidateResponse fails the test if resp does not match the contract for
// req's route. Requests are not validated (tests deliberately send bad
// bodies); responses always are, including error bodies.
func ValidateResponse(t *testing.T, router routers.Router, req *http.Request, resp *http.Response, body []byte) {
	t.Helper()
	route, pathParams, err := router.FindRoute(req)
	if err != nil {
		t.Fatalf("%s %s is not in openapi.yaml: %v", req.Method, req.URL.Path, err)
	}
	input := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, PathParams: pathParams, Route: route,
			Options: &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc}},
		Status: resp.StatusCode, Header: resp.Header, Body: io.NopCloser(bytes.NewReader(body)),
	}
	if err := openapi3filter.ValidateResponse(req.Context(), input); err != nil {
		t.Fatalf("%s %s → %d violates openapi.yaml: %v\n%s", req.Method, req.URL.Path, resp.StatusCode, err, body)
	}
}
```

Каждый статус, который тест получает (200, 201, 204, 401, 403, 404, 409), должен быть описан у соответствующего пути в `openapi.yaml`, иначе `ValidateResponse` упадёт — это желаемое поведение.

- [ ] **Step 7: Run** `go test ./cmd/api/ ./contracts/... -v` → PASS. `make check` чист.

- [ ] **Step 8: Commit**

```bash
git add backend/cmd/api backend/contracts backend/internal/arena backend/internal/platform/auth backend/go.mod backend/go.sum
git commit -m "Wire cmd/api route groups, CORS, stats and phases; add OpenAPI contract with response validation

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 10: `cmd/seed`, README

**Files:**
- Create: `cmd/seed/main.go`, `fixtures/seed/users.json`, `fixtures/seed/agents.json`, `fixtures/seed/competitions.json`, `fixtures/seed/submissions.json`, `fixtures/seed/badges.json`, `fixtures/seed/seed.go` (embed), `cmd/seed/main_test.go`
- Modify: `backend/README.md`, `README.md` (корень)

**Interfaces:**
- Produces: `seed.Load(ctx, pool *db.Pool) error` в пакете `tolerance/fixtures/seed` — идемпотентно отказывается работать на непустой БД (`ErrNotEmpty`).

- [ ] **Step 1: Данные**

Перенести из `frontend/lib/data.ts` один к одному:

`users.json` — шесть пользователей: `{"id": "user_seed_nualimov", "email": "dev@arena.local", "handle": "nualimov", "display_name": "N. Alimov"}`, затем `mira`, `kira`, `dmitri`, `leo`, `sana` с e-mail `<handle>@arena.local`; `oidc_issuer = "seed"`, `oidc_subject = email`. Мок использует автора `you` для Atlas — во фронте это заменяется сравнением с `/me`, поэтому владелец Atlas — `nualimov` (e-mail `dev@arena.local`, тот же, что у статического пользователя Dex в срезе 4).

`agents.json` — `agentProfiles`: `{"id": "agent_seed_atlas", "owner_handle": "nualimov", "name": "Atlas", "model": "Custom · GPT-based", "bio": "…", "created_at": "2026-05-02T00:00:00Z"}` и остальные пять с их `joined`.

`competitions.json` — шесть соревнований `competitions[]`: `id` = `comp_seed_<slug>`, `slug` = `id` мока, `deadline` = `<date>T23:59:59Z`, `status` = `active` для `status: "active"`, `closed` для `"past"` (с `closed_at = deadline`), `published_at = deadline − 30 дней`, `match_duration_seconds = 900`, `created_by` = `nualimov`, `criteria` как в моке. Поле `participants` мока не переносится (оно вычисляется).

`submissions.json` — восемь сдач: `id` = `sub_seed_<id мока>`, `competition_slug`, `agent_name`, `submitted_at` = `<date>T12:00:00Z`, `artifact`, `summary`, `preview_url`, `repo_url`, `scores[]` (`name`, `score`), и `preview` для `cr-atlas`/`cr-sable` (diff из `ArtifactPreview`) и `ms-atlas` (схема). `total` не переносится — считается по критериям (`round(Σ w×s/100)`), так что итог у `wp-atlas` будет 92.4 → 92, как в моке; расхождения на ±1 у других допустимы.

`badges.json` — `{"agent_name": "Atlas", "codes": ["top3_finisher", "win_streak_5", "clean_coder"]}` и т. д. по моку.

- [ ] **Step 2: Тест**

```go
package main_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/fixtures/seed"
	"tolerance/internal/platform/dbtest"
)

func TestSeed_LoadsMockAndRefusesTwice(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	if err := seed.Load(ctx, d.AdminPool); err != nil {
		t.Fatal(err)
	}
	var comps, agents, subs, badges int
	var atlasTotal int
	_ = d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_ = tx.QueryRow(ctx, `SELECT count(*) FROM competitions`).Scan(&comps)
		_ = tx.QueryRow(ctx, `SELECT count(*) FROM agents`).Scan(&agents)
		_ = tx.QueryRow(ctx, `SELECT count(*) FROM submissions WHERE score_status = 'scored'`).Scan(&subs)
		_ = tx.QueryRow(ctx, `SELECT count(*) FROM agent_badges`).Scan(&badges)
		return tx.QueryRow(ctx, `SELECT total FROM submissions WHERE id = 'sub_seed_wp-atlas'`).Scan(&atlasTotal)
	})
	if comps != 6 || agents != 6 || subs != 8 || badges != 6 || atlasTotal != 92 {
		t.Fatalf("comps=%d agents=%d subs=%d badges=%d atlasTotal=%d", comps, agents, subs, badges, atlasTotal)
	}
	if err := seed.Load(ctx, d.AdminPool); err != seed.ErrNotEmpty {
		t.Fatalf("second load must refuse: %v", err)
	}
}
```

- [ ] **Step 3: `fixtures/seed/seed.go`**

```go
// Package seed loads the frontend mock data so a fresh stand looks like
// the v0 prototype. It runs with the migration role and only on an empty
// database; scores are inserted as completed human judgments so they go
// through the same columns the judge fills in slice 2.
package seed

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
	"tolerance/internal/platform/idgen"
)

//go:embed *.json
var files embed.FS

var ErrNotEmpty = errors.New("seed: database is not empty")

type user struct{ ID, Email, Handle, DisplayName string }
type agent struct {
	ID, OwnerHandle, Name, Model, Bio string
	CreatedAt time.Time
}
type criterion struct {
	Name        string `json:"name"`
	Weight      int    `json:"weight"`
	Description string `json:"description"`
}
type competition struct {
	ID, Slug, Title, Summary, Brief, Category, Difficulty, Status string
	Points                                                     int
	Deadline                                                   time.Time
	Criteria                                                   []criterion
}
type score struct {
	Name  string `json:"name"`
	Score int    `json:"score"`
}
type preview struct{ Kind, Body string }
type submission struct {
	ID, CompetitionSlug, AgentName, Artifact, Summary string
	PreviewURL, RepoURL                              *string
	Preview                                          *preview
	SubmittedAt                                      time.Time
	Scores                                           []score
}
type badge struct {
	AgentName string
	Codes     []string
}

func load[T any](name string) ([]T, error) {
	raw, err := files.ReadFile(name)
	if err != nil {
		return nil, err
	}
	var out []T
	return out, json.Unmarshal(raw, &out)
}

func Load(ctx context.Context, pool *db.Pool) error {
	users, err := load[user]("users.json")
	if err != nil {
		return err
	}
	agents, err := load[agent]("agents.json")
	if err != nil {
		return err
	}
	comps, err := load[competition]("competitions.json")
	if err != nil {
		return err
	}
	subs, err := load[submission]("submissions.json")
	if err != nil {
		return err
	}
	badges, err := load[badge]("badges.json")
	if err != nil {
		return err
	}
	return pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrNotEmpty
		}
		userByHandle := map[string]string{}
		for _, u := range users {
			userByHandle[u.Handle] = u.ID
			if _, err := tx.Exec(ctx, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, handle, display_name) VALUES ($1,'seed',$2,$2,$3,$4)`,
				u.ID, u.Email, u.Handle, u.DisplayName); err != nil {
				return err
			}
		}
		agentByName := map[string]string{}
		for _, a := range agents {
			agentByName[a.Name] = a.ID
			if _, err := tx.Exec(ctx, `INSERT INTO agents (id, owner_user_id, name, model, bio, created_at) VALUES ($1,$2,$3,$4,$5,$6)`,
				a.ID, userByHandle[a.OwnerHandle], a.Name, a.Model, a.Bio, a.CreatedAt); err != nil {
				return err
			}
		}
		compBySlug := map[string]competition{}
		for _, c := range comps {
			compBySlug[c.Slug] = c
			criteria, _ := json.Marshal(c.Criteria)
			var closedAt *time.Time
			if c.Status == "closed" {
				d := c.Deadline
				closedAt = &d
			}
			if _, err := tx.Exec(ctx, `INSERT INTO competitions (id, slug, title, summary, brief, category, difficulty, status, points, deadline, criteria, created_by, published_at, closed_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
				c.ID, c.Slug, c.Title, c.Summary, c.Brief, c.Category, c.Difficulty, c.Status, c.Points, c.Deadline, criteria,
				userByHandle["nualimov"], c.Deadline.Add(-30*24*time.Hour), closedAt); err != nil {
				return err
			}
		}
		for _, s := range subs {
			c := compBySlug[s.CompetitionSlug]
			total := Total(c.Criteria, s.Scores)
			points := int(math.Round(float64(c.Points) * float64(total) / 100))
			scores := make([]map[string]any, 0, len(s.Scores))
			for _, sc := range s.Scores {
				scores = append(scores, map[string]any{"name": sc.Name, "score": sc.Score, "rationale": "Seeded from the prototype dataset."})
			}
			scoresJSON, _ := json.Marshal(scores)
			var previewKind, previewBody *string
			if s.Preview != nil {
				previewKind, previewBody = &s.Preview.Kind, &s.Preview.Body
			}
			jdg := idgen.New("jdg")
			if _, err := tx.Exec(ctx, `INSERT INTO submissions (id, competition_id, agent_id, source, artifact, summary, preview_url, repo_url, preview_kind, preview_body, submitted_at, score_status, scores, total, points_awarded, judged_at)
				VALUES ($1,$2,$3,'manual',$4,$5,$6,$7,$8,$9,$10,'scored',$11,$12,$13,$10)`,
				s.ID, c.ID, agentByName[s.AgentName], s.Artifact, s.Summary, s.PreviewURL, s.RepoURL, previewKind, previewBody, s.SubmittedAt, scoresJSON, total, points); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO judgments (id, submission_id, kind, status, scores, total, overall, judge_user_id, created_at, completed_at)
				VALUES ($1,$2,'human','completed',$3,$4,'Seeded from the prototype dataset.',$5,$6,$6)`,
				jdg, s.ID, scoresJSON, total, userByHandle["nualimov"], s.SubmittedAt); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE submissions SET judgment_id = $2 WHERE id = $1`, s.ID, jdg); err != nil {
				return err
			}
		}
		for _, b := range badges {
			for _, code := range b.Codes {
				if _, err := tx.Exec(ctx, `INSERT INTO agent_badges (agent_id, code) VALUES ($1, $2)`, agentByName[b.AgentName], code); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// Total implements invariant 6: round(Σ weight×score / 100). Slice 2's
// judging module reuses this function.
func Total(criteria []criterion, scores []score) int {
	weight := map[string]int{}
	for _, c := range criteria {
		weight[c.Name] = c.Weight
	}
	sum := 0.0
	for _, s := range scores {
		sum += float64(weight[s.Name]) * float64(s.Score)
	}
	return int(math.Round(sum / 100))
}

var _ = fmt.Sprintf
```

JSON-ключи структур — `snake_case` в файлах (`owner_handle`, `created_at`, `competition_slug`, `agent_name`, `preview_url`, `repo_url`, `submitted_at`, `display_name`); добавить теги `json:"…"` к полям выше соответственно.

`cmd/seed/main.go`:

```go
// Command seed fills an empty database with the frontend prototype data.
package main

import (
	"context"
	"log"
	"os"

	"tolerance/fixtures/seed"
	"tolerance/internal/platform/db"
)

func main() {
	dsn := os.Getenv("ARENA_MIGRATE_DATABASE_URL")
	if dsn == "" {
		log.Fatal("ARENA_MIGRATE_DATABASE_URL must be set")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err := seed.Load(ctx, pool); err != nil {
		log.Fatal(err)
	}
	log.Print("seed loaded")
}
```

- [ ] **Step 4: Run** `go test ./cmd/seed/` → PASS.

- [ ] **Step 5: README**

`backend/README.md` переписать: название Agent Arena, ссылка на `docs/arena-backend-design.md` и `docs/plans/arena-slices.md`, статус «срез 1 реализован», локальный запуск:

```sh
export ARENA_APP_ROLE_PASSWORD=some-local-dev-password
make up && make migrate && make seed
export ARENA_OIDC_ISSUER=… ARENA_OIDC_JWKS_URL=… ARENA_OIDC_AUDIENCE=arena-web
export ARENA_WEB_ORIGIN=http://localhost:3000 ARENA_ADMIN_EMAILS=admin@arena.local
make run
curl -s localhost:8080/api/v1/competitions | jq .
```

Секция Colima из старого README сохраняется. Корневой `README.md`: «Agent Arena — платформа, где ИИ-агенты соревнуются и ранжируются»; `backend/` — Go API; `frontend/` — Next.js; ссылки на дизайн и план; примечание, что документы FORGE в `backend/docs` устарели.

- [ ] **Step 6: Commit**

```bash
git add backend/cmd/seed backend/fixtures/seed backend/README.md README.md
git commit -m "Add seed data from the frontend prototype and rewrite READMEs for Agent Arena

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

## Проверка среза

- [ ] `make test` (с Docker) и `make check` зелёные.
- [ ] Критерии «Готово, когда» среза 1 из `arena-slices.md` отмечены.
- [ ] Ручная проверка: `make up && make migrate && make seed && make run`, затем `curl localhost:8080/api/v1/competitions`, `/leaderboard`, `/agents/Atlas`, `/stats`, `/phases` — ответы соответствуют разделу 8.1 спеки.
