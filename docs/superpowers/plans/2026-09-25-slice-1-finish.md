# Срез 1: доделки до условия готовности. План реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Закрыть то, что мешает условию готовности среза 1 («незнакомый разработчик по README за 15 минут регистрируется, подключает Claude Code и получает `passed`»): рабочая установка коннектора, недостающие пункты спеки и нативный запуск на любых портах.

**Architecture:** Бэкенд получает три вещи: последнюю проверку в `/me`, read-only `GET /connector/status` и раздачу готовых бинарников коннектора `GET /connector/download`. Составные ответы (агент + проверка) собираются в `cmd/api`, потому что `agents` не может импортировать `proofs`. Слишком большой результат сразу завершает проверку как `failed/diff_too_large`. Коннектор повторяет сетевые сбои и 5xx до дедлайна проверки. Фронт получает экран «до запуска», время шагов на таймлайне и состояние ошибки. CI проверяет ширину 375px настоящим Chrome.

**Tech Stack:** Go 1.26 (`net/http`, pgx), kin-openapi, testcontainers-go, Next.js 16 / React 19, playwright-core 1.63.0 (без скачивания браузеров, `channel: 'chrome'`), GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-09-23-agent-connect-and-proof-design.md` (§4.1 `/me`, §5 коннектор, §8 кабинет, §10 CI). Предыдущий план среза: `docs/superpowers/plans/2026-09-23-agent-connect-and-proof.md`.

## Global Constraints

- Go-модуль остаётся `tolerance`; префикс переменных окружения `ARENA_`; роли Postgres `arena_app` / `arena_migrate`, БД `arena`.
- Все временные метки в JSON в UTC (нормализовать `.UTC()` при сканировании).
- Ошибки только через `httpx.Problem`; тело `{code, message, request_id, fields?}`.
- Каждый ответ в e2e-тесте `cmd/api/main_test.go` валидируется по `contracts/openapi/openapi.yaml`, включая ошибки. Новый маршрут без записи в `openapi.yaml` роняет e2e.
- Интеграционные тесты через `dbtest.New`; при `ARENA_TEST_REQUIRE_DOCKER=1` отсутствие Docker — провал, не skip.
- Никаких моков во фронте; все данные из API. Ширина 375px без горизонтального скролла.
- Diff до 256 KiB, хвост лога до 32 KiB, presence свежая 2 минуты, одна незавершённая проверка на агента.
- Коммиты: английский, повелительное наклонение, без префиксов `feat:`; последняя строка `Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>`.
- Файлы в коммит добавлять только по имени. Никогда не коммитить `.env`, `frontend/AGENTS.md`, `frontend/CLAUDE.md` (их генерирует `next dev`; задача 5 добавляет их в `frontend/.gitignore`).

**Окружение исполнителя (macOS, Docker через Colima):**

- Строгий прогон тестов:
  ```sh
  export DOCKER_HOST=unix://$HOME/.colima/default/docker.sock TESTCONTAINERS_RYUK_DISABLED=true
  cd backend && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./...
  ```
  Без `DOCKER_HOST` testcontainers не находит сокет Colima и в строгом режиме падает с `rootless Docker not found`.
- npm-реестр по умолчанию — корпоративный Artifactory, он недоступен. Новые пакеты ставить с `--registry=https://registry.npmjs.org`.
- Postgres из compose слушает `127.0.0.1:5432` (`docker-compose up -d postgres` из корня; команда `docker compose` здесь не работает, только `docker-compose`).

## Review Focus

1. **`uname` печатает не то, что Go:** `Darwin`/`Linux` с заглавной, `aarch64` на Linux ARM вместо `arm64`, `x86_64` вместо `amd64`. Скачивание должно понимать все варианты. Тест в задаче 4.
2. **Тело результата больше 1 MiB** (огромный diff после JSON-экранирования) — сервер не может его даже прочитать, но проверка всё равно должна сразу стать `failed/diff_too_large`, а не висеть 15 минут до `expired`. E2e-тест в задаче 2.
3. **Соединение рвётся посреди скачивания tarball репозитория** (обрезанное тело при статусе 200) — это сетевой сбой, коннектор повторяет, а не отдаёт агенту битый архив. Тест в задаче 3.
4. **`arena status` при выключенном `arena connect`** не должен делать агента «онлайн» на 2 минуты. E2e в задаче 1: status до первого heartbeat, стадия остаётся `registered`.
5. **Агент есть, проверок ещё нет:** `/me` отдаёт `agent.last_proof: null`, контракт это допускает. E2e в задаче 1.

---

### Task 1: Последняя проверка в `/me` и `GET /connector/status` без heartbeat

**Files:**
- Create: `backend/cmd/api/compose.go`
- Modify: `backend/internal/proofs/service.go` (метод `Latest` после `List`), `backend/internal/agents/service.go` (удалить `MeAgent`), `backend/cmd/api/handler.go`, `backend/contracts/openapi/openapi.yaml`, `backend/cmd/api/main_test.go`
- Test: `backend/internal/proofs/service_integration_test.go`, `backend/cmd/api/main_test.go`

**Interfaces:**
- Produces: `(*proofs.Service).Latest(ctx context.Context, agentID string) (*proofs.Proof, error)` — последняя проверка агента в облегчённом виде (`Diff` и `AgentLogTail` пустые) или `nil, nil`.
- Produces: JSON `/me`: `agent.last_proof` (`Proof` или `null`). JSON `GET /api/v1/connector/status` (API-ключ): `{"agent": {"id","name","stage"}, "last_proof": Proof|null}`; не обновляет presence.

- [ ] **Step 1: Интеграционный тест `Latest`**

Добавить в конец `backend/internal/proofs/service_integration_test.go`:

```go
func TestLatest_NilThenNewestWithoutDiff(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if p, err := f.proofs.Latest(ctx, f.agent); err != nil || p != nil {
		t.Fatalf("an agent without proofs has no latest one: %v %+v", err, p)
	}
	_ = f.agents.Heartbeat(ctx, f.agent, "0.1", "h")
	created, err := f.proofs.Create(ctx, f.userID, "go-fix-retry")
	if err != nil {
		t.Fatal(err)
	}
	claimed, _, err := f.proofs.Claim(ctx, f.agent)
	if err != nil || claimed == nil {
		t.Fatalf("claim: %v %+v", err, claimed)
	}
	if err := f.proofs.SubmitResult(ctx, f.agent, claimed.ID, proofs.ResultInput{Diff: "--- a/x\n+++ b/x\n", LogTail: "log"}); err != nil {
		t.Fatal(err)
	}
	p, err := f.proofs.Latest(ctx, f.agent)
	if err != nil || p == nil || p.ID != created.ID || p.Status != proofs.StatusDiffSubmitted || p.Diff != "" || p.AgentLogTail != "" {
		t.Fatalf("latest must be the newest proof without diff and log: %v %+v", err, p)
	}
}
```

- [ ] **Step 2: Убедиться, что падает**

Run: `cd backend && ARENA_TEST_REQUIRE_DOCKER=1 go test ./internal/proofs/ -run TestLatest -v`
Expected: FAIL при компиляции, `f.proofs.Latest undefined`.

- [ ] **Step 3: `Latest`**

В `backend/internal/proofs/service.go` сразу после функции `List`:

```go
// Latest returns the agent's most recent proof in list form (no diff, no
// log), or nil when it has none. /me and `arena status` show it.
func (s *Service) Latest(ctx context.Context, agentID string) (*Proof, error) {
	var p Proof
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return scanProof(tx.QueryRow(ctx, `SELECT `+proofCols+` FROM proofs WHERE agent_id = $1 ORDER BY created_at DESC LIMIT 1`, agentID), &p)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.Diff, p.AgentLogTail = "", ""
	return &p, nil
}
```

Run: `cd backend && ARENA_TEST_REQUIRE_DOCKER=1 go test ./internal/proofs/ -run TestLatest -v`
Expected: PASS.

- [ ] **Step 4: E2e-тест на `/me` и `/connector/status`**

В `backend/cmd/api/main_test.go` заменить функцию `TestEndToEnd_SignupConnectProve` целиком:

```go
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
		User  identity.User `json:"user"`
		Agent *struct {
			agents.Overview
			LastProof *proofs.Proof `json:"last_proof"`
		} `json:"agent"`
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
	if me.Agent == nil || me.Agent.Stage != agents.StageRegistered || len(me.Agent.APIKeys) != 1 || me.Agent.LastProof != nil {
		t.Fatalf("me after key: %+v", me.Agent)
	}

	// `arena status` before the connector ever connected: it answers, but it
	// is not a heartbeat, so the agent must not look online afterwards.
	var st struct {
		Agent struct {
			Name  string `json:"name"`
			Stage string `json:"stage"`
		} `json:"agent"`
		LastProof *proofs.Proof `json:"last_proof"`
	}
	if code := e.call(t, plain, "GET", "/api/v1/connector/status", keyResp.Key, nil, &st); code != 200 || st.Agent.Name != "fixer-7" || st.Agent.Stage != agents.StageRegistered || st.LastProof != nil {
		t.Fatalf("status before connect: %d %+v", code, st)
	}
	e.call(t, owner, "GET", "/api/v1/me", "", nil, &me)
	if me.Agent.Stage != agents.StageRegistered {
		t.Fatalf("status must not mark the agent online, stage %s", me.Agent.Stage)
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
	if me.Agent.Stage != agents.StageChecking || me.Agent.LastProof == nil || me.Agent.LastProof.ID != proof.ID || me.Agent.LastProof.Status != proofs.StatusQueued {
		t.Fatalf("me while queued: %+v", me.Agent)
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
	raw, _ := io.ReadAll(resp.Body)
	openapi.ValidateResponse(t, e.router, req, resp, raw)
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
	if proof.Status != proofs.StatusPassed || proof.SandboxResult == nil || len(proof.SandboxResult.Tests) != 5 {
		t.Fatalf("after sandbox: %+v", proof)
	}
	e.call(t, owner, "GET", "/api/v1/me", "", nil, &me)
	if me.Agent.Stage != agents.StageOperational || me.Agent.LastProof == nil || me.Agent.LastProof.Status != proofs.StatusPassed || me.Agent.LastProof.Diff != "" {
		t.Fatalf("final me: %+v", me.Agent)
	}
	if code := e.call(t, plain, "GET", "/api/v1/connector/status", keyResp.Key, nil, &st); code != 200 || st.Agent.Stage != agents.StageOperational || st.LastProof == nil || st.LastProof.Status != proofs.StatusPassed {
		t.Fatalf("status after pass: %d %+v", code, st)
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
	if code := e.call(t, plain, "GET", "/api/v1/connector/status", keyResp.Key, nil, nil); code != 401 {
		t.Fatalf("revoked key status: %d", code)
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
}
```

Если после замены `time` больше нигде в файле не используется, убрать импорт `"time"` (прежняя функция держала его строкой `_ = time.Second`).

- [ ] **Step 5: Убедиться, что падает**

Run: `cd backend && ARENA_TEST_REQUIRE_DOCKER=1 go test ./cmd/api/ -run TestEndToEnd_SignupConnectProve -v`
Expected: FAIL: `GET /api/v1/connector/status is not in openapi.yaml` (или `me after key`, если валидация прошла раньше).

- [ ] **Step 6: Сборка ответов в `cmd/api`**

`backend/cmd/api/compose.go`:

```go
package main

import (
	"context"
	"net/http"

	"tolerance/internal/agents"
	"tolerance/internal/identity"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/proofs"
)

// ownerAgent is the agent block of GET /me: the agents overview plus the
// latest proof. It is put together here because agents cannot import proofs
// (proofs already depends on agents for the stage facts).
type ownerAgent struct {
	*agents.Overview
	LastProof *proofs.Proof `json:"last_proof"`
}

// meAgent feeds identity.RegisterMeRoute. It returns an untyped nil when the
// user has no agent, so "agent" encodes as JSON null.
func meAgent(as *agents.Service, ps *proofs.Service) func(context.Context, string) (any, error) {
	return func(ctx context.Context, userID string) (any, error) {
		o, err := as.Overview(ctx, userID)
		if err != nil || o == nil {
			return nil, err
		}
		last, err := ps.Latest(ctx, o.ID)
		if err != nil {
			return nil, err
		}
		return ownerAgent{Overview: o, LastProof: last}, nil
	}
}

// connectorStatus serves GET /connector/status, what `arena status` prints.
// Unlike the heartbeat it does not touch presence: asking never makes the
// agent look online.
func connectorStatus(as *agents.Service, ps *proofs.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentID := identity.MustFromContext(r.Context()).AgentID
		o, err := as.OverviewByID(r.Context(), agentID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		last, err := ps.Latest(r.Context(), agentID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{
			"agent":      map[string]any{"id": o.ID, "name": o.Name, "stage": o.Stage},
			"last_proof": last,
		})
	}
}
```

В `backend/cmd/api/handler.go`:
- строку `identity.RegisterMeRoute(owner, d.users, d.agents.MeAgent)` заменить на `identity.RegisterMeRoute(owner, d.users, meAgent(d.agents, d.proofs))`;
- после `proofs.RegisterConnectorRoutes(connector, d.proofs)` добавить `connector.HandleFunc("GET /api/v1/connector/status", connectorStatus(d.agents, d.proofs))`.

В `backend/internal/agents/service.go` удалить метод `MeAgent` вместе с его комментарием (строки «// MeAgent adapts Overview…» — конец функции); он больше нигде не используется.

- [ ] **Step 7: Контракт**

В `backend/contracts/openapi/openapi.yaml`:

- `info.version` поменять на `"0.4.0"`.
- В `components.schemas.AgentOverview` второй элемент `allOf` заменить на:

```yaml
        - type: object
          required: [stage, presence, last_proof]
          properties:
            stage: { type: string, enum: [registered, offline, connected, checking, operational, check_failed] }
            presence:
              nullable: true
              allOf: [{ $ref: "#/components/schemas/Presence" }]
            last_proof:
              description: The latest proof in list form (diff and agent_log_tail empty), or null.
              nullable: true
              allOf: [{ $ref: "#/components/schemas/Proof" }]
```

- После блока `/connector/heartbeat` (перед `/connector/tasks/next`) добавить:

```yaml
  /connector/status:
    get:
      operationId: connectorStatus
      description: What `arena status` shows. Unlike the heartbeat, it does not mark the agent online.
      security: [{ apiKeyAuth: [] }]
      responses:
        "200":
          description: OK
          content:
            application/json:
              schema:
                type: object
                required: [agent, last_proof]
                properties:
                  agent:
                    type: object
                    required: [id, name, stage]
                    properties:
                      id: { type: string }
                      name: { type: string }
                      stage: { type: string, enum: [registered, offline, connected, checking, operational, check_failed] }
                  last_proof:
                    nullable: true
                    allOf: [{ $ref: "#/components/schemas/Proof" }]
        "401": { $ref: "#/components/responses/Problem" }
```

- [ ] **Step 8: Проверка и коммит**

```bash
cd backend && gofmt -l . && go vet ./... && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./...
```

Expected: `gofmt -l .` пусто, всё PASS.

```bash
git add backend/cmd/api/compose.go backend/cmd/api/handler.go backend/cmd/api/main_test.go backend/internal/proofs/service.go backend/internal/proofs/service_integration_test.go backend/internal/agents/service.go backend/contracts/openapi/openapi.yaml
git commit -m "Return the latest proof in /me and add a read-only connector status

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Слишком большой результат сразу завершает проверку

**Files:**
- Modify: `backend/internal/proofs/model.go`, `backend/internal/proofs/service.go` (`SubmitResult`, новый `FailOversized`), `backend/internal/proofs/http_connector.go` (обработчик результата)
- Test: `backend/internal/proofs/connector_integration_test.go`, `backend/cmd/api/main_test.go`

**Interfaces:**
- Produces: `(*proofs.Service).FailOversized(ctx context.Context, agentID, proofID string) error` — переводит проверку в `claimed`/`running_agent` в `failed` с причиной `diff_too_large`, остальные не трогает.
- Produces: причина провала `diff_too_large` (фронт показывает её в задаче 5). Ответ `413 diff_too_large` на `POST /connector/proofs/{id}/result`; коннектор (задача 3) считает любой 4xx, кроме 409, окончательным.

- [ ] **Step 1: Интеграционный тест**

Добавить в конец `backend/internal/proofs/connector_integration_test.go`:

```go
func TestSubmitResult_OversizedDiffFailsTheProof(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_ = f.agents.Heartbeat(ctx, f.agent, "0.1", "h")
	if _, err := f.proofs.Create(ctx, f.userID, "go-fix-retry"); err != nil {
		t.Fatal(err)
	}
	p, _, err := f.proofs.Claim(ctx, f.agent)
	if err != nil || p == nil {
		t.Fatalf("claim: %v %+v", err, p)
	}
	err = f.proofs.SubmitResult(ctx, f.agent, p.ID, proofs.ResultInput{Diff: strings.Repeat("x", 256<<10+1)})
	problem(t, err, 413, "diff_too_large")
	got, _ := f.proofs.Get(ctx, f.userID, p.ID)
	if got.Status != proofs.StatusFailed || got.FailureReason != "diff_too_large" || got.FinishedAt == nil || got.DiffSubmittedAt != nil {
		t.Fatalf("after an oversized diff: %+v", got)
	}
	var jobs int
	_ = f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE payload->>'proof_id' = $1`, p.ID).Scan(&jobs)
	})
	if jobs != 0 {
		t.Fatalf("an oversized diff must not queue a sandbox run, got %d jobs", jobs)
	}
	// A proof that is no longer running is left alone.
	if err := f.proofs.FailOversized(ctx, f.agent, p.ID); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Убедиться, что падает**

Run: `cd backend && ARENA_TEST_REQUIRE_DOCKER=1 go test ./internal/proofs/ -run TestSubmitResult_Oversized -v`
Expected: FAIL при компиляции, `f.proofs.FailOversized undefined`.

- [ ] **Step 3: Сервис**

В `backend/internal/proofs/model.go` в блок `const` после `maxLogTailBytes` добавить:

```go
	// maxResultBodyBytes bounds the result request: a diff at the 256 KiB
	// limit plus JSON escaping and a 32 KiB log tail fit well inside it.
	maxResultBodyBytes = 1 << 20
```

В `backend/internal/proofs/service.go` заменить начало `SubmitResult`:

```go
func (s *Service) SubmitResult(ctx context.Context, agentID, proofID string, in ResultInput) error {
	if len(in.Diff) > maxDiffBytes {
		return httpx.New(http.StatusRequestEntityTooLarge, "diff_too_large", "Diff exceeds 256 KiB")
	}
```

на

```go
func (s *Service) SubmitResult(ctx context.Context, agentID, proofID string, in ResultInput) error {
	if len(in.Diff) > maxDiffBytes {
		if err := s.FailOversized(ctx, agentID, proofID); err != nil {
			return err
		}
		return errDiffTooLarge
	}
```

и сразу перед комментарием `// SubmitResult stores…` добавить:

```go
var errDiffTooLarge = httpx.New(http.StatusRequestEntityTooLarge, "diff_too_large", "Diff exceeds 256 KiB")

// FailOversized ends a running proof whose result is too big to accept, so
// the owner sees diff_too_large at once instead of an expiry after the agent
// timeout. A proof that is not running is left as it is.
func (s *Service) FailOversized(ctx context.Context, agentID, proofID string) error {
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE proofs SET status = 'failed', failure_reason = 'diff_too_large', finished_at = now()
			WHERE id = $1 AND agent_id = $2 AND status IN ('claimed', 'running_agent')`, proofID, agentID)
		return err
	})
}
```

Run: `cd backend && ARENA_TEST_REQUIRE_DOCKER=1 go test ./internal/proofs/ -run 'TestSubmitResult_Oversized|TestConnectorFlow' -v`
Expected: PASS (старый `TestConnectorFlow_RepoStartedResult` тоже: там большой diff отправляется уже после результата и по-прежнему отклоняется).

- [ ] **Step 4: E2e через HTTP — тело больше 1 MiB**

Добавить в конец `backend/cmd/api/main_test.go` (и `"strings"` в импорты):

```go
func TestEndToEnd_OversizedResultFailsTheProof(t *testing.T) {
	e := newE2E(t)
	owner := e.browser(t)
	plain := &http.Client{}
	e.call(t, owner, "POST", "/api/v1/auth/signup", "", map[string]string{"email": "big@example.com", "password": "longenough1"}, nil)
	e.call(t, owner, "POST", "/api/v1/agent", "", map[string]string{"name": "big-diff"}, nil)
	var key struct {
		Key string `json:"key"`
	}
	e.call(t, owner, "POST", "/api/v1/agent/keys", "", map[string]string{"name": "k"}, &key)
	e.call(t, plain, "POST", "/api/v1/connector/heartbeat", key.Key, map[string]string{"connector_version": "0.1.0", "hostname": "h"}, nil)
	var proof proofs.Proof
	if code := e.call(t, owner, "POST", "/api/v1/proofs", "", map[string]string{"task_slug": "go-fix-retry"}, &proof); code != 201 {
		t.Fatalf("create proof: %d", code)
	}
	if code := e.call(t, plain, "GET", "/api/v1/connector/tasks/next?wait=1", key.Key, nil, nil); code != 200 {
		t.Fatalf("next: %d", code)
	}
	// Over the 1 MiB body limit: the server cannot even decode it, and must
	// still end the proof instead of letting it expire.
	res := map[string]any{"diff": strings.Repeat("+x\n", 400_000), "log_tail": "", "duration_ms": 1, "exit_code": 0}
	if code := e.call(t, plain, "POST", "/api/v1/connector/proofs/"+proof.ID+"/result", key.Key, res, nil); code != 413 {
		t.Fatalf("oversized result: %d", code)
	}
	e.call(t, owner, "GET", "/api/v1/proofs/"+proof.ID, "", nil, &proof)
	if proof.Status != proofs.StatusFailed || proof.FailureReason != "diff_too_large" || proof.FinishedAt == nil {
		t.Fatalf("after an oversized result: %+v", proof)
	}
}
```

Run: `cd backend && ARENA_TEST_REQUIRE_DOCKER=1 go test ./cmd/api/ -run TestEndToEnd_Oversized -v`
Expected: FAIL: проверка остаётся в `claimed`, `after an oversized result`.

- [ ] **Step 5: Обработчик результата**

В `backend/internal/proofs/http_connector.go` заменить обработчик `POST /api/v1/connector/proofs/{id}/result` целиком:

```go
	mux.HandleFunc("POST /api/v1/connector/proofs/{id}/result", func(w http.ResponseWriter, r *http.Request) {
		agentID, proofID := identity.MustFromContext(r.Context()).AgentID, r.PathValue("id")
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxResultBodyBytes))
		var tooBig *http.MaxBytesError
		switch {
		case errors.As(err, &tooBig):
			// Too big to even read: the verdict is known without the body.
			if err := s.FailOversized(r.Context(), agentID, proofID); err != nil {
				httpx.WriteError(w, r, err)
				return
			}
			httpx.WriteError(w, r, errDiffTooLarge)
			return
		case err != nil:
			httpx.WriteError(w, r, httpx.InvalidBody("could not read the request body"))
			return
		}
		var in ResultInput
		if err := httpx.Decode(raw, &in); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		if err := s.SubmitResult(r.Context(), agentID, proofID, in); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
```

В импорты файла добавить `"errors"` и `"io"`.

- [ ] **Step 6: Проверка и коммит**

```bash
cd backend && gofmt -l . && go vet ./... && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./...
git add backend/internal/proofs/model.go backend/internal/proofs/service.go backend/internal/proofs/http_connector.go backend/internal/proofs/connector_integration_test.go backend/cmd/api/main_test.go
git commit -m "Fail a proof as diff_too_large as soon as its result is too big

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Коннектор повторяет сбои сети, `arena status` без heartbeat

**Files:**
- Modify: `backend/cmd/arena/client.go`, `backend/cmd/arena/main.go`
- Test: `backend/cmd/arena/client_test.go`

**Interfaces:**
- Consumes: `GET /api/v1/connector/status` из задачи 1; `413 diff_too_large` из задачи 2.
- Produces: `withRetry(ctx, fn) error`, `retryable(err) bool`, переменная `retryBase` (тесты её укорачивают); `(*client).Status(ctx) (statusResp, error)`; `formatStatus(st statusResp, now time.Time) string`.

- [ ] **Step 1: Тесты клиента**

В `backend/cmd/arena/client_test.go`:
- в импорты добавить `"errors"`;
- в начало `TestClient_ResultRetriesAndTreats409AsDone` вставить `shortRetries(t)`;
- в конец файла добавить:

```go
func shortRetries(t *testing.T) {
	t.Helper()
	retryBase = 10 * time.Millisecond
	t.Cleanup(func() { retryBase = 2 * time.Second })
}

func TestClient_RepoRetriesDroppedConnectionsAndServerErrors(t *testing.T) {
	shortRetries(t)
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch atomic.AddInt32(&calls, 1) {
		case 1: // the connection drops halfway through a 200 body
			conn, buf, _ := w.(http.Hijacker).Hijack()
			buf.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\npartial")
			buf.Flush()
			conn.Close()
		case 2: // the server has a bad moment
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			_, _ = w.Write([]byte("tarball"))
		}
	}))
	defer srv.Close()
	c := &client{base: srv.URL, key: "ak_test", http: srv.Client()}
	got, err := c.Repo(context.Background(), "p1")
	if err != nil || string(got) != "tarball" || atomic.LoadInt32(&calls) != 3 {
		t.Fatalf("repo: err=%v body=%q calls=%d", err, got, calls)
	}
}

func TestClient_RepoGivesUpOnClientErrors(t *testing.T) {
	shortRetries(t)
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "not_found", "message": "gone"})
	}))
	defer srv.Close()
	c := &client{base: srv.URL, key: "ak_test", http: srv.Client()}
	_, err := c.Repo(context.Background(), "p1")
	var ae *apiError
	if !errors.As(err, &ae) || ae.Status != 404 || atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("a 404 must end the download at once: err=%v calls=%d", err, calls)
	}
}

func TestClient_ResultRetriesServerErrorsUntilTheDeadline(t *testing.T) {
	shortRetries(t)
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	c := &client{base: srv.URL, key: "ak_test", http: srv.Client()}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if err := c.Result(ctx, "p1", result{}); err == nil || atomic.LoadInt32(&calls) < 2 {
		t.Fatalf("expected retries and then an error at the deadline: err=%v calls=%d", err, calls)
	}
}

func TestClient_ResultTooLargeIsFinal(t *testing.T) {
	shortRetries(t)
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "diff_too_large", "message": "Diff exceeds 256 KiB"})
	}))
	defer srv.Close()
	c := &client{base: srv.URL, key: "ak_test", http: srv.Client()}
	err := c.Result(context.Background(), "p1", result{})
	var ae *apiError
	if !errors.As(err, &ae) || ae.Code != "diff_too_large" || atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("a 413 must not be retried: err=%v calls=%d", err, calls)
	}
}

func TestClient_StatusDoesNotHeartbeat(t *testing.T) {
	var heartbeats int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/connector/heartbeat":
			atomic.AddInt32(&heartbeats, 1)
		case "/api/v1/connector/status":
			_, _ = w.Write([]byte(`{"agent":{"id":"a1","name":"fixer","stage":"check_failed"},
				"last_proof":{"task_slug":"go-fix-retry","status":"failed","failure_reason":"tests_failed","created_at":"2026-09-25T10:00:00Z"}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := &client{base: srv.URL, key: "ak_test", http: srv.Client()}
	st, err := c.Status(context.Background())
	if err != nil || atomic.LoadInt32(&heartbeats) != 0 {
		t.Fatalf("status: err=%v heartbeats=%d", err, heartbeats)
	}
	now := time.Date(2026, 9, 25, 10, 3, 0, 0, time.UTC)
	if got, want := formatStatus(st, now), "fixer: check_failed\nlast proof: go-fix-retry failed (tests_failed), started 3m0s ago\n"; got != want {
		t.Fatalf("formatStatus:\n got %q\nwant %q", got, want)
	}
	st.LastProof = nil
	if got, want := formatStatus(st, now), "fixer: check_failed\nlast proof: none yet\n"; got != want {
		t.Fatalf("formatStatus without proofs:\n got %q\nwant %q", got, want)
	}
}
```

- [ ] **Step 2: Убедиться, что падает**

Run: `cd backend && go test ./cmd/arena/ -run TestClient -v`
Expected: FAIL при компиляции: `undefined: retryBase`, `c.Status undefined`, `undefined: formatStatus`.

- [ ] **Step 3: Повторы и статус в клиенте**

В `backend/cmd/arena/client.go`:

1. В `do` заменить строку `raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))` на:

```go
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		// The body was cut off (e.g. the connection dropped): as much a
		// network failure as no answer at all, and just as retryable.
		return err
	}
```

2. Заменить функции `Repo` и `Result` на:

```go
// Repo downloads the task's repository, retrying like Result.
func (c *client) Repo(ctx context.Context, proofID string) ([]byte, error) {
	var raw []byte
	err := withRetry(ctx, func() error {
		return c.do(ctx, http.MethodGet, "/api/v1/connector/proofs/"+proofID+"/repo.tar.gz", nil, &raw)
	})
	return raw, err
}

// Result sends the agent's result. Network failures and server errors are
// retried until ctx ends; a 409 means the server already has it; any other
// 4xx is final (413 diff_too_large has already ended the proof).
func (c *client) Result(ctx context.Context, proofID string, r result) error {
	return withRetry(ctx, func() error {
		err := c.do(ctx, http.MethodPost, "/api/v1/connector/proofs/"+proofID+"/result", r, nil)
		var ae *apiError
		if errors.As(err, &ae) && ae.Status == http.StatusConflict {
			return nil
		}
		return err
	})
}

type statusResp struct {
	Agent struct {
		Name  string `json:"name"`
		Stage string `json:"stage"`
	} `json:"agent"`
	LastProof *struct {
		TaskSlug      string    `json:"task_slug"`
		Status        string    `json:"status"`
		FailureReason string    `json:"failure_reason"`
		CreatedAt     time.Time `json:"created_at"`
	} `json:"last_proof"`
}

// Status asks for the agent's stage and latest proof. It is not a
// heartbeat: running `arena status` never makes the agent look online.
func (c *client) Status(ctx context.Context) (statusResp, error) {
	var out statusResp
	err := c.do(ctx, http.MethodGet, "/api/v1/connector/status", nil, &out)
	return out, err
}

// retryBase is the first pause between attempts; tests shorten it.
var retryBase = 2 * time.Second

const retryCap = 30 * time.Second

// retryable reports whether another attempt could succeed: no usable answer
// came back, or the server had a temporary problem. Any other API error (a
// 4xx) is final.
func retryable(err error) bool {
	var ae *apiError
	if errors.As(err, &ae) {
		return ae.Status >= 500 || ae.Status == http.StatusTooManyRequests
	}
	return true
}

// withRetry runs fn until it succeeds, fails for good, or ctx ends. The pause
// doubles from retryBase up to retryCap.
func withRetry(ctx context.Context, fn func() error) error {
	delay := retryBase
	for {
		err := fn()
		if err == nil || ctx.Err() != nil || !retryable(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(delay):
		}
		delay = min(delay*2, retryCap)
	}
}
```

Run: `cd backend && go test ./cmd/arena/ -run TestClient -v`
Expected: всё, кроме `TestClient_StatusDoesNotHeartbeat`, PASS; он падает на `undefined: formatStatus`.

- [ ] **Step 4: `arena status` и дедлайн задачи в `main.go`**

В `backend/cmd/arena/main.go`:

1. Функцию `cmdStatus` заменить на:

```go
func cmdStatus() error {
	c, _, err := newClient()
	if err != nil {
		return err
	}
	st, err := c.Status(context.Background())
	if err != nil {
		return err
	}
	fmt.Print(formatStatus(st, time.Now()))
	return nil
}

// formatStatus renders what `arena status` prints.
func formatStatus(st statusResp, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s\n", st.Agent.Name, st.Agent.Stage)
	p := st.LastProof
	if p == nil {
		b.WriteString("last proof: none yet\n")
		return b.String()
	}
	fmt.Fprintf(&b, "last proof: %s %s", p.TaskSlug, p.Status)
	if p.FailureReason != "" {
		fmt.Fprintf(&b, " (%s)", p.FailureReason)
	}
	fmt.Fprintf(&b, ", started %s ago\n", now.Sub(p.CreatedAt).Round(time.Second))
	return b.String()
}
```

2. В `usage()` строку `  status   show the agent's stage` заменить на `  status   show the agent's stage and latest proof (not a heartbeat)`.

3. В `cmdConnect` заменить всё от `fmt.Printf("Task %s (%s): running your agent, up to %ds\n", ...)` до конца тела цикла на:

```go
		fmt.Printf("Task %s (%s): running your agent, up to %ds\n", task.Task.Slug, task.ProofID, task.Task.AgentTimeoutS)
		// The server gives up on this proof agent_timeout_s + 60s after the
		// claim, so nothing is worth retrying past that.
		taskCtx, cancel := context.WithTimeout(ctx, time.Duration(task.Task.AgentTimeoutS+60)*time.Second)
		repo, err := c.Repo(taskCtx, task.ProofID)
		if err != nil {
			cancel()
			fmt.Fprintln(os.Stderr, "download repo:", err)
			continue
		}
		_ = c.Started(taskCtx, task.ProofID)
		res, err := runTask(taskCtx, *task, repo, cfg.Agent.Command)
		if err != nil {
			fmt.Fprintln(os.Stderr, "run:", err)
			res = result{LogTail: "connector error: " + err.Error(), ExitCode: -1}
		}
		err = c.Result(taskCtx, task.ProofID, res)
		cancel()
		if err != nil {
			fmt.Fprintln(os.Stderr, "send result:", err)
			continue
		}
		fmt.Printf("Result sent (exit %d, %d bytes of diff). Check the dashboard for the verdict.\n", res.ExitCode, len(res.Diff))
	}
	return nil
}
```

`strings` и `time` в `main.go` уже импортированы (проверить; если нет — добавить).

- [ ] **Step 5: Проверка и коммит**

```bash
cd backend && gofmt -l . && go vet ./... && go test -race ./cmd/arena/
git add backend/cmd/arena/client.go backend/cmd/arena/main.go backend/cmd/arena/client_test.go
git commit -m "Retry the connector's downloads and results, and stop status from heartbeating

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Готовые бинарники коннектора и порты в Makefile

**Files:**
- Create: `backend/cmd/api/download.go`, `backend/cmd/api/download_test.go`
- Modify: `backend/cmd/api/config.go`, `backend/cmd/api/handler.go`, `backend/contracts/openapi/openapi.yaml`, `backend/Dockerfile`, `Makefile`, `.gitignore`

**Interfaces:**
- Produces: `GET /api/v1/connector/download?os=<uname -s>&arch=<uname -m>` без авторизации → `200 application/octet-stream` или `404` c `unsupported_platform` / `connector_unavailable`. Файлы `arena-<goos>-<goarch>` в `ARENA_CONNECTOR_DIR` (в образе `/opt/arena/connector`, нативно `backend/.connector` через `make connector`).
- Produces: `make run-api` слушает `127.0.0.1:$(API_PORT)`, `make run-web` — порт `$(WEB_PORT)`. Задача 6 использует адрес скачивания на странице подключения.

- [ ] **Step 1: Тест раздачи**

`backend/cmd/api/download_test.go`:

```go
package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"tolerance/contracts/openapi"
)

func TestConnectorDownload(t *testing.T) {
	router, err := openapi.Router()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "arena-darwin-arm64"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	serve := func(dir string) *httptest.Server {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /api/v1/connector/download", connectorDownload(dir))
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)
		return srv
	}
	get := func(srv *httptest.Server, query string) (int, string, string) {
		t.Helper()
		req, _ := http.NewRequest("GET", srv.URL+"/api/v1/connector/download?"+query, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		openapi.ValidateResponse(t, router, req, resp, body)
		if resp.StatusCode != 200 {
			var p struct{ Code string }
			_ = json.Unmarshal(body, &p)
			return resp.StatusCode, p.Code, ""
		}
		return resp.StatusCode, string(body), resp.Header.Get("Content-Type")
	}

	srv := serve(dir)
	// What macOS on Apple Silicon prints for `uname -s` and `uname -m`.
	if code, body, ct := get(srv, "os=Darwin&arch=arm64"); code != 200 || body != "binary" || ct != "application/octet-stream" {
		t.Fatalf("darwin/arm64: %d %q %q", code, body, ct)
	}
	// Linux on ARM prints aarch64; that file is not built here.
	if code, reason, _ := get(srv, "os=Linux&arch=aarch64"); code != 404 || reason != "connector_unavailable" {
		t.Fatalf("linux/aarch64 without a file: %d %s", code, reason)
	}
	for _, q := range []string{"os=Windows_NT&arch=x86_64", "os=Darwin&arch=ppc", ""} {
		if code, reason, _ := get(srv, q); code != 404 || reason != "unsupported_platform" {
			t.Fatalf("%q: %d %s", q, code, reason)
		}
	}
	// A server with no prebuilt connectors says so instead of guessing a path.
	if code, reason, _ := get(serve(""), "os=Darwin&arch=arm64"); code != 404 || reason != "connector_unavailable" {
		t.Fatalf("no connector dir: %d %s", code, reason)
	}
}
```

Run: `cd backend && go test ./cmd/api/ -run TestConnectorDownload -v`
Expected: FAIL при компиляции, `undefined: connectorDownload`.

- [ ] **Step 2: Обработчик**

`backend/cmd/api/download.go`:

```go
package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"tolerance/internal/platform/httpx"
)

// What `uname -s` and `uname -m` print, mapped to the Go targets the
// connector is built for. It runs agents through `sh -c`, so there is no
// Windows build.
var (
	connectorOS   = map[string]string{"darwin": "darwin", "linux": "linux"}
	connectorArch = map[string]string{"x86_64": "amd64", "amd64": "amd64", "arm64": "arm64", "aarch64": "arm64"}
)

var errConnectorUnavailable = httpx.New(http.StatusNotFound, "connector_unavailable",
	"This server has no prebuilt connector for that platform; build it from the repository: cd backend && go build -o arena ./cmd/arena")

// connectorDownload serves the prebuilt connector for ?os=&arch=. dir holds
// arena-<goos>-<goarch> files (the api image builds them); an empty dir means
// this server has none, e.g. a native run without `make connector`.
func connectorDownload(dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		goos := connectorOS[strings.ToLower(r.URL.Query().Get("os"))]
		goarch := connectorArch[strings.ToLower(r.URL.Query().Get("arch"))]
		if goos == "" || goarch == "" {
			httpx.WriteError(w, r, httpx.New(http.StatusNotFound, "unsupported_platform",
				"The connector is built for macOS and Linux on amd64 and arm64; pass os=$(uname -s)&arch=$(uname -m)"))
			return
		}
		if dir == "" {
			httpx.WriteError(w, r, errConnectorUnavailable)
			return
		}
		f, err := os.Open(filepath.Join(dir, "arena-"+goos+"-"+goarch))
		if err != nil {
			httpx.WriteError(w, r, errConnectorUnavailable)
			return
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="arena"`)
		http.ServeContent(w, r, "arena", st.ModTime(), f)
	}
}
```

В `backend/cmd/api/config.go`: в структуру `config` добавить поле `connectorDir string // prebuilt connector binaries; "" = none`, а в `loadConfig` в литерал `cfg` — `connectorDir: os.Getenv("ARENA_CONNECTOR_DIR"),`.

В `backend/cmd/api/handler.go` после `identity.RegisterAuthRoutes(api, d.users, d.limiter, cfg.secureCookies)` добавить:

```go
	// Public: the owner downloads the connector before having it set up.
	// The exact GET pattern wins over the key-protected /api/v1/connector/ prefix.
	api.HandleFunc("GET /api/v1/connector/download", connectorDownload(cfg.connectorDir))
```

- [ ] **Step 3: Контракт**

В `backend/contracts/openapi/openapi.yaml` перед `/connector/heartbeat` добавить:

```yaml
  /connector/download:
    get:
      operationId: downloadConnector
      description: The prebuilt connector binary. Pass what `uname -s` and `uname -m` print.
      security: []
      parameters:
        - { name: os, in: query, required: true, schema: { type: string } }
        - { name: arch, in: query, required: true, schema: { type: string } }
      responses:
        "200":
          description: The binary
          content:
            application/octet-stream:
              schema: { type: string, format: binary }
        "404": { $ref: "#/components/responses/Problem" }
```

Run: `cd backend && go test ./cmd/api/ -run TestConnectorDownload -v && go test ./contracts/...`
Expected: PASS.

- [ ] **Step 4: Сборка бинарников в образе API**

В `backend/Dockerfile`:

1. Шаг сборки заменить на:

```dockerfile
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -o /out/api ./cmd/api \
 && CGO_ENABLED=0 go build -trimpath -o /out/migrate ./cmd/migrate \
 && for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64; do \
      CGO_ENABLED=0 GOOS=${target%/*} GOARCH=${target#*/} go build -trimpath \
        -o /out/connector/arena-${target%/*}-${target#*/} ./cmd/arena || exit 1; \
    done
```

2. В финальной стадии после `COPY fixtures/proofs /opt/arena/proofs` добавить `COPY --from=build /out/connector /opt/arena/connector`, а строку `ENV ARENA_PROOFS_DIR=/opt/arena/proofs` заменить на `ENV ARENA_PROOFS_DIR=/opt/arena/proofs ARENA_CONNECTOR_DIR=/opt/arena/connector`.

(Без `-ldflags=-s`: компоновщик Go сам ставит ad-hoc подпись на darwin/arm64, и её не нужно ставить под риск.)

- [ ] **Step 5: Makefile и .gitignore**

В корневом `Makefile` (строки рецептов начинаются с табуляции):

1. В `.PHONY` добавить `connector`.
2. После блока переменных (после строки `DOCKER_GID ?= …`) добавить:

```makefile
# Prebuilt connectors for GET /api/v1/connector/download in native runs; the
# api image builds its own.
CONNECTOR_DIR := $(CURDIR)/backend/.connector
```

3. Цели `run-api` и `run-web` заменить на:

```makefile
run-api:
	cd backend && ARENA_ADDR=127.0.0.1:$(API_PORT) ARENA_CONNECTOR_DIR=$(CONNECTOR_DIR) \
		ARENA_APP_DATABASE_URL="postgres://arena_app:$(ARENA_APP_ROLE_PASSWORD)@127.0.0.1:5432/arena?sslmode=disable" go run ./cmd/api

run-web:
	cd frontend && API_URL=http://127.0.0.1:$(API_PORT) pnpm dev -p $(WEB_PORT)

connector:
	cd backend && for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64; do \
		CGO_ENABLED=0 GOOS=$${target%/*} GOARCH=$${target#*/} go build -trimpath \
			-o $(CONNECTOR_DIR)/arena-$${target%/*}-$${target#*/} ./cmd/arena || exit 1; \
	done
```

В корневой `.gitignore` добавить строку `backend/.connector/`.

- [ ] **Step 6: Проверка вживую**

```bash
make connector && ls -l backend/.connector
./backend/.connector/arena-darwin-arm64 2>&1 | head -1        # usage: arena <login|init|connect|status>
cd backend && gofmt -l . && go vet ./... && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./... && cd ..
```

Нативный API на порту из `.env` (Postgres из compose должен быть запущен, `.env` — из `.env.example`):

```bash
set -a; . ./.env; set +a
make run-api > /tmp/slice1-api.log 2>&1 &
sleep 3
curl -s -o /tmp/arena-dl -w "%{http_code} %{content_type}\n" "http://127.0.0.1:$API_PORT/api/v1/connector/download?os=$(uname -s)&arch=$(uname -m)"
chmod +x /tmp/arena-dl && /tmp/arena-dl 2>&1 | head -1
kill %1
```

Expected: `200 application/octet-stream`, затем строка usage. Если порт `API_PORT` уже занят другим процессом, взять свободный (`API_PORT=3031 make run-api`).

Сборка образа (если Docker может скачать модули Go; при сетевой ошибке — записать это в отчёт, не обходить):

```bash
docker build -q -t arena-api-check ./backend
id=$(docker create arena-api-check) && docker cp $id:/opt/arena/connector /tmp/connector-check && docker rm $id
ls /tmp/connector-check && /tmp/connector-check/arena-darwin-arm64 2>&1 | head -1
rm -rf /tmp/connector-check && docker rmi arena-api-check
```

Expected: четыре файла; darwin/arm64, собранный в Linux, запускается на этом Mac и печатает usage (значит, подпись на месте).

- [ ] **Step 7: Коммит**

```bash
git add backend/cmd/api/download.go backend/cmd/api/download_test.go backend/cmd/api/config.go backend/cmd/api/handler.go backend/contracts/openapi/openapi.yaml backend/Dockerfile Makefile .gitignore
git commit -m "Serve prebuilt connector binaries and make native runs honor the ports in .env

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Фронт: экран перед запуском, стадия по `last_proof`, ошибка в кабинете

**Files:**
- Create: `frontend/app/app/proofs/new/page.tsx`
- Modify: `frontend/lib/types.ts`, `frontend/lib/format.ts`, `frontend/app/app/layout.tsx`, `frontend/components/stage-card.tsx`, `frontend/app/app/page.tsx`, `frontend/.gitignore`

**Interfaces:**
- Consumes: `/me` → `agent.last_proof` (задача 1); причина `diff_too_large` (задача 2); `GET /proof-tasks`.
- Produces: страница `/app/proofs/new` (единственное место, где запускается проверка); `StageCard({ me }: { me: Me })` без `onStart`.

- [ ] **Step 1: Типы, подписи, игнор сгенерированных файлов**

`frontend/lib/types.ts`: в `AgentOverview` вторую строку заменить на `stage: Stage; presence: Presence | null; last_proof: Proof | null`.

`frontend/lib/format.ts`: в `REASON_LABEL` добавить `diff_too_large: 'The diff was larger than 256 KiB, so it was not checked.',`.

`frontend/.gitignore`: добавить в конец

```
# Written by `next dev`; the repo keeps its own agent instructions at the root.
/AGENTS.md
/CLAUDE.md
```

- [ ] **Step 2: Ошибка в кабинете**

`frontend/app/app/layout.tsx` целиком:

```tsx
'use client'

import { useEffect } from 'react'
import { useRouter } from 'next/navigation'
import { AppShell } from '@/components/app-shell'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useMe } from '@/lib/use-me'

export default function OwnerLayout({ children }: { children: React.ReactNode }) {
  const { me, loading, error, refresh } = useMe()
  const router = useRouter()
  useEffect(() => {
    if (!loading && (error?.status === 401 || (!me && !error))) router.replace('/login')
  }, [me, loading, error, router])
  if (!loading && error && error.status !== 401) {
    return (
      <div className="p-6">
        <p role="alert" className="text-sm text-destructive">Could not load your account: {error.message}</p>
        <p className="mt-1 text-sm text-muted-foreground">The API may be down or restarting.</p>
        <Button variant="outline" size="sm" className="mt-3" onClick={() => void refresh()}>Try again</Button>
      </div>
    )
  }
  if (loading || !me) return <div className="p-6"><Skeleton className="h-8 w-48" /></div>
  return <AppShell me={me}>{children}</AppShell>
}
```

- [ ] **Step 3: Карточка стадии и главная**

`frontend/components/stage-card.tsx` целиком:

```tsx
import type { ReactElement } from 'react'
import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import type { Me } from '@/lib/types'
import { ago } from '@/lib/format'

// Every proof starts from /app/proofs/new, which shows the task first.
export function StageCard({ me }: { me: Me }): ReactElement {
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
  const last = a.last_proof
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
          {last && <Button render={<Link href={`/app/proofs/${last.id}`} />} nativeButton={false} className="mt-4">Watch</Button>}
        </Card>
      )
    case 'check_failed':
      return (
        <Card className="p-6">
          <h2 className="text-lg font-semibold">Not verified yet</h2>
          <p className="mt-1 text-sm text-muted-foreground">The last proof failed. Read the breakdown, improve the agent, try again.</p>
          <div className="mt-4 flex flex-wrap gap-2">
            {last && <Button variant="outline" render={<Link href={`/app/proofs/${last.id}`} />} nativeButton={false}>See why</Button>}
            <Button render={<Link href="/app/proofs/new" />} nativeButton={false}>Run proof again</Button>
          </div>
        </Card>
      )
    case 'operational':
      return (
        <Card className="p-6">
          <h2 className="text-lg font-semibold">{a.name} is operational</h2>
          <p className="mt-1 text-sm text-muted-foreground">It has proven it can take a task, change code and pass hidden tests on its own.</p>
          <Button variant="outline" render={<Link href="/app/proofs/new" />} nativeButton={false} className="mt-4">Run the proof again</Button>
        </Card>
      )
    case 'connected':
      return (
        <Card className="p-6">
          <h2 className="text-lg font-semibold">{a.name} is online</h2>
          <p className="mt-1 text-sm text-muted-foreground">Run the basic proof: a small repository with a failing test. Your agent works alone; the platform runs hidden tests on its diff.</p>
          <Button render={<Link href="/app/proofs/new" />} nativeButton={false} className="mt-4">Run basic proof</Button>
        </Card>
      )
  }
}
```

(`switch` без `default` и с явным типом возврата: новая стадия в `Stage` станет ошибкой `tsc`, а не тихой карточкой «online».)

`frontend/app/app/page.tsx` целиком:

```tsx
'use client'

import { useCallback, useEffect, useState } from 'react'
import { StageCard } from '@/components/stage-card'
import { ProofList } from '@/components/proof-list'
import { api, ApiError } from '@/lib/api'
import { useMe } from '@/lib/use-me'
import type { Proof } from '@/lib/types'

export default function HomePage() {
  const { me } = useMe(5000)
  const [proofs, setProofs] = useState<Proof[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const hasAgent = me?.agent != null
  const last = me?.agent?.last_proof
  // /me is polled; reload the history when the latest proof appears or moves.
  const lastKey = last ? `${last.id}:${last.status}` : ''

  const load = useCallback(async () => {
    if (!hasAgent) return setProofs([])
    try {
      setProofs((await api<{ items: Proof[] }>('/proofs')).items)
      setError(null)
    } catch (e) {
      setError((e as ApiError).message)
    }
  }, [hasAgent])
  useEffect(() => { void load() }, [load, lastKey])

  if (!me) return null
  return (
    <div className="flex flex-col gap-6">
      <StageCard me={me} />
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      <section>
        <h3 className="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">Proof history</h3>
        {proofs ? <ProofList items={proofs} /> : <p className="text-sm text-muted-foreground">Loading…</p>}
      </section>
    </div>
  )
}
```

- [ ] **Step 4: Экран перед запуском**

`frontend/app/app/proofs/new/page.tsx`:

```tsx
'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { api, post, ApiError } from '@/lib/api'
import { useMe } from '@/lib/use-me'
import { duration } from '@/lib/format'
import type { Proof, ProofTask } from '@/lib/types'

// The catalog has one proof task in slice 1.
const TASK_SLUG = 'go-fix-retry'
const CAN_START = ['connected', 'operational', 'check_failed']

export default function NewProofPage() {
  const { me } = useMe(5000)
  const router = useRouter()
  const [task, setTask] = useState<ProofTask | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [starting, setStarting] = useState(false)

  useEffect(() => {
    api<{ items: ProofTask[] }>('/proof-tasks')
      .then((r) => {
        const t = r.items.find((x) => x.slug === TASK_SLUG)
        if (t) setTask(t)
        else setError('This proof task is not available on the server.')
      })
      .catch((e) => setError((e as ApiError).message))
  }, [])

  async function start() {
    setStarting(true)
    setError(null)
    try {
      const p = await post<Proof>('/proofs', { task_slug: TASK_SLUG })
      router.push(`/app/proofs/${p.id}`)
    } catch (e) {
      const a = e as ApiError
      setError(a.code === 'agent_offline' ? 'The connector is not online. Start `arena connect` first.' : a.message)
      setStarting(false)
    }
  }

  if (!me) return null
  const a = me.agent
  if (!a) {
    return <p className="text-sm text-muted-foreground">Create an agent first. <Link href="/app/agent/new" className="underline">Create agent</Link></p>
  }
  if (!task) {
    return error ? <p role="alert" className="text-sm text-destructive">{error}</p> : <p className="text-sm text-muted-foreground">Loading…</p>
  }
  const running = a.stage === 'checking' ? a.last_proof : null
  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div>
        <p className="font-mono text-xs text-muted-foreground">{task.slug}</p>
        <h1 className="text-2xl font-semibold">{task.title}</h1>
      </div>
      <Card className="space-y-3 p-5 text-sm">
        <p>Your agent gets this task on its own machine through the connector, changes the code and sends back a diff. The platform then runs hidden tests on that diff in a sandbox.</p>
        <p className="font-medium">You only see the result. Once the proof starts, the agent works alone: you can watch, but you can’t help.</p>
        <dl className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <div><dt className="text-xs text-muted-foreground">Language</dt><dd>{task.language}</dd></div>
          <div><dt className="text-xs text-muted-foreground">Agent time limit</dt><dd>{duration(task.agent_timeout_s * 1000)}</dd></div>
          <div><dt className="text-xs text-muted-foreground">Test time limit</dt><dd>{duration(task.sandbox_timeout_s * 1000)}</dd></div>
          <div><dt className="text-xs text-muted-foreground">Tests</dt><dd>{task.visible_tests} visible, {task.hidden_tests} hidden</dd></div>
        </dl>
      </Card>
      <section>
        <h3 className="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">What your agent receives (TASK.md)</h3>
        <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-words rounded-md border border-border bg-muted/30 p-3 font-mono text-xs">{task.task_md}</pre>
      </section>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      {CAN_START.includes(a.stage) && (
        <Button onClick={start} disabled={starting}>{starting ? 'Starting…' : 'Start the proof'}</Button>
      )}
      {running && (
        <p className="text-sm text-muted-foreground">A proof is already running. <Link href={`/app/proofs/${running.id}`} className="underline">Watch it</Link></p>
      )}
      {(a.stage === 'registered' || a.stage === 'offline') && (
        <p className="text-sm text-muted-foreground">The connector is not online, so the proof can’t start. <Link href="/app/agent/connect" className="underline">Connect instructions</Link></p>
      )}
    </div>
  )
}
```

- [ ] **Step 5: Проверка и коммит**

```bash
cd frontend && pnpm typecheck && pnpm build
```

Expected: оба без ошибок; в таблице маршрутов есть `/app/proofs/new` (static) рядом с `ƒ /app/proofs/[id]`. `git status` не должен показывать изменённый `pnpm-lock.yaml`; если показывает — `git checkout -- frontend/pnpm-lock.yaml` и написать об этом в отчёте.

Ручная проверка на запущенном стеке (API и сайт из задачи 4 или уже запущенные): главная ведёт кнопкой «Run basic proof» на `/app/proofs/new`, там видны TASK.md и лимиты; при выключенном коннекторе кнопки старта нет, есть ссылка на подключение; остановленный API → в кабинете сообщение «Could not load your account» и «Try again».

```bash
git add frontend/lib/types.ts frontend/lib/format.ts frontend/app/app/layout.tsx frontend/components/stage-card.tsx frontend/app/app/page.tsx frontend/app/app/proofs/new/page.tsx frontend/.gitignore
git commit -m "Show the task before a proof starts and surface API errors in the cabinet

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Фронт: время шагов проверки и установка коннектора со страницы

**Files:**
- Modify: `frontend/lib/format.ts`, `frontend/components/proof/timeline.tsx`, `frontend/app/app/agent/connect/page.tsx`

**Interfaces:**
- Consumes: `GET /api/v1/connector/download` (задача 4); поля проверки `created_at`, `claimed_at`, `diff_submitted_at`, `finished_at`, `agent_duration_ms`.
- Produces: `clock(iso: string): string`, `between(fromIso: string, to: string | number): string` в `lib/format.ts`.

- [ ] **Step 1: Форматирование времени**

В конец `frontend/lib/format.ts`:

```ts
// Local wall-clock time of an event, e.g. "14:03:27".
export function clock(iso: string) {
  return new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

// How long passed between two moments; `to` is an ISO string or epoch ms.
export function between(fromIso: string, to: string | number) {
  const end = typeof to === 'number' ? to : new Date(to).getTime()
  return duration(Math.max(0, end - new Date(fromIso).getTime()))
}
```

- [ ] **Step 2: Таймлайн со временем**

`frontend/components/proof/timeline.tsx` целиком:

```tsx
import { Check, Loader2, X } from 'lucide-react'
import type { Proof } from '@/lib/types'
import { between, clock, duration } from '@/lib/format'
import { cn } from '@/lib/utils'

type Step = {
  label: string
  reached: (p: Proof) => boolean
  // Shown on the right: when the step happened, how long it took, or how
  // long it has been running. null shows nothing.
  note: (p: Proof, now: number) => string | null
}

const STEPS: Step[] = [
  { label: 'Queued', reached: () => true, note: (p) => clock(p.created_at) },
  {
    label: 'Connector picked it up',
    reached: (p) => p.claimed_at != null,
    note: (p) => (p.claimed_at ? clock(p.claimed_at) : null),
  },
  {
    label: 'Agent running',
    reached: (p) => ['running_agent', 'diff_submitted', 'running_sandbox', 'passed', 'failed'].includes(p.status),
    note: (p, now) =>
      p.agent_duration_ms != null ? `took ${duration(p.agent_duration_ms)}`
        : p.status === 'running_agent' && p.claimed_at ? `${between(p.claimed_at, now)} so far`
          : null,
  },
  {
    label: 'Diff received',
    reached: (p) => p.diff_submitted_at != null,
    note: (p) => (p.diff_submitted_at ? clock(p.diff_submitted_at) : null),
  },
  {
    label: 'Hidden tests',
    reached: (p) => ['running_sandbox', 'passed', 'failed'].includes(p.status),
    note: (p, now) =>
      !p.diff_submitted_at ? null
        : p.finished_at ? `took ${between(p.diff_submitted_at, p.finished_at)}`
          : p.status === 'running_sandbox' ? `${between(p.diff_submitted_at, now)} so far`
            : null,
  },
  {
    label: 'Verdict',
    reached: (p) => p.finished_at != null,
    note: (p) => (p.finished_at ? clock(p.finished_at) : null),
  },
]

export function Timeline({ proof }: { proof: Proof }) {
  const terminal = proof.finished_at != null
  const dead = proof.status === 'expired' || proof.status === 'infra_error'
  const now = Date.now()
  let activeShown = false
  return (
    <ol className="flex flex-col gap-0">
      {STEPS.map((s) => {
        const done = s.reached(proof)
        const active = !done && !activeShown && !terminal
        if (active) activeShown = true
        const note = done || active ? s.note(proof, now) : null
        return (
          <li key={s.label} className="flex items-center gap-3 border-b border-border py-2.5 text-sm last:border-b-0">
            <span className={cn('flex size-5 shrink-0 items-center justify-center rounded-full border text-[10px]',
              done && !dead && 'border-success bg-success text-white', active && 'border-primary', dead && done && 'border-destructive text-destructive')}>
              {done && !dead && <Check className="size-3" strokeWidth={3} />}
              {done && dead && <X className="size-3" />}
              {active && <Loader2 className="size-3 animate-spin" />}
            </span>
            <span className={cn(done || active ? 'text-foreground' : 'text-muted-foreground')}>{s.label}</span>
            {note && <span className="ml-auto shrink-0 font-mono text-xs text-muted-foreground">{note}</span>}
          </li>
        )
      })}
    </ol>
  )
}
```

(Страница проверки перезагружает данные раз в 3 секунды, поэтому «so far» обновляется вместе с ней.)

- [ ] **Step 3: Установка со страницы подключения**

В `frontend/app/app/agent/connect/page.tsx`:

1. После строки `const origin = typeof window === 'undefined' ? '' : window.location.origin` добавить:

```tsx
  const install = `curl -fsSL "${origin}/api/v1/connector/download?os=$(uname -s)&arch=$(uname -m)" -o arena\nchmod +x arena && sudo mv arena /usr/local/bin/arena`
```

2. Строку с шагом «1. Install» заменить на:

```tsx
        <div>
          <p className="mb-1 text-xs text-muted-foreground">1. Install the connector (macOS or Linux)</p>
          <CopyBlock text={install} />
          <p className="mt-1 text-xs text-muted-foreground">Or build it from the repository: <code>cd backend &amp;&amp; go build -o arena ./cmd/arena</code></p>
        </div>
```

- [ ] **Step 4: Проверка и коммит**

```bash
cd frontend && pnpm typecheck && pnpm build
```

Ручная проверка на запущенном стеке: страница проверки показывает время справа от шагов; страница подключения показывает команду с адресом текущего сайта; эта команда, выполненная в терминале, скачивает рабочий бинарник (если на сервере собраны коннекторы — `make connector`).

```bash
git add frontend/lib/format.ts frontend/components/proof/timeline.tsx frontend/app/app/agent/connect/page.tsx
git commit -m "Show step times on the proof timeline and install the connector from the connect page

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: CI: ширина 375px без горизонтального скролла

**Files:**
- Create: `frontend/scripts/check-mobile.mjs`
- Modify: `frontend/package.json`, `frontend/pnpm-lock.yaml`, `.github/workflows/ci.yml`

**Interfaces:**
- Consumes: все страницы кабинета из задач 5–6; `/api/v1/*` через тот же адрес сайта.
- Produces: `pnpm check:mobile` (`BASE_URL`, по умолчанию `http://127.0.0.1:3000`), job `mobile` в CI.

- [ ] **Step 1: Зависимость**

```bash
cd frontend && pnpm add -D playwright-core@1.63.0 --registry=https://registry.npmjs.org
grep -c artifactory pnpm-lock.yaml   # 0: в lockfile не должно быть ссылок на корпоративный реестр
```

`playwright-core` не скачивает браузеры; скрипт использует установленный Chrome (`channel: 'chrome'`): на этой машине он есть, на раннерах `ubuntu-latest` тоже.

В `frontend/package.json` в `scripts` добавить `"check:mobile": "node scripts/check-mobile.mjs"`.

- [ ] **Step 2: Скрипт**

`frontend/scripts/check-mobile.mjs`:

```js
// Every page must fit a 375px-wide screen without horizontal scrolling
// (spec §8). Needs the site (with /api proxied to the API) at BASE_URL and a
// Chrome install. It signs up its own owner, agent, key and a queued proof,
// so the signed-in pages render with real data.
import { chromium } from 'playwright-core'

const BASE = process.env.BASE_URL ?? 'http://127.0.0.1:3000'
const WIDTH = 375

const browser = await chromium.launch({ channel: 'chrome' })
const failures = []

async function widthOf(context, path) {
  const page = await context.newPage()
  try {
    await page.goto(BASE + path)
    await page.locator('h1, h2').first().waitFor({ timeout: 15_000 })
    await page.waitForTimeout(300) // late layout: fonts, polled data
    return await page.evaluate(() => document.documentElement.scrollWidth)
  } finally {
    await page.close()
  }
}

async function check(context, paths) {
  for (const path of paths) {
    const width = await widthOf(context, path)
    console.log(`${width > WIDTH ? 'FAIL' : 'ok  '} ${path} scrollWidth=${width}`)
    if (width > WIDTH) failures.push(`${path}: ${width}px`)
  }
}

try {
  const anonymous = await browser.newContext({ viewport: { width: WIDTH, height: 800 } })
  await check(anonymous, ['/login', '/signup'])

  const owner = await browser.newContext({ viewport: { width: WIDTH, height: 800 } })
  const call = async (method, path, { data, key } = {}) => {
    const res = await owner.request.fetch(`${BASE}/api/v1${path}`, {
      method,
      data,
      headers: key ? { Authorization: `Bearer ${key}` } : {},
    })
    if (!res.ok()) throw new Error(`${method} ${path}: ${res.status()} ${await res.text()}`)
    return res.status() === 204 ? null : res.json()
  }
  const run = Date.now()
  await call('POST', '/auth/signup', { data: { email: `mobile-${run}@example.com`, password: 'longenough-ci-1' } })
  await call('POST', '/agent', { data: { name: `mobile-${run % 1_000_000}`, description: 'CI width check' } })
  const { key } = await call('POST', '/agent/keys', { data: { name: 'ci' } })
  await call('POST', '/connector/heartbeat', { key, data: { connector_version: 'ci', hostname: 'ci' } })
  await check(owner, ['/app', '/app/agent/connect', '/app/proofs/new', '/app/agent/new'])
  const proof = await call('POST', '/proofs', { data: { task_slug: 'go-fix-retry' } })
  await check(owner, ['/app', `/app/proofs/${proof.id}`])
} finally {
  await browser.close()
}

if (failures.length > 0) {
  console.error(`Wider than ${WIDTH}px:\n  ${failures.join('\n  ')}`)
  process.exit(1)
}
```

- [ ] **Step 3: Прогон локально**

Нужен стек со свежим кодом: Postgres из compose, API и сайт. Порты взять свободные, чтобы не мешать уже запущенному:

```bash
set -a; . ./.env; set +a
API_PORT=3031 make run-api > /tmp/mobile-api.log 2>&1 &
API_PORT=3031 WEB_PORT=3030 make run-web > /tmp/mobile-web.log 2>&1 &
until curl -sf -o /dev/null http://127.0.0.1:3030/login; do sleep 1; done
cd frontend && BASE_URL=http://127.0.0.1:3030 pnpm check:mobile
```

Expected: все строки `ok`, код выхода 0. Затем остановить оба процесса (`kill` по PID из `jobs -l` или `lsof -ti tcp:3030 -ti tcp:3031 | xargs kill`).

Контрольный провал: временно добавить на `/app/agent/connect` элемент `<div style={{ width: 600 }} />`, убедиться, что скрипт печатает `FAIL /app/agent/connect` и выходит с кодом 1, затем убрать элемент (в коммит не попадает).

- [ ] **Step 4: Job в CI**

В `.github/workflows/ci.yml` добавить в `jobs` третью задачу:

```yaml
  mobile:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:16-alpine
        env:
          POSTGRES_USER: arena_migrate
          POSTGRES_PASSWORD: ci_migrate_password
          POSTGRES_DB: arena
        ports: ["5432:5432"]
        options: >-
          --health-cmd "pg_isready -U arena_migrate -d arena"
          --health-interval 2s --health-timeout 2s --health-retries 30
    env:
      ARENA_APP_ROLE_PASSWORD: ci_app_password
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: backend/go.mod
      - uses: pnpm/action-setup@v4
        with:
          version: 12
      - uses: actions/setup-node@v4
        with:
          node-version: 24
          cache: pnpm
          cache-dependency-path: frontend/pnpm-lock.yaml
      - name: migrate
        run: |
          cd backend
          ARENA_MIGRATE_DATABASE_URL="postgres://arena_migrate:ci_migrate_password@127.0.0.1:5432/arena?sslmode=disable" \
            ARENA_PROOFS_DIR=./fixtures/proofs go run ./cmd/migrate
      - name: start the api (fake sandbox; nothing reaches it here)
        run: |
          cd backend && go build -o /tmp/arena-api ./cmd/api
          ARENA_APP_DATABASE_URL="postgres://arena_app:$ARENA_APP_ROLE_PASSWORD@127.0.0.1:5432/arena?sslmode=disable" \
            ARENA_SANDBOX=fake nohup /tmp/arena-api > /tmp/api.log 2>&1 &
          for i in $(seq 30); do curl -sf http://127.0.0.1:8080/healthz && break; sleep 1; done
      - name: build and start the site
        run: |
          cd frontend
          pnpm install --frozen-lockfile
          API_URL=http://127.0.0.1:8080 pnpm build
          cp -r .next/static .next/standalone/.next/static
          cp -r public .next/standalone/public
          PORT=3000 HOSTNAME=127.0.0.1 nohup node .next/standalone/server.js > /tmp/web.log 2>&1 &
          for i in $(seq 30); do curl -sf -o /dev/null http://127.0.0.1:3000/login && break; sleep 1; done
      - name: every page fits 375px without horizontal scroll
        run: cd frontend && BASE_URL=http://127.0.0.1:3000 pnpm check:mobile
      - name: server logs
        if: failure()
        run: cat /tmp/api.log /tmp/web.log
```

- [ ] **Step 5: Коммит**

```bash
git add frontend/scripts/check-mobile.mjs frontend/package.json frontend/pnpm-lock.yaml .github/workflows/ci.yml
git commit -m "Check in CI that every page fits a 375px screen

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

(Job `mobile` реально проверится только в CI после пуша; если он упадёт там, это находка для финального ревью.)

---

### Task 8: Документация

**Files:**
- Modify: `README.md`, `docs/how-it-works.md`

**Interfaces:**
- Consumes: всё из задач 1–7.

- [ ] **Step 1: README**

В `README.md` раздел «## Подключить агента» целиком заменить на:

```markdown
## Подключить агента

1. Зарегистрируйтесь на сайте, создайте агента, скопируйте ключ.
2. На машине с агентом (macOS или Linux) скачайте коннектор — команда с
   адресом вашего сайта есть на странице Connect:

   ```sh
   curl -fsSL "http://localhost:3000/api/v1/connector/download?os=$(uname -s)&arch=$(uname -m)" -o arena
   chmod +x arena && sudo mv arena /usr/local/bin/arena
   ```

   Или соберите из репозитория: `cd backend && go build -o arena ./cmd/arena`.
3. `arena login`, `ARENA_URL=http://localhost:3000 arena init`, впишите
   команду запуска агента в `~/.arena/config.yaml`, затем `arena connect`.
4. На сайте нажмите «Run basic proof», посмотрите задачу и запустите.
```

В разделе «## Разработка» строку `make migrate && make run-api && make run-web   # нативно, postgres из compose` заменить на:

```
make migrate && make connector && make run-api   # нативно, postgres из compose; порты из .env
make run-web
```

- [ ] **Step 2: `docs/how-it-works.md`**

1. В «Путь одной проверки», шаг 1, заменить текст на: «**Владелец жмёт «Run basic proof».** Открывается экран задачи: условие (`TASK.md`), лимиты времени, число видимых и скрытых тестов и правило «владелец видит только результат». Кнопка «Start the proof» создаёт проверку в статусе `queued`.»

2. В шаге 5 после «…и отправляет всё на платформу.» вставить: «Сбои сети и ошибки сервера (5xx) при скачивании репозитория и отправке результата коннектор повторяет с паузой от 2 до 30 секунд, пока платформа ещё ждёт результат (`agent_timeout_s + 60` секунд от выдачи задачи).»

3. В таблице «Как решается вердикт» первой строкой добавить:
   `| failed | \`diff_too_large\` | результат больше 256 KiB; проверка завершается сразу, песочница не запускается |`

4. В «Что видит владелец» список заменить на:

```markdown
- **Главная `/app`** — одна карточка «что сейчас» по стадии агента и список
  проверок.
- **Экран задачи `/app/proofs/new`** — перед запуском: условие, лимиты,
  число тестов. Проверка запускается только отсюда.
- **Страница подключения `/app/agent/connect`** — команда скачивания
  коннектора, остальные команды, индикатор онлайна (обновляется раз в 5
  секунд), выпуск и отзыв ключей.
- **Страница проверки `/app/proofs/<id>`** — таймлайн шагов со временем
  каждого шага и длительностью работы агента и тестов, вердикт, таблица
  тестов, diff агента с подсветкой, хвост лога.

Если API недоступен, кабинет показывает ошибку и кнопку «Try again».
```

5. Раздел «## Запустить локально» от «Нативно (Postgres из compose, API и сайт на хосте):» до абзаца «В Superset-воркспейсе…» (не включая его) заменить на:

```markdown
Нативно (Postgres из compose, API и сайт на хосте):

```sh
cp -n .env.example .env
docker compose up -d postgres        # или docker-compose
make migrate
make proof-image                     # образ песочницы, если его ещё нет
make connector                       # бинарники коннектора для страницы Connect
make run-api                         # слушает 127.0.0.1:$API_PORT
make run-web                         # сайт на :$WEB_PORT, /api проксируется в API
```
```

6. В «## Подключить агента» шаг 2 заменить на:

```markdown
2. Скачать коннектор (macOS или Linux; команда с адресом твоего сайта есть
   на странице подключения):

   ```sh
   curl -fsSL "http://localhost:3000/api/v1/connector/download?os=$(uname -s)&arch=$(uname -m)" -o arena
   chmod +x arena && sudo mv arena /usr/local/bin/arena
   ```

   Сервер отдаёт готовые бинарники для darwin и linux на amd64 и arm64: в
   Docker-образе API они собираются при сборке, при нативном запуске —
   командой `make connector`. Без них скачивание отвечает 404
   `connector_unavailable`, и коннектор можно собрать из репозитория:
   `cd backend && go build -o arena ./cmd/arena`.
```

7. В конец «## Подключить агента» (после абзаца про `ARENA_HOME`) добавить: «`arena status` показывает стадию агента и последнюю проверку. Это не heartbeat: он не делает агента онлайн.»

8. В «## Частые проблемы»: строку про `/app` заменить на
   `| «Could not load your account» в кабинете | API недоступен или перезапускается; «Try again» повторит запрос |`
   и добавить строку
   `| \`failed\`, \`diff_too_large\` | агент наменял больше 256 KiB (часто — сгенерированные файлы или зависимости в репозитории); проверь, что он правит только нужное |`

9. Раздел «## Известные недоделки» целиком заменить на:

```markdown
## Известные недоделки

- Песочница без `--read-only` (см. раздел «Песочница»).
- Коннектор работает только на macOS и Linux: агент запускается через
  `sh -c`.
```

- [ ] **Step 3: Проверка и коммит**

Сверить каждое утверждение документа с кодом этой ветки (команды Makefile, тексты кнопок, причины провала, адрес скачивания). Открыть `README.md` и `docs/how-it-works.md` в просмотрщике Markdown и убедиться, что вложенные блоки кода не ломают разметку (во вложенных в список блоках — отступ 3 пробела, как в остальном файле).

```bash
git add README.md docs/how-it-works.md
git commit -m "Update the guide and README for connector downloads and the new screens

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

## Самопроверка плана

**Покрытие.** Установка коннектора (спека §5 «скачать со страницы подключения») → задачи 4, 6, 8. `/me` с последней проверкой (§4.1) → задача 1. `arena status` со стадией и последней проверкой (§5) → задачи 1, 3. Повтор при потере сети (§5) → задача 3; слишком большой diff больше не ждёт `expired` → задача 2. Экран до запуска и шаги «с временем» (§8) → задачи 5, 6. Состояния «ошибка» (§8) → задача 5. Проверка 375px в CI (§8, §10) → задача 7. Порты нативного запуска → задача 4. Документация → задача 8.

**Типы.** `proofs.Service.Latest` (задача 1) используется только в `cmd/api/compose.go`. `last_proof` одинаково назван в Go (`ownerAgent.LastProof`, JSON `last_proof`), в `openapi.yaml` и во фронтовом `AgentOverview.last_proof`. `diff_too_large` одинаково в сервисе, контракте ответа 413 и `REASON_LABEL`. `connectorDownload(dir string)` и `config.connectorDir` / `ARENA_CONNECTOR_DIR` совпадают в задаче 4 и Dockerfile. `retryBase`, `withRetry`, `formatStatus`, `statusResp` определены и использованы только в задаче 3.

**Review Focus.** 1 → задача 4 (`Darwin`, `aarch64`, `Windows_NT`, пустой запрос). 2 → задача 2 (`TestEndToEnd_OversizedResultFailsTheProof`). 3 → задача 3 (`TestClient_RepoRetriesDroppedConnectionsAndServerErrors`). 4 → задача 1 (status до heartbeat, стадия `registered`). 5 → задача 1 (`me after key`: `LastProof == nil`).
