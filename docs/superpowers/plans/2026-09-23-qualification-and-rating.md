# Срез 2: квалификация и рейтинг. План реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Агент в стадии `operational` проходит три скрытые задачи по направлению через тот же коннектор, платформа считает балл и рейтинг с неопределённостью, привязанный к версии агента, и показывает подтверждённые направления в кабинете и публичном профиле.

**Architecture:** Расширяем срез 1, не переписываем: `proofs` получает `kind` и ссылку на прогон квалификации; воркер после каждого завершённого `proof` продвигает прогон; рейтинг — чистая функция в `internal/rating`, вызываемая из сервиса квалификаций; версии агента создаются на heartbeat по digest конфигурации коннектора; каталог направлений грузится тем же механизмом, что `proof_tasks`.

**Tech Stack:** как в срезе 1 (Go 1.26+, pgx, goose, Docker CLI, Next.js 16). Новое: образ `python:3.12-alpine` с pytest.

**Spec:** `docs/superpowers/specs/2026-09-23-qualification-and-rating-design.md`. Базовый срез: `docs/superpowers/specs/2026-09-23-agent-connect-and-proof-design.md` и его план `2026-09-23-agent-connect-and-proof.md` (интерфейсы среза 1 считаются существующими: `agents.Service`, `agents.ComputeStage`, `proofs.Service`, `proofs.Worker`, `proofs.LoadCatalog/TarDir/Untar/SyncCatalog`, `sandbox.Runner`, `sandbox.ParseGoTestJSON`, `identity.RequireSession/RequireAgent`, e2e-хелпер `newE2E` и `call` в `cmd/api/main_test.go`).

## Global Constraints

- Все правила среза 1: `ARENA_` префикс, UTC в JSON, ошибки только `httpx.Problem`, ответы e2e валидируются по `openapi.yaml`, `ARENA_TEST_REQUIRE_DOCKER=1` в CI, никаких моков во фронте, 375px без горизонтального скролла.
- Шкала рейтинга 1000–2400; `target = 1000 + 1400·score`; `uncertainty = max(60, round(350/√n))`; уровни по `access = rating − uncertainty`: `verified ≥ 1500`, `strong ≥ 1800`, `elite ≥ 2100`.
- Прогон = ровно 3 задачи, последовательно; один открытый `proof` на агента (индекс среза 1 не меняется); 3 прогона на направление в сутки; квалификация только в стадии `operational`.
- Балл задачи = `passed_hidden / hidden_tests` из манифеста (не из числа найденных тестов: провал сборки = 0); `expired` = 0; `infra_error` переставляется один раз, второй раз — исключается из среднего.
- Скрытые тесты квалификации никогда не покидают сервер; в API для `kind = qualification` `sandbox_result.tests` отдаётся без имён (`name` заменяется на `hidden-1..N`), `output` не отдаётся.
- Статусы `qualification_runs`: `running | scored | aborted`. Статусы `proofs` не меняются.
- Коммиты завершаются строкой `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`.

## Review Focus

1. **Heartbeat с тем же digest, но другим текстом модели** (владелец поправил только `model:` в конфиге): digest включает весь `agent`-блок, значит версия новая. Тест в задаче 2.
2. **Задача вернула `infra_error` дважды подряд**: прогон продолжается с 2 задачами в среднем, а не зависает и не считает 0. Тест в задаче 4.
3. **Прогон в `running`, владелец отозвал все ключи**: воркер переводит прогон в `aborted` при истечении `proof`, рейтинг не меняется, `runs_today` не растёт для лимита. Тест в задаче 4.
4. **Пул из 3 задач при третьем прогоне подряд**: ротация не может исключить все виденные, выбирает случайно из всех, а не падает. Тест в задаче 3.
5. **Публичный профиль по имени в другом регистре** (`/agents/Fixer-7`): находится (индекс `lower(name)`), e-mail не утекает. Тест в задаче 6.

## Структура файлов

```
backend/
  migrations/00003_qualification.sql        версии, направления, задачи, прогоны, рейтинги, колонки proofs
  internal/rating/rating.go, rating_test.go  чистая формула
  internal/agents/version.go                 версии агента: EnsureVersion, текущая версия в Overview
  internal/skills/catalog.go                 skills + skill_tasks из fixtures/skills
  internal/skills/service.go, http.go        GET /skills, публичный профиль-часть
  internal/qualifications/model.go, service.go, http.go, advance.go   прогоны
  internal/proofs/*                          kind, skill_task_slug, маскирование, хук OnProofFinished
  internal/proofs/sandbox/pytest.go          парсер pytest
  fixtures/skills/go/{skill.json,Dockerfile,lru-cache-eviction,worker-pool-shutdown,cursor-pagination}
  fixtures/skills/python/{skill.json,Dockerfile,sliding-rate-limiter,interval-merge,toposort-deps}
  cmd/arena/version.go                       digest конфигурации
  cmd/api/handler.go, main_test.go           маршруты, e2e
  contracts/openapi/openapi.yaml
frontend/app/app/skills/page.tsx, app/app/qualifications/[id]/page.tsx, app/agents/[name]/page.tsx,
  components/skill-card.tsx, components/rating-pill.tsx, lib/access.ts, lib/types.ts
```

---

### Task 1: Рейтинг как чистая функция

**Files:**
- Create: `backend/internal/rating/rating.go`, `backend/internal/rating/rating_test.go`

**Interfaces:**
- Produces: `rating.Target(score float64) int`; `rating.Uncertainty(runs int) int`; `rating.State{Rating, Uncertainty, Runs, SumTargets int; Prior *int}`; `rating.Apply(s State, score float64) State`; `rating.NewVersion(s State) State`; `rating.Access(r, u int) int`; `rating.Tier(access int) string` (`none|verified|strong|elite`); `rating.Verified(r, u int) bool`.

- [ ] **Step 1: Тест**

`backend/internal/rating/rating_test.go`:

```go
package rating

import "testing"

func TestTargetAndUncertainty(t *testing.T) {
	if Target(0) != 1000 || Target(1) != 2400 || Target(0.5) != 1700 {
		t.Fatalf("target: %d %d %d", Target(0), Target(1), Target(0.5))
	}
	for n, want := range map[int]int{1: 350, 2: 247, 3: 202, 4: 175, 9: 117, 34: 60, 100: 60} {
		if got := Uncertainty(n); got != want {
			t.Fatalf("Uncertainty(%d) = %d, want %d", n, got, want)
		}
	}
	if Uncertainty(0) != 350 {
		t.Fatalf("zero runs must be max uncertainty")
	}
}

func TestApply_AveragesTargetsOnOneVersion(t *testing.T) {
	s := Apply(State{}, 1.0)
	if s.Rating != 2400 || s.Uncertainty != 350 || s.Runs != 1 || s.SumTargets != 2400 {
		t.Fatalf("first run: %+v", s)
	}
	s = Apply(s, 0.5)
	if s.Rating != 2050 || s.Uncertainty != 247 || s.Runs != 2 {
		t.Fatalf("second run: %+v", s)
	}
	s = Apply(s, 0.0)
	if s.Rating != 1700 || s.Runs != 3 {
		t.Fatalf("third run: %+v", s)
	}
}

func TestNewVersion_KeepsPriorAndResetsConfidence(t *testing.T) {
	s := Apply(Apply(State{}, 1.0), 1.0) // 2400, n=2
	v := NewVersion(s)
	if v.Runs != 0 || v.SumTargets != 0 || v.Prior == nil || *v.Prior != 2400 || v.Rating != 2400 || v.Uncertainty != 350 {
		t.Fatalf("new version: %+v", v)
	}
	first := Apply(v, 0.0) // target 1000, blended with prior 2400
	if first.Rating != 1700 || first.Runs != 1 || first.Uncertainty != 350 {
		t.Fatalf("first run on new version: %+v", first)
	}
	second := Apply(first, 0.0)
	if second.Rating != 1000 || second.Runs != 2 {
		t.Fatalf("prior must stop influencing after the first run: %+v", second)
	}
}

func TestAccessAndTier(t *testing.T) {
	cases := []struct {
		r, u int
		tier string
		ok   bool
	}{
		{2400, 350, "strong", true}, {1842, 350, "none", false}, {1850, 350, "verified", true},
		{2100, 60, "elite", true}, {2150, 60, "elite", true}, {1500, 0, "verified", true}, {1499, 0, "none", false},
	}
	for _, c := range cases {
		a := Access(c.r, c.u)
		if Tier(a) != c.tier || Verified(c.r, c.u) != c.ok {
			t.Fatalf("%d-%d: access %d tier %s verified %v", c.r, c.u, a, Tier(a), Verified(c.r, c.u))
		}
	}
}
```

- [ ] **Step 2: Запуск, ожидаем провал**

Run: `cd backend && go test ./internal/rating/ -v` → FAIL, `undefined: Target`.

- [ ] **Step 3: Реализация**

`backend/internal/rating/rating.go`:

```go
// Package rating is the one place the platform's skill rating is defined.
// It is a pure function of run scores so it can be tested on a table and
// explained to an owner in two sentences: your rating is the average of
// your runs on this agent version, mapped onto 1000–2400; the ± shrinks
// with every run.
package rating

import "math"

const (
	Floor     = 1000
	Span      = 1400
	MaxUncert = 350
	MinUncert = 60

	TierVerified = 1500
	TierStrong   = 1800
	TierElite    = 2100
)

type State struct {
	Rating      int
	Uncertainty int
	Runs        int  // runs on the current version
	SumTargets  int  // sum of Target over those runs
	Prior       *int // rating carried from the previous version, nil on the first
}

// Target maps a run score in [0,1] onto the rating scale.
func Target(score float64) int {
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return Floor + int(math.Round(Span*score))
}

func Uncertainty(runs int) int {
	if runs <= 0 {
		return MaxUncert
	}
	u := int(math.Round(MaxUncert / math.Sqrt(float64(runs))))
	if u < MinUncert {
		return MinUncert
	}
	return u
}

// Apply folds one scored run into the state. The first run on a new
// version is averaged with the prior version's rating so a model change
// neither inherits the old number outright nor throws it away.
func Apply(s State, score float64) State {
	t := Target(score)
	s.Runs++
	s.SumTargets += t
	s.Rating = int(math.Round(float64(s.SumTargets) / float64(s.Runs)))
	if s.Runs == 1 && s.Prior != nil {
		s.Rating = int(math.Round(float64(*s.Prior+t) / 2))
	}
	s.Uncertainty = Uncertainty(s.Runs)
	return s
}

// NewVersion is what happens to a rating when the agent's version changes.
func NewVersion(s State) State {
	prior := s.Rating
	return State{Rating: s.Rating, Uncertainty: MaxUncert, Prior: &prior}
}

func Access(rating, uncertainty int) int { return rating - uncertainty }

func Tier(access int) string {
	switch {
	case access >= TierElite:
		return "elite"
	case access >= TierStrong:
		return "strong"
	case access >= TierVerified:
		return "verified"
	}
	return "none"
}

func Verified(rating, uncertainty int) bool { return Access(rating, uncertainty) >= TierVerified }
```

Run: `go test ./internal/rating/ -v` → PASS.

- [ ] **Step 4: Коммит**

```bash
cd backend && gofmt -l . && go vet ./internal/rating/
git add internal/rating && git commit -m "Add rating: pure target/uncertainty/tier formula

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 2: Миграция среза 2 и версии агента

**Files:**
- Create: `backend/migrations/00003_qualification.sql`, `backend/internal/agents/version.go`, `backend/internal/agents/version_integration_test.go`
- Modify: `backend/internal/agents/model.go`, `backend/internal/agents/presence.go`, `backend/internal/agents/http.go`, `backend/internal/platform/db/db_integration_test.go`

**Interfaces:**
- Produces: `agents.Version{ID string; Number int; Model, Harness, ConfigDigest string; CreatedAt time.Time}`; `agents.VersionInput{Model, Harness, ConfigDigest string}`; `(*Service).EnsureVersion(ctx, agentID string, in VersionInput) (Version, bool /*created*/, error)`; `(*Service).CurrentVersion(ctx, agentID) (*Version, error)`; `Overview.Version *Version`; хук `agents.VersionListener interface{ OnNewVersion(ctx, tx pgx.Tx, agentID, versionID string) error }` и `(*Service).SetVersionListener(l VersionListener)` (реализует сервис рейтингов в задаче 4); heartbeat принимает `version`.

- [ ] **Step 1: Миграция**

`backend/migrations/00003_qualification.sql`:

```sql
-- +goose Up

CREATE TABLE agent_versions (
    id text PRIMARY KEY,
    agent_id text NOT NULL REFERENCES agents (id),
    number int NOT NULL,
    model text NOT NULL DEFAULT '',
    harness text NOT NULL DEFAULT '',
    config_digest text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (agent_id, config_digest),
    UNIQUE (agent_id, number)
);
ALTER TABLE agents ADD COLUMN current_version_id text REFERENCES agent_versions (id);

CREATE TABLE skills (
    slug text PRIMARY KEY,
    title text NOT NULL,
    language text NOT NULL,
    image text NOT NULL,
    run_cmd text NOT NULL,
    description text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE skill_tasks (
    slug text PRIMARY KEY,
    skill_slug text NOT NULL REFERENCES skills (slug),
    title text NOT NULL,
    difficulty int NOT NULL CHECK (difficulty BETWEEN 1 AND 3),
    agent_timeout_s int NOT NULL,
    sandbox_timeout_s int NOT NULL,
    hidden_tests int NOT NULL,
    task_md text NOT NULL,
    repo_tar bytea NOT NULL,
    hidden_tar bytea NOT NULL,
    repo_sha256 text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX skill_tasks_skill_idx ON skill_tasks (skill_slug);

CREATE TABLE qualification_runs (
    id text PRIMARY KEY,
    agent_id text NOT NULL REFERENCES agents (id),
    version_id text NOT NULL REFERENCES agent_versions (id),
    skill_slug text NOT NULL REFERENCES skills (slug),
    status text NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'scored', 'aborted')),
    created_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    score numeric(5,4),
    rating_before int,
    rating_after int,
    uncertainty_after int,
    task_slugs text[] NOT NULL
);
CREATE UNIQUE INDEX qualification_runs_one_open_idx ON qualification_runs (agent_id) WHERE status = 'running';
CREATE INDEX qualification_runs_agent_idx ON qualification_runs (agent_id, created_at DESC);

ALTER TABLE proofs
    ADD COLUMN kind text NOT NULL DEFAULT 'proof' CHECK (kind IN ('proof', 'qualification')),
    ADD COLUMN qualification_run_id text REFERENCES qualification_runs (id),
    ADD COLUMN position int,
    ADD COLUMN skill_task_slug text REFERENCES skill_tasks (slug),
    ADD COLUMN retried_infra boolean NOT NULL DEFAULT false;
ALTER TABLE proofs ALTER COLUMN task_slug DROP NOT NULL;
ALTER TABLE proofs ADD CONSTRAINT proofs_task_ref CHECK (
    (kind = 'proof' AND task_slug IS NOT NULL) OR (kind = 'qualification' AND skill_task_slug IS NOT NULL AND qualification_run_id IS NOT NULL AND position BETWEEN 1 AND 3));
CREATE INDEX proofs_run_idx ON proofs (qualification_run_id, position);

CREATE TABLE skill_ratings (
    agent_id text NOT NULL REFERENCES agents (id),
    skill_slug text NOT NULL REFERENCES skills (slug),
    version_id text NOT NULL REFERENCES agent_versions (id),
    rating int NOT NULL,
    uncertainty int NOT NULL,
    runs int NOT NULL DEFAULT 0,
    sum_targets int NOT NULL DEFAULT 0,
    prior_rating int,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (agent_id, skill_slug)
);

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO arena_app;

-- +goose Down
DROP TABLE skill_ratings;
ALTER TABLE proofs DROP CONSTRAINT proofs_task_ref;
ALTER TABLE proofs DROP COLUMN kind, DROP COLUMN qualification_run_id, DROP COLUMN position, DROP COLUMN skill_task_slug, DROP COLUMN retried_infra;
ALTER TABLE proofs ALTER COLUMN task_slug SET NOT NULL;
DROP TABLE qualification_runs;
DROP TABLE skill_tasks;
DROP TABLE skills;
ALTER TABLE agents DROP COLUMN current_version_id;
DROP TABLE agent_versions;
```

В `db_integration_test.go` добавить в список таблиц `agent_versions, skills, skill_tasks, qualification_runs, skill_ratings`.

- [ ] **Step 2: Интеграционный тест версий**

`backend/internal/agents/version_integration_test.go`:

```go
package agents_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/agents"
	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
)

type recorder struct{ calls []string }

func (r *recorder) OnNewVersion(_ context.Context, _ pgx.Tx, agentID, versionID string) error {
	r.calls = append(r.calls, versionID)
	return nil
}

func TestEnsureVersion_NewDigestCreatesNumberedVersion(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	us := identity.NewService(d.AppPool, nil)
	u, _, _ := us.Signup(ctx, "o@example.com", "longenough1")
	s := agents.NewService(d.AppPool, agents.NoProofFacts{})
	rec := &recorder{}
	s.SetVersionListener(rec)
	a, _ := s.Create(ctx, u.ID, agents.CreateInput{Name: "fixer"})

	if v, _ := s.CurrentVersion(ctx, a.ID); v != nil {
		t.Fatalf("no version before the first heartbeat")
	}
	v1, created, err := s.EnsureVersion(ctx, a.ID, agents.VersionInput{Model: "claude-opus-5-5", Harness: "claude-code", ConfigDigest: "d1"})
	if err != nil || !created || v1.Number != 1 {
		t.Fatalf("v1: %v %v %+v", err, created, v1)
	}
	same, created, _ := s.EnsureVersion(ctx, a.ID, agents.VersionInput{Model: "claude-opus-5-5", Harness: "claude-code", ConfigDigest: "d1"})
	if created || same.ID != v1.ID {
		t.Fatalf("same digest must not create a version")
	}
	// Only the model text changed: the connector hashes the whole agent block, so the digest changes too.
	v2, created, _ := s.EnsureVersion(ctx, a.ID, agents.VersionInput{Model: "claude-sonnet-5", Harness: "claude-code", ConfigDigest: "d2"})
	if !created || v2.Number != 2 {
		t.Fatalf("v2: %+v", v2)
	}
	cur, _ := s.CurrentVersion(ctx, a.ID)
	if cur == nil || cur.ID != v2.ID {
		t.Fatalf("current must be v2")
	}
	if len(rec.calls) != 2 || rec.calls[1] != v2.ID {
		t.Fatalf("listener calls: %v", rec.calls)
	}
	o, _ := s.Overview(ctx, u.ID)
	if o.Version == nil || o.Version.Number != 2 || o.Version.Model != "claude-sonnet-5" {
		t.Fatalf("overview version: %+v", o.Version)
	}
}
```

- [ ] **Step 3: Реализация**

`backend/internal/agents/version.go`:

```go
package agents

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/idgen"
)

type Version struct {
	ID           string    `json:"id"`
	Number       int       `json:"number"`
	Model        string    `json:"model"`
	Harness      string    `json:"harness"`
	ConfigDigest string    `json:"config_digest"`
	CreatedAt    time.Time `json:"created_at"`
}

type VersionInput struct {
	Model        string `json:"model"`
	Harness      string `json:"harness"`
	ConfigDigest string `json:"config_digest"`
}

// VersionListener is told, inside the same transaction, that an agent got
// a new version. The ratings module uses it to reset confidence.
type VersionListener interface {
	OnNewVersion(ctx context.Context, tx pgx.Tx, agentID, versionID string) error
}

func (s *Service) SetVersionListener(l VersionListener) { s.versions = l }

const versionCols = `id, number, model, harness, config_digest, created_at`

func scanVersion(row interface{ Scan(...any) error }, v *Version) error {
	if err := row.Scan(&v.ID, &v.Number, &v.Model, &v.Harness, &v.ConfigDigest, &v.CreatedAt); err != nil {
		return err
	}
	v.CreatedAt = v.CreatedAt.UTC()
	return nil
}

// EnsureVersion returns the version for this digest, creating the next
// numbered one when the digest is new and making it current.
func (s *Service) EnsureVersion(ctx context.Context, agentID string, in VersionInput) (Version, bool, error) {
	if in.ConfigDigest == "" {
		return Version{}, false, errors.New("agents: config digest is required")
	}
	if len(in.Model) > 80 {
		in.Model = in.Model[:80]
	}
	if len(in.Harness) > 80 {
		in.Harness = in.Harness[:80]
	}
	var v Version
	created := false
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT 1 FROM agents WHERE id = $1 FOR UPDATE`, agentID); err != nil {
			return err
		}
		err := scanVersion(tx.QueryRow(ctx, `SELECT `+versionCols+` FROM agent_versions WHERE agent_id = $1 AND config_digest = $2`, agentID, in.ConfigDigest), &v)
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err := scanVersion(tx.QueryRow(ctx, `INSERT INTO agent_versions (id, agent_id, number, model, harness, config_digest)
			VALUES ($1, $2, (SELECT coalesce(max(number), 0) + 1 FROM agent_versions WHERE agent_id = $2), $3, $4, $5)
			RETURNING `+versionCols, idgen.New("ver"), agentID, in.Model, in.Harness, in.ConfigDigest), &v); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE agents SET current_version_id = $2 WHERE id = $1`, agentID, v.ID); err != nil {
			return err
		}
		created = true
		if s.versions != nil {
			return s.versions.OnNewVersion(ctx, tx, agentID, v.ID)
		}
		return nil
	})
	return v, created, err
}

func (s *Service) CurrentVersion(ctx context.Context, agentID string) (*Version, error) {
	var v Version
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return scanVersion(tx.QueryRow(ctx, `SELECT `+versionCols+` FROM agent_versions v JOIN agents a ON a.current_version_id = v.id WHERE a.id = $1`, agentID), &v)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &v, err
}
```

В `service.go`: поле `versions VersionListener` в `Service`. В `model.go`: `Overview` получает `Version *Version \`json:"version"\``; `heartbeatInput` получает `Version *VersionInput \`json:"version"\``. В `presence.go` `overview`: после presence `o.Version, err = s.CurrentVersion(ctx, a.ID)`. В `http.go` heartbeat: если `in.Version != nil && in.Version.ConfigDigest != ""`, вызвать `s.EnsureVersion` перед `Heartbeat`; ответ дополнить `"version": o.Version`.

Run: `ARENA_TEST_REQUIRE_DOCKER=1 go test ./internal/agents/ ./internal/platform/db/ -v` → PASS.

- [ ] **Step 4: Коммит**

```bash
cd backend && gofmt -l . && go vet ./... && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./...
git add -A && git commit -m "Add slice 2 schema and agent versions from connector config digest

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 3: Каталог направлений, шесть задач, парсер pytest, образы

**Files:**
- Create: `backend/internal/skills/catalog.go`, `backend/internal/skills/catalog_test.go`, `backend/internal/skills/pick.go`, `backend/internal/skills/pick_test.go`, `backend/internal/proofs/sandbox/pytest.go`, `backend/internal/proofs/sandbox/pytest_test.go`, `backend/fixtures/skills/go/skill.json`, `backend/fixtures/skills/go/Dockerfile`, `backend/fixtures/skills/python/skill.json`, `backend/fixtures/skills/python/Dockerfile`, шесть каталогов задач
- Modify: `backend/cmd/migrate/main.go`, `backend/Dockerfile`, `Makefile`, `.github/workflows/ci.yml`, `backend/internal/proofs/sandbox/docker.go` (выбор парсера)

**Interfaces:**
- Produces: `skills.Skill{Slug, Title, Language, Image, RunCmd, Description string}`; `skills.Task{Slug, SkillSlug, Title string; Difficulty, AgentTimeoutS, SandboxTimeoutS, HiddenTests int; TaskMD string; RepoTar, HiddenTar []byte; RepoSHA256 string}`; `skills.LoadCatalog(dir) ([]Skill, []Task, error)`; `skills.SyncCatalog(ctx, pool, skills, tasks) error`; `skills.Pick(pool []string, recent []string, n int, rnd *rand.Rand) []string`; `sandbox.ParsePytest([]byte) []TestResult`; `sandbox.Request.Language string` и выбор парсера по нему в `Docker.Run`.

- [ ] **Step 1: Направления и образы**

`backend/fixtures/skills/go/skill.json`:

```json
{"slug": "go", "title": "Go backend", "language": "go", "image": "arena-skill-go:1", "run_cmd": "go test ./... -json -count=1",
 "description": "Small Go services and libraries: correctness, concurrency, APIs."}
```

`backend/fixtures/skills/go/Dockerfile`:

```dockerfile
FROM golang:1.27-alpine
ENV GOTOOLCHAIN=local GOFLAGS=-mod=mod CGO_ENABLED=0
WORKDIR /work
```

`backend/fixtures/skills/python/skill.json`:

```json
{"slug": "python", "title": "Python backend", "language": "python", "image": "arena-skill-python:1", "run_cmd": "python -m pytest -q -rA -p no:cacheprovider",
 "description": "Small Python modules: data handling, algorithms, robustness."}
```

`backend/fixtures/skills/python/Dockerfile`:

```dockerfile
FROM python:3.12-alpine
RUN pip install --no-cache-dir pytest==8.3.3
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONPATH=/work
WORKDIR /work
```

- [ ] **Step 2: Три задачи Go**

`backend/fixtures/skills/go/lru-cache-eviction/manifest.json`:

```json
{"slug": "go-lru-cache-eviction", "title": "LRU cache evicts the wrong entry", "difficulty": 1,
 "agent_timeout_s": 900, "sandbox_timeout_s": 120, "hidden_tests": 5}
```

`TASK.md`:

```markdown
# LRU cache

Package `lru` implements a fixed-capacity least-recently-used cache. `Get`
must count as a use, `Put` on an existing key must update the value and
count as a use, and when the cache is full the least recently used key is
evicted. `go test ./...` fails. Fix the package without changing the tests
or adding dependencies. Hidden tests exercise the same contract.
```

`repo/go.mod`: `module lru` / `go 1.22`. `repo/lru.go`:

```go
// Package lru is a fixed-capacity least-recently-used cache.
package lru

type entry struct {
	key, value string
	prev, next *entry
}

type Cache struct {
	cap        int
	items      map[string]*entry
	head, tail *entry // head = most recent
}

func New(capacity int) *Cache {
	return &Cache{cap: capacity, items: map[string]*entry{}}
}

func (c *Cache) Len() int { return len(c.items) }

func (c *Cache) Get(key string) (string, bool) {
	e, ok := c.items[key]
	if !ok {
		return "", false
	}
	return e.value, true
}

func (c *Cache) Put(key, value string) {
	if e, ok := c.items[key]; ok {
		e.value = value
		return
	}
	if len(c.items) >= c.cap {
		c.evict()
	}
	e := &entry{key: key, value: value}
	c.pushFront(e)
	c.items[key] = e
}

func (c *Cache) pushFront(e *entry) {
	e.prev, e.next = nil, c.head
	if c.head != nil {
		c.head.prev = e
	}
	c.head = e
	if c.tail == nil {
		c.tail = e
	}
}

func (c *Cache) unlink(e *entry) {
	if e.prev != nil {
		e.prev.next = e.next
	} else {
		c.head = e.next
	}
	if e.next != nil {
		e.next.prev = e.prev
	} else {
		c.tail = e.prev
	}
	e.prev, e.next = nil, nil
}

func (c *Cache) evict() {
	if c.tail == nil {
		return
	}
	victim := c.tail
	c.unlink(victim)
	delete(c.items, victim.key)
}
```

`repo/lru_test.go`:

```go
package lru

import "testing"

func TestPutGet(t *testing.T) {
	c := New(2)
	c.Put("a", "1")
	if v, ok := c.Get("a"); !ok || v != "1" {
		t.Fatalf("get a: %q %v", v, ok)
	}
}

func TestEvictsLeastRecentlyUsed(t *testing.T) {
	c := New(2)
	c.Put("a", "1")
	c.Put("b", "2")
	c.Get("a") // a is now most recent
	c.Put("c", "3")
	if _, ok := c.Get("b"); ok {
		t.Fatalf("b should have been evicted")
	}
	if _, ok := c.Get("a"); !ok {
		t.Fatalf("a should survive")
	}
}
```

`_hidden/lru_hidden_test.go`:

```go
package lru

import "testing"

func TestHidden_PutExistingCountsAsUse(t *testing.T) {
	c := New(2)
	c.Put("a", "1")
	c.Put("b", "2")
	c.Put("a", "9")
	c.Put("c", "3")
	if _, ok := c.Get("b"); ok {
		t.Fatalf("b must be evicted, a was touched by Put")
	}
	if v, _ := c.Get("a"); v != "9" {
		t.Fatalf("a must hold the updated value")
	}
}

func TestHidden_GetMovesToFront(t *testing.T) {
	c := New(3)
	for _, k := range []string{"a", "b", "c"} {
		c.Put(k, k)
	}
	c.Get("a")
	c.Get("b")
	c.Put("d", "d")
	if _, ok := c.Get("c"); ok {
		t.Fatalf("c is the least recently used")
	}
}

func TestHidden_LenNeverExceedsCapacity(t *testing.T) {
	c := New(3)
	for i := 0; i < 50; i++ {
		c.Put(string(rune('a'+i%26)), "x")
		if c.Len() > 3 {
			t.Fatalf("len %d > cap", c.Len())
		}
	}
}

func TestHidden_HotKeySurvivesLongSequence(t *testing.T) {
	c := New(3)
	c.Put("hot", "h")
	for i := 0; i < 20; i++ {
		c.Put(string(rune('a'+i)), "x")
		if _, ok := c.Get("hot"); !ok {
			t.Fatalf("hot key evicted after %d puts although it is read every time", i+1)
		}
	}
}

func TestHidden_MissingKey(t *testing.T) {
	c := New(2)
	if _, ok := c.Get("nope"); ok {
		t.Fatalf("missing key must report false")
	}
}
```

Эталонный фикс: в `Get` после нахождения `c.unlink(e); c.pushFront(e)`; в `Put` для существующего ключа `e.value = value; c.unlink(e); c.pushFront(e); return`.

`backend/fixtures/skills/go/worker-pool-shutdown/manifest.json`: `{"slug": "go-worker-pool-shutdown", "title": "Worker pool loses results on shutdown", "difficulty": 2, "agent_timeout_s": 900, "sandbox_timeout_s": 120, "hidden_tests": 4}`.

`TASK.md`:

```markdown
# Worker pool

`pool.Run(ctx, workers, jobs, fn)` processes every job with `workers`
goroutines and returns all results, in any order, exactly one per job.
It must return `ctx.Err()` if the context is cancelled before all jobs
finished, and must never leak goroutines or drop results. `go test ./...`
fails. Fix it without changing the tests. Hidden tests check the same
contract under `-race`.
```

`repo/go.mod`: `module pool` / `go 1.22`. `repo/pool.go`:

```go
// Package pool runs jobs on a fixed number of goroutines.
package pool

import (
	"context"
	"sync"
)

// Run applies fn to every job using `workers` goroutines and returns the results.
func Run(ctx context.Context, workers int, jobs []int, fn func(int) int) ([]int, error) {
	in := make(chan int)
	out := make(chan int)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range in {
				out <- fn(j)
			}
		}()
	}
	go func() {
		for _, j := range jobs {
			in <- j
		}
		close(in)
	}()
	results := make([]int, 0, len(jobs))
	for range jobs {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case r := <-out:
			results = append(results, r)
		}
	}
	wg.Wait()
	return results, nil
}
```

`repo/pool_test.go`:

```go
package pool

import (
	"context"
	"runtime"
	"sort"
	"testing"
	"time"
)

func TestRun_AllResults(t *testing.T) {
	jobs := []int{1, 2, 3, 4, 5, 6, 7, 8}
	got, err := Run(context.Background(), 3, jobs, func(x int) int { return x * 2 })
	if err != nil {
		t.Fatal(err)
	}
	sort.Ints(got)
	if len(got) != 8 || got[0] != 2 || got[7] != 16 {
		t.Fatalf("got %v", got)
	}
}

func TestRun_CancelledContextReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	block := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := Run(ctx, 2, []int{1, 2, 3, 4}, func(x int) int { <-block; return x })
		done <- err
	}()
	cancel()
	err := <-done
	close(block)
	if err != context.Canceled {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestRun_CancelDoesNotLeakGoroutines(t *testing.T) {
	before := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	block := make(chan struct{})
	done := make(chan struct{})
	go func() {
		_, _ = Run(ctx, 2, []int{1, 2, 3, 4, 5, 6}, func(x int) int { <-block; return x })
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done
	close(block)
	time.Sleep(50 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before {
		t.Fatalf("goroutines leaked: before %d after %d", before, after)
	}
}
```

(импорты `context`, `runtime`, `sort`, `testing`, `time`.)

`_hidden/pool_hidden_test.go`:

```go
package pool

import (
	"context"
	"runtime"
	"sort"
	"testing"
	"time"
)

func leakCheck(t *testing.T, workers int, jobs []int) {
	t.Helper()
	before := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	block := make(chan struct{})
	done := make(chan struct{})
	go func() {
		_, _ = Run(ctx, workers, jobs, func(x int) int { <-block; return x })
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done
	close(block)
	time.Sleep(50 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before {
		t.Fatalf("goroutines leaked: before %d after %d", before, after)
	}
}

func TestHidden_NoLeakWhenWorkersBlocked(t *testing.T) { leakCheck(t, 4, []int{1, 2, 3, 4, 5, 6}) }

func TestHidden_NoLeakWhenProducerBlocked(t *testing.T) {
	jobs := make([]int, 100)
	leakCheck(t, 1, jobs)
}

func TestHidden_MoreWorkersThanJobs(t *testing.T) {
	got, err := Run(context.Background(), 10, []int{1, 2}, func(x int) int { return x })
	if err != nil || len(got) != 2 {
		t.Fatalf("%v %v", got, err)
	}
}

func TestHidden_ManyJobsExactlyOnce(t *testing.T) {
	jobs := make([]int, 500)
	for i := range jobs {
		jobs[i] = i
	}
	got, err := Run(context.Background(), 7, jobs, func(x int) int { return x })
	if err != nil {
		t.Fatal(err)
	}
	sort.Ints(got)
	for i, v := range got {
		if v != i {
			t.Fatalf("result %d missing or duplicated (got %d)", i, v)
		}
	}
}
```

Эталонный фикс (проверен вручную): цикл чтения результатов не меняется (он уже возвращает `ctx.Err()` сразу), а утечка в том, что после его выхода воркеры навсегда блокируются на `out <- fn(j)`, а producer — на `in <- j`. Оба места превращаются в `select` с веткой `<-ctx.Done()`: воркер `select { case out <- fn(j): case <-ctx.Done(): return }`, producer `defer close(in)` и `select { case in <- j: case <-ctx.Done(): return }`. `wg.Wait()` остаётся в успешном пути.

`backend/fixtures/skills/go/cursor-pagination/manifest.json`: `{"slug": "go-cursor-pagination", "title": "Cursor pagination skips and repeats rows", "difficulty": 2, "agent_timeout_s": 900, "sandbox_timeout_s": 120, "hidden_tests": 5}`.

`TASK.md`:

```markdown
# Cursor pagination

`page.Page(items, cursor, limit)` returns up to `limit` items after the
item whose ID equals `cursor` (empty cursor = from the start), plus the
next cursor (the last returned ID) or "" when nothing follows. Items are
sorted by ID ascending. Iterating page by page must visit every item
exactly once. `go test ./...` fails; fix `page.go` without changing tests.
```

`repo/go.mod`: `module page` / `go 1.22`. `repo/page.go`:

```go
// Package page slices a sorted list into cursor pages.
package page

import "sort"

type Item struct {
	ID   string
	Name string
}

type Result struct {
	Items []Item
	Next  string
}

func Page(items []Item, cursor string, limit int) Result {
	sorted := append([]Item(nil), items...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	start := 0
	if cursor != "" {
		start = sort.Search(len(sorted), func(i int) bool { return sorted[i].ID >= cursor })
	}
	end := start + limit
	if end > len(sorted) {
		end = len(sorted)
	}
	out := sorted[start:end]
	next := ""
	if end < len(sorted) && len(out) > 0 {
		next = out[len(out)-1].ID
	}
	return Result{Items: out, Next: next}
}
```

`repo/page_test.go`:

```go
package page

import "testing"

func items(n int) []Item {
	out := make([]Item, n)
	for i := range out {
		out[i] = Item{ID: string(rune('a' + i)), Name: "n"}
	}
	return out
}

func TestFirstPage(t *testing.T) {
	r := Page(items(5), "", 2)
	if len(r.Items) != 2 || r.Items[0].ID != "a" || r.Next != "b" {
		t.Fatalf("%+v", r)
	}
}

func TestSecondPageStartsAfterCursor(t *testing.T) {
	r := Page(items(5), "b", 2)
	if len(r.Items) != 2 || r.Items[0].ID != "c" || r.Items[1].ID != "d" {
		t.Fatalf("%+v", r)
	}
}
```

`_hidden/page_hidden_test.go`:

```go
package page

import "testing"

func walk(all []Item, limit int) []string {
	var seen []string
	cursor := ""
	for i := 0; i < 100; i++ {
		r := Page(all, cursor, limit)
		for _, it := range r.Items {
			seen = append(seen, it.ID)
		}
		if r.Next == "" {
			break
		}
		cursor = r.Next
	}
	return seen
}

func TestHidden_WalkVisitsEachOnce(t *testing.T) {
	for _, limit := range []int{1, 2, 3, 5, 7} {
		seen := walk(items(7), limit)
		if len(seen) != 7 {
			t.Fatalf("limit %d: visited %v", limit, seen)
		}
		for i, id := range seen {
			if id != string(rune('a'+i)) {
				t.Fatalf("limit %d: order %v", limit, seen)
			}
		}
	}
}

func TestHidden_LastPageHasNoNext(t *testing.T) {
	if r := Page(items(4), "b", 2); r.Next != "" {
		t.Fatalf("c,d is the last page, next=%q", r.Next)
	}
}

func TestHidden_UnknownCursorStartsAfterItsPosition(t *testing.T) {
	r := Page([]Item{{ID: "a"}, {ID: "c"}, {ID: "e"}}, "b", 10)
	if len(r.Items) != 2 || r.Items[0].ID != "c" {
		t.Fatalf("%+v", r)
	}
}

func TestHidden_UnsortedInput(t *testing.T) {
	r := Page([]Item{{ID: "c"}, {ID: "a"}, {ID: "b"}}, "", 2)
	if r.Items[0].ID != "a" || r.Items[1].ID != "b" || r.Next != "b" {
		t.Fatalf("%+v", r)
	}
}

func TestHidden_CursorAtLastItemReturnsEmpty(t *testing.T) {
	r := Page(items(3), "c", 2)
	if len(r.Items) != 0 || r.Next != "" {
		t.Fatalf("%+v", r)
	}
}
```

Эталонный фикс: при непустом курсоре `start = sort.Search(..., sorted[i].ID > cursor)` (строго больше).

- [ ] **Step 3: Три задачи Python**

Общее: `repo/` содержит модуль и `test_<name>.py`; `_hidden/` содержит `test_hidden_<name>.py`. Python-задачи без `go.mod`; `Dockerfile` у направления.

`backend/fixtures/skills/python/sliding-rate-limiter/manifest.json`: `{"slug": "py-sliding-rate-limiter", "title": "Sliding-window rate limiter lets bursts through", "difficulty": 2, "agent_timeout_s": 900, "sandbox_timeout_s": 120, "hidden_tests": 4}`.

`TASK.md`:

```markdown
# Sliding-window rate limiter

`limiter.RateLimiter(limit, window_seconds)` has `allow(key, now) -> bool`.
A key may be allowed at most `limit` times in any window of
`window_seconds` seconds (sliding, not fixed buckets). `now` is a float
timestamp supplied by the caller. Old hits must be forgotten so memory does
not grow. `pytest` fails; fix `limiter.py` without changing the tests.
```

`repo/limiter.py`:

```python
"""Sliding-window rate limiter."""
from collections import defaultdict, deque


class RateLimiter:
    def __init__(self, limit: int, window_seconds: float):
        self.limit = limit
        self.window = window_seconds
        self._hits: dict[str, deque[float]] = defaultdict(deque)

    def allow(self, key: str, now: float) -> bool:
        hits = self._hits[key]
        while hits and hits[0] < now - self.window:
            hits.popleft()
        hits.append(now)
        return len(hits) <= self.limit
```

`repo/test_limiter.py`:

```python
from limiter import RateLimiter


def test_allows_up_to_limit():
    rl = RateLimiter(3, 10)
    assert all(rl.allow("k", t) for t in (0, 1, 2))
    assert not rl.allow("k", 3)


def test_window_slides():
    rl = RateLimiter(2, 10)
    assert rl.allow("k", 0)
    assert rl.allow("k", 5)
    assert not rl.allow("k", 9)
    assert rl.allow("k", 11)
```

`_hidden/test_hidden_limiter.py`:

```python
from limiter import RateLimiter


def test_hidden_denied_calls_do_not_count():
    rl = RateLimiter(2, 10)
    assert rl.allow("k", 0) and rl.allow("k", 1)
    for t in range(2, 9):
        assert not rl.allow("k", t)
    # at t=10.5 the hit at t=0 has expired; only the hit at t=1 remains
    assert rl.allow("k", 10.5)


def test_hidden_keys_are_independent():
    rl = RateLimiter(1, 10)
    assert rl.allow("a", 0)
    assert rl.allow("b", 0)
    assert not rl.allow("a", 1)


def test_hidden_boundary_is_exclusive():
    rl = RateLimiter(1, 10)
    assert rl.allow("k", 0)
    assert not rl.allow("k", 10)
    assert rl.allow("k", 10.01)


def test_hidden_memory_is_bounded():
    rl = RateLimiter(5, 1)
    for t in range(10_000):
        rl.allow("k", float(t))
    assert len(rl._hits["k"]) <= 5
```

Эталонный фикс (проверен вручную): отказ не записывается как hit — перед `hits.append(now)` проверить `if len(hits) >= self.limit: return False`. Условие устаревания остаётся строгим `hits[0] < now - self.window`: хит ровно на границе окна ещё считается (тест `boundary_is_exclusive`).

`backend/fixtures/skills/python/interval-merge/manifest.json`: `{"slug": "py-interval-merge", "title": "Interval merge drops touching ranges", "difficulty": 1, "agent_timeout_s": 900, "sandbox_timeout_s": 120, "hidden_tests": 5}`.

`TASK.md`:

```markdown
# Merge intervals

`intervals.merge(ranges)` takes a list of `(start, end)` integer pairs with
`start <= end`, in any order, and returns the minimal list of merged
intervals sorted by start. Touching intervals like `(1, 3)` and `(3, 5)`
merge into `(1, 5)`. The input must not be modified. `pytest` fails; fix
`intervals.py` without changing the tests.
```

`repo/intervals.py`:

```python
"""Merge overlapping integer intervals."""


def merge(ranges: list[tuple[int, int]]) -> list[tuple[int, int]]:
    ranges.sort()
    out: list[tuple[int, int]] = []
    for start, end in ranges:
        if out and start < out[-1][1]:
            out[-1] = (out[-1][0], max(out[-1][1], end))
        else:
            out.append((start, end))
    return out
```

`repo/test_intervals.py`:

```python
from intervals import merge


def test_overlapping():
    assert merge([(1, 4), (2, 5)]) == [(1, 5)]


def test_touching_merge():
    assert merge([(1, 3), (3, 5)]) == [(1, 5)]
```

`_hidden/test_hidden_intervals.py`:

```python
from intervals import merge


def test_hidden_input_not_mutated():
    data = [(5, 6), (1, 2)]
    merge(data)
    assert data == [(5, 6), (1, 2)]


def test_hidden_unsorted_and_nested():
    assert merge([(6, 8), (1, 9), (2, 4)]) == [(1, 9)]


def test_hidden_touching_chain():
    assert merge([(3, 4), (1, 2), (2, 3)]) == [(1, 4)]


def test_hidden_empty_and_single():
    assert merge([]) == []
    assert merge([(3, 3)]) == [(3, 3)]


def test_hidden_points_touching():
    assert merge([(1, 1), (1, 1), (2, 2)]) == [(1, 1), (2, 2)]
```

Эталонный фикс: `ranges = sorted(ranges)` и условие `start <= out[-1][1]`.

`backend/fixtures/skills/python/toposort-deps/manifest.json`: `{"slug": "py-toposort-deps", "title": "Topological sort misses cycles and isolated nodes", "difficulty": 3, "agent_timeout_s": 900, "sandbox_timeout_s": 120, "hidden_tests": 5}`.

`TASK.md`:

```markdown
# Dependency order

`deps.order(graph)` takes `{node: [dependencies]}` and returns a list where
every node appears after all of its dependencies. Nodes that appear only as
dependencies must be included. Ties are broken alphabetically so the result
is deterministic. A cycle raises `deps.CycleError` naming one node of the
cycle. `pytest` fails; fix `deps.py` without changing the tests.
```

`repo/deps.py`:

```python
"""Deterministic topological order of a dependency graph."""


class CycleError(ValueError):
    pass


def order(graph: dict[str, list[str]]) -> list[str]:
    seen: set[str] = set()
    out: list[str] = []

    def visit(node: str) -> None:
        if node in seen:
            return
        seen.add(node)
        for dep in sorted(graph.get(node, [])):
            visit(dep)
        out.append(node)

    for node in sorted(graph):
        visit(node)
    return out
```

`repo/test_deps.py`:

```python
import pytest

from deps import CycleError, order


def test_simple_chain():
    assert order({"app": ["lib"], "lib": ["core"], "core": []}) == ["core", "lib", "app"]


def test_cycle_raises():
    with pytest.raises(CycleError):
        order({"a": ["b"], "b": ["a"]})
```

`_hidden/test_hidden_deps.py`:

```python
import pytest

from deps import CycleError, order


def test_hidden_dependency_only_nodes_included():
    assert order({"app": ["lib"]}) == ["lib", "app"]


def test_hidden_alphabetical_ties():
    assert order({"b": [], "a": [], "c": ["a", "b"]}) == ["a", "b", "c"]


def test_hidden_cycle_inside_larger_graph():
    with pytest.raises(CycleError):
        order({"app": ["lib"], "lib": ["util"], "util": ["lib"], "docs": []})


def test_hidden_self_cycle():
    with pytest.raises(CycleError):
        order({"a": ["a"]})


def test_hidden_cycle_error_names_a_node():
    with pytest.raises(CycleError) as e:
        order({"x": ["y"], "y": ["z"], "z": ["x"]})
    assert any(n in str(e.value) for n in ("x", "y", "z"))
```

Эталонный фикс: три состояния узла (не посещён / в стеке / готов), `CycleError(node)` при встрече узла в стеке, обход по `sorted(set(graph) | {d for ds in graph.values() for d in ds})`.

- [ ] **Step 4: Каталог и выбор задач**

`backend/internal/skills/catalog_test.go`:

```go
package skills

import (
	"path/filepath"
	"testing"
)

func TestLoadCatalog(t *testing.T) {
	sk, tasks, err := LoadCatalog(filepath.Join("..", "..", "fixtures", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	if len(sk) != 2 || sk[0].Slug != "go" || sk[1].Slug != "python" {
		t.Fatalf("skills: %+v", sk)
	}
	if len(tasks) != 6 {
		t.Fatalf("tasks: %d", len(tasks))
	}
	for _, task := range tasks {
		if task.SkillSlug == "" || task.Difficulty < 1 || task.HiddenTests == 0 || len(task.RepoTar) == 0 || len(task.HiddenTar) == 0 {
			t.Fatalf("bad task %+v", task.Slug)
		}
	}
}
```

`backend/internal/skills/pick_test.go`:

```go
package skills

import (
	"math/rand"
	"testing"
)

func TestPick_PrefersUnseenThenFallsBackToAll(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	pool := []string{"a", "b", "c", "d", "e"}
	got := Pick(pool, []string{"a", "b", "c"}, 3, rnd)
	if len(got) != 3 || !contains(got, "d") || !contains(got, "e") {
		t.Fatalf("must include both unseen: %v", got)
	}
	got = Pick(pool, []string{"a", "b", "c", "d", "e"}, 3, rnd)
	if len(got) != 3 || !distinct(got) {
		t.Fatalf("all seen: still picks 3 distinct: %v", got)
	}
	got = Pick([]string{"x", "y"}, nil, 3, rnd)
	if len(got) != 2 {
		t.Fatalf("pool smaller than n returns the pool: %v", got)
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func distinct(xs []string) bool {
	m := map[string]bool{}
	for _, x := range xs {
		if m[x] {
			return false
		}
		m[x] = true
	}
	return true
}
```

`backend/internal/skills/catalog.go`:

```go
// Package skills holds the qualification catalog: skills and their hidden
// task pools, loaded from fixtures/skills the same way proofs loads its
// catalog.
package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
	"tolerance/internal/proofs"
)

type Skill struct {
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Language    string `json:"language"`
	Image       string `json:"image"`
	RunCmd      string `json:"run_cmd"`
	Description string `json:"description"`
}

type Task struct {
	Slug            string
	SkillSlug       string
	Title           string
	Difficulty      int
	AgentTimeoutS   int
	SandboxTimeoutS int
	HiddenTests     int
	TaskMD          string
	RepoTar         []byte
	HiddenTar       []byte
	RepoSHA256      string
}

type taskManifest struct {
	Slug            string `json:"slug"`
	Title           string `json:"title"`
	Difficulty      int    `json:"difficulty"`
	AgentTimeoutS   int    `json:"agent_timeout_s"`
	SandboxTimeoutS int    `json:"sandbox_timeout_s"`
	HiddenTests     int    `json:"hidden_tests"`
}

// LoadCatalog reads fixtures/skills/<skill>/skill.json and every task
// directory beside it (a directory containing manifest.json).
func LoadCatalog(dir string) ([]Skill, []Task, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("skills: read %s: %w", dir, err)
	}
	var skills []Skill
	var tasks []Task
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sdir := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(filepath.Join(sdir, "skill.json"))
		if err != nil {
			return nil, nil, fmt.Errorf("skills: %s: %w", sdir, err)
		}
		var s Skill
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, nil, fmt.Errorf("skills: %s/skill.json: %w", sdir, err)
		}
		if s.Slug == "" || s.Image == "" || s.RunCmd == "" || s.Language == "" {
			return nil, nil, fmt.Errorf("skills: %s/skill.json: slug, language, image, run_cmd required", sdir)
		}
		skills = append(skills, s)
		sub, err := os.ReadDir(sdir)
		if err != nil {
			return nil, nil, err
		}
		for _, te := range sub {
			if !te.IsDir() {
				continue
			}
			tdir := filepath.Join(sdir, te.Name())
			if _, err := os.Stat(filepath.Join(tdir, "manifest.json")); err != nil {
				continue
			}
			t, err := loadTask(tdir, s.Slug)
			if err != nil {
				return nil, nil, err
			}
			tasks = append(tasks, t)
		}
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Slug < skills[j].Slug })
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].Slug < tasks[j].Slug })
	return skills, tasks, nil
}

func loadTask(dir, skill string) (Task, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return Task{}, err
	}
	var m taskManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return Task{}, fmt.Errorf("skills: %s/manifest.json: %w", dir, err)
	}
	if m.Slug == "" || m.Difficulty < 1 || m.Difficulty > 3 || m.HiddenTests <= 0 || m.AgentTimeoutS <= 0 || m.SandboxTimeoutS <= 0 {
		return Task{}, fmt.Errorf("skills: %s/manifest.json: slug, difficulty 1-3, hidden_tests, timeouts required", dir)
	}
	md, err := os.ReadFile(filepath.Join(dir, "TASK.md"))
	if err != nil {
		return Task{}, err
	}
	repoTar, err := proofs.TarDir(filepath.Join(dir, "repo"))
	if err != nil {
		return Task{}, err
	}
	hiddenTar, err := proofs.TarDir(filepath.Join(dir, "_hidden"))
	if err != nil {
		return Task{}, err
	}
	sum := sha256.Sum256(repoTar)
	return Task{Slug: m.Slug, SkillSlug: skill, Title: m.Title, Difficulty: m.Difficulty, AgentTimeoutS: m.AgentTimeoutS,
		SandboxTimeoutS: m.SandboxTimeoutS, HiddenTests: m.HiddenTests, TaskMD: string(md), RepoTar: repoTar, HiddenTar: hiddenTar,
		RepoSHA256: hex.EncodeToString(sum[:])}, nil
}

func SyncCatalog(ctx context.Context, pool *db.Pool, skills []Skill, tasks []Task) error {
	return pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		for _, s := range skills {
			if _, err := tx.Exec(ctx, `INSERT INTO skills (slug, title, language, image, run_cmd, description, updated_at) VALUES ($1,$2,$3,$4,$5,$6, now())
				ON CONFLICT (slug) DO UPDATE SET title = $2, language = $3, image = $4, run_cmd = $5, description = $6, updated_at = now()`,
				s.Slug, s.Title, s.Language, s.Image, s.RunCmd, s.Description); err != nil {
				return fmt.Errorf("skills: sync %s: %w", s.Slug, err)
			}
		}
		for _, t := range tasks {
			if _, err := tx.Exec(ctx, `INSERT INTO skill_tasks (slug, skill_slug, title, difficulty, agent_timeout_s, sandbox_timeout_s, hidden_tests, task_md, repo_tar, hidden_tar, repo_sha256, updated_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11, now())
				ON CONFLICT (slug) DO UPDATE SET skill_slug = $2, title = $3, difficulty = $4, agent_timeout_s = $5, sandbox_timeout_s = $6, hidden_tests = $7,
				  task_md = $8, repo_tar = $9, hidden_tar = $10, repo_sha256 = $11, updated_at = now()`,
				t.Slug, t.SkillSlug, t.Title, t.Difficulty, t.AgentTimeoutS, t.SandboxTimeoutS, t.HiddenTests, t.TaskMD, t.RepoTar, t.HiddenTar, t.RepoSHA256); err != nil {
				return fmt.Errorf("skills: sync task %s: %w", t.Slug, err)
			}
		}
		return nil
	})
}
```

`backend/internal/skills/pick.go`:

```go
package skills

import "math/rand"

// Pick chooses n distinct task slugs from pool, preferring ones not in
// recent. When fewer than n unseen tasks exist, the rest come from the
// whole pool; when the pool itself is smaller than n, it is returned whole.
func Pick(pool, recent []string, n int, rnd *rand.Rand) []string {
	if len(pool) <= n {
		out := append([]string(nil), pool...)
		rnd.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
		return out
	}
	seen := map[string]bool{}
	for _, r := range recent {
		seen[r] = true
	}
	var unseen, rest []string
	for _, p := range pool {
		if seen[p] {
			rest = append(rest, p)
		} else {
			unseen = append(unseen, p)
		}
	}
	rnd.Shuffle(len(unseen), func(i, j int) { unseen[i], unseen[j] = unseen[j], unseen[i] })
	rnd.Shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] })
	out := append(unseen, rest...)
	return out[:n]
}
```

Run: `go test ./internal/skills/ -v` → PASS.

- [ ] **Step 5: Парсер pytest и выбор парсера**

`backend/internal/proofs/sandbox/pytest_test.go`:

```go
package sandbox

import "testing"

func TestParsePytest(t *testing.T) {
	out := []byte(`............F
=================================== FAILURES ===================================
____________________________ test_hidden_boundary ____________________________
assert False
=========================== short test summary info ============================
PASSED test_limiter.py::test_allows_up_to_limit
PASSED test_limiter.py::test_window_slides
FAILED test_hidden_limiter.py::test_hidden_boundary_is_exclusive - assert False
ERROR test_other.py::test_broken - ImportError
1 failed, 2 passed in 0.03s
`)
	got := ParsePytest(out)
	if len(got) != 4 || got[0].Name != "test_limiter.py::test_allows_up_to_limit" || !got[0].Passed ||
		got[2].Name != "test_hidden_limiter.py::test_hidden_boundary_is_exclusive" || got[2].Passed || got[3].Passed {
		t.Fatalf("%+v", got)
	}
	if len(ParsePytest([]byte("ImportError while importing test module"))) != 0 {
		t.Fatalf("collection failure yields no tests")
	}
}
```

`backend/internal/proofs/sandbox/pytest.go`:

```go
package sandbox

import (
	"bufio"
	"bytes"
	"strings"
)

// ParsePytest reads the `-rA` short summary lines pytest prints at the end
// of a run: "PASSED path::name", "FAILED path::name - reason",
// "ERROR path::name - reason". Everything else is ignored.
func ParsePytest(out []byte) []TestResult {
	var res []TestResult
	seen := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		var passed bool
		var rest string
		switch {
		case strings.HasPrefix(line, "PASSED "):
			passed, rest = true, line[len("PASSED "):]
		case strings.HasPrefix(line, "FAILED "):
			rest = line[len("FAILED "):]
		case strings.HasPrefix(line, "ERROR "):
			rest = line[len("ERROR "):]
		default:
			continue
		}
		name := rest
		if i := strings.Index(rest, " - "); i >= 0 {
			name = rest[:i]
		}
		name = strings.TrimSpace(name)
		if name == "" || !strings.Contains(name, "::") || seen[name] {
			continue
		}
		seen[name] = true
		res = append(res, TestResult{Name: name, Passed: passed})
	}
	return res
}
```

В `sandbox/runner.go` добавить в `Request` поле `Language string`. В `docker.go` заменить `res.Tests = ParseGoTestJSON([]byte(res.Output))` на:

```go
	res.Tests = ParseOutput(req.Language, []byte(res.Output))
```

и добавить в `runner.go`:

```go
func ParseOutput(language string, out []byte) []TestResult {
	if language == "python" {
		return ParsePytest(out)
	}
	return ParseGoTestJSON(out)
}
```

`proofs/worker.go`: в `RunProof` передавать `Language` в `sandbox.Request` (в задаче 4 запрос к БД расширяется полем языка).

- [ ] **Step 6: Docker-тест одной go- и одной python-задачи**

Добавить в `backend/internal/proofs/sandbox/docker_integration_test.go`:

```go
func TestDocker_SkillTasks(t *testing.T) {
	requireDocker(t)
	for _, img := range []struct{ tag, dir string }{{"arena-skill-go:1", "../../../fixtures/skills/go"}, {"arena-skill-python:1", "../../../fixtures/skills/python"}} {
		if out, err := exec.Command("docker", "build", "-q", "-t", img.tag, img.dir).CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", img.tag, err, out)
		}
	}
	r := sandbox.NewDocker()
	cases := []struct {
		dir, image, run, language, file, find, replace string
		hidden                                        int
	}{
		{"../../../fixtures/skills/python/interval-merge", "arena-skill-python:1", "python -m pytest -q -rA -p no:cacheprovider", "python",
			"intervals.py", "    ranges.sort()\n", "    ranges = sorted(ranges)\n", 5},
		{"../../../fixtures/skills/go/cursor-pagination", "arena-skill-go:1", "go test ./... -json -count=1", "go",
			"page.go", "sorted[i].ID >= cursor", "sorted[i].ID > cursor", 5},
	}
	for _, c := range cases {
		dir := t.TempDir()
		task, err := loadOne(c.dir)
		if err != nil {
			t.Fatal(err)
		}
		_ = proofs.Untar(task.RepoTar, dir)
		src, _ := os.ReadFile(filepath.Join(dir, c.file))
		fixed := strings.Replace(string(src), c.find, c.replace, 1)
		if c.language == "python" && c.file == "intervals.py" {
			fixed = strings.Replace(fixed, "start < out[-1][1]", "start <= out[-1][1]", 1)
		}
		_ = os.WriteFile(filepath.Join(dir, c.file), []byte(fixed), 0o644)
		_ = proofs.Untar(task.HiddenTar, dir)
		res, err := r.Run(context.Background(), sandbox.Request{WorkDir: dir, Image: c.image, RunCmd: c.run, Language: c.language, Timeout: 2 * time.Minute})
		if err != nil {
			t.Fatalf("%s: %v", c.dir, err)
		}
		hiddenPassed := 0
		for _, tr := range res.Tests {
			if strings.Contains(strings.ToLower(tr.Name), "hidden") && tr.Passed {
				hiddenPassed++
			}
		}
		if res.ExitCode != 0 || hiddenPassed != c.hidden {
			t.Fatalf("%s: exit %d hidden passed %d/%d\n%s", c.dir, res.ExitCode, hiddenPassed, c.hidden, res.Output)
		}
	}
}
```

Хелпер `loadOne(dir) (skills.Task, error)` в том же тестовом файле: читает манифест и делает `proofs.TarDir` для `repo` и `_hidden` (10 строк, по образцу `skills.loadTask`, без импорта `skills`, чтобы не тянуть цикл; либо экспортировать `skills.LoadTask(dir, skill)` и вызвать его — реализатор выбирает второе, если нет цикла импортов: `skills` импортирует `proofs`, `sandbox` не импортирует ни того, ни другого в не-тестовом коде, значит тестовый пакет `sandbox_test` может импортировать `skills`).

Run: `ARENA_TEST_REQUIRE_DOCKER=1 go test ./internal/proofs/sandbox/ -run TestDocker_SkillTasks -v` → PASS.

- [ ] **Step 7: Синхронизация, образы, коммит**

`cmd/migrate/main.go`: после proofs-каталога, если `ARENA_SKILLS_DIR` задан, `skills.LoadCatalog` + `skills.SyncCatalog`. `backend/Dockerfile`: `COPY fixtures/skills /opt/arena/skills`, `ENV ARENA_SKILLS_DIR=/opt/arena/skills`. `Makefile`: цель `proof-image` дополняется `docker build -q -t arena-skill-go:1 backend/fixtures/skills/go && docker build -q -t arena-skill-python:1 backend/fixtures/skills/python`; `migrate` получает `ARENA_SKILLS_DIR=./fixtures/skills`. `.github/workflows/ci.yml`: те же две сборки образов перед тестами.

```bash
cd backend && gofmt -l . && go vet ./... && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./...
git add -A && git commit -m "Add skills catalog with six hidden tasks, pytest parser and skill images

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```
