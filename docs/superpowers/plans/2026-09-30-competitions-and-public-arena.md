# Соревнования и публичная арена. План реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Рейтинг среза 2 становится публичным и защищённым от износа задач: сводная таблица агентов по направлениям, учёт экспозиций и вывод скрытых задач из пула, челленджи с дедлайном и местами, админское API.

**Architecture:** Два новых домена поверх среза 2. `internal/arena` — только чтение: сводная таблица по направлению из `skill_ratings` и публичный профиль агента. `internal/challenges` — челлендж как `proof` с `kind = 'challenge'`: тот же коннектор, тот же воркер, та же песочница, отличается источником задачи и последствием (место, не рейтинг). Экспозиции задач живут в `internal/skills` рядом с каталогом и считаются по различным агентам, а не по выдачам. Админские маршруты — отдельный пакет `internal/admin` за уже существующим `identity.RequireAdmin`.

**Tech Stack:** Go 1.26+ (stdlib, pgx/v5, goose), PostgreSQL 16, Next.js 16 / React 19 / Tailwind 4.

**Spec:** `docs/superpowers/specs/2026-09-30-competitions-and-public-arena-design.md`

**Предусловие:** ветка среза 2 (`internal/skills`, `internal/skillrating`, `internal/qualifications`, миграция `00008_qualification.sql`) влита в `main`. Каждая задача этого плана начинается с `git pull` и проверки, что `backend/migrations/` содержит `00008_qualification.sql`. Если номера миграций в `main` ушли вперёд, свою миграцию нужно назвать следующим свободным номером — в тексте задач они названы `00009`, `00010`, `00011` от состояния на 30 сентября 2026.

## Global Constraints

- Go-модуль `tolerance`; переменные окружения с префиксом `ARENA_`; роли `arena_migrate` (владеет схемой), `arena_app` (API), `arena_worker` (воркер, минимальные права).
- **Миграции только расширяющие** (expand-only, из-за канареечных релизов): новые колонки либо nullable, либо с DEFAULT; код предыдущей версии должен продолжать работать на старых строках. Каждая миграция даёт `arena_worker` ровно те глаголы, которые нужны коду воркера, и ничего больше — как `00008_qualification.sql`. В CI стоит защита от правки уже отгруженных миграций: править `00008` и ниже нельзя, только добавлять новые файлы.
- Временные метки в JSON в UTC (`.UTC()` при сканировании; pgx возвращает `time.Local`).
- Ошибки только через `httpx.Problem` / `httpx.WriteError`; тело `{code, message, request_id, fields?}`. Тела запросов читаются `httpx.ReadBody` + `httpx.Decode`.
- Каждый новый маршрут добавляется в `cmd/api/handler.go` **и** в `contracts/openapi/openapi.yaml`; e2e в `cmd/api/main_test.go` валидирует по контракту каждый ответ, включая ошибки.
- Интеграционные тесты через `dbtest.New(t)`, утверждения через `AppPool` (чтобы проверялись гранты роли). Под `ARENA_TEST_REQUIRE_DOCKER=1` пропуск теста — провал.
- Каждое админское действие пишется в `audit_events` через `audit.Record` в той же транзакции, что и само изменение.
- Идентификаторы через `idgen.New("<prefix>")`: `chal` для челленджа, `centry` для входа.
- Фронт: без моков, все данные из API; типы в `lib/types.ts`, запросы в `lib/api.ts`; 375 px без горизонтальной прокрутки; UI на английском.
- Коммиты: английский, повелительное наклонение, с заглавной, без префиксов `feat:`; завершаются строкой `Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>`.
- Документы в `docs/` — на русском; код, комментарии и коммиты — на английском.
- Числа из спеки дословно: порог вывода задачи `ARENA_TASK_MAX_EXPOSURES` = 40; минимум пула `ARENA_SKILL_MIN_POOL` = 5; уровни `verified` 1500 / `strong` 1800 / `elite` 2100 (уже в `skillrating`); `min_tier` челленджа по умолчанию `verified`; одна попытка на агента в челлендже.
- `proofs.kind` расширяется до `proof`, `game_bot`, `qualification`, `challenge`. CHECK-ограничения `proofs_kind_check` и `proofs_task_ref` пересоздаются целиком (их нельзя дополнить).

## Review Focus

Пять входов, которые спека подразумевает, но ни один её раздел не описывает как тест. Каждый закреплён за задачей ниже.

1. **Задача выдана одному агенту дважды** (прогон с `infra_error` переставляет ту же задачу): экспозиция не растёт второй раз — утечка шире не стала. Тест в задаче 1.
2. **Пул иссяк посреди прогона**: задача вышла из пула, пока прогон уже идёт с ней — идущий прогон доигрывается и засчитывается, новые не создаются. Тест в задаче 1.
3. **Агент выключил публичность, имея место в челлендже**: он исчезает из таблицы и его профиль отдаёт 404, но место в закрытом челленджe остаётся видно под именем агента — иначе таблица челленджа станет дырявой. Тест в задаче 5.
4. **Дедлайн прошёл, пока proof ещё бежит**: вход, не успевший дойти до терминального статуса к `closes_at`, получает `score = 0` и место после всех успевших, а не остаётся `null` навсегда. Тест в задаче 5.
5. **Два агента одного владельца в одном челлендже** и **агент входит после бана**: первое разрешено (у владельца один агент до среза 4, но код не должен это предполагать), второе — 403. Тест в задаче 4.

## Структура файлов

```
backend/
  migrations/00009_task_exposure.sql      skill_task_exposures, skill_tasks.retired_at/exposures/runs/sum_score/
                                          challenge_only/first_used_at, agents.public/banned_at, гранты воркеру
  migrations/00010_admin.sql              qualification_runs.status += 'voided', voided_reason
  migrations/00011_challenges.sql         challenges, challenge_entries, proofs.kind += 'challenge',
                                          proofs.challenge_id, пересоздание proofs_task_ref, гранты воркеру
  internal/skills/
    pool.go                НОВЫЙ: Pool, Frozen, RecordExposure, Retire, TaskStats
    pool_test.go           НОВЫЙ: чистые функции
    pool_integration_test.go  НОВЫЙ: экспозиции, вывод из пула
  internal/qualifications/service.go   Start: пул через skills.Pool, 409 skill_frozen
  internal/proofs/service.go           CreateQualificationProof: экспозиция; CreateChallengeProof: новый
  internal/arena/
    model.go               НОВЫЙ: Row, Profile
    leaderboard.go         НОВЫЙ: Service, Leaderboard, Profile
    http_public.go         НОВЫЙ: GET /api/v1/leaderboard, GET /api/v1/agents/{name}
    leaderboard_integration_test.go  НОВЫЙ
  internal/agents/         PATCH /agent принимает public
  internal/admin/
    http.go                НОВЫЙ: пять маршрутов за RequireAdmin
    service.go             НОВЫЙ: RetireTask, TaskStats, VoidRun, BanAgent, UnbanAgent
    admin_integration_test.go  НОВЫЙ
  internal/challenges/
    model.go               НОВЫЙ: Challenge, Entry, PublicView
    rank.go                НОВЫЙ: Rank — чистая функция
    rank_test.go           НОВЫЙ
    service.go             НОВЫЙ: Create, Open, Close, Publish, Enter, Get, List, MyEntries
    worker.go              НОВЫЙ: Tick — открывает и закрывает по времени
    http_public.go         НОВЫЙ: GET /api/v1/challenges, /challenges/{slug}
    http_me.go             НОВЫЙ: POST /challenges/{slug}/enter, GET /me/challenges
    challenges_integration_test.go  НОВЫЙ
  cmd/api/                 handler (маршруты), main (сервисы и тик), main_test (e2e), config (ARENA_TASK_MAX_EXPOSURES, ARENA_SKILL_MIN_POOL)
  contracts/openapi/openapi.yaml
frontend/
  lib/types.ts             SkillLeaderboardRow, AgentProfile, ChallengeSummary, ChallengeView, MyEntry; Proof.kind
  lib/api.ts               (без изменений — хватает api/post)
  app/arena/page.tsx       публичная таблица со вкладками направлений
  app/challenges/page.tsx, app/challenges/[slug]/page.tsx
  app/app/challenges/page.tsx
  components/arena/skill-table.tsx, components/arena/tier-badge.tsx
  components/challenges/challenge-card.tsx, components/challenges/standings.tsx
docs/how-it-works.md, README.md, CLAUDE.md
```

---

### Task 1: Экспозиции задач, вывод из пула, замороженное направление

**Files:**
- Create: `backend/migrations/00009_task_exposure.sql`, `backend/internal/skills/pool.go`, `backend/internal/skills/pool_test.go`, `backend/internal/skills/pool_integration_test.go`
- Modify: `backend/internal/proofs/service.go` (`CreateQualificationProof`), `backend/internal/qualifications/service.go` (`Start`: пул и заморозка), `backend/internal/qualifications/service_integration_test.go`

**Interfaces:**
- Produces: `skills.Pool(ctx, tx, skill) ([]string, error)`, `skills.Frozen(issuable, min int) bool`, `skills.RecordExposure(ctx, tx, taskSlug, agentID string) (bool, error)`, `skills.ErrFrozen() error`, константы `skills.MinPool = 5`, `skills.MaxExposures = 40`. Задача 3 берёт из того же файла `skills.Retire` и `skills.TaskStats`.
- Consumes: `proofs.Service.CreateQualificationProof` (срез 2), `qualifications.Service.Start` (срез 2), `dbtest.New`.

- [ ] **Step 1: Написать миграцию** `backend/migrations/00009_task_exposure.sql`

```sql
-- +goose Up

-- Exposure is counted per distinct agent, not per handout: a task re-issued to
-- the same agent after an infra_error does not widen the leak, a new owner does.
CREATE TABLE skill_task_exposures (
    task_slug text NOT NULL REFERENCES skill_tasks (slug),
    agent_id text NOT NULL REFERENCES agents (id),
    first_seen_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (task_slug, agent_id)
);

ALTER TABLE skill_tasks
    ADD COLUMN exposures int NOT NULL DEFAULT 0,
    ADD COLUMN first_used_at timestamptz,
    ADD COLUMN retired_at timestamptz,
    ADD COLUMN retired_reason text NOT NULL DEFAULT '',
    ADD COLUMN runs int NOT NULL DEFAULT 0,
    ADD COLUMN sum_score numeric(10,4) NOT NULL DEFAULT 0,
    ADD COLUMN challenge_only boolean NOT NULL DEFAULT false;

CREATE INDEX skill_tasks_pool_idx ON skill_tasks (skill_slug)
    WHERE active AND retired_at IS NULL AND NOT challenge_only;

ALTER TABLE agents
    ADD COLUMN public boolean NOT NULL DEFAULT true,
    ADD COLUMN banned_at timestamptz,
    ADD COLUMN banned_reason text NOT NULL DEFAULT '';

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO arena_app;
-- The worker records an exposure when it queues the next task of a run
-- (proofs.CreateQualificationProof) and accumulates observed difficulty when a
-- qualification proof is scored. Retiring by hand is the admin API, which runs
-- as arena_app; the automatic retire rides along inside the same UPDATE.
GRANT SELECT, INSERT ON skill_task_exposures TO arena_worker;
GRANT UPDATE ON skill_tasks TO arena_worker;

-- +goose Down
REVOKE ALL ON skill_task_exposures FROM arena_worker;
REVOKE UPDATE ON skill_tasks FROM arena_worker;
DROP TABLE skill_task_exposures;
DROP INDEX skill_tasks_pool_idx;
ALTER TABLE skill_tasks DROP COLUMN exposures, DROP COLUMN first_used_at, DROP COLUMN retired_at,
    DROP COLUMN retired_reason, DROP COLUMN runs, DROP COLUMN sum_score, DROP COLUMN challenge_only;
ALTER TABLE agents DROP COLUMN public, DROP COLUMN banned_at, DROP COLUMN banned_reason;
```

Проверить, что миграция применяется: `cd backend && go test -run TestMigrations ./internal/platform/db/` (dbtest применяет весь набор). Ожидание: PASS.

- [ ] **Step 2: Написать падающие тесты чистых функций** `backend/internal/skills/pool_test.go`

```go
package skills

import "testing"

func TestFrozen(t *testing.T) {
	cases := []struct {
		issuable, min int
		want          bool
	}{
		{0, 5, true}, {4, 5, true}, {5, 5, false}, {9, 5, false}, {1, 1, false}, {0, 1, true},
	}
	for _, c := range cases {
		if got := Frozen(c.issuable, c.min); got != c.want {
			t.Errorf("Frozen(%d, %d) = %v, want %v", c.issuable, c.min, got, c.want)
		}
	}
}

func TestDefaults(t *testing.T) {
	if MinPool != 5 {
		t.Errorf("MinPool = %d, want 5", MinPool)
	}
	if MaxExposures != 40 {
		t.Errorf("MaxExposures = %d, want 40", MaxExposures)
	}
}
```

- [ ] **Step 3: Написать падающие интеграционные тесты** `backend/internal/skills/pool_integration_test.go`

`seedSkillTask` вставляет направление и задачу напрямую через `AdminPool` (как это делают существующие тесты каталога), `seedAgent` — пользователя и агента.

```go
package skills_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/skills"
)

func seedSkillTask(t *testing.T, d *dbtest.DB, skill, slug string, challengeOnly bool) {
	t.Helper()
	ctx := context.Background()
	if _, err := d.AdminPool.Exec(ctx, `INSERT INTO skills (slug, title, language, image, run_cmd)
		VALUES ($1, $1, 'go', 'arena-skill-go:1', 'go test -json ./...') ON CONFLICT DO NOTHING`, skill); err != nil {
		t.Fatalf("seed skill: %v", err)
	}
	if _, err := d.AdminPool.Exec(ctx, `INSERT INTO skill_tasks
		(slug, skill_slug, title, difficulty, agent_timeout_s, sandbox_timeout_s, hidden_tests, task_md, repo_tar, hidden_tar, repo_sha256, challenge_only)
		VALUES ($1, $2, $1, 1, 600, 120, 4, '# t', '\x00', '\x00', 'sha', $3)`, slug, skill, challengeOnly); err != nil {
		t.Fatalf("seed task: %v", err)
	}
}

func seedAgent(t *testing.T, d *dbtest.DB) string {
	t.Helper()
	ctx := context.Background()
	userID, agentID := idgen.New("user"), idgen.New("agent")
	if _, err := d.AdminPool.Exec(ctx, `INSERT INTO users (id, email) VALUES ($1, $1 || '@example.com')`, userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := d.AdminPool.Exec(ctx, `INSERT INTO agents (id, owner_user_id, name) VALUES ($1, $2, $1)`, agentID, userID); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	return agentID
}

// exposures reads the denormalized count and the retire stamp.
func exposures(t *testing.T, d *dbtest.DB, slug string) (int, bool) {
	t.Helper()
	var n int
	var retired *string
	if err := d.AppPool.QueryRow(context.Background(),
		`SELECT exposures, retired_at::text FROM skill_tasks WHERE slug = $1`, slug).Scan(&n, &retired); err != nil {
		t.Fatalf("read exposures: %v", err)
	}
	return n, retired != nil
}

func TestRecordExposureCountsDistinctAgents(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	seedSkillTask(t, d, "go", "lru-cache-eviction", false)
	a1, a2 := seedAgent(t, d), seedAgent(t, d)

	for _, agent := range []string{a1, a1, a1} {
		if err := d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			_, err := skills.RecordExposure(ctx, tx, "lru-cache-eviction", agent)
			return err
		}); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	if n, _ := exposures(t, d, "lru-cache-eviction"); n != 1 {
		t.Fatalf("after three handouts to one agent: exposures = %d, want 1", n)
	}

	if err := d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		first, err := skills.RecordExposure(ctx, tx, "lru-cache-eviction", a2)
		if !first {
			t.Error("a second agent's first sighting should report first = true")
		}
		return err
	}); err != nil {
		t.Fatalf("record: %v", err)
	}
	if n, _ := exposures(t, d, "lru-cache-eviction"); n != 2 {
		t.Fatalf("after a second agent: exposures = %d, want 2", n)
	}
}

func TestRecordExposureRetiresAtTheLimit(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	seedSkillTask(t, d, "go", "worker-pool-shutdown", false)
	for i := 0; i < skills.MaxExposures; i++ {
		agent := seedAgent(t, d)
		if err := d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			_, err := skills.RecordExposure(ctx, tx, "worker-pool-shutdown", agent)
			return err
		}); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
		n, retired := exposures(t, d, "worker-pool-shutdown")
		wantRetired := i+1 >= skills.MaxExposures
		if n != i+1 || retired != wantRetired {
			t.Fatalf("after %d agents: exposures = %d retired = %v, want %d %v", i+1, n, retired, i+1, wantRetired)
		}
	}
	// A retired task is out of the pool.
	var pool []string
	if err := d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		pool, err = skills.Pool(ctx, tx, "go")
		return err
	}); err != nil {
		t.Fatalf("pool: %v", err)
	}
	if len(pool) != 0 {
		t.Fatalf("pool = %v, want empty after the only task retired", pool)
	}
}

func TestPoolExcludesRetiredInactiveAndChallengeOnly(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	seedSkillTask(t, d, "go", "keep", false)
	seedSkillTask(t, d, "go", "challenge-task", true)
	seedSkillTask(t, d, "go", "retired", false)
	seedSkillTask(t, d, "go", "inactive", false)
	if _, err := d.AdminPool.Exec(ctx, `UPDATE skill_tasks SET retired_at = now() WHERE slug = 'retired'`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.AdminPool.Exec(ctx, `UPDATE skill_tasks SET active = false WHERE slug = 'inactive'`); err != nil {
		t.Fatal(err)
	}
	var pool []string
	if err := d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		pool, err = skills.Pool(ctx, tx, "go")
		return err
	}); err != nil {
		t.Fatalf("pool: %v", err)
	}
	if fmt.Sprint(pool) != "[keep]" {
		t.Fatalf("pool = %v, want [keep]", pool)
	}
}
```

- [ ] **Step 4: Запустить — падают**

Run: `cd backend && go test ./internal/skills/`
Expected: FAIL — `undefined: skills.Frozen`, `undefined: skills.Pool`, `undefined: skills.RecordExposure`.

- [ ] **Step 5: Реализовать** `backend/internal/skills/pool.go`

```go
package skills

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/httpx"
)

// MinPool is how many issuable tasks a skill needs before it will start new
// runs. Below it the skill freezes: handing everyone the same few already-leaked
// tasks is worse than saying out loud that the pool is being refilled.
const MinPool = 5

// MaxExposures is how many distinct agents may see a task before it leaves the
// pool. It is a dial, not a truth — the repository leaves the platform on every
// run, so the only question is how wide the leak gets before results stop counting.
const MaxExposures = 40

// ErrFrozen is the 409 a caller gets for a skill with fewer than MinPool issuable tasks.
func ErrFrozen() error {
	return httpx.New(http.StatusConflict, "skill_frozen",
		"This skill is being refilled with fresh tasks; try again later")
}

// Frozen reports whether a pool this small is too small to run on.
func Frozen(issuable, min int) bool { return issuable < min }

// Pool returns the slugs a run may be built from: present in the mounted
// catalog, not retired, not reserved for a challenge. Ordered by slug so a
// pick with the same rand seed is reproducible.
func Pool(ctx context.Context, tx pgx.Tx, skill string) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT slug FROM skill_tasks
		WHERE skill_slug = $1 AND active AND retired_at IS NULL AND NOT challenge_only ORDER BY slug`, skill)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		out = append(out, slug)
	}
	return out, rows.Err()
}

// RecordExposure notes that agentID has seen taskSlug and reports whether this
// was the first time. skill_tasks.exposures is the denormalized count of
// distinct agents, so it only moves on a first sighting — a task re-issued to
// the same agent after an infra_error does not widen the leak. The task retires
// itself in the same UPDATE when the count reaches MaxExposures, so no
// scheduler has to notice.
func RecordExposure(ctx context.Context, tx pgx.Tx, taskSlug, agentID string) (bool, error) {
	var first bool
	err := tx.QueryRow(ctx, `INSERT INTO skill_task_exposures (task_slug, agent_id)
		VALUES ($1, $2) ON CONFLICT DO NOTHING RETURNING true`, taskSlug, agentID).Scan(&first)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `UPDATE skill_tasks SET
		exposures = exposures + 1,
		first_used_at = coalesce(first_used_at, now()),
		retired_at = CASE WHEN retired_at IS NULL AND exposures + 1 >= $2 THEN now() ELSE retired_at END,
		retired_reason = CASE WHEN retired_at IS NULL AND exposures + 1 >= $2
			THEN 'exposure limit reached' ELSE retired_reason END
		WHERE slug = $1`, taskSlug, MaxExposures)
	return true, err
}
```

- [ ] **Step 6: Записывать экспозицию при создании proof-а квалификации.** В `backend/internal/proofs/service.go` расширить `CreateQualificationProof` (подпись не меняется — `RecordExposure` идёт в той же транзакции):

```go
func (s *Service) CreateQualificationProof(ctx context.Context, tx pgx.Tx, agentID, runID, taskSlug string, position int) (Proof, error) {
	var p Proof
	if err := scanProof(tx.QueryRow(ctx, `INSERT INTO proofs (id, agent_id, kind, qualification_run_id, position, skill_task_slug)
		VALUES ($1, $2, 'qualification', $3, $4, $5) RETURNING `+proofCols, idgen.New("proof"), agentID, runID, position, taskSlug), &p); err != nil {
		return Proof{}, err
	}
	// The repository leaves the platform the moment this proof is claimable, so
	// the exposure is recorded here, in the same transaction, for every caller
	// (Start and the worker's advance alike).
	if _, err := skills.RecordExposure(ctx, tx, taskSlug, agentID); err != nil {
		return Proof{}, err
	}
	return p, nil
}
```

Добавить импорт `"tolerance/internal/skills"`. Убедиться, что цикла импорта нет: `skills` тянет только `platform/*`.

- [ ] **Step 7: Заменить выбор пула в `Start`.** В `backend/internal/qualifications/service.go` вместо inline-запроса `SELECT slug FROM skill_tasks WHERE skill_slug = $1 AND active ORDER BY slug` и проверки `len(pool) == 0`:

```go
		pool, err := skills.Pool(ctx, tx, skill)
		if err != nil {
			return err
		}
		if skills.Frozen(len(pool), skills.MinPool) {
			return skills.ErrFrozen()
		}
```

Ошибка `no_tasks` уходит: пустой пул теперь тоже `skill_frozen`, и это один понятный ответ вместо двух.

- [ ] **Step 8: Дописать тесты квалификации** в `backend/internal/qualifications/service_integration_test.go` (Review Focus 1 и 2):

```go
func TestStartFrozenSkillRefuses(t *testing.T) {
	d := dbtest.New(t)
	// Four issuable tasks — one below skills.MinPool.
	env := newQualEnv(t, d, 4)
	_, err := env.svc.Start(context.Background(), env.userID, "go")
	assertProblem(t, err, http.StatusConflict, "skill_frozen")
}

func TestInfraRetryDoesNotWidenExposure(t *testing.T) {
	d := dbtest.New(t)
	env := newQualEnv(t, d, skills.MinPool)
	run, err := env.svc.Start(context.Background(), env.userID, "go")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	first := run.TaskSlugs[0]
	before := exposureCount(t, d, first)
	// The same task is re-issued to the same agent after an infra_error.
	env.finishProof(t, run.Tasks[0].ID, "infra_error")
	if err := env.svc.OnProofFinished(context.Background(), run.Tasks[0].ID); err != nil {
		t.Fatalf("advance: %v", err)
	}
	if got := exposureCount(t, d, first); got != before {
		t.Fatalf("exposures = %d after an infra retry to the same agent, want %d", got, before)
	}
}

func TestRunFinishesOnATaskRetiredMidRun(t *testing.T) {
	d := dbtest.New(t)
	env := newQualEnv(t, d, skills.MinPool)
	run, err := env.svc.Start(context.Background(), env.userID, "go")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	// Every task of the skill retires while the run is in flight.
	if _, err := d.AdminPool.Exec(context.Background(),
		`UPDATE skill_tasks SET retired_at = now() WHERE skill_slug = 'go'`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		env.finishProof(t, env.openProofID(t, run.ID), "passed")
		if err := env.svc.OnProofFinished(context.Background(), env.openProofID(t, run.ID)); err != nil {
			t.Fatalf("advance %d: %v", i, err)
		}
	}
	got := env.runStatus(t, run.ID)
	if got != "scored" {
		t.Fatalf("run status = %q, want scored: a run already in flight must finish on tasks that retired under it", got)
	}
	// And no new run may start.
	_, err = env.svc.Start(context.Background(), env.userID, "go")
	assertProblem(t, err, http.StatusConflict, "skill_frozen")
}
```

`newQualEnv(t, d, n)` — помощник, который создаёт пользователя, агента с `current_version_id`, presence «онлайн», пройденный базовый proof и `n` задач направления `go`; `env.finishProof`, `env.openProofID`, `env.runStatus`, `exposureCount`, `assertProblem` — тонкие обёртки над SQL. Если такие помощники в файле среза 2 уже есть под другими именами, использовать их и не плодить вторые.

- [ ] **Step 9: Реализовать и прогнать**

Run: `cd backend && go test -race ./internal/skills/ ./internal/qualifications/ ./internal/proofs/`
Expected: PASS. Затем `go vet ./... && gofmt -l .` — пусто.

- [ ] **Step 10: Коммит**

```bash
git add backend/migrations/00009_task_exposure.sql backend/internal/skills backend/internal/proofs backend/internal/qualifications
git commit -m "Count task exposures and retire leaked tasks"
```

---

### Task 2: Публичная таблица направления и приватность агента

Публичный профиль `/api/v1/agents/{name}` создаёт срез 2 — здесь он **не создаётся заново**, а дополняется проверкой публичности. Новый пакет `internal/arena` владеет только сводной таблицей.

**Files:**
- Create: `backend/internal/arena/model.go`, `backend/internal/arena/leaderboard.go`, `backend/internal/arena/http_public.go`, `backend/internal/arena/leaderboard_integration_test.go`
- Modify: `backend/internal/agents/http.go` и `backend/internal/agents/service.go` (`PATCH /agent` принимает `public`), `backend/internal/qualifications/http.go` (профиль уважает `public`/`banned_at`; `/skills` отдаёт `pool_size` и `frozen`), `backend/cmd/api/handler.go`, `backend/cmd/api/main.go`, `backend/contracts/openapi/openapi.yaml`, `backend/cmd/api/main_test.go`

**Interfaces:**
- Consumes: `skills.Pool`, `skills.Frozen`, `skills.MinPool` (задача 1); `skillrating.Access`, `skillrating.Tier` (срез 2).
- Produces: `arena.NewService(pool *db.Pool) *Service`, `arena.Service.Leaderboard(ctx, skill string, limit int) ([]Row, error)`, `arena.RegisterPublicRoutes(mux *http.ServeMux, s *Service)`. Задача 5 добавляет в тот же `model.go` поле `Challenges` в профиль.

- [ ] **Step 1: Написать падающий тест** `backend/internal/arena/leaderboard_integration_test.go`

```go
package arena_test

import (
	"context"
	"testing"

	"tolerance/internal/arena"
	"tolerance/internal/platform/dbtest"
)

// seedRated creates an agent with a rating on the named skill. onCurrent = false
// means the rating was earned on a version the agent has since replaced, so
// agents.current_version_id points at a newer version than the rating does.
func seedRated(t *testing.T, d *dbtest.DB, name, skill string, rating, uncertainty int, onCurrent, public bool) {
	t.Helper()
	ctx := context.Background()
	userID, agentID := idgen.New("user"), idgen.New("agent")
	mustExec(t, d, `INSERT INTO users (id, email) VALUES ($1, $1 || '@example.com')`, userID)
	mustExec(t, d, `INSERT INTO agents (id, owner_user_id, name, public) VALUES ($1, $2, $3, $4)`,
		agentID, userID, name, public)
	mustExec(t, d, `INSERT INTO skills (slug, title, language, image, run_cmd)
		VALUES ($1, $1, 'go', 'arena-skill-go:1', 'go test -json ./...') ON CONFLICT DO NOTHING`, skill)
	ratedVersion := idgen.New("ver")
	mustExec(t, d, `INSERT INTO agent_versions (id, agent_id, number, model, harness, config_digest)
		VALUES ($1, $2, 1, 'claude-opus-5', 'claude-code 2.1', $1)`, ratedVersion, agentID)
	current := ratedVersion
	if !onCurrent {
		current = idgen.New("ver")
		mustExec(t, d, `INSERT INTO agent_versions (id, agent_id, number, model, harness, config_digest)
			VALUES ($1, $2, 2, 'claude-sonnet-5-5', 'claude-code 2.1', $1)`, current, agentID)
	}
	mustExec(t, d, `UPDATE agents SET current_version_id = $2 WHERE id = $1`, agentID, current)
	mustExec(t, d, `INSERT INTO skill_ratings (agent_id, skill_slug, version_id, rating, uncertainty, runs, sum_targets)
		VALUES ($1, $2, $3, $4, $5, 1, $4)`, agentID, skill, ratedVersion, rating, uncertainty)
	_ = ctx
}

func mustExec(t *testing.T, d *dbtest.DB, sql string, args ...any) {
	t.Helper()
	if _, err := d.AdminPool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("seed (%s): %v", sql, err)
	}
}

func TestLeaderboardOrdersByAccessThenConfirmed(t *testing.T) {
	d := dbtest.New(t)
	seedRated(t, d, "low", "go", 1600, 100, true, true)     // access 1500
	seedRated(t, d, "high", "go", 2000, 100, true, true)    // access 1900
	seedRated(t, d, "stale", "go", 2000, 100, false, true)  // access 1900, not confirmed
	seedRated(t, d, "hidden", "go", 2400, 60, true, false)  // opted out
	seedRated(t, d, "other", "python", 2400, 60, true, true)

	rows, err := arena.NewService(d.AppPool).Leaderboard(context.Background(), "go", 100)
	if err != nil {
		t.Fatalf("leaderboard: %v", err)
	}
	var got []string
	for _, r := range rows {
		got = append(got, r.AgentName)
	}
	// Equal access: the version-confirmed row comes first. An opted-out agent and
	// another skill's agent are absent.
	want := []string{"high", "stale", "low"}
	if len(got) != len(want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rows = %v, want %v", got, want)
		}
	}
	if rows[0].Rank != 1 || rows[2].Rank != 3 {
		t.Errorf("ranks = %d..%d, want 1..3", rows[0].Rank, rows[2].Rank)
	}
	if rows[0].Access != 1900 || rows[0].Tier != "strong" {
		t.Errorf("first row = access %d tier %q, want 1900 strong", rows[0].Access, rows[0].Tier)
	}
	if rows[1].OnCurrentVersion {
		t.Error("the stale row must report on_current_version = false")
	}
}

func TestLeaderboardHidesBannedAgents(t *testing.T) {
	d := dbtest.New(t)
	seedRated(t, d, "fine", "go", 1800, 100, true, true)
	seedRated(t, d, "banned", "go", 2400, 60, true, true)
	if _, err := d.AdminPool.Exec(context.Background(),
		`UPDATE agents SET banned_at = now(), banned_reason = 'test' WHERE name = 'banned'`); err != nil {
		t.Fatal(err)
	}
	rows, err := arena.NewService(d.AppPool).Leaderboard(context.Background(), "go", 100)
	if err != nil {
		t.Fatalf("leaderboard: %v", err)
	}
	if len(rows) != 1 || rows[0].AgentName != "fine" {
		t.Fatalf("rows = %v, want only [fine]", rows)
	}
}

func TestLeaderboardClampsLimit(t *testing.T) {
	d := dbtest.New(t)
	for _, n := range []string{"a", "b", "c"} {
		seedRated(t, d, n, "go", 1800, 100, true, true)
	}
	rows, err := arena.NewService(d.AppPool).Leaderboard(context.Background(), "go", 2)
	if err != nil {
		t.Fatalf("leaderboard: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
}
```

- [ ] **Step 2: Запустить — падает**

Run: `cd backend && go test ./internal/arena/`
Expected: FAIL — пакета `arena` нет.

- [ ] **Step 3: Реализовать** `backend/internal/arena/model.go`

```go
package arena

import "time"

// Row is one line of a skill's public table. Model and harness are the two
// things a reader actually wants to compare, so they travel with every row
// instead of living only on the agent's profile.
type Row struct {
	Rank             int       `json:"rank"`
	AgentName        string    `json:"agent_name"`
	VersionNumber    int       `json:"version_number"`
	Model            string    `json:"model"`
	Harness          string    `json:"harness"`
	Rating           int       `json:"rating"`
	Uncertainty      int       `json:"uncertainty"`
	Access           int       `json:"access"`
	Tier             string    `json:"tier"`
	Runs             int       `json:"runs"`
	OnCurrentVersion bool      `json:"on_current_version"`
	ScoredAt         time.Time `json:"scored_at"`
}
```

- [ ] **Step 4: Реализовать** `backend/internal/arena/leaderboard.go`

```go
package arena

import (
	"context"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
	"tolerance/internal/skillrating"
)

type Service struct{ pool *db.Pool }

func NewService(pool *db.Pool) *Service { return &Service{pool: pool} }

// Leaderboard returns one skill's table, best first. Ordering is
// access = rating − uncertainty descending, and on equal access a rating
// confirmed on the agent's current version outranks one carried over from an
// older configuration — a rating earned by a configuration the owner has since
// replaced is weaker evidence, and the table says so rather than hiding it.
func (s *Service) Leaderboard(ctx context.Context, skill string, limit int) ([]Row, error) {
	out := []Row{}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT a.name, av.number, av.model, av.harness,
				r.rating, r.uncertainty, r.runs, r.updated_at, (r.version_id = a.current_version_id) AS on_current
			FROM skill_ratings r
			JOIN agents a ON a.id = r.agent_id
			JOIN agent_versions av ON av.id = r.version_id
			WHERE r.skill_slug = $1 AND a.public AND a.banned_at IS NULL
			ORDER BY (r.rating - r.uncertainty) DESC, on_current DESC, r.rating DESC, a.name
			LIMIT $2`, skill, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r Row
			if err := rows.Scan(&r.AgentName, &r.VersionNumber, &r.Model, &r.Harness,
				&r.Rating, &r.Uncertainty, &r.Runs, &r.ScoredAt, &r.OnCurrentVersion); err != nil {
				return err
			}
			r.ScoredAt = r.ScoredAt.UTC()
			r.Access = skillrating.Access(r.Rating, r.Uncertainty)
			r.Tier = skillrating.Tier(r.Access)
			r.Rank = len(out) + 1
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}
```

- [ ] **Step 5: Реализовать** `backend/internal/arena/http_public.go` — `parseLimit` копировать не нужно, но и импортировать его из `games` нельзя (разные домены) — написать свою такую же функцию в этом файле, слово в слово как в `games/http_public.go`, с `def = 100`, `max = 500`.

```go
// RegisterPublicRoutes registers the arena's public, unauthenticated reads.
func RegisterPublicRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/leaderboard", func(w http.ResponseWriter, r *http.Request) {
		skill := r.URL.Query().Get("skill")
		if skill == "" {
			httpx.WriteError(w, r, httpx.WithField(http.StatusUnprocessableEntity, "validation_failed",
				"skill is required", "skill", "required"))
			return
		}
		limit, err := parseLimit(r, 100, 500)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		items, err := s.Leaderboard(r.Context(), skill, limit)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items})
	})
}
```

- [ ] **Step 6: Приватность в профиле и в `/skills`.** В `backend/internal/qualifications/http.go` (файл среза 2, где живёт `GET /api/v1/agents/{name}`) добавить в запрос профиля `AND a.public AND a.banned_at IS NULL`, чтобы непубличный агент отдавал `httpx.NotFound()` — 404, а не 403: существование агента тоже приватно. В обработчик `GET /api/v1/skills` добавить два поля на направление:

```go
		pool, err := skills.Pool(ctx, tx, sk.Slug)
		if err != nil {
			return err
		}
		item.PoolSize = len(pool)
		item.Frozen = skills.Frozen(len(pool), skills.MinPool)
```

- [ ] **Step 7: Переключатель публичности.** В `backend/internal/agents/service.go` у входной структуры `PATCH /agent` добавить `Public *bool` (указатель: отсутствие поля не меняет значение), в `UPDATE agents SET ... public = coalesce($n, public)`. В `AgentOverview` добавить `Public bool` — владелец должен видеть текущее состояние.

- [ ] **Step 8: Прошить маршрут и контракт.** В `backend/cmd/api/handler.go`:

```go
	public := http.NewServeMux()
	games.RegisterPublicRoutes(public, d.games)
	arena.RegisterPublicRoutes(public, d.arena)
	...
	api.Handle("/api/v1/leaderboard", public)
```

В `backend/cmd/api/main.go` создать `arena.NewService(pool)` и положить в `deps`. В `openapi.yaml` добавить путь `/leaderboard` (параметры `skill` обязательный, `limit`), схему `LeaderboardRow` и поле `public` у `Agent`, `pool_size`/`frozen` у элемента `/skills`.

- [ ] **Step 9: e2e** в `backend/cmd/api/main_test.go`:

```go
func TestArenaLeaderboardPublic(t *testing.T) {
	e := newE2E(t)
	// No session, no API key: a reader lands on the table straight from the landing page.
	var body struct{ Items []arena.Row }
	e.getJSON(t, "/api/v1/leaderboard?skill=go", &body) // 200, validated against openapi.yaml
	if body.Items == nil {
		t.Fatal("items must be [] rather than null on an empty arena")
	}
	e.get(t, "/api/v1/leaderboard", http.StatusUnprocessableEntity)       // skill missing
	e.get(t, "/api/v1/leaderboard?skill=go&limit=0", http.StatusUnprocessableEntity)
}
```

- [ ] **Step 10: Прогнать и закоммитить**

Run: `cd backend && go test -race ./internal/arena/ ./internal/agents/ ./internal/qualifications/ ./cmd/api/ && go vet ./... && gofmt -l .`
Expected: PASS, `gofmt` пусто.

```bash
git add backend/internal/arena backend/internal/agents backend/internal/qualifications backend/cmd/api backend/contracts/openapi/openapi.yaml
git commit -m "Publish a per-skill agent leaderboard"
```

---

### Task 3: Админское API: вывод задачи, наблюдаемая сложность, аннулирование прогона, бан

`identity.RequireAdmin` уже существует (`backend/internal/identity/middleware.go`) — писать его не надо, надо только прошить. UI администратора в этот срез не входит: маршруты вызываются `curl` с сессионной cookie.

**Files:**
- Create: `backend/migrations/00010_admin.sql`, `backend/internal/admin/service.go`, `backend/internal/admin/http.go`, `backend/internal/admin/admin_integration_test.go`
- Modify: `backend/internal/skillrating/rating.go` (+`Remove`), `backend/internal/skillrating/rating_test.go`, `backend/internal/skills/pool.go` (+`Retire`, `TaskStats`), `backend/cmd/api/handler.go`, `backend/cmd/api/main.go`, `backend/contracts/openapi/openapi.yaml`, `backend/cmd/api/main_test.go`

**Interfaces:**
- Consumes: `skills.RecordExposure` не нужен; нужны `skills.Pool` (задача 1), `skillrating.State/Uncertainty/Target` (срез 2), `audit.Record`, `identity.RequireAdmin`.
- Produces: `skillrating.Remove(s State, target int) State`; `skills.Retire(ctx, tx, slug, reason string) error`; `skills.TaskStats(ctx, tx, skill string) ([]TaskStat, error)`; `admin.NewService(pool *db.Pool) *Service` с методами `RetireTask`, `TaskStats`, `VoidRun`, `BanAgent`, `UnbanAgent`; `admin.RegisterRoutes(mux *http.ServeMux, s *Service)`.

- [ ] **Step 1: Написать миграцию** `backend/migrations/00010_admin.sql`

```sql
-- +goose Up
-- A voided run is the one operation that edits the past, so it is named in the
-- status rather than hidden in a flag, and it carries its reason.
ALTER TABLE qualification_runs DROP CONSTRAINT qualification_runs_status_check;
ALTER TABLE qualification_runs ADD CONSTRAINT qualification_runs_status_check
    CHECK (status IN ('running', 'scored', 'aborted', 'voided'));
ALTER TABLE qualification_runs
    ADD COLUMN voided_at timestamptz,
    ADD COLUMN voided_reason text NOT NULL DEFAULT '';

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO arena_app;
-- Voiding, retiring and banning all run in the API as arena_app; the worker
-- gets nothing new here. The partial unique index on running runs already
-- ignores any other status, so 'voided' needs no index change.

-- +goose Down
ALTER TABLE qualification_runs DROP COLUMN voided_at, DROP COLUMN voided_reason;
UPDATE qualification_runs SET status = 'aborted' WHERE status = 'voided';
ALTER TABLE qualification_runs DROP CONSTRAINT qualification_runs_status_check;
ALTER TABLE qualification_runs ADD CONSTRAINT qualification_runs_status_check
    CHECK (status IN ('running', 'scored', 'aborted'));
```

Если фактическое имя ограничения другое, узнать его: `select conname from pg_constraint where conrelid = 'qualification_runs'::regclass;` — и подставить.

- [ ] **Step 2: Написать падающий тест обратной функции рейтинга** в `backend/internal/skillrating/rating_test.go`

```go
func TestRemoveIsInverseOfApply(t *testing.T) {
	prior := 1700
	for _, st := range []State{
		{Uncertainty: MaxUncert},
		{Uncertainty: MaxUncert, Prior: &prior},
	} {
		base := st
		for _, scores := range [][]float64{{0.5}, {0.5, 1}, {0, 0.25, 0.75}} {
			s := base
			for _, sc := range scores {
				s = Apply(s, sc)
			}
			last := Target(scores[len(scores)-1])
			got := Remove(s, last)
			want := base
			for _, sc := range scores[:len(scores)-1] {
				want = Apply(want, sc)
			}
			if got.Runs != want.Runs || got.SumTargets != want.SumTargets ||
				got.Rating != want.Rating || got.Uncertainty != want.Uncertainty {
				t.Errorf("Remove after %v = %+v, want %+v", scores, got, want)
			}
		}
	}
}

func TestRemoveLastRunFallsBackToPrior(t *testing.T) {
	prior := 1900
	s := Apply(State{Uncertainty: MaxUncert, Prior: &prior}, 0.1)
	got := Remove(s, Target(0.1))
	if got.Runs != 0 || got.Rating != prior || got.Uncertainty != MaxUncert {
		t.Fatalf("Remove of the only run = %+v, want runs 0, rating %d, uncertainty %d", got, prior, MaxUncert)
	}
}

func TestRemoveOnEmptyStateIsNoop(t *testing.T) {
	s := State{Rating: 1500, Uncertainty: MaxUncert}
	if got := Remove(s, 1800); got != s {
		t.Fatalf("Remove on zero runs changed the state: %+v", got)
	}
}
```

- [ ] **Step 3: Запустить — падает**

Run: `cd backend && go test ./internal/skillrating/`
Expected: FAIL — `undefined: Remove`.

- [ ] **Step 4: Реализовать** `Remove` в `backend/internal/skillrating/rating.go`

```go
// Remove takes one scored run back out of the state — the exact inverse of
// Apply, for an administrator voiding a run. This is the only operation on the
// platform that edits a rating after the fact, which is why it is a named,
// tested function and not an ad-hoc UPDATE. Removing the last run on a version
// leaves the previous version's rating standing with uncertainty at maximum,
// the same state NewVersion produces.
func Remove(s State, target int) State {
	if s.Runs <= 0 {
		return s
	}
	s.Runs--
	s.SumTargets -= target
	if s.Runs == 0 {
		s.SumTargets = 0
		if s.Prior != nil {
			s.Rating = *s.Prior
		}
		s.Uncertainty = MaxUncert
		return s
	}
	s.Rating = int(math.Round(float64(s.SumTargets) / float64(s.Runs)))
	if s.Runs == 1 && s.Prior != nil {
		s.Rating = int(math.Round(float64(*s.Prior+s.SumTargets) / 2))
	}
	s.Uncertainty = Uncertainty(s.Runs)
	return s
}
```

- [ ] **Step 5: Дописать `skills.Retire` и `skills.TaskStats`** в `backend/internal/skills/pool.go`

```go
// TaskStat is what an administrator needs to judge a task: how far it has
// leaked and how hard it turned out to be, next to the difficulty someone
// typed by hand. A big gap between them is the signal to re-weight — by
// editing difficulty, which affects only future runs, never past ratings.
type TaskStat struct {
	Slug          string     `json:"slug"`
	Title         string     `json:"title"`
	Difficulty    int        `json:"difficulty"`
	Exposures     int        `json:"exposures"`
	Runs          int        `json:"runs"`
	AvgScore      *float64   `json:"avg_score"`
	Active        bool       `json:"active"`
	ChallengeOnly bool       `json:"challenge_only"`
	FirstUsedAt   *time.Time `json:"first_used_at"`
	RetiredAt     *time.Time `json:"retired_at"`
	RetiredReason string     `json:"retired_reason"`
}

// Retire takes a task out of the pool by hand. Already scored results keep
// counting: a retired task never changes a rating that was earned on it.
func Retire(ctx context.Context, tx pgx.Tx, slug, reason string) error {
	tag, err := tx.Exec(ctx, `UPDATE skill_tasks SET retired_at = coalesce(retired_at, now()),
		retired_reason = CASE WHEN retired_at IS NULL THEN $2 ELSE retired_reason END
		WHERE slug = $1`, slug, reason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound()
	}
	return nil
}

// TaskStats lists one skill's catalog with exposure and observed difficulty.
func TaskStats(ctx context.Context, tx pgx.Tx, skill string) ([]TaskStat, error) {
	rows, err := tx.Query(ctx, `SELECT slug, title, difficulty, exposures, runs,
			CASE WHEN runs > 0 THEN (sum_score / runs)::float8 ELSE NULL END,
			active, challenge_only, first_used_at, retired_at, retired_reason
		FROM skill_tasks WHERE skill_slug = $1 ORDER BY slug`, skill)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TaskStat{}
	for rows.Next() {
		var s TaskStat
		if err := rows.Scan(&s.Slug, &s.Title, &s.Difficulty, &s.Exposures, &s.Runs, &s.AvgScore,
			&s.Active, &s.ChallengeOnly, &s.FirstUsedAt, &s.RetiredAt, &s.RetiredReason); err != nil {
			return nil, err
		}
		s.FirstUsedAt, s.RetiredAt = utcp(s.FirstUsedAt), utcp(s.RetiredAt)
		out = append(out, s)
	}
	return out, rows.Err()
}

// utcp normalizes a nullable timestamp; pgx hands back time.Local.
func utcp(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
```

Накопление `runs`/`sum_score` — в `qualifications.scoreTx` (срез 2), в той же транзакции, что считает балл прогона: для каждого proof-а прогона взять его долю пройденных скрытых тестов (ту же, из которой складывается балл прогона) и выполнить

```go
	if _, err := tx.Exec(ctx, `UPDATE skill_tasks SET runs = runs + 1, sum_score = sum_score + $2 WHERE slug = $1`,
		p.SkillTaskSlug, taskScore); err != nil {
		return err
	}
```

Proof со статусом `infra_error` пропускается: сбой платформы не говорит ничего о сложности задачи.

- [ ] **Step 6: Написать падающие тесты админских операций** `backend/internal/admin/admin_integration_test.go`

```go
func TestVoidRunSubtractsItFromTheRating(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	env := seedScoredRuns(t, d, 2) // two scored runs on one version, skill "go"
	before := readRating(t, d, env.agentID, "go")

	svc := admin.NewService(d.AppPool)
	if err := svc.VoidRun(ctx, env.adminUserID, env.runIDs[1], "hand of a human in the log"); err != nil {
		t.Fatalf("void: %v", err)
	}

	after := readRating(t, d, env.agentID, "go")
	if after.Runs != before.Runs-1 {
		t.Fatalf("runs = %d, want %d", after.Runs, before.Runs-1)
	}
	if after.Uncertainty <= before.Uncertainty {
		t.Error("voiding a run must raise uncertainty, not lower it")
	}
	if status := readRunStatus(t, d, env.runIDs[1]); status != "voided" {
		t.Fatalf("run status = %q, want voided", status)
	}
	if !auditHas(t, d, "qualification_run", env.runIDs[1], "admin.run_voided") {
		t.Error("voiding must leave an audit event")
	}
}

func TestVoidRunTwiceIsRefused(t *testing.T) {
	d := dbtest.New(t)
	env := seedScoredRuns(t, d, 2)
	svc := admin.NewService(d.AppPool)
	if err := svc.VoidRun(context.Background(), env.adminUserID, env.runIDs[0], "first"); err != nil {
		t.Fatalf("void: %v", err)
	}
	err := svc.VoidRun(context.Background(), env.adminUserID, env.runIDs[0], "again")
	assertProblem(t, err, http.StatusConflict, "already_voided")
}

func TestRetireTaskKeepsPastResults(t *testing.T) {
	d := dbtest.New(t)
	env := seedScoredRuns(t, d, 1)
	before := readRating(t, d, env.agentID, "go")
	if err := admin.NewService(d.AppPool).RetireTask(context.Background(), env.adminUserID,
		env.taskSlugs[0], "leaked on a forum"); err != nil {
		t.Fatalf("retire: %v", err)
	}
	if after := readRating(t, d, env.agentID, "go"); after != before {
		t.Fatalf("rating changed on retire: %+v -> %+v", before, after)
	}
}

func TestBanAgentHidesItAndStopsRuns(t *testing.T) {
	d := dbtest.New(t)
	env := seedScoredRuns(t, d, 1)
	if err := admin.NewService(d.AppPool).BanAgent(context.Background(), env.adminUserID,
		env.agentID, "human in the loop"); err != nil {
		t.Fatalf("ban: %v", err)
	}
	rows, err := arena.NewService(d.AppPool).Leaderboard(context.Background(), "go", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("banned agent still on the leaderboard: %v", rows)
	}
}
```

- [ ] **Step 7: Запустить — падают. Step 8: реализовать** `admin/service.go` и `admin/http.go`.

`VoidRun` в одной транзакции: `SELECT ... FOR UPDATE` прогона (409 `already_voided`, если статус уже `voided`; 409 `run_not_scored`, если `running`), вычисление `target := skillrating.Target(run.Score)`, загрузка состояния из `skill_ratings` **только если `version_id` совпадает** с версией прогона (иначе рейтинг уже переехал на новую версию, и вычитать нечего — тогда прогон просто помечается, а рейтинг не трогается), `skillrating.Remove`, `UPDATE skill_ratings`, `UPDATE qualification_runs SET status='voided', voided_at=now(), voided_reason=$2`, `audit.Record` с `Action: "admin.run_voided"`, `Reason: reason`. `BanAgent`/`UnbanAgent` — `UPDATE agents SET banned_at`/`NULL` + аудит. `RetireTask` — `skills.Retire` + аудит с `Action: "admin.task_retired"`.

Маршруты в `admin/http.go` (все за `RequireAdmin`, все тела через `httpx.ReadBody`/`httpx.Decode`, непустой `reason` обязателен — иначе 422 `validation_failed` на поле `reason`):

```go
func RegisterRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/admin/skill-tasks", ...)                     // ?skill=go
	mux.HandleFunc("POST /api/v1/admin/skill-tasks/{slug}/retire", ...)
	mux.HandleFunc("POST /api/v1/admin/qualifications/{id}/void", ...)
	mux.HandleFunc("POST /api/v1/admin/agents/{id}/ban", ...)
	mux.HandleFunc("POST /api/v1/admin/agents/{id}/unban", ...)
}
```

- [ ] **Step 9: Прошить в `cmd/api/handler.go`**

```go
	adminMux := http.NewServeMux()
	admin.RegisterRoutes(adminMux, d.admin)
	...
	// RequireAdmin reads the session actor, so it sits inside session(), not outside.
	api.Handle("/api/v1/admin/", session(identity.RequireAdmin(adminMux)))
```

- [ ] **Step 10: e2e** в `main_test.go`: `TestAdminRoutesRefuseNonAdmin` — обычная сессия на каждый из пяти маршрутов даёт 403 `forbidden` (тело валидируется по контракту), и в `audit_events` не появилось ни одной записи; после `UPDATE users SET role='admin'` тот же вызов проходит. Добавить пути и схемы в `openapi.yaml`.

- [ ] **Step 11: Прогнать и закоммитить**

Run: `cd backend && go test -race ./internal/skillrating/ ./internal/skills/ ./internal/admin/ ./cmd/api/ && go vet ./... && gofmt -l .`

```bash
git add backend/migrations/00010_admin.sql backend/internal/admin backend/internal/skillrating backend/internal/skills backend/cmd/api backend/contracts/openapi/openapi.yaml
git commit -m "Add the admin API for tasks, runs and agents"
```

---

### Task 4: Челлендж: схема, ранжирование, вход

**Files:**
- Create: `backend/migrations/00011_challenges.sql`, `backend/internal/challenges/model.go`, `backend/internal/challenges/rank.go`, `backend/internal/challenges/rank_test.go`, `backend/internal/challenges/service.go`, `backend/internal/challenges/http_me.go`, `backend/internal/challenges/challenges_integration_test.go`
- Modify: `backend/internal/proofs/service.go` (+`CreateChallengeProof`), `backend/cmd/api/handler.go`, `backend/cmd/api/main.go`

**Interfaces:**
- Consumes: `skillrating.Tier`, `skillrating.Access` (срез 2); `proofs.Service` и инвариант «одна открытая проверка на агента» (индекс `proofs_one_open_idx`); `audit.Record`.
- Produces: `challenges.Rank(rs []Result) []Result`; `challenges.NewService(pool *db.Pool, ps *proofs.Service) *Service` с `Create`, `Enter`, `Get`, `List`, `MyEntries`; `proofs.Service.CreateChallengeProof(ctx, tx, agentID, challengeID, taskSlug string) (Proof, error)`. Задача 5 добавляет `Open`, `Close`, `Publish`, `Tick`.

- [ ] **Step 1: Написать миграцию** `backend/migrations/00011_challenges.sql`

```sql
-- +goose Up
CREATE TABLE challenges (
    id text PRIMARY KEY,
    slug text NOT NULL UNIQUE,
    title text NOT NULL,
    summary text NOT NULL DEFAULT '',
    skill_task_slug text NOT NULL REFERENCES skill_tasks (slug),
    min_tier text NOT NULL DEFAULT 'verified' CHECK (min_tier IN ('none', 'verified', 'strong', 'elite')),
    opens_at timestamptz NOT NULL,
    closes_at timestamptz NOT NULL,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'open', 'closed', 'published')),
    prizes text NOT NULL DEFAULT '',
    publish_tests boolean NOT NULL DEFAULT true,
    payout_note text NOT NULL DEFAULT '',
    created_by text NOT NULL REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (closes_at > opens_at)
);
CREATE INDEX challenges_status_idx ON challenges (status, closes_at DESC);

-- One row per (challenge, agent): the primary key is what makes "one attempt"
-- an invariant of the schema rather than a check someone can forget.
CREATE TABLE challenge_entries (
    challenge_id text NOT NULL REFERENCES challenges (id),
    agent_id text NOT NULL REFERENCES agents (id),
    version_id text NOT NULL REFERENCES agent_versions (id),
    proof_id text NOT NULL REFERENCES proofs (id),
    consent_publish boolean NOT NULL DEFAULT true,
    score numeric(5,4),
    diff_lines int,
    submitted_at timestamptz,
    rank int,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (challenge_id, agent_id)
);
CREATE INDEX challenge_entries_proof_idx ON challenge_entries (proof_id);

ALTER TABLE proofs DROP CONSTRAINT proofs_kind_check;
ALTER TABLE proofs ADD CONSTRAINT proofs_kind_check
    CHECK (kind IN ('proof', 'game_bot', 'qualification', 'challenge'));
ALTER TABLE proofs ADD COLUMN challenge_id text REFERENCES challenges (id);
ALTER TABLE proofs DROP CONSTRAINT proofs_task_ref;
ALTER TABLE proofs ADD CONSTRAINT proofs_task_ref CHECK (
       (kind IN ('proof', 'game_bot') AND task_slug IS NOT NULL)
    OR (kind = 'qualification' AND skill_task_slug IS NOT NULL AND qualification_run_id IS NOT NULL AND position BETWEEN 1 AND 3)
    OR (kind = 'challenge' AND skill_task_slug IS NOT NULL AND challenge_id IS NOT NULL));

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO arena_app;
-- The worker opens and closes challenges on time and writes entry scores and
-- ranks; it never creates a challenge or an entry.
GRANT SELECT, UPDATE ON challenges TO arena_worker;
GRANT SELECT, UPDATE ON challenge_entries TO arena_worker;

-- +goose Down
REVOKE ALL ON challenges, challenge_entries FROM arena_worker;
DELETE FROM proofs WHERE kind = 'challenge';
ALTER TABLE proofs DROP CONSTRAINT proofs_task_ref;
ALTER TABLE proofs ADD CONSTRAINT proofs_task_ref CHECK (
       (kind IN ('proof', 'game_bot') AND task_slug IS NOT NULL)
    OR (kind = 'qualification' AND skill_task_slug IS NOT NULL AND qualification_run_id IS NOT NULL AND position BETWEEN 1 AND 3));
ALTER TABLE proofs DROP COLUMN challenge_id;
ALTER TABLE proofs DROP CONSTRAINT proofs_kind_check;
ALTER TABLE proofs ADD CONSTRAINT proofs_kind_check CHECK (kind IN ('proof', 'game_bot', 'qualification'));
DROP TABLE challenge_entries;
DROP TABLE challenges;
```

- [ ] **Step 2: Написать падающий тест ранжирования** `backend/internal/challenges/rank_test.go`

```go
package challenges

import (
	"testing"
	"time"
)

func at(min int) time.Time { return time.Date(2026, 10, 1, 12, min, 0, 0, time.UTC) }

func TestRankOrdersByScoreThenDiffThenTime(t *testing.T) {
	in := []Result{
		{AgentName: "slow-but-right", Score: 1, DiffLines: 40, SubmittedAt: at(50)},
		{AgentName: "tidy", Score: 1, DiffLines: 12, SubmittedAt: at(55)},
		{AgentName: "early-tie", Score: 1, DiffLines: 12, SubmittedAt: at(30)},
		{AgentName: "partial", Score: 0.5, DiffLines: 3, SubmittedAt: at(10)},
		{AgentName: "zero", Score: 0, DiffLines: 0, SubmittedAt: at(5)},
	}
	got := Rank(in)
	want := []struct {
		name string
		rank int
	}{
		{"early-tie", 1}, {"tidy", 2}, {"slow-but-right", 3}, {"partial", 4}, {"zero", 5},
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].AgentName != w.name || got[i].Rank != w.rank {
			t.Fatalf("position %d = %s/%d, want %s/%d", i, got[i].AgentName, got[i].Rank, w.name, w.rank)
		}
	}
}

func TestRankSharesAPlaceOnAFullTie(t *testing.T) {
	in := []Result{
		{AgentName: "a", Score: 1, DiffLines: 10, SubmittedAt: at(20)},
		{AgentName: "b", Score: 1, DiffLines: 10, SubmittedAt: at(20)},
		{AgentName: "c", Score: 0.9, DiffLines: 5, SubmittedAt: at(1)},
	}
	got := Rank(in)
	// Standard competition ranking: two firsts, then third. Sharing a place is
	// what the spec says happens to entries equal on every key, and the next
	// place is skipped so "third" still means "two ahead of you".
	if got[0].Rank != 1 || got[1].Rank != 1 || got[2].Rank != 3 {
		t.Fatalf("ranks = %d, %d, %d, want 1, 1, 3", got[0].Rank, got[1].Rank, got[2].Rank)
	}
}

func TestRankEmpty(t *testing.T) {
	if got := Rank(nil); len(got) != 0 {
		t.Fatalf("Rank(nil) = %v, want empty", got)
	}
}
```

- [ ] **Step 3: Запустить — падает.** Run: `cd backend && go test ./internal/challenges/` → FAIL, нет пакета.

- [ ] **Step 4: Реализовать** `backend/internal/challenges/rank.go`

```go
package challenges

import (
	"sort"
	"time"
)

// Result is one entry as ranking sees it. AgentName travels along so a closed
// challenge's table stays readable even for an agent that later went private.
type Result struct {
	AgentName   string    `json:"agent_name"`
	Score       float64   `json:"score"`
	DiffLines   int       `json:"diff_lines"`
	SubmittedAt time.Time `json:"submitted_at"`
	Rank        int       `json:"rank"`
}

// Rank sorts entries into places: more hidden tests passed first, then a
// smaller diff, then an earlier submission. Entries equal on all three share a
// place and the following places are skipped, so a place always means "this
// many ahead of you". No judge, no code-quality score — every key is one a
// participant can check for themselves.
func Rank(rs []Result) []Result {
	out := make([]Result, len(rs))
	copy(out, rs)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.DiffLines != b.DiffLines {
			return a.DiffLines < b.DiffLines
		}
		return a.SubmittedAt.Before(b.SubmittedAt)
	})
	for i := range out {
		if i > 0 && out[i].Score == out[i-1].Score && out[i].DiffLines == out[i-1].DiffLines &&
			out[i].SubmittedAt.Equal(out[i-1].SubmittedAt) {
			out[i].Rank = out[i-1].Rank
			continue
		}
		out[i].Rank = i + 1
	}
	return out
}
```

- [ ] **Step 5: Написать падающие тесты входа** `backend/internal/challenges/challenges_integration_test.go` (включая Review Focus 5)

```go
func TestEnterRequiresTheMinimumTier(t *testing.T) {
	d := dbtest.New(t)
	env := newChallengeEnv(t, d, "verified")       // an open challenge on a go task
	env.rate(t, env.agentID, "go", 1600, 350)      // access 1250 — below verified
	_, err := env.svc.Enter(context.Background(), env.userID, env.slug, true)
	assertProblem(t, err, http.StatusForbidden, "tier_too_low")

	env.rate(t, env.agentID, "go", 2000, 100)      // access 1900
	if _, err := env.svc.Enter(context.Background(), env.userID, env.slug, true); err != nil {
		t.Fatalf("enter: %v", err)
	}
}

func TestEnterTwiceIsRefused(t *testing.T) {
	d := dbtest.New(t)
	env := newChallengeEnv(t, d, "none")
	if _, err := env.svc.Enter(context.Background(), env.userID, env.slug, true); err != nil {
		t.Fatalf("enter: %v", err)
	}
	env.finishProof(t, env.openProofID(t), "passed") // free the one-open-proof slot
	_, err := env.svc.Enter(context.Background(), env.userID, env.slug, true)
	assertProblem(t, err, http.StatusConflict, "already_entered")
}

func TestEnterUsesTheTierOfTheCurrentVersion(t *testing.T) {
	d := dbtest.New(t)
	env := newChallengeEnv(t, d, "verified")
	env.rate(t, env.agentID, "go", 2000, 100) // earned on version 1
	env.newVersion(t, env.agentID)            // owner switched model: rating is now stale
	_, err := env.svc.Enter(context.Background(), env.userID, env.slug, true)
	// A rating earned by a configuration the owner has since replaced must not
	// open the door for the new one.
	assertProblem(t, err, http.StatusForbidden, "tier_too_low")
}

func TestBannedAgentCannotEnter(t *testing.T) {
	d := dbtest.New(t)
	env := newChallengeEnv(t, d, "none")
	if _, err := d.AdminPool.Exec(context.Background(),
		`UPDATE agents SET banned_at = now() WHERE id = $1`, env.agentID); err != nil {
		t.Fatal(err)
	}
	_, err := env.svc.Enter(context.Background(), env.userID, env.slug, true)
	assertProblem(t, err, http.StatusForbidden, "agent_banned")
}

func TestTwoAgentsOfOneOwnerMayBothEnter(t *testing.T) {
	d := dbtest.New(t)
	env := newChallengeEnv(t, d, "none")
	// One agent per owner is a slice 4 limit, not a challenge rule: the entry key
	// is (challenge, agent), so the code must not assume one entry per user.
	second := env.addAgent(t, env.userID)
	if _, err := env.svc.Enter(context.Background(), env.userID, env.slug, true); err != nil {
		t.Fatalf("first agent: %v", err)
	}
	if err := env.enterAs(t, second); err != nil {
		t.Fatalf("second agent of the same owner: %v", err)
	}
}

func TestChallengeDoesNotChangeTheSkillRating(t *testing.T) {
	d := dbtest.New(t)
	env := newChallengeEnv(t, d, "none")
	env.rate(t, env.agentID, "go", 2000, 100)
	before := readRating(t, d, env.agentID, "go")
	if _, err := env.svc.Enter(context.Background(), env.userID, env.slug, true); err != nil {
		t.Fatalf("enter: %v", err)
	}
	env.finishProof(t, env.openProofID(t), "passed")
	// A challenge is one task that gets published afterwards; letting it weigh as
	// much as a run over a rotated pool would let a well-studied task pump the
	// rating. Places and the public page are the whole reward.
	if after := readRating(t, d, env.agentID, "go"); after != before {
		t.Fatalf("rating moved on a challenge: %+v -> %+v", before, after)
	}
}

func TestEnterBeforeOpenAndAfterClose(t *testing.T) {
	d := dbtest.New(t)
	for _, status := range []string{"draft", "closed", "published"} {
		env := newChallengeEnvWithStatus(t, d, "none", status)
		_, err := env.svc.Enter(context.Background(), env.userID, env.slug, true)
		assertProblem(t, err, http.StatusConflict, "challenge_not_open")
	}
}
```

- [ ] **Step 6: Запустить — падают. Step 7: реализовать** `model.go`, `service.go`, `http_me.go` и `proofs.CreateChallengeProof`.

`CreateChallengeProof` рядом с `CreateQualificationProof`, но **без** `skills.RecordExposure`: задача челленджа не в пуле квалификации, её экспозиция ничего не решает, а после публикации она вообще открыта.

```go
func (s *Service) CreateChallengeProof(ctx context.Context, tx pgx.Tx, agentID, challengeID, taskSlug string) (Proof, error) {
	var p Proof
	err := scanProof(tx.QueryRow(ctx, `INSERT INTO proofs (id, agent_id, kind, challenge_id, skill_task_slug)
		VALUES ($1, $2, 'challenge', $3, $4) RETURNING `+proofCols,
		idgen.New("proof"), agentID, challengeID, taskSlug), &p)
	return p, err
}
```

`Enter` в одной транзакции: агент владельца (404, если его нет), `SELECT ... FOR UPDATE` челленджа по slug, `status = 'open'` иначе 409 `challenge_not_open`, `banned_at IS NULL` иначе 403 `agent_banned`, `current_version_id` не NULL иначе 409 `no_version`, presence онлайн иначе 409 `agent_offline`, уровень: прочитать `skill_ratings` по направлению задачи челленджа и потребовать `version_id = current_version_id` и `skillrating.Tier(Access(...)) >= min_tier` (порядок уровней сравнивать через маленькую таблицу `tierRank`), иначе 403 `tier_too_low`; `INSERT INTO challenge_entries` (нарушение PK → 409 `already_entered`); `CreateChallengeProof`; `audit.Record` с `Action: "challenge.entered"`. Инвариант «одна открытая проверка на агента» соблюдается сам: proof входа попадает под тот же индекс `proofs_one_open_idx`, и попытка войти во время квалификации даёт 409 — это ожидаемо и проверяется тестом `TestEnterTwiceIsRefused`, который освобождает слот перед вторым входом.

`http_me.go`: `POST /api/v1/challenges/{slug}/enter` (тело `{consent_publish: bool}`, по умолчанию `true`, если поле отсутствует) и `GET /api/v1/me/challenges`.

- [ ] **Step 8: Прогнать и закоммитить**

Run: `cd backend && go test -race ./internal/challenges/ ./internal/proofs/ && go vet ./... && gofmt -l .`

```bash
git add backend/migrations/00011_challenges.sql backend/internal/challenges backend/internal/proofs backend/cmd/api
git commit -m "Add challenges and the entry path"
```

---

### Task 5: Челлендж: воркер по времени, закрытие с местами, публикация, публичные страницы API

**Files:**
- Create: `backend/internal/challenges/worker.go`, `backend/internal/challenges/http_public.go`, `backend/internal/challenges/http_admin.go`
- Modify: `backend/internal/challenges/service.go` (+`Open`, `Close`, `Publish`, `OnProofFinished`), `backend/internal/challenges/challenges_integration_test.go`, `backend/internal/admin/http.go` (маршруты челленджа), `backend/cmd/api/main.go` (тик), `backend/cmd/api/handler.go`, `backend/contracts/openapi/openapi.yaml`, `backend/cmd/api/main_test.go`, `backend/internal/qualifications/http.go` (профиль отдаёт места)

**Interfaces:**
- Consumes: `challenges.Rank` (задача 4), `proofs.Proof.SandboxResult` для доли пройденных скрытых тестов, `proofs.Service.WaitForProof` не нужен.
- Produces: `challenges.Service.OnProofFinished(ctx, proofID string) error` — вызывается из воркера proofs рядом с `qualifications.OnProofFinished`; `challenges.Service.Tick(ctx) error`; `challenges.Service.Public(ctx, slug string) (PublicView, error)`; `challenges.Service.List(ctx) (Lists, error)`.
- Типы в `model.go`: `Standing` — это `Result` (задача 4) плюс `Diff string` с тегом `json:"diff"`; `PublicView{Slug, Title, Summary, Status, SkillSlug, MinTier, OpensAt, ClosesAt, Prizes, Entrants int, TaskMD string, HiddenTests []string, Standings []Standing}`; `Lists{Open, Upcoming, Past []Summary}`.

**Помощник тестов.** `newChallengeEnv(t, d, minTier)` создаёт: администратора (`users.role = 'admin'`, поле `adminUserID`), владельца с агентом онлайн и текущей версией (`userID`, `agentID`), направление `go` с задачей `challenge_only = true`, открытый челлендж на ней (`slug`), и сервис (`svc`). Методы: `newChallengeEnvWithStatus(t, d, minTier, status)`; `rate(t, agentID, skill, rating, uncertainty)`; `newVersion(t, agentID)`; `addAgent(t, userID) string`; `enterAgent(t, name) string` — создаёт агента с достаточным уровнем и входит им, возвращает `agentID`; `enterAgentWithConsent(t, name, consent)`; `enterAs(t, agentID) error`; `score(t, agentID, score, diffLines)` — доводит proof входа до `passed` с нужной долей скрытых тестов и вызывает `OnProofFinished`; `openProofID(t) string`; `proofStatus(t, agentID) string`; `status(t) string`; `setWindow(t, opensIn, closesIn time.Duration)`.

- [ ] **Step 1: Написать падающие тесты** (Review Focus 3 и 4) в `backend/internal/challenges/challenges_integration_test.go`

```go
func TestCloseRanksEntriesAndKeepsTheirNames(t *testing.T) {
	d := dbtest.New(t)
	env := newChallengeEnv(t, d, "none")
	a := env.enterAgent(t, "winner")
	b := env.enterAgent(t, "runner-up")
	env.score(t, a, 1.0, 12)  // all hidden tests, 12 diff lines
	env.score(t, b, 1.0, 80)

	// The winner goes private after submitting.
	if _, err := d.AdminPool.Exec(context.Background(),
		`UPDATE agents SET public = false WHERE id = $1`, a); err != nil {
		t.Fatal(err)
	}
	if err := env.svc.Close(context.Background(), env.adminUserID, env.slug); err != nil {
		t.Fatalf("close: %v", err)
	}
	view, err := env.svc.Public(context.Background(), env.slug)
	if err != nil {
		t.Fatalf("public: %v", err)
	}
	if len(view.Standings) != 2 {
		t.Fatalf("standings = %d entries, want 2: an agent that opted out of the arena tables still holds its place here", len(view.Standings))
	}
	if view.Standings[0].AgentName != "winner" || view.Standings[0].Rank != 1 {
		t.Fatalf("first place = %s/%d, want winner/1", view.Standings[0].AgentName, view.Standings[0].Rank)
	}
}

func TestCloseScoresAnUnfinishedEntryAsZero(t *testing.T) {
	d := dbtest.New(t)
	env := newChallengeEnv(t, d, "none")
	done := env.enterAgent(t, "done")
	stuck := env.enterAgent(t, "still-running") // proof left in running_agent
	env.score(t, done, 0.25, 5)

	if err := env.svc.Close(context.Background(), env.adminUserID, env.slug); err != nil {
		t.Fatalf("close: %v", err)
	}
	view, err := env.svc.Public(context.Background(), env.slug)
	if err != nil {
		t.Fatal(err)
	}
	last := view.Standings[len(view.Standings)-1]
	if last.AgentName != "still-running" || last.Rank != 2 || last.Score != 0 {
		t.Fatalf("unfinished entry = %+v, want still-running ranked last with score 0", last)
	}
	// Its proof must not be left open forever either.
	if st := env.proofStatus(t, stuck); st != "expired" {
		t.Fatalf("proof of an unfinished entry = %q, want expired", st)
	}
}

func TestTickOpensAndClosesOnTime(t *testing.T) {
	d := dbtest.New(t)
	env := newChallengeEnvWithStatus(t, d, "none", "draft")
	env.setWindow(t, -time.Hour, time.Hour) // opened an hour ago, closes in an hour
	if err := env.svc.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if st := env.status(t); st != "open" {
		t.Fatalf("status = %q, want open", st)
	}
	env.setWindow(t, -2*time.Hour, -time.Hour) // closed an hour ago
	if err := env.svc.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if st := env.status(t); st != "closed" {
		t.Fatalf("status = %q, want closed", st)
	}
}

func TestPublicHidesTheTaskUntilPublished(t *testing.T) {
	d := dbtest.New(t)
	env := newChallengeEnv(t, d, "none")
	a := env.enterAgent(t, "one")
	env.score(t, a, 1.0, 9)

	open, err := env.svc.Public(context.Background(), env.slug)
	if err != nil {
		t.Fatal(err)
	}
	if open.TaskMD != "" || len(open.HiddenTests) != 0 || len(open.Standings) != 0 {
		t.Fatal("an open challenge must not leak the task, the tests or the standings")
	}
	if open.Entrants != 1 {
		t.Errorf("entrants = %d, want 1", open.Entrants)
	}

	if err := env.svc.Close(context.Background(), env.adminUserID, env.slug); err != nil {
		t.Fatal(err)
	}
	closed, _ := env.svc.Public(context.Background(), env.slug)
	if closed.TaskMD != "" {
		t.Error("a closed but unpublished challenge still hides the task")
	}
	if len(closed.Standings) != 1 {
		t.Error("a closed challenge shows its standings")
	}

	if err := env.svc.Publish(context.Background(), env.adminUserID, env.slug); err != nil {
		t.Fatal(err)
	}
	pub, _ := env.svc.Public(context.Background(), env.slug)
	if pub.TaskMD == "" || len(pub.HiddenTests) == 0 {
		t.Error("a published challenge shows the task and the hidden tests")
	}
	if pub.Standings[0].Diff == "" {
		t.Error("a consenting entrant's diff is published")
	}
}

func TestPublishWithholdsANonConsentingDiff(t *testing.T) {
	d := dbtest.New(t)
	env := newChallengeEnv(t, d, "none")
	a := env.enterAgentWithConsent(t, "shy", false)
	env.score(t, a, 1.0, 9)
	if err := env.svc.Close(context.Background(), env.adminUserID, env.slug); err != nil {
		t.Fatal(err)
	}
	if err := env.svc.Publish(context.Background(), env.adminUserID, env.slug); err != nil {
		t.Fatal(err)
	}
	view, _ := env.svc.Public(context.Background(), env.slug)
	if view.Standings[0].Diff != "" {
		t.Fatal("a diff must stay private without the entrant's consent")
	}
	if view.TaskMD == "" {
		t.Error("the task is published regardless of any entrant's consent")
	}
}
```

- [ ] **Step 2: Запустить — падают. Step 3: реализовать.**

`OnProofFinished(ctx, proofID)`: если proof `kind = 'challenge'`, записать в `challenge_entries` балл и метрики — `score` = доля пройденных скрытых тестов из `sandbox_result` (`passed/total`, а `0` для `failed`, `expired` и любого нетерминального), `diff_lines` = число строк diff, начинающихся с `+` или `-` (кроме заголовков `+++`/`---`), `submitted_at` = `finished_at`. `infra_error` — балл не пишется, вход остаётся незаполненным: это сбой платформы, и закрытие поставит ему 0 только если он так и не доиграет.

`Close(ctx, actorUserID, slug)` в одной транзакции: `FOR UPDATE` челленджа, статус `open` иначе 409 `challenge_not_open`; каждому входу без `score` поставить `score = 0`, `submitted_at = now()`, а его незавершённый proof — в `expired` (Review Focus 4: дедлайн прошёл, значит попытка не состоялась, и слот агента освобождается); собрать `Result`, вызвать `Rank`, записать `rank` каждому входу; `status = 'closed'`; `audit.Record` `admin.challenge_closed`.

`Publish` — статус `closed` → `published` + аудит. `Open` — `draft` → `open` + аудит.

`Tick(ctx)`: `UPDATE challenges SET status='open' WHERE status='draft' AND opens_at <= now()`, затем для каждого `status='open' AND closes_at <= now()` вызвать `Close` от системного актора (`identity.System.ID`). Ошибка по одному челленджу логируется и не мешает остальным.

`Public(ctx, slug)` собирает `PublicView` по статусу, строго по спеке: `draft` — 404 (черновик публично не существует); `open` — метаданные и `entrants`; `closed` — плюс `standings` без diff-ов и без `task_md`; `published` — плюс `task_md`, `hidden_tests` (если `publish_tests`) и diff-ы тех, кто согласился. Имя агента в `standings` берётся из `agents.name` **без** фильтра `public` (Review Focus 3): место в закрытом соревновании — публичный факт, приватность убирает агента из рейтинговых таблиц, а не из истории.

`http_public.go`: `GET /api/v1/challenges`, `GET /api/v1/challenges/{slug}`. `http_admin.go` (регистрируется в админский mux рядом с задачей 3): `POST /api/v1/admin/challenges`, `POST /api/v1/admin/challenges/{slug}/open|close|publish`, `PATCH /api/v1/admin/challenges/{slug}` (только `payout_note` и `prizes`, чтобы администратор отмечал выплату вручную — денег на платформе нет).

**Ограничение целостности вердикта (§3 спеки) исполняется в коде, не на словах.** В `Create`: если `prizes != ""`, а язык направления задачи — тот, где вердикт считается в том же процессе, что код агента (сейчас `python`: diff может подменить `_pytest`), вернуть 422 `prizes_not_allowed_for_language` с текстом про вынос харнесса из процесса. Список языков — константа `inProcessVerdictLanguages = []string{"python"}` в `service.go` с комментарием, что строка уходит из списка вместе с выносом харнесса. Тест:

```go
func TestCreateRefusesPrizesWhereTheVerdictIsInProcess(t *testing.T) {
	d := dbtest.New(t)
	env := newChallengeEnv(t, d, "none")
	pyTask := env.addSkillTask(t, "python", "sliding-rate-limiter-cup", true)
	_, err := env.svc.Create(context.Background(), env.adminUserID, challenges.NewInput{
		Slug: "python-cup", Title: "Python cup", SkillTaskSlug: pyTask,
		Prizes: "$500", OpensAt: time.Now(), ClosesAt: time.Now().Add(24 * time.Hour),
	})
	assertProblem(t, err, http.StatusUnprocessableEntity, "prizes_not_allowed_for_language")

	// The same challenge without prizes is fine, and so is a Go challenge with them.
	if _, err := env.svc.Create(context.Background(), env.adminUserID, challenges.NewInput{
		Slug: "python-open", Title: "Python open", SkillTaskSlug: pyTask,
		OpensAt: time.Now(), ClosesAt: time.Now().Add(24 * time.Hour),
	}); err != nil {
		t.Fatalf("prize-free python challenge: %v", err)
	}
}
```

- [ ] **Step 4: Прошить воркер.** В `backend/cmd/api/main.go` в тик воркера (рядом с `ExpireStale` и `qualifications.SweepStalled`, интервал 30 с) добавить `challenges.Tick`. В воркере proofs, там же, где вызывается `qualifications.OnProofFinished`, добавить `challenges.OnProofFinished` — оба вызова идут после каждого терминального перехода proof-а и сами решают, их ли это proof по `kind`.

- [ ] **Step 5: Места в профиле.** В профиль `GET /api/v1/agents/{name}` (срез 2) добавить `challenges: [{slug, title, rank, of}]` — только по челленджам в статусах `closed`/`published`.

- [ ] **Step 6: Контракт и e2e.** Пути `/challenges`, `/challenges/{slug}`, `/challenges/{slug}/enter`, `/me/challenges`, `/admin/challenges*` в `openapi.yaml`. e2e `TestChallengeLifecycle` в `main_test.go`: админ создаёт челлендж на задаче с `challenge_only = true` → публичный `GET /challenges/{slug}` до открытия отдаёт метаданные без задачи → `open` → агент владельца входит (201) → fake-песочница доводит proof до `passed` → `close` → публичная таблица показывает место 1 → `publish` → отдаётся условие и тест-имена. Каждый ответ валидируется по контракту.

- [ ] **Step 7: Прогнать и закоммитить**

Run: `cd backend && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./... && go vet ./... && gofmt -l .`

```bash
git add backend/internal/challenges backend/internal/admin backend/internal/qualifications backend/cmd/api backend/contracts/openapi/openapi.yaml
git commit -m "Close challenges on time and publish their standings"
```

---

### Task 6: Фронт: `/arena`, челленджи, приватность агента

**Files:**
- Create: `frontend/app/arena/page.tsx`, `frontend/app/challenges/page.tsx`, `frontend/app/challenges/[slug]/page.tsx`, `frontend/app/app/challenges/page.tsx`, `frontend/components/arena/skill-table.tsx`, `frontend/components/arena/tier-badge.tsx`, `frontend/components/challenges/challenge-card.tsx`, `frontend/components/challenges/standings.tsx`
- Modify: `frontend/lib/types.ts`, `frontend/components/public/site-header.tsx` (ссылки Arena и Challenges), `frontend/app/app/agent/page.tsx` или существующая страница агента (переключатель публичности), `frontend/app/app/page.tsx` (карточка открытого челленджа)

**Interfaces:**
- Consumes: `GET /leaderboard?skill=`, `GET /skills`, `GET /challenges`, `GET /challenges/{slug}`, `POST /challenges/{slug}/enter`, `GET /me/challenges`, `PATCH /agent {public}`.
- Produces: типы `SkillLeaderboardRow`, `SkillSummary`, `ChallengeSummary`, `ChallengeView`, `Standing`, `MyEntry` в `lib/types.ts`.

- [ ] **Step 1: Добавить типы** в `frontend/lib/types.ts`. Имя `LeaderboardEntry` уже занято танками — брать `SkillLeaderboardRow`, иначе типы молча перепутаются.

```ts
export type Tier = 'none' | 'verified' | 'strong' | 'elite'

export interface SkillLeaderboardRow {
  rank: number; agent_name: string; version_number: number; model: string; harness: string
  rating: number; uncertainty: number; access: number; tier: Tier; runs: number
  on_current_version: boolean; scored_at: string
}

export interface SkillSummary {
  slug: string; title: string; language: string; description: string
  pool_size: number; frozen: boolean
}

export interface Standing {
  rank: number; agent_name: string; score: number; diff_lines: number; submitted_at: string; diff: string
}

export interface ChallengeSummary {
  slug: string; title: string; summary: string; status: 'open' | 'closed' | 'published'
  skill_slug: string; min_tier: Tier; opens_at: string; closes_at: string; prizes: string; entrants: number
}

export interface ChallengeView extends ChallengeSummary {
  task_md: string; hidden_tests: string[]; standings: Standing[]
}

export interface MyEntry { slug: string; title: string; rank: number | null; proof_id: string; score: number | null }
```

Там же расширить `Proof.kind` до `'proof' | 'game_bot' | 'qualification' | 'challenge'` и добавить `public: boolean` в `AgentOverview`.

- [ ] **Step 2: Реализовать `/arena`.**

```tsx
'use client'

export default function ArenaPage() {
  const [skills, setSkills] = useState<SkillSummary[] | null>(null)
  const [active, setActive] = useState<string | null>(null)
  const [rows, setRows] = useState<SkillLeaderboardRow[] | null>(null)

  useEffect(() => {
    void api<{ items: SkillSummary[] }>('/skills')
      .then((r) => { setSkills(r.items); setActive(r.items[0]?.slug ?? null) })
      .catch(() => setSkills([]))
  }, [])

  useEffect(() => {
    if (!active) return
    setRows(null)
    void api<{ items: SkillLeaderboardRow[] }>(`/leaderboard?skill=${encodeURIComponent(active)}&limit=200`)
      .then((r) => setRows(r.items))
      .catch(() => setRows([]))
  }, [active])
  // Tabs over `skills` + a Tanks tab linking to /tanks/leaderboard; <SkillTable rows={rows} />
}
```

Вкладки направлений из `GET /skills` плюс вкладка Tanks, которая ведёт на существующий `/tanks/leaderboard` (не дублировать танковую таблицу). Для выбранного направления — `GET /leaderboard?skill=<slug>&limit=200` в `components/arena/skill-table.tsx`: колонки Rank, Agent (ссылка на `/agents/<name>`), Model, Harness, Rating (`2014 ± 350`), Tier (`tier-badge`), Runs. У строки с `on_current_version = false` — приписка `on v{n}, not confirmed` и приглушённый цвет. У замороженного направления вместо таблицы плашка: `This skill is being refilled with fresh tasks.` На 375 px таблица сворачивается в карточки (как `components/tanks/leaderboard.tsx` — взять его как образец, чтобы не изобретать вторую верстку таблицы).

- [ ] **Step 3: Реализовать страницы челленджей.** `/challenges` — три группы (Open, Upcoming, Past) карточками `challenge-card`. `/challenges/[slug]` — по статусу: открытый показывает окно, уровень, призы, число участников и кнопку Enter (для вошедшего — «You are in», для незалогиненного — ссылка на вход, при 403 `tier_too_low` — текст причины); закрытый — `standings`; опубликованный — плюс условие (`task_md` в `<pre>`), список имён скрытых тестов и раскрывающийся diff у тех, у кого он есть.

- [ ] **Step 4: Кабинет.** `/app/challenges` — свои входы из `GET /me/challenges` со ссылкой на proof. На `/app` карточка открытого челленджа, если агент `operational`. На странице агента — переключатель `Show this agent in the public arena tables` через `PATCH /agent {public}`.

- [ ] **Step 5: Проверить**

Run: `cd frontend && pnpm typecheck && pnpm build`
Expected: обе зелёные.

Затем `make up` и вручную: `/arena` открывается без входа; переключение вкладок; `/challenges` и страница челленджа; ширина 375 px — нет горизонтальной прокрутки (CI проверяет это отдельным шагом).

- [ ] **Step 6: Коммит**

```bash
git add frontend
git commit -m "Add the public arena and challenge pages"
```

---

### Task 7: Документация и живая приёмка

**Files:**
- Modify: `README.md`, `docs/how-it-works.md`, `CLAUDE.md`, `.env.example`, `docker-compose.yml`, `docs/superpowers/specs/2026-09-23-platform-roadmap.md`

- [ ] **Step 1: Обновить документы.** В `CLAUDE.md`: пакеты `internal/arena`, `internal/challenges`, `internal/admin`; `proofs.kind` теперь `proof | game_bot | qualification | challenge`; публичные маршруты `/api/v1/leaderboard`, `/api/v1/challenges*`, `/api/v1/agents/{name}`; админские `/api/v1/admin/*` за `identity.RequireAdmin`; правило «экспозиция считается по различным агентам, задача выводится из пула сама на пороге `skills.MaxExposures`»; правило «рейтинг никогда не пересчитывается задним числом, единственное исключение — `admin.VoidRun`». В `docs/how-it-works.md` — раздел «Арена: направления, таблицы, челленджи». В `.env.example` и `docker-compose.yml` — `ARENA_TASK_MAX_EXPOSURES`, `ARENA_SKILL_MIN_POOL` (если задачи 1 и 3 вынесли пороги в конфиг; если константы оставлены в коде, записать это решение в `CLAUDE.md` и env не трогать). В роадмапе строку среза 3 перевести в «готов».

- [ ] **Step 2: Полная проверка как в CI**

```sh
make check
cd backend && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./...
cd frontend && pnpm typecheck && pnpm build
```

- [ ] **Step 3: Живая приёмка.** На локальном стеке: агент проходит квалификацию по `go` (срез 2), его строка появляется на `/arena`; администратор выводит одну задачу из пула и убеждается, что рейтинг не изменился; администратор создаёт челлендж на задаче с `challenge_only`, открывает, агент входит через `arena connect`, челлендж закрывается по времени, на публичной странице видно место; выключение публичности убирает агента из таблицы, но не из таблицы челленджа.

- [ ] **Step 4: Коммит**

```bash
git add README.md docs CLAUDE.md .env.example docker-compose.yml
git commit -m "Document the public arena and challenges"
```

---

## Самопроверка

**Покрытие спеки.** §1.1 публичная арена → задача 2 (таблица) + задача 6 (страница). §1.2 жизненный цикл задачи → задача 1, ручной вывод и наблюдаемая сложность → задача 3. §1.3 челленджи → задачи 4 (схема, места, вход) и 5 (воркер, закрытие, публикация). §2 решения — каждая строка закреплена: приватность → задача 2, шаги 6–7; «результаты не пересчитываются» → задача 3, `TestRetireTaskKeepsPastResults`; «челлендж не двигает рейтинг» → задача 4, `TestChallengeDoesNotChangeTheSkillRating`; «одна попытка» → первичный ключ `challenge_entries` плюс `TestEnterTwiceIsRefused`; допуск по текущей версии → `TestEnterUsesTheTierOfTheCurrentVersion`; призы текстом → `PATCH /admin/challenges/{slug}`. §3 риск целостности вердикта → задача 4, `inProcessVerdictLanguages` и `TestCreateRefusesPrizesWhereTheVerdictIsInProcess`. §4 домен → миграции `00009`–`00011`. §5 API → задачи 2–5, у каждой свой шаг контракта и e2e. §6 страницы → задача 6. §7 тесты → распределены по задачам. §8 порядок → задачи 1–5 идут в том же порядке. Документация → задача 7.

**Чего в плане сознательно нет.** UI администратора — вне среза по спеке, маршруты вызываются `curl`. Уведомление о завершении прогона: спека называет его дыркой арены, но почты на платформе нет вообще, и тянуть внутрь этого среза провайдера, шаблоны и отписку значит раздуть его вдвое; это отдельная работа, и в срез 3 она не входит.

**Согласованность типов.** `arena.Row` ↔ `SkillLeaderboardRow` (TS): имя в TS другое, потому что `LeaderboardEntry` уже занят танками. `challenges.Result` ↔ `Standing` (TS): в Go `Standing` = `Result` + `Diff`. `Score` — `float64` в Go, `numeric(5,4)` в SQL, `number` в TS. `skills.TaskStat.AvgScore` — `*float64`, NULL при `runs = 0`. `min_tier` и `Tier` — одна и та же строка из `skillrating.Tier`.

**Review Focus → тесты.** 1 → задача 1, `TestInfraRetryDoesNotWidenExposure`; 2 → задача 1, `TestRunFinishesOnATaskRetiredMidRun`; 3 → задача 5, `TestCloseRanksEntriesAndKeepsTheirNames`; 4 → задача 5, `TestCloseScoresAnUnfinishedEntryAsZero`; 5 → задача 4, `TestTwoAgentsOfOneOwnerMayBothEnter` и `TestBannedAgentCannotEnter`.

**Номера миграций.** `00009`, `00010`, `00011` верны для `main` на 30 сентября 2026 (последняя — `00008_qualification.sql` из среза 2). Если `main` ушёл вперёд, брать следующие свободные: правка уже отгруженной миграции падает в CI.
