# Вход через GitHub и Google — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** заменить вход по e-mail и паролю на OAuth через GitHub и Google, с входом разработчика для локальных запусков и CI.

**Architecture:** `internal/identity` получает `Service.SignIn(Identity)` вместо `Signup/Login`, интерфейс `Provider` с реализациями GitHub и Google на `golang.org/x/oauth2`, и маршруты `start/callback`, которые держат `state` и PKCE-verifier в короткой HttpOnly-cookie. Сессии и `RequireSession` не меняются. Фронт рисует кнопки по `GET /auth/providers`.

**Tech Stack:** Go 1.26, pgx, goose, `golang.org/x/oauth2` (+ `/endpoints`), Next.js, TypeScript.

**Spec:** `docs/superpowers/specs/2026-09-25-oauth-login-design.md`

## Global Constraints

- Совместимость не нужна: пользователей нет. Схема правится прямо в `backend/migrations/00002_schema.sql`, новой миграции нет. Номер `00003` не занимать, он за танками.
- Ошибки JSON-эндпоинтов — только `httpx.WriteError`/`httpx.Problem`. Исключение: `start` и `callback` — навигация браузера, их ошибки — `302 /login?error=<код>`.
- Коды ошибок колбэка ровно такие: `oauth_denied`, `oauth_state`, `oauth_failed`, `email_unverified`, `rate_limited`.
- Cookie состояния: имя `arena_oauth`, `HttpOnly`, `SameSite=Lax`, `Path=/api/v1/auth/`, `MaxAge=600`, `Secure` = `ARENA_SECURE_COOKIES`.
- `ARENA_DEV_LOGIN=true` вместе с `ARENA_SECURE_COOKIES=true` — ошибка конфига; `make up` с `ARENA_ENV=production` и `ARENA_DEV_LOGIN=true` в `.env` — отказ.
- Не импортировать `golang.org/x/oauth2/google` (тянет cloud.google.com), только `golang.org/x/oauth2` и `golang.org/x/oauth2/endpoints`.
- Время в ответах — UTC; сканеры делают `.UTC()`.
- Каждый ответ e2e-теста, включая 302, валидируется по `contracts/openapi/openapi.yaml`.
- Коммиты: английский, повелительное наклонение, без префиксов `feat:`; в конце `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Перед отчётом «готово»: `cd backend && go vet ./... && test -z "$(gofmt -l .)" && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./...`; на фронте `cd frontend && pnpm typecheck && pnpm build`. Под Colima: `export DOCKER_HOST=unix://$HOME/.colima/default/docker.sock TESTCONTAINERS_RYUK_DISABLED=true`. Пропущенный интеграционный тест не считается зелёным.

## Review Focus

1. Открытый редирект через `next` (`//evil.com`, `/\evil.com`, `https://evil.com`, пусто) — всегда `/app`, кроме безопасного пути. Тест в задаче 2.
2. Колбэк без cookie, с cookie другого провайдера или с чужим `state` — `oauth_state`, сессия не создаётся. Тесты в задаче 3.
3. Пользователь нажал «Отмена» у провайдера (`?error=access_denied`) — `oauth_denied`, а не 500. Тест в задаче 3.
4. У GitHub-аккаунта нет подтверждённой первичной почты — `email_unverified`, пользователь не создан. Тесты в задачах 1 и 2.
5. Две одновременные первые попытки входа одной личности — один пользователь, одна личность, обе попытки успешны. Тест в задаче 1.

---

## Файлы

```
backend/migrations/00002_schema.sql          users без password_hash; новая user_identities
backend/internal/identity/service.go         SignIn вместо Signup/Login; Identity; ErrEmailUnverified
backend/internal/identity/password.go        удалить
backend/internal/identity/password_test.go   удалить
backend/internal/identity/http.go            /auth/dev, /auth/providers, /auth/{provider}/start|callback; AuthConfig
backend/internal/identity/oauth.go           NEW: Provider, GitHub, Google, oauthState, safeNext
backend/internal/identity/oauth_test.go      NEW: провайдеры против httptest, safeNext, состояние
backend/internal/identity/http_test.go       + тесты start/callback
backend/internal/identity/auth_integration_test.go  переписать под SignIn
backend/internal/agents/agents_integration_test.go  Signup → SignIn
backend/internal/agents/service_integration_test.go createUser без password_hash
backend/internal/proofs/service_integration_test.go Signup → SignIn
backend/cmd/api/config.go                    devLogin, publicURL, github/google ключи
backend/cmd/api/handler.go                   deps.providers, AuthConfig
backend/cmd/api/main.go                      сборка провайдеров из конфига, slog.SetDefault
backend/cmd/api/main_test.go                 вход через /auth/dev и через фейковый GitHub
backend/contracts/openapi/openapi.yaml       новые /auth/*, без signup/login/Credentials
backend/go.mod, go.sum                       + golang.org/x/oauth2; x/crypto уходит, если больше не нужен
frontend/lib/types.ts                        AuthProviders
frontend/components/auth-form.tsx            кнопки провайдеров, dev-вход, ошибки
frontend/app/terms/page.tsx                  строка про данные аккаунта
frontend/scripts/check-mobile.mjs            вход через /auth/dev
.github/workflows/ci.yml                     ARENA_DEV_LOGIN=true в mobile
.env.example, docker-compose.yml, Makefile   новые переменные, защита продакшна
README.md, backend/README.md, docs/how-it-works.md, CLAUDE.md, спека среза 1   документация
```

---

### Task 1: Вход без паролей в сервисе и вход разработчика

Удаляет пароли целиком и даёт единственный способ входа — `POST /auth/dev`. После задачи стек работает локально и в CI.

**Files:**
- Modify: `backend/migrations/00002_schema.sql`
- Modify: `backend/internal/identity/service.go`, `backend/internal/identity/http.go`
- Delete: `backend/internal/identity/password.go`, `backend/internal/identity/password_test.go`
- Modify: `backend/internal/identity/auth_integration_test.go`
- Modify: `backend/internal/agents/agents_integration_test.go`, `backend/internal/agents/service_integration_test.go`, `backend/internal/proofs/service_integration_test.go`
- Modify: `backend/cmd/api/config.go`, `backend/cmd/api/handler.go`, `backend/cmd/api/main_test.go`
- Modify: `backend/contracts/openapi/openapi.yaml`
- Modify: `frontend/scripts/check-mobile.mjs`, `.github/workflows/ci.yml`, `.env.example`, `docker-compose.yml`, `Makefile`

**Interfaces:**
- Produces:
  - `type identity.Identity struct { Provider, Subject, Email string; EmailVerified bool; Login string }`
  - `func (s *identity.Service) SignIn(ctx context.Context, id identity.Identity) (identity.User, string, error)` — токен для cookie вторым значением
  - `var identity.ErrEmailUnverified *httpx.Problem` (403, `email_unverified`)
  - `type identity.AuthConfig struct { DevLogin, Secure bool }` (задача 3 добавит поля)
  - `func identity.RegisterAuthRoutes(mux *http.ServeMux, s *identity.Service, limiter *ratelimit.Limiter, cfg identity.AuthConfig)`
  - `config.devLogin bool`

- [ ] **Step 1: Схема.** В `backend/migrations/00002_schema.sql` убрать строку `password_hash text NOT NULL,` из `users` и сразу после `CREATE TABLE users (...)` добавить:

```sql
-- One row per external account. subject is the provider's stable user id;
-- 'dev' identities exist only where ARENA_DEV_LOGIN is on.
CREATE TABLE user_identities (
    provider text NOT NULL CHECK (provider IN ('github', 'google', 'dev')),
    subject text NOT NULL,
    user_id text NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    email text NOT NULL,
    login text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    last_login_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, subject)
);
CREATE INDEX user_identities_user_idx ON user_identities (user_id);
```

В секции `-- +goose Down` добавить `DROP TABLE IF EXISTS user_identities;` перед удалением `users` (посмотреть, как там удаляются таблицы, и повторить стиль).

- [ ] **Step 2: Падающий интеграционный тест.** Заменить содержимое `backend/internal/identity/auth_integration_test.go`:

```go
package identity_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
)

func gh(subject, email string) identity.Identity {
	return identity.Identity{Provider: "github", Subject: subject, Email: email, EmailVerified: true, Login: "octo"}
}

func TestSignIn_CreatesThenReturnsTheSameUser(t *testing.T) {
	d := dbtest.New(t)
	s := identity.NewService(d.AppPool, []string{"Admin@Arena.local"})
	ctx := context.Background()

	u, tok, err := s.SignIn(ctx, gh("42", " User@Example.com "))
	if err != nil {
		t.Fatalf("first sign-in: %v", err)
	}
	if u.Email != "user@example.com" || u.Role != "user" || tok == "" {
		t.Fatalf("unexpected user %+v", u)
	}
	got, err := s.UserBySession(ctx, tok)
	if err != nil || got.ID != u.ID {
		t.Fatalf("session lookup: %v %+v", err, got)
	}
	again, tok2, err := s.SignIn(ctx, gh("42", "renamed@example.com"))
	if err != nil || again.ID != u.ID {
		t.Fatalf("returning sign-in must find the same user: %v %+v", err, again)
	}
	if again.Email != "user@example.com" {
		t.Fatalf("users.email keeps the first address, got %q", again.Email)
	}
	if err := s.Logout(ctx, tok); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UserBySession(ctx, tok); !errors.Is(err, identity.ErrNoSession) {
		t.Fatalf("logged-out session must be gone, got %v", err)
	}
	if _, err := s.UserBySession(ctx, tok2); err != nil {
		t.Fatalf("other session must survive: %v", err)
	}
}

func TestSignIn_LinksASecondProviderByVerifiedEmail(t *testing.T) {
	d := dbtest.New(t)
	s := identity.NewService(d.AppPool, nil)
	ctx := context.Background()
	u, _, err := s.SignIn(ctx, gh("42", "same@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	g, _, err := s.SignIn(ctx, identity.Identity{Provider: "google", Subject: "g-1", Email: "Same@Example.com", EmailVerified: true})
	if err != nil || g.ID != u.ID {
		t.Fatalf("google with the same verified email must link: %v %+v", err, g)
	}
	var n int
	if err := d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM user_identities WHERE user_id = $1`, u.ID).Scan(&n)
	}); err != nil || n != 2 {
		t.Fatalf("want 2 identities, got %d %v", n, err)
	}
}

func TestSignIn_RefusesANewIdentityWithoutAVerifiedEmail(t *testing.T) {
	d := dbtest.New(t)
	s := identity.NewService(d.AppPool, nil)
	ctx := context.Background()
	_, _, err := s.SignIn(ctx, identity.Identity{Provider: "github", Subject: "7", Email: "x@example.com", EmailVerified: false})
	if !errors.Is(err, identity.ErrEmailUnverified) {
		t.Fatalf("want ErrEmailUnverified, got %v", err)
	}
	_, _, err = s.SignIn(ctx, identity.Identity{Provider: "github", Subject: "8", Email: "", EmailVerified: true})
	if !errors.Is(err, identity.ErrEmailUnverified) {
		t.Fatalf("empty email must be refused too, got %v", err)
	}
	var n int
	_ = d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n)
	})
	if n != 0 {
		t.Fatalf("no user may be created, got %d", n)
	}
}

func TestSignIn_AdminRoleFollowsConfig(t *testing.T) {
	d := dbtest.New(t)
	s := identity.NewService(d.AppPool, []string{"Admin@Arena.local"})
	u, _, err := s.SignIn(context.Background(), gh("1", "admin@arena.local"))
	if err != nil || u.Role != "admin" {
		t.Fatalf("admin email must get admin role: %v %+v", err, u)
	}
}

func TestSignIn_ConcurrentFirstSignInsMakeOneUser(t *testing.T) {
	d := dbtest.New(t)
	s := identity.NewService(d.AppPool, nil)
	ctx := context.Background()
	var wg sync.WaitGroup
	ids := make([]string, 4)
	errs := make([]error, 4)
	for i := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			u, _, err := s.SignIn(ctx, gh("99", "race@example.com"))
			ids[i], errs[i] = u.ID, err
		}()
	}
	wg.Wait()
	for i := range ids {
		if errs[i] != nil || ids[i] != ids[0] {
			t.Fatalf("attempt %d: %v id=%s want %s", i, errs[i], ids[i], ids[0])
		}
	}
	var users, idents int
	_ = d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&users); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT count(*) FROM user_identities`).Scan(&idents)
	})
	if users != 1 || idents != 1 {
		t.Fatalf("want 1 user and 1 identity, got %d and %d", users, idents)
	}
}
```

- [ ] **Step 3: Прогнать — должен не собраться** (`SignIn` нет).

Run: `cd backend && go test -race ./internal/identity/...`
Expected: FAIL, `s.SignIn undefined`.

- [ ] **Step 4: Сервис.** В `backend/internal/identity/service.go`:
  - удалить `minPasswordLen`, `validateCredentials`, `Signup`, `errInvalidCredentials`, `Login`, импорты `net/mail`, `pgconn`, которые станут лишними; удалить файлы `password.go` и `password_test.go`;
  - оставить `NormalizeEmail`, `userColumns`, `scanUser`, `Get`, `roleFor`, `Logout`, `UserBySession`;
  - добавить:

```go
// Identity is who a sign-in provider says the person is.
type Identity struct {
	Provider      string // "github" | "google" | "dev"
	Subject       string // the provider's stable account id
	Email         string
	EmailVerified bool
	Login         string // GitHub login; empty for other providers
}

var ErrEmailUnverified = httpx.New(http.StatusForbidden, "email_unverified", "Your account has no verified email address")

// SignIn finds or creates the user behind an external identity and opens a
// session. A known (provider, subject) is that user. A new one is linked to
// the user with the same verified email, or creates one; without a verified
// email it is refused, or anyone could claim an account by typing its email
// at the provider. Concurrent first sign-ins of the same identity converge
// on one user through the ON CONFLICT clauses.
func (s *Service) SignIn(ctx context.Context, id Identity) (User, string, error) {
	email := NormalizeEmail(id.Email)
	if id.Provider == "" || id.Subject == "" {
		return User{}, "", errors.New("identity: provider and subject are required")
	}
	token, sid, err := newSessionToken()
	if err != nil {
		return User{}, "", err
	}
	var u User
	err = s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var userID string
		err := tx.QueryRow(ctx, `UPDATE user_identities SET email = $3, login = $4, last_login_at = now()
			WHERE provider = $1 AND subject = $2 RETURNING user_id`, id.Provider, id.Subject, email, id.Login).Scan(&userID)
		if errors.Is(err, pgx.ErrNoRows) {
			userID, err = s.attachIdentity(ctx, tx, id, email)
		}
		if err != nil {
			return err
		}
		if err := scanUser(tx.QueryRow(ctx, `UPDATE users SET role = CASE WHEN email = ANY($2) THEN 'admin' ELSE 'user' END
			WHERE id = $1 RETURNING `+userColumns, userID, s.adminList()), &u); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO sessions (id, user_id, expires_at) VALUES ($1, $2, now() + $3::interval)`, sid, u.ID, sessionTTL.String())
		return err
	})
	if err != nil {
		return User{}, "", err
	}
	return u, token, nil
}

// attachIdentity records a first sign-in of id: to the user with its email,
// or to a new user.
func (s *Service) attachIdentity(ctx context.Context, tx pgx.Tx, id Identity, email string) (string, error) {
	if !id.EmailVerified || email == "" {
		return "", ErrEmailUnverified
	}
	created, err := tx.Exec(ctx, `INSERT INTO users (id, email, role) VALUES ($1, $2, $3) ON CONFLICT (email) DO NOTHING`,
		idgen.New("user"), email, s.roleFor(email))
	if err != nil {
		return "", err
	}
	var userID string
	if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, email).Scan(&userID); err != nil {
		return "", err
	}
	linked, err := tx.Exec(ctx, `INSERT INTO user_identities (provider, subject, user_id, email, login) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (provider, subject) DO NOTHING`, id.Provider, id.Subject, userID, email, id.Login)
	if err != nil {
		return "", err
	}
	if linked.RowsAffected() == 0 {
		return userID, nil // a concurrent first sign-in of the same identity got there first
	}
	action := "user.identity_linked"
	if created.RowsAffected() == 1 {
		action = "user.signed_up"
	}
	return userID, audit.Record(ctx, tx, audit.Event{ActorID: userID, ActorKind: KindUser, Action: action,
		AggregateKind: "user", AggregateID: userID, RequestID: httpx.RequestID(ctx),
		Payload: map[string]string{"provider": id.Provider}})
}

func (s *Service) adminList() []string {
	out := make([]string, 0, len(s.adminEmails))
	for e := range s.adminEmails {
		out = append(out, e)
	}
	return out
}
```

Если `ON CONFLICT` на `users` в параллельном тесте отдаёт `23505` на `user_identities` (не должен: `DO NOTHING` ждёт первую транзакцию), не глушить ошибку, а разобраться: `pool.Tx` использует READ COMMITTED, и `SELECT` после `INSERT … DO NOTHING` видит закоммиченную строку.

- [ ] **Step 5: Прогнать интеграционные тесты identity.**

Run: `cd backend && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./internal/identity/...`
Expected: `http.go` пока не собирается (там `Signup/Login`) — сначала шаг 6, потом этот прогон должен пройти.

- [ ] **Step 6: HTTP.** В `backend/internal/identity/http.go` удалить `credentials` и маршруты signup/login, оставить `clientIP` и logout. Новая регистрация:

```go
// AuthConfig is what the auth routes need from the server config.
type AuthConfig struct {
	DevLogin bool // mounts POST /auth/dev; never on in production
	Secure   bool // Secure flag on cookies
}

// RegisterAuthRoutes mounts the unauthenticated auth routes: the provider
// list, logout, and (when enabled) the development sign-in.
func RegisterAuthRoutes(mux *http.ServeMux, s *Service, limiter *ratelimit.Limiter, cfg AuthConfig) {
	mux.HandleFunc("GET /api/v1/auth/providers", func(w http.ResponseWriter, r *http.Request) {
		httpx.Respond(w, http.StatusOK, map[string]any{"providers": []string{}, "dev_login": cfg.DevLogin})
	})
	if cfg.DevLogin {
		mux.HandleFunc("POST /api/v1/auth/dev", devSignIn(s, limiter, cfg.Secure))
	}
	mux.HandleFunc("POST /api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(SessionCookie); err == nil {
			if err := s.Logout(r.Context(), c.Value); err != nil {
				httpx.WriteError(w, r, err)
				return
			}
		}
		ClearSessionCookie(w, cfg.Secure)
		w.WriteHeader(http.StatusNoContent)
	})
}

// devSignIn signs in as any email, no password: local runs and CI only.
func devSignIn(s *Service, limiter *ratelimit.Limiter, secure bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := httpx.ReadBody(w, r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var in struct {
			Email string `json:"email"`
		}
		if err := httpx.Decode(raw, &in); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		email := NormalizeEmail(in.Email)
		if _, err := mail.ParseAddress(email); err != nil || len(email) > 254 {
			httpx.WriteError(w, r, httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "email must be a valid address", "email", "invalid"))
			return
		}
		if !limiter.Allow("ip:"+clientIP(r), 10, time.Minute) || !limiter.Allow("email:"+email, 10, time.Minute) {
			httpx.WriteError(w, r, httpx.New(http.StatusTooManyRequests, "rate_limited", "Too many attempts, try again in a minute"))
			return
		}
		u, token, err := s.SignIn(r.Context(), Identity{Provider: "dev", Subject: email, Email: email, EmailVerified: true})
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		SetSessionCookie(w, token, secure)
		httpx.Respond(w, http.StatusOK, map[string]any{"user": u})
	}
}
```

- [ ] **Step 7: Конфиг и роутинг.** В `backend/cmd/api/config.go` добавить поле `devLogin bool` (`os.Getenv("ARENA_DEV_LOGIN") == "true"`) и проверку после чтения полей:

```go
	if cfg.devLogin && cfg.secureCookies {
		return config{}, errors.New("ARENA_DEV_LOGIN is for local runs and CI; it cannot be on with ARENA_SECURE_COOKIES=true")
	}
```

В `backend/cmd/api/handler.go` вызов заменить на
`identity.RegisterAuthRoutes(api, d.users, d.limiter, identity.AuthConfig{DevLogin: cfg.devLogin, Secure: cfg.secureCookies})`.

- [ ] **Step 8: Остальные тесты.**
  - `backend/internal/agents/agents_integration_test.go` и `backend/internal/proofs/service_integration_test.go`: `users.Signup(ctx, "o@example.com", "longenough1")` → `users.SignIn(ctx, identity.Identity{Provider: "dev", Subject: "o@example.com", Email: "o@example.com", EmailVerified: true})` (для `us` в proofs аналогично).
  - `backend/internal/agents/service_integration_test.go`: `INSERT INTO users (id, email) VALUES ($1, $2)` без `password_hash`; комментарий над `createUser` поправить: «inserts a user row directly, bypassing SignIn».
  - `backend/cmd/api/main_test.go`: в `newE2E` конфиг `config{..., devLogin: true}`, вызов `newHandler` не меняется. Добавить помощник:

```go
// devLogin signs c in through POST /auth/dev.
func (e *e2e) devLogin(t *testing.T, c *http.Client, email string) {
	t.Helper()
	if code := e.call(t, c, "POST", "/api/v1/auth/dev", "", map[string]string{"email": email}, nil); code != 200 {
		t.Fatalf("dev login %s: %d", email, code)
	}
}
```

  В `TestEndToEnd_SignupConnectProve` (переименовать в `TestEndToEnd_SignInConnectProve`) заменить два вызова signup на `e.devLogin(t, owner, "Owner@Example.com")`; проверка `me.User.Email == "owner@example.com"` остаётся. В `TestEndToEnd_OversizedResultFailsTheProof` — `e.devLogin(t, owner, "big@example.com")`. `TestLogin_RateLimited` → `TestDevLogin_RateLimited`: 10 раз `POST /api/v1/auth/dev {"email": "x@example.com"}`, 11-й — 429. Добавить:

```go
func TestAuthProviders_ListsNothingWithoutKeysButDevLogin(t *testing.T) {
	e := newE2E(t)
	var out struct {
		Providers []string `json:"providers"`
		DevLogin  bool     `json:"dev_login"`
	}
	if code := e.call(t, &http.Client{}, "GET", "/api/v1/auth/providers", "", nil, &out); code != 200 {
		t.Fatalf("providers: %d", code)
	}
	if len(out.Providers) != 0 || !out.DevLogin {
		t.Fatalf("providers: %+v", out)
	}
}
```

  Добавить в `backend/cmd/api` тест конфига (файл `config_test.go`, если его нет):

```go
func TestLoadConfig_RefusesDevLoginWithSecureCookies(t *testing.T) {
	t.Setenv("ARENA_APP_DATABASE_URL", "postgres://x")
	t.Setenv("ARENA_DEV_LOGIN", "true")
	t.Setenv("ARENA_SECURE_COOKIES", "true")
	if _, err := loadConfig(); err == nil {
		t.Fatal("dev login with secure cookies must be refused")
	}
}
```

- [ ] **Step 9: OpenAPI.** В `backend/contracts/openapi/openapi.yaml` удалить `/auth/signup`, `/auth/login` и `components.requestBodies.Credentials` (если `requestBodies` опустеет — удалить ключ). Добавить перед `/auth/logout`:

```yaml
  /auth/providers:
    get:
      operationId: listAuthProviders
      security: []
      responses:
        "200":
          description: Sign-in options this server offers
          content:
            application/json:
              schema:
                type: object
                required: [providers, dev_login]
                properties:
                  providers:
                    type: array
                    items: { type: string, enum: [github, google] }
                  dev_login: { type: boolean }
  /auth/dev:
    post:
      operationId: devLogin
      description: Development sign-in by email; exists only when ARENA_DEV_LOGIN=true.
      security: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [email]
              properties:
                email: { type: string }
      responses:
        "200": { $ref: "#/components/responses/UserEnvelope" }
        "422": { $ref: "#/components/responses/Problem" }
        "429": { $ref: "#/components/responses/Problem" }
```

В `info.description` заменить «sign up» на «sign in with GitHub or Google».

- [ ] **Step 10: CI, compose, Makefile, скрипт.**
  - `frontend/scripts/check-mobile.mjs`: `await call('POST', '/auth/signup', { data: { email: …, password: … } })` → `await call('POST', '/auth/dev', { data: { email: \`mobile-${run}@example.com\` } })`.
  - `.github/workflows/ci.yml`, шаг «start the api»: к переменным запуска добавить `ARENA_DEV_LOGIN=true`.
  - `docker-compose.yml`, `api.environment`: `ARENA_DEV_LOGIN: ${ARENA_DEV_LOGIN:-false}`.
  - `.env.example`, после `ARENA_ADMIN_EMAILS=`:

```
# Sign in as any email without GitHub/Google. Local runs and CI only; the
# api refuses to start with it next to ARENA_SECURE_COOKIES=true.
ARENA_DEV_LOGIN=true
```

    и в блоке «Server only» строку `# ARENA_DEV_LOGIN=false`.
  - `Makefile`, цель `up`, после проверки dev-паролей:

```make
	@if [ "$(ARENA_ENV)" = "production" ] && [ "$(ARENA_DEV_LOGIN)" = "true" ]; then echo "refusing to start production with ARENA_DEV_LOGIN=true"; exit 1; fi
```

    цель `run-api`: добавить `ARENA_DEV_LOGIN=$(or $(ARENA_DEV_LOGIN),true)` к переменным.

- [ ] **Step 11: Зависимости и полный прогон.**

Run: `cd backend && go mod tidy && go vet ./... && test -z "$(gofmt -l .)" && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./...`
Expected: PASS. `golang.org/x/crypto` исчезает из `go.mod`, если его больше никто не импортирует (argon2 был единственным пользователем) — это нормально.

- [ ] **Step 12: Commit.**

```bash
git add -A backend frontend/scripts/check-mobile.mjs .github/workflows/ci.yml docker-compose.yml .env.example Makefile
git commit -m "Replace password sign-in with provider identities and a dev login"
```

---

### Task 2: Провайдеры GitHub и Google

Чистый код без HTTP-маршрутов и базы: провайдеры, cookie-состояние, проверка `next`.

**Files:**
- Create: `backend/internal/identity/oauth.go`
- Create: `backend/internal/identity/oauth_test.go`
- Modify: `backend/go.mod`, `backend/go.sum`

**Interfaces:**
- Consumes: `identity.Identity` из задачи 1.
- Produces:
  - `type identity.Provider interface { AuthCodeURL(state, verifier, redirectURL string) string; Identify(ctx context.Context, code, verifier, redirectURL string) (identity.Identity, error) }`
  - `type identity.GitHub struct { ClientID, ClientSecret, AuthURL, TokenURL, APIURL string; HTTP *http.Client }` — пустые URL означают github.com
  - `type identity.Google struct { ClientID, ClientSecret, AuthURL, TokenURL, UserInfoURL string; HTTP *http.Client }` — пустые URL означают Google
  - `type oauthState struct { Provider, State, Verifier, Next string }`, `func encodeState(oauthState) string`, `func decodeState(string) (oauthState, error)` (неэкспортируемые)
  - `func safeNext(string) string` (неэкспортируемая)

- [ ] **Step 1: Зависимость.**

Run: `cd backend && go get golang.org/x/oauth2@latest`

- [ ] **Step 2: Падающие тесты.** `backend/internal/identity/oauth_test.go`:

```go
package identity

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

// fakeOAuth is a provider's token endpoint that checks the PKCE verifier
// against the challenge sent to the authorize URL, plus JSON API routes.
type fakeOAuth struct {
	t         *testing.T
	challenge string
	routes    map[string]any
	srv       *httptest.Server
}

func newFakeOAuth(t *testing.T, routes map[string]any) *fakeOAuth {
	f := &fakeOAuth{t: t, routes: routes}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_ = r.ParseForm()
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if r.Form.Get("code") != "good-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"tok","token_type":"bearer"}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, ok := f.routes[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// authorize reads the challenge off an authorize URL, as the provider would.
func (f *fakeOAuth) authorize(t *testing.T, raw string) url.Values {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" {
		t.Fatalf("PKCE S256 missing: %s", raw)
	}
	f.challenge = q.Get("code_challenge")
	return q
}

func TestGitHub_IdentifiesByIDWithThePrimaryVerifiedEmail(t *testing.T) {
	f := newFakeOAuth(t, map[string]any{
		"/user": map[string]any{"id": 4242, "login": "octo"},
		"/user/emails": []map[string]any{
			{"email": "old@example.com", "primary": false, "verified": true},
			{"email": "octo@example.com", "primary": true, "verified": true},
		},
	})
	p := &GitHub{ClientID: "cid", ClientSecret: "sec", AuthURL: f.srv.URL + "/authorize", TokenURL: f.srv.URL + "/token", APIURL: f.srv.URL}
	verifier := oauth2.GenerateVerifier()
	q := f.authorize(t, p.AuthCodeURL("st", verifier, "http://arena.test/cb"))
	if q.Get("state") != "st" || q.Get("client_id") != "cid" || q.Get("redirect_uri") != "http://arena.test/cb" || !strings.Contains(q.Get("scope"), "user:email") {
		t.Fatalf("authorize params: %v", q)
	}
	id, err := p.Identify(context.Background(), "good-code", verifier, "http://arena.test/cb")
	if err != nil {
		t.Fatal(err)
	}
	want := Identity{Provider: "github", Subject: "4242", Email: "octo@example.com", EmailVerified: true, Login: "octo"}
	if id != want {
		t.Fatalf("got %+v want %+v", id, want)
	}
}

func TestGitHub_NoPrimaryVerifiedEmailIsUnverified(t *testing.T) {
	f := newFakeOAuth(t, map[string]any{
		"/user":        map[string]any{"id": 1, "login": "x"},
		"/user/emails": []map[string]any{{"email": "x@example.com", "primary": true, "verified": false}},
	})
	p := &GitHub{ClientID: "c", ClientSecret: "s", AuthURL: f.srv.URL + "/authorize", TokenURL: f.srv.URL + "/token", APIURL: f.srv.URL}
	v := oauth2.GenerateVerifier()
	f.authorize(t, p.AuthCodeURL("s", v, "http://arena.test/cb"))
	id, err := p.Identify(context.Background(), "good-code", v, "http://arena.test/cb")
	if err != nil {
		t.Fatal(err)
	}
	if id.EmailVerified {
		t.Fatalf("must not be verified: %+v", id)
	}
}

func TestGitHub_WrongVerifierFails(t *testing.T) {
	f := newFakeOAuth(t, map[string]any{"/user": map[string]any{"id": 1}})
	p := &GitHub{ClientID: "c", ClientSecret: "s", AuthURL: f.srv.URL + "/authorize", TokenURL: f.srv.URL + "/token", APIURL: f.srv.URL}
	f.authorize(t, p.AuthCodeURL("s", oauth2.GenerateVerifier(), "http://arena.test/cb"))
	if _, err := p.Identify(context.Background(), "good-code", oauth2.GenerateVerifier(), "http://arena.test/cb"); err == nil {
		t.Fatal("a verifier that does not match the challenge must fail")
	}
}

func TestGoogle_IdentifiesBySub(t *testing.T) {
	f := newFakeOAuth(t, map[string]any{
		"/userinfo": map[string]any{"sub": "g-123", "email": "a@gmail.com", "email_verified": true},
	})
	p := &Google{ClientID: "c", ClientSecret: "s", AuthURL: f.srv.URL + "/authorize", TokenURL: f.srv.URL + "/token", UserInfoURL: f.srv.URL + "/userinfo"}
	v := oauth2.GenerateVerifier()
	q := f.authorize(t, p.AuthCodeURL("s", v, "http://arena.test/cb"))
	if !strings.Contains(q.Get("scope"), "openid") || !strings.Contains(q.Get("scope"), "email") {
		t.Fatalf("scope: %q", q.Get("scope"))
	}
	id, err := p.Identify(context.Background(), "good-code", v, "http://arena.test/cb")
	if err != nil {
		t.Fatal(err)
	}
	want := Identity{Provider: "google", Subject: "g-123", Email: "a@gmail.com", EmailVerified: true}
	if id != want {
		t.Fatalf("got %+v want %+v", id, want)
	}
}

func TestSafeNext(t *testing.T) {
	for in, want := range map[string]string{
		"":                 "/app",
		"/app":             "/app",
		"/app/proofs/p_1":  "/app/proofs/p_1",
		"//evil.com":       "/app",
		"/\\evil.com":      "/app",
		"https://evil.com": "/app",
		"app":              "/app",
		"/app\r\nX: y":     "/app",
	} {
		if got := safeNext(in); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOAuthState_RoundTripsAndRejectsGarbage(t *testing.T) {
	st := oauthState{Provider: "github", State: "abc", Verifier: "ver", Next: "/app/x"}
	got, err := decodeState(encodeState(st))
	if err != nil || got != st {
		t.Fatalf("round trip: %v %+v", err, got)
	}
	for _, bad := range []string{"", "not base64!", base64.RawURLEncoding.EncodeToString([]byte("{}"))} {
		if _, err := decodeState(bad); err == nil {
			t.Errorf("decodeState(%q) must fail", bad)
		}
	}
}
```

- [ ] **Step 3: Прогнать — не собирается.**

Run: `cd backend && go test ./internal/identity/ -run 'GitHub|Google|SafeNext|OAuthState'`
Expected: FAIL, `undefined: GitHub`.

- [ ] **Step 4: Реализация** `backend/internal/identity/oauth.go`:

```go
package identity

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/endpoints"
)

// Provider is one external sign-in: the URL to send the browser to, and the
// identity behind the code it comes back with. Both use PKCE (S256).
type Provider interface {
	AuthCodeURL(state, verifier, redirectURL string) string
	Identify(ctx context.Context, code, verifier, redirectURL string) (Identity, error)
}

// GitHub signs in with a GitHub OAuth App. Empty URLs mean github.com; tests
// point them at a fake.
type GitHub struct {
	ClientID, ClientSecret     string
	AuthURL, TokenURL, APIURL  string
	HTTP                       *http.Client
}

func (g *GitHub) config(redirectURL string) *oauth2.Config {
	ep := endpoints.GitHub
	if g.AuthURL != "" {
		ep.AuthURL = g.AuthURL
	}
	if g.TokenURL != "" {
		ep.TokenURL = g.TokenURL
	}
	return &oauth2.Config{ClientID: g.ClientID, ClientSecret: g.ClientSecret, Endpoint: ep,
		RedirectURL: redirectURL, Scopes: []string{"read:user", "user:email"}}
}

func (g *GitHub) AuthCodeURL(state, verifier, redirectURL string) string {
	return g.config(redirectURL).AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
}

func (g *GitHub) Identify(ctx context.Context, code, verifier, redirectURL string) (Identity, error) {
	client, err := exchange(ctx, g.HTTP, g.config(redirectURL), code, verifier)
	if err != nil {
		return Identity{}, err
	}
	api := g.APIURL
	if api == "" {
		api = "https://api.github.com"
	}
	var user struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	}
	if err := getJSON(ctx, client, api+"/user", &user); err != nil {
		return Identity{}, err
	}
	if user.ID == 0 {
		return Identity{}, errors.New("github: user without an id")
	}
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := getJSON(ctx, client, api+"/user/emails", &emails); err != nil {
		return Identity{}, err
	}
	id := Identity{Provider: "github", Subject: strconv.FormatInt(user.ID, 10), Login: user.Login}
	for _, e := range emails {
		if e.Primary && e.Verified {
			id.Email, id.EmailVerified = e.Email, true
		}
	}
	return id, nil
}

// Google signs in with OpenID Connect, reading the identity from userinfo
// with the access token (fetched directly from Google over TLS, so the
// id_token's signature does not need checking). Empty URLs mean Google's.
type Google struct {
	ClientID, ClientSecret           string
	AuthURL, TokenURL, UserInfoURL   string
	HTTP                             *http.Client
}

func (g *Google) config(redirectURL string) *oauth2.Config {
	ep := endpoints.Google
	if g.AuthURL != "" {
		ep.AuthURL = g.AuthURL
	}
	if g.TokenURL != "" {
		ep.TokenURL = g.TokenURL
	}
	return &oauth2.Config{ClientID: g.ClientID, ClientSecret: g.ClientSecret, Endpoint: ep,
		RedirectURL: redirectURL, Scopes: []string{"openid", "email", "profile"}}
}

func (g *Google) AuthCodeURL(state, verifier, redirectURL string) string {
	return g.config(redirectURL).AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
}

func (g *Google) Identify(ctx context.Context, code, verifier, redirectURL string) (Identity, error) {
	client, err := exchange(ctx, g.HTTP, g.config(redirectURL), code, verifier)
	if err != nil {
		return Identity{}, err
	}
	u := g.UserInfoURL
	if u == "" {
		u = "https://openidconnect.googleapis.com/v1/userinfo"
	}
	var info struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := getJSON(ctx, client, u, &info); err != nil {
		return Identity{}, err
	}
	if info.Sub == "" {
		return Identity{}, errors.New("google: userinfo without sub")
	}
	return Identity{Provider: "google", Subject: info.Sub, Email: info.Email, EmailVerified: info.EmailVerified}, nil
}

// exchange trades the code for a token and returns a client that sends it.
func exchange(ctx context.Context, hc *http.Client, cfg *oauth2.Config, code, verifier string) (*http.Client, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, hc)
	tok, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}
	return cfg.Client(ctx, tok), nil
}

func getJSON(ctx context.Context, c *http.Client, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
}

// oauthState rides in the arena_oauth cookie between start and callback.
type oauthState struct {
	Provider string `json:"p"`
	State    string `json:"s"`
	Verifier string `json:"v"`
	Next     string `json:"n"`
}

func encodeState(st oauthState) string {
	b, _ := json.Marshal(st)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeState(v string) (oauthState, error) {
	b, err := base64.RawURLEncoding.DecodeString(v)
	if err != nil {
		return oauthState{}, err
	}
	var st oauthState
	if err := json.Unmarshal(b, &st); err != nil {
		return oauthState{}, err
	}
	if st.Provider == "" || st.State == "" || st.Verifier == "" {
		return oauthState{}, errors.New("identity: incomplete oauth state")
	}
	return st, nil
}

// safeNext keeps a post-sign-in redirect on this site: a path, never a
// scheme-relative or backslash URL a browser would read as another host.
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") ||
		strings.ContainsAny(next, "\r\n\t") {
		return "/app"
	}
	return next
}
```

(добавить `"time"` в импорты; `gofmt` выровняет поля структур.)

- [ ] **Step 5: Прогнать.**

Run: `cd backend && go test -race ./internal/identity/ -run 'GitHub|Google|SafeNext|OAuthState' && go vet ./internal/identity/`
Expected: PASS.

- [ ] **Step 6: Commit.**

```bash
git add backend/internal/identity/oauth.go backend/internal/identity/oauth_test.go backend/go.mod backend/go.sum
git commit -m "Add GitHub and Google sign-in providers with PKCE"
```

---

### Task 3: Маршруты start и callback, конфиг провайдеров, e2e

**Files:**
- Modify: `backend/internal/identity/http.go`, `backend/internal/identity/http_test.go`
- Modify: `backend/cmd/api/config.go`, `backend/cmd/api/config_test.go`, `backend/cmd/api/handler.go`, `backend/cmd/api/main.go`, `backend/cmd/api/main_test.go`
- Modify: `backend/contracts/openapi/openapi.yaml`
- Modify: `.env.example`, `docker-compose.yml`

**Interfaces:**
- Consumes: `Provider`, `GitHub`, `Google`, `oauthState`, `encodeState`, `decodeState`, `safeNext` (задача 2); `SignIn`, `ErrEmailUnverified`, `AuthConfig`, `RegisterAuthRoutes` (задача 1).
- Produces:
  - `AuthConfig` получает поля `Providers map[string]Provider` и `PublicURL string`
  - `const identity.OAuthCookie = "arena_oauth"`
  - `deps.providers map[string]identity.Provider` в `cmd/api`
  - `config.publicURL string`, `config.githubID, config.githubSecret, config.googleID, config.googleSecret string`
  - `func providersFromConfig(cfg config) map[string]identity.Provider` в `cmd/api`

- [ ] **Step 1: Падающие тесты HTTP** — дописать в `backend/internal/identity/http_test.go` (пакет `identity`, базы не нужно: до `SignIn` колбэк в этих случаях не доходит):

```go
type stubProvider struct{ authURL string }

func (p stubProvider) AuthCodeURL(state, verifier, redirectURL string) string {
	v := url.Values{"state": {state}, "redirect_uri": {redirectURL}, "code_challenge_method": {"S256"}}
	return p.authURL + "?" + v.Encode()
}
func (stubProvider) Identify(context.Context, string, string, string) (Identity, error) {
	return Identity{}, errors.New("stub: not reached in these tests")
}

func authMux() *http.ServeMux {
	mux := http.NewServeMux()
	RegisterAuthRoutes(mux, NewService(nil, nil), ratelimit.New(nil), AuthConfig{
		Providers: map[string]Provider{"github": stubProvider{authURL: "https://gh.test/authorize"}},
		PublicURL: "https://tolerance.test/",
	})
	return mux
}

func serve(mux *http.ServeMux, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestOAuthStart_RedirectsWithStateAndSetsTheCookie(t *testing.T) {
	rec := serve(authMux(), httptest.NewRequest("GET", "/api/v1/auth/github/start?next=//evil.com", nil))
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d", rec.Code)
	}
	loc, _ := url.Parse(rec.Header().Get("Location"))
	if loc.Host != "gh.test" || loc.Query().Get("redirect_uri") != "https://tolerance.test/api/v1/auth/github/callback" {
		t.Fatalf("location %s", loc)
	}
	var c *http.Cookie
	for _, k := range rec.Result().Cookies() {
		if k.Name == OAuthCookie {
			c = k
		}
	}
	if c == nil || !c.HttpOnly || c.Path != "/api/v1/auth/" || c.MaxAge != 600 {
		t.Fatalf("cookie %+v", c)
	}
	st, err := decodeState(c.Value)
	if err != nil || st.State != loc.Query().Get("state") || st.Provider != "github" || st.Next != "/app" {
		t.Fatalf("state %+v %v", st, err)
	}
}

func TestOAuthStart_UnknownProviderIs404(t *testing.T) {
	if rec := serve(authMux(), httptest.NewRequest("GET", "/api/v1/auth/google/start", nil)); rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestOAuthCallback_Failures(t *testing.T) {
	good := encodeState(oauthState{Provider: "github", State: "s1", Verifier: "v1", Next: "/app"})
	other := encodeState(oauthState{Provider: "google", State: "s1", Verifier: "v1", Next: "/app"})
	for name, tc := range map[string]struct {
		query, cookie, want string
	}{
		"provider error": {"error=access_denied&state=s1", good, "oauth_denied"},
		"no cookie":      {"code=c&state=s1", "", "oauth_state"},
		"wrong state":    {"code=c&state=s2", good, "oauth_state"},
		"other provider": {"code=c&state=s1", other, "oauth_state"},
		"garbage cookie": {"code=c&state=s1", "%%%", "oauth_state"},
		"identify fails": {"code=c&state=s1", good, "oauth_failed"},
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/auth/github/callback?"+tc.query, nil)
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: OAuthCookie, Value: tc.cookie})
			}
			rec := serve(authMux(), req)
			if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/login?error="+tc.want {
				t.Fatalf("got %d %q", rec.Code, rec.Header().Get("Location"))
			}
			for _, k := range rec.Result().Cookies() {
				if k.Name == SessionCookie {
					t.Fatal("no session may be set on failure")
				}
			}
		})
	}
}
```

(импорты: `context`, `errors`, `net/http`, `net/http/httptest`, `net/url`, `tolerance/internal/platform/ratelimit`.)

Run: `cd backend && go test ./internal/identity/ -run OAuth`
Expected: FAIL, `unknown field Providers`.

- [ ] **Step 2: Маршруты** в `backend/internal/identity/http.go`. Расширить `AuthConfig`:

```go
type AuthConfig struct {
	Providers map[string]Provider // "github", "google": only the configured ones
	PublicURL string              // base of redirect_uri, e.g. https://tolerance.cc
	DevLogin  bool
	Secure    bool
}

const OAuthCookie = "arena_oauth"
```

В `/auth/providers` возвращать отсортированные имена `cfg.Providers` (`slices.Sorted(maps.Keys(cfg.Providers))`, пустой срез, а не nil). В `RegisterAuthRoutes` добавить:

```go
	mux.HandleFunc("GET /api/v1/auth/{provider}/start", oauthStart(limiter, cfg))
	mux.HandleFunc("GET /api/v1/auth/{provider}/callback", oauthCallback(s, limiter, cfg))
```

и функции:

```go
func (c AuthConfig) redirectURL(provider string) string {
	return strings.TrimRight(c.PublicURL, "/") + "/api/v1/auth/" + provider + "/callback"
}

func setOAuthCookie(w http.ResponseWriter, value string, maxAge int, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: OAuthCookie, Value: value, Path: "/api/v1/auth/", HttpOnly: true,
		Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
}

// oauthStart sends the browser to the provider. state and the PKCE verifier
// stay in a ten-minute cookie; the callback checks them.
func oauthStart(limiter *ratelimit.Limiter, cfg AuthConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("provider")
		p, ok := cfg.Providers[name]
		if !ok {
			httpx.WriteError(w, r, httpx.NotFound())
			return
		}
		if !limiter.Allow("oauth:ip:"+clientIP(r), 20, time.Minute) {
			http.Redirect(w, r, "/login?error=rate_limited", http.StatusFound)
			return
		}
		state, _, err := newSessionToken() // 32 random bytes, hex
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		st := oauthState{Provider: name, State: state, Verifier: oauth2.GenerateVerifier(), Next: safeNext(r.URL.Query().Get("next"))}
		setOAuthCookie(w, encodeState(st), 600, cfg.Secure)
		http.Redirect(w, r, p.AuthCodeURL(st.State, st.Verifier, cfg.redirectURL(name)), http.StatusFound)
	}
}

// oauthCallback finishes the sign-in. It is a browser navigation, so every
// failure is a redirect to /login with a code the page explains.
func oauthCallback(s *Service, limiter *ratelimit.Limiter, cfg AuthConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("provider")
		p, ok := cfg.Providers[name]
		if !ok {
			httpx.WriteError(w, r, httpx.NotFound())
			return
		}
		setOAuthCookie(w, "", -1, cfg.Secure) // single use, whatever happens next
		fail := func(code string) { http.Redirect(w, r, "/login?error="+code, http.StatusFound) }
		if !limiter.Allow("oauth:ip:"+clientIP(r), 20, time.Minute) {
			fail("rate_limited")
			return
		}
		q := r.URL.Query()
		if q.Get("error") != "" {
			fail("oauth_denied")
			return
		}
		c, err := r.Cookie(OAuthCookie)
		if err != nil {
			fail("oauth_state")
			return
		}
		st, err := decodeState(c.Value)
		if err != nil || st.Provider != name || subtle.ConstantTimeCompare([]byte(st.State), []byte(q.Get("state"))) != 1 {
			fail("oauth_state")
			return
		}
		id, err := p.Identify(r.Context(), q.Get("code"), st.Verifier, cfg.redirectURL(name))
		if err != nil {
			slog.WarnContext(r.Context(), "oauth identify failed", "provider", name, "err", err)
			fail("oauth_failed")
			return
		}
		id.Provider = name
		_, token, err := s.SignIn(r.Context(), id)
		if errors.Is(err, ErrEmailUnverified) {
			fail("email_unverified")
			return
		}
		if err != nil {
			slog.ErrorContext(r.Context(), "oauth sign-in failed", "provider", name, "err", err)
			fail("oauth_failed")
			return
		}
		SetSessionCookie(w, token, cfg.Secure)
		http.Redirect(w, r, st.Next, http.StatusFound)
	}
}
```

(импорты: `crypto/subtle`, `errors`, `log/slog`, `maps`, `slices`, `golang.org/x/oauth2`.)

Run: `cd backend && go test -race ./internal/identity/ -run OAuth`
Expected: PASS.

- [ ] **Step 3: Конфиг.** В `backend/cmd/api/config.go` поля `publicURL, githubID, githubSecret, googleID, googleSecret string` из `ARENA_PUBLIC_URL`, `ARENA_GITHUB_CLIENT_ID`, `ARENA_GITHUB_CLIENT_SECRET`, `ARENA_GOOGLE_CLIENT_ID`, `ARENA_GOOGLE_CLIENT_SECRET`. Проверки:

```go
	for _, p := range [][3]string{{"GITHUB", cfg.githubID, cfg.githubSecret}, {"GOOGLE", cfg.googleID, cfg.googleSecret}} {
		if (p[1] == "") != (p[2] == "") {
			return config{}, fmt.Errorf("set both ARENA_%s_CLIENT_ID and ARENA_%s_CLIENT_SECRET, or neither", p[0], p[0])
		}
	}
	if (cfg.githubID != "" || cfg.googleID != "") && !strings.HasPrefix(cfg.publicURL, "http://") && !strings.HasPrefix(cfg.publicURL, "https://") {
		return config{}, errors.New("ARENA_PUBLIC_URL (http:// or https://) is required when a sign-in provider is configured")
	}
```

и функция рядом:

```go
// providersFromConfig builds the sign-in providers that have both keys set.
func providersFromConfig(cfg config) map[string]identity.Provider {
	ps := map[string]identity.Provider{}
	if cfg.githubID != "" {
		ps["github"] = &identity.GitHub{ClientID: cfg.githubID, ClientSecret: cfg.githubSecret}
	}
	if cfg.googleID != "" {
		ps["google"] = &identity.Google{ClientID: cfg.googleID, ClientSecret: cfg.googleSecret}
	}
	return ps
}
```

В `config_test.go` добавить: только ID без секрета — ошибка; ключи GitHub без `ARENA_PUBLIC_URL` — ошибка; ключи GitHub с `ARENA_PUBLIC_URL=https://tolerance.cc` — ок, и `providersFromConfig` содержит ровно `github`.

- [ ] **Step 4: Проводка.** `deps` получает поле `providers map[string]identity.Provider`; `newHandler` передаёт `identity.AuthConfig{Providers: d.providers, PublicURL: cfg.publicURL, DevLogin: cfg.devLogin, Secure: cfg.secureCookies}`. В `main.go`: `slog.SetDefault(log)` сразу после создания логгера и `providers: providersFromConfig(cfg)` в `deps`.

- [ ] **Step 5: OpenAPI** — добавить после `/auth/dev`:

```yaml
  /auth/{provider}/start:
    get:
      operationId: oauthStart
      security: []
      parameters:
        - { name: provider, in: path, required: true, schema: { type: string } }
        - { name: next, in: query, required: false, schema: { type: string } }
      responses:
        "302": { description: To the provider's consent screen, or to /login?error=rate_limited }
        "404": { $ref: "#/components/responses/Problem" }
  /auth/{provider}/callback:
    get:
      operationId: oauthCallback
      security: []
      parameters:
        - { name: provider, in: path, required: true, schema: { type: string } }
        - { name: code, in: query, required: false, schema: { type: string } }
        - { name: state, in: query, required: false, schema: { type: string } }
        - { name: error, in: query, required: false, schema: { type: string } }
      responses:
        "302":
          description: >-
            Signed in: to the start's next path with the session cookie. Failed: to
            /login?error= one of oauth_denied, oauth_state, oauth_failed, email_unverified, rate_limited.
        "404": { $ref: "#/components/responses/Problem" }
```

- [ ] **Step 6: e2e полного потока** в `backend/cmd/api/main_test.go`. `browser()` перестаёт ходить по редиректам:

```go
	return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
```

`newE2E` принимает провайдеров: сигнатура `newE2E(t *testing.T, providers ...map[string]identity.Provider)`, и если передан — кладёт в `deps.providers`, а `cfg.publicURL = "http://arena.test"`. Тест:

```go
func TestEndToEnd_GitHubSignIn(t *testing.T) {
	var challenge string
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			_ = r.ParseForm()
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if r.Form.Get("code") != "good-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"tok","token_type":"bearer"}`))
		case "/user":
			_, _ = w.Write([]byte(`{"id": 777, "login": "octo"}`))
		case "/user/emails":
			_, _ = w.Write([]byte(`[{"email":"Octo@Example.com","primary":true,"verified":true}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(gh.Close)
	e := newE2E(t, map[string]identity.Provider{"github": &identity.GitHub{ClientID: "cid", ClientSecret: "sec",
		AuthURL: gh.URL + "/authorize", TokenURL: gh.URL + "/token", APIURL: gh.URL}})
	owner := e.browser(t)

	var providers struct {
		Providers []string `json:"providers"`
	}
	e.call(t, owner, "GET", "/api/v1/auth/providers", "", nil, &providers)
	if len(providers.Providers) != 1 || providers.Providers[0] != "github" {
		t.Fatalf("providers: %+v", providers)
	}

	// start → the provider, with state and a PKCE challenge
	req, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/auth/github/start?next=/app/agent/connect", nil)
	resp, err := owner.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	openapi.ValidateResponse(t, e.router, req, resp, nil)
	loc, _ := url.Parse(resp.Header.Get("Location"))
	if resp.StatusCode != 302 || !strings.HasPrefix(loc.String(), gh.URL+"/authorize") {
		t.Fatalf("start: %d %s", resp.StatusCode, loc)
	}
	if loc.Query().Get("redirect_uri") != "http://arena.test/api/v1/auth/github/callback" {
		t.Fatalf("redirect_uri: %s", loc.Query().Get("redirect_uri"))
	}
	challenge = loc.Query().Get("code_challenge")

	// the provider sends the browser back with a code
	cb := "/api/v1/auth/github/callback?code=good-code&state=" + url.QueryEscape(loc.Query().Get("state"))
	req, _ = http.NewRequest("GET", e.srv.URL+cb, nil)
	resp, err = owner.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	openapi.ValidateResponse(t, e.router, req, resp, nil)
	if resp.StatusCode != 302 || resp.Header.Get("Location") != "/app/agent/connect" {
		t.Fatalf("callback: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	var me struct {
		User identity.User `json:"user"`
	}
	if code := e.call(t, owner, "GET", "/api/v1/me", "", nil, &me); code != 200 || me.User.Email != "octo@example.com" {
		t.Fatalf("me: %d %+v", code, me)
	}

	// replaying the same callback in the same browser: the state cookie was
	// cleared, so it is single use
	req, _ = http.NewRequest("GET", e.srv.URL+cb, nil)
	resp, _ = owner.Do(req)
	resp.Body.Close()
	if resp.Header.Get("Location") != "/login?error=oauth_state" {
		t.Fatalf("replay: %s", resp.Header.Get("Location"))
	}
}
```

(импорты: `crypto/sha256`, `encoding/base64`, `net/url`.)

Также проверить, что без dev-входа маршрута нет: `TestDevLogin_OffIs404` — `newE2E` с `devLogin` выключенным нельзя без параметра; вместо этого в `internal/identity/http_test.go` добавить тест: `RegisterAuthRoutes` с `AuthConfig{}` и `POST /api/v1/auth/dev` → 404 (стандартный 404 `ServeMux`, это нормально — маршрута нет) и `GET /api/v1/auth/providers` → `{"providers":[],"dev_login":false}`.

- [ ] **Step 7: Переменные.** `docker-compose.yml`, `api.environment`:

```yaml
      ARENA_PUBLIC_URL: ${ARENA_PUBLIC_URL:-http://localhost:3000}
      ARENA_GITHUB_CLIENT_ID: ${ARENA_GITHUB_CLIENT_ID:-}
      ARENA_GITHUB_CLIENT_SECRET: ${ARENA_GITHUB_CLIENT_SECRET:-}
      ARENA_GOOGLE_CLIENT_ID: ${ARENA_GOOGLE_CLIENT_ID:-}
      ARENA_GOOGLE_CLIENT_SECRET: ${ARENA_GOOGLE_CLIENT_SECRET:-}
```

`.env.example`, после `ARENA_DEV_LOGIN`:

```
# Sign-in providers. Leave empty to have only the dev login locally. Register
# a GitHub OAuth App (github.com/settings/developers) and a Google OAuth client
# (console.cloud.google.com → APIs & Services → Credentials, Web application)
# with the callback {ARENA_PUBLIC_URL}/api/v1/auth/{github,google}/callback.
ARENA_PUBLIC_URL=http://localhost:3000
ARENA_GITHUB_CLIENT_ID=
ARENA_GITHUB_CLIENT_SECRET=
ARENA_GOOGLE_CLIENT_ID=
ARENA_GOOGLE_CLIENT_SECRET=
```

и в блок «Server only»: `# ARENA_PUBLIC_URL=https://arena.example.com`.

- [ ] **Step 8: Полный прогон.**

Run: `cd backend && go vet ./... && test -z "$(gofmt -l .)" && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./...`
Expected: PASS.

- [ ] **Step 9: Commit.**

```bash
git add -A backend .env.example docker-compose.yml
git commit -m "Sign in through GitHub and Google with state and PKCE"
```

---

### Task 4: Страница входа

**Files:**
- Modify: `frontend/lib/types.ts`, `frontend/components/auth-form.tsx`, `frontend/app/terms/page.tsx`

**Interfaces:**
- Consumes: `GET /api/v1/auth/providers` → `{providers: ('github'|'google')[], dev_login: boolean}`; `POST /api/v1/auth/dev {email}`; `GET /api/v1/auth/{provider}/start?next=/app`; коды `?error=` из задачи 3.
- Produces: `export interface AuthProviders` в `lib/types.ts`.

- [ ] **Step 1: Тип** в `frontend/lib/types.ts`:

```ts
export interface AuthProviders { providers: ('github' | 'google')[]; dev_login: boolean }
```

- [ ] **Step 2: Компонент.** Переписать `frontend/components/auth-form.tsx`, сохранив правую колонку (`aside` с `ProofTicket`) и заголовки как есть:

```tsx
'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Brand } from '@/components/brand'
import { ProofTicket } from '@/components/public/proof-ticket'
import { api, post, ApiError } from '@/lib/api'
import type { AuthProviders } from '@/lib/types'

const errors: Record<string, string> = {
  oauth_denied: 'Sign-in was cancelled.',
  oauth_state: 'That sign-in link expired. Try again.',
  oauth_failed: 'The provider did not let us in. Try again in a moment.',
  email_unverified: 'Your account needs a verified email address. Verify one with the provider and try again.',
  rate_limited: 'Too many attempts, wait a minute.',
}

const labels = { github: 'Continue with GitHub', google: 'Continue with Google' } as const

function ProviderIcon({ id }: { id: 'github' | 'google' }) {
  if (id === 'github') {
    return (
      <svg aria-hidden viewBox="0 0 16 16" className="size-4" fill="currentColor">
        <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z" />
      </svg>
    )
  }
  return (
    <svg aria-hidden viewBox="0 0 18 18" className="size-4">
      <path fill="#4285F4" d="M17.64 9.2c0-.64-.06-1.25-.16-1.84H9v3.48h4.84a4.14 4.14 0 01-1.8 2.72v2.26h2.92c1.7-1.57 2.68-3.88 2.68-6.62z" />
      <path fill="#34A853" d="M9 18c2.43 0 4.47-.8 5.96-2.18l-2.92-2.26c-.8.54-1.84.86-3.04.86-2.34 0-4.32-1.58-5.03-3.7H.96v2.33A9 9 0 009 18z" />
      <path fill="#FBBC05" d="M3.97 10.72A5.41 5.41 0 013.68 9c0-.6.1-1.18.29-1.72V4.95H.96A9 9 0 000 9c0 1.45.35 2.83.96 4.05l3.01-2.33z" />
      <path fill="#EA4335" d="M9 3.58c1.32 0 2.5.45 3.44 1.35l2.58-2.58A9 9 0 00.96 4.95l3.01 2.33C4.68 5.16 6.66 3.58 9 3.58z" />
    </svg>
  )
}

export function AuthForm({ mode }: { mode: 'login' | 'signup' }) {
  const router = useRouter()
  const [options, setOptions] = useState<AuthProviders | null>(null)
  const [email, setEmail] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    const code = new URLSearchParams(window.location.search).get('error')
    if (code) setError(errors[code] ?? 'Sign-in failed. Try again.')
    api<AuthProviders>('/auth/providers')
      .then(setOptions)
      .catch(() => setOptions({ providers: [], dev_login: false }))
  }, [])

  async function devSignIn(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await post('/auth/dev', { email })
      router.replace('/app')
    } catch (err) {
      const a = err as ApiError
      setError(a.status === 429 ? errors.rate_limited : a.message)
    } finally {
      setBusy(false)
    }
  }

  const signup = mode === 'signup'
  const nothing = options && options.providers.length === 0 && !options.dev_login
  return (
    <main className="grid min-h-dvh lg:grid-cols-[1fr_1.05fr]">
      <div className="flex flex-col px-4 py-6 sm:px-10">
        <Brand />
        <div className="mx-auto flex w-full max-w-sm flex-1 flex-col justify-center py-12">
          <h1 className="display text-[2.2rem]">{signup ? 'Create your account' : 'Sign in'}</h1>
          <p className="mt-2 text-muted-foreground">
            {signup ? 'Then create your agent and connect it. It takes about five minutes.' : 'Welcome back. Your agent is where you left it.'}
          </p>
          <div className="mt-8 flex min-h-24 flex-col gap-3">
            {options?.providers.map((p) => (
              <Button key={p} size="lg" variant="outline" className="gap-2.5"
                render={<a href={`/api/v1/auth/${p}/start?next=/app`} />} nativeButton={false}>
                <ProviderIcon id={p} />
                {labels[p]}
              </Button>
            ))}
            {nothing && <p className="text-sm text-muted-foreground">Sign-in is not configured on this server yet.</p>}
          </div>
          {error && <p role="alert" className="mt-4 rounded-[9px] bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>}
          {options?.dev_login && (
            <form onSubmit={devSignIn} className="mt-6 flex flex-col gap-3 border-t border-dashed pt-6">
              <Label htmlFor="email">Development sign-in</Label>
              <Input id="email" type="email" autoComplete="email" required placeholder="you@example.com"
                value={email} onChange={(e) => setEmail(e.target.value)} />
              <p className="text-xs text-muted-foreground">Any email, no password. Only on local and CI servers.</p>
              <Button type="submit" variant="secondary" disabled={busy}>{busy ? 'Please wait…' : 'Dev sign in'}</Button>
            </form>
          )}
          <p className="mt-6 text-xs leading-relaxed text-muted-foreground">
            By continuing you accept the <Link className="font-semibold text-foreground underline underline-offset-2" href="/terms">terms and fair play rules</Link>.
          </p>
          <p className="mt-6 text-sm text-muted-foreground">
            {signup ? (
              <>Already have an account? <Link className="font-semibold text-primary hover:underline" href="/login">Sign in</Link></>
            ) : (
              <>New here? The same buttons create your account.</>
            )}
          </p>
        </div>
      </div>
      <aside className="relative hidden overflow-hidden bg-[#15212b] lg:flex lg:flex-col lg:justify-center lg:px-14">
        <p className="display max-w-md text-[2rem] text-[#eef2f5]">A verdict you can trust, because nobody can help.</p>
        <p className="mt-4 max-w-md text-[#eef2f5]/70">Once a proof starts, your agent works alone. Hidden tests decide.</p>
        <ProofTicket className="mt-10 w-full max-w-md [&_figcaption]:text-[#eef2f5]/50" />
      </aside>
    </main>
  )
}
```

Проверить, что у `Button` есть варианты `outline` и `secondary` (`frontend/components/ui/button.tsx`); если какого-то нет — взять ближайший существующий.

- [ ] **Step 3: Условия.** В `frontend/app/terms/page.tsx` строку `'Account: email, a password hash (argon2id), and sessions stored as hashes of their tokens.'` заменить на `'Account: your email and account id at GitHub or Google (and your GitHub login), and sessions stored as hashes of their tokens. We never see or store a password.'`

- [ ] **Step 4: Проверка.**

Run: `cd frontend && pnpm typecheck && pnpm build`
Expected: PASS. Затем вручную: `make up` (из корня, `.env` с `ARENA_DEV_LOGIN=true`), открыть `http://localhost:3000/login` — видна только форма dev-входа (ключей нет), вход ведёт в `/app`; `/login?error=oauth_state` показывает фразу про истёкшую ссылку; ширина 375px без горизонтального скролла (`cd frontend && BASE_URL=http://localhost:3000 pnpm check:mobile`).

- [ ] **Step 5: Commit.**

```bash
git add frontend
git commit -m "Offer GitHub and Google buttons on the sign-in page"
```

---

### Task 5: Документация

**Files:**
- Modify: `README.md`, `backend/README.md`, `docs/how-it-works.md`, `CLAUDE.md`, `docs/superpowers/specs/2026-09-23-agent-connect-and-proof-design.md`

- [ ] **Step 1:** Найти все упоминания старого входа: `grep -rniE 'signup|sign up|/auth/login|password|argon' README.md backend/README.md docs/how-it-works.md CLAUDE.md` и переписать:
  - README и `docs/how-it-works.md` (по-русски): вход через GitHub или Google; локально без ключей — dev-вход по любому e-mail (`ARENA_DEV_LOGIN=true` из `.env.example`); как зарегистрировать OAuth-приложения и какие callback-адреса указать (`{ARENA_PUBLIC_URL}/api/v1/auth/github/callback`, `…/google/callback`); переменные `ARENA_PUBLIC_URL`, `ARENA_GITHUB_CLIENT_ID/SECRET`, `ARENA_GOOGLE_CLIENT_ID/SECRET`; в продакшне `ARENA_DEV_LOGIN` не ставить. Строку «Готовых аккаунтов нет, регистрируйся на `/signup`» заменить на «Войди через GitHub, Google или (локально) dev-вход на `/login`». Упоминания паролей Postgres (`dev_password`) не трогать.
  - `backend/README.md` (по-английски, если он английский — смотреть по файлу): маршруты `/auth/providers`, `/auth/{provider}/start|callback`, `/auth/dev`.
  - `CLAUDE.md`: строка `backend/internal/identity  argon2id passwords, sessions, …` → `GitHub/Google OAuth (state + PKCE), dev login, sessions, RequireSession/RequireAgent, /auth/*, /me`; в «Conventions» или «Architecture» одна фраза: «Owner sign-in is OAuth only (GitHub, Google); `ARENA_DEV_LOGIN=true` adds `POST /auth/dev` for local runs and CI and is refused next to `ARENA_SECURE_COOKIES=true`».
  - Спека среза 1, строка таблицы «Вход»: дописать в конец ячейки «Почему» — « — заменено 25 сентября: `specs/2026-09-25-oauth-login-design.md`».
- [ ] **Step 2:** `grep -rn "auth/signup\|auth/login" --exclude-dir=node_modules --exclude-dir=.next --exclude-dir=.git . | grep -v docs/superpowers` — пусто.
- [ ] **Step 3: Commit.**

```bash
git add README.md backend/README.md docs CLAUDE.md
git commit -m "Document sign-in through GitHub and Google"
```

---

## После реализации

Что остаётся человеку и в код не входит: зарегистрировать GitHub OAuth App и Google OAuth client с callback-адресами `https://tolerance.cc/api/v1/auth/{github,google}/callback` (и `http://localhost:3000/...` для разработки), положить ключи и `ARENA_PUBLIC_URL=https://tolerance.cc` в `/opt/tolerance/.env`, пересоздать базу продакшна (схема `00002` изменилась на месте).
