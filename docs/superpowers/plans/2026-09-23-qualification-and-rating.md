# Срез 2: квалификация и рейтинг. План реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Агент в стадии `operational` проходит три скрытые задачи по направлению через тот же коннектор, платформа считает балл и рейтинг с неопределённостью, привязанный к версии агента, и показывает подтверждённые направления в кабинете и публичном профиле.

**Architecture:** Расширяем срез 1 и танки, не переписываем: `proofs.kind` (его вводит задача 8 танков) получает значение `qualification`, а proof — ссылку на прогон квалификации; воркер после каждого завершённого `proof` продвигает прогон; рейтинг — чистая функция в `internal/rating`, вызываемая из сервиса квалификаций; версии агента создаются на heartbeat по digest конфигурации коннектора; каталог направлений грузится тем же механизмом, что `proof_tasks`.

**Tech Stack:** как в срезе 1 (Go 1.26+, pgx, goose, Docker CLI, Next.js 16). Новое: образ `python:3.12-alpine` с pytest.

**Spec:** `docs/superpowers/specs/2026-09-23-qualification-and-rating-design.md`. База — main со срезом 1, его доделками (`plans/2026-09-25-slice-1-finish.md`) и задачами 8 и 11 танков (`plans/2026-09-25-tanks-arena.md`). Считаются существующими: `agents.Service` (`NewService(pool, ProofFactsSource)`, `Heartbeat(ctx, agentID, connectorVersion, hostname)`, `Overview`, `OverviewByID`, `ByID`), `agents.ComputeStage`, `agents.NoProofFacts`, `proofs.Service` (`Tasks`, `Create`, `CreateWithRepo`, `List`, `Latest`, `Get`, `Retry`, `Claim`, `RepoTar`, `Started`, `ExpireStale`, `FailOversized`, `SubmitResult`, `ProofFacts` только по `kind = 'proof'`), `proofs.Worker` (`RunProof` с веткой `game_bot`, `SetGameBotJudge`, `MarkInfraError`), `proofs.KindProof/KindGameBot`, `Proof.Kind`, `proofs.LoadCatalog/TarDir/TarFiles/Untar/SyncCatalog`, `sandbox.Runner`, `sandbox.Fake{Result, Err, Calls}`, `sandbox.PassAll`, `sandbox.ParseGoTestJSON`, `identity.NewService(pool, adminEmails)`, `identity.RequireSession/RequireAgent`, `cmd/api/compose.go` (`ownerAgent`, `meAgent`, `connectorStatus` — это `GET /me` и `GET /connector/status`), e2e-хелперы `newE2E`, `browser`, `call` и поля `e.worker`, `e.fake` в `cmd/api/main_test.go`, в коннекторе `client.Heartbeat`, `client.Status`, `formatStatus`, `withRetry`.

**Ревизия 25 сентября 2026.** План написан 23 сентября против среза 1 до его доделок и до танков; сверен с кодом main (`5c988761`) и с планом танков. **Предусловие:** не начинать, пока в main нет задач 8 и 11 танков (`migrations/00003_games.sql`, `proofs.kind` со значениями `proof | game_bot`, `proofs.TarFiles`, `GameBotJudge`). Что изменилось против версии 23 сентября:

- миграция `00004`, и она расширяет существующий `proofs.kind` и его CHECK, а не добавляет колонку;
- скрытые тесты определяются по именам для обоих языков (`proofs.HiddenTestNames`), и балл задачи считает только их: тест, которого нет в скрытом наборе, балла не прибавляет; каталог сверяет число имён с `hidden_tests`. Прежний вариант считал любой пройденный тест, а воркер среза 1 разбирал имена только для Go — для Python правило «прошёл каждый скрытый тест по имени» ничего бы не проверяло;
- правка тестовых файлов запрещена и для Python (`proofs.TestFileTouched`): `test_*.py`, `*_test.py`, `conftest.py`, конфиги pytest, `sitecustomize.py`, `*.pth`; парсер pytest читает только итоговую сводку, и провал теста в ней перевешивает строку `PASSED` с тем же именем;
- `ExpireStale` видит задачи направлений (у квалификационного proof `task_slug` пуст, а старый запрос соединял только с `proof_tasks`) и возвращает id; прогон без открытого proof дольше минуты продвигает `SweepStalled` — так закрываются `FailOversized` и падение API между завершением proof и продвижением прогона; `OnProofFinished` идемпотентен;
- пока прогон идёт, `proofs.Create`, `CreateWithRepo` и `Retry` отвечают 409 `qualification_in_progress`: иначе базовая проверка или прогон танков займут единственный открытый слот между задачами, и прогон зависнет;
- квалификационные proof не попадают в `List`, `Latest` (`last_proof` в `/me` и `/connector/status`), дневной лимит проверок и `Retry`;
- `Start` требует онлайн-коннектор (`agent_offline`), как `proofs.Create`, иначе офлайн-агент сжигает попытку дня на `not_claimed`;
- версия агента и рейтинги попадают в `/me` через `agents.Overview`, а в `GET /connector/status` — через `cmd/api/compose.go`; `arena status` не шлёт heartbeat (так сделано в доделках среза 1) и печатает версию и рейтинги из `/connector/status`;
- тестовые diff настоящие: `git apply` отвергает патч без изменений, поэтому тесты шлют `noteDiff`, который добавляет `NOTES.md`; fake-результаты берут имена скрытых тестов из каталога;
- новые страницы добавляются в `frontend/scripts/check-mobile.mjs` (CI проверяет ширину 375px);
- **репозиторий `artur-gavronchuk/tolerance` публичный**, поэтому скрытые тесты шести задач из этого плана (и `backend/fixtures/skills`) видны всем — это учебные задачи для тестов, CI и локальной разработки. Рейтинг в проде считается по приватному каталогу того же формата: он лежит в отдельном приватном репозитории и подключается в контейнер API томом на `ARENA_SKILLS_DIR`; в образ и в этот репозиторий он не попадает (шаг 8 задачи 3).

## Global Constraints

- Все правила среза 1: `ARENA_` префикс, UTC в JSON, ошибки только `httpx.Problem`, ответы e2e валидируются по `openapi.yaml`, `ARENA_TEST_REQUIRE_DOCKER=1` в CI, никаких моков во фронте, 375px без горизонтального скролла.
- Шкала рейтинга 1000–2400; `target = 1000 + 1400·score`; `uncertainty = max(60, round(350/√n))`; уровни по `access = rating − uncertainty`: `verified ≥ 1500`, `strong ≥ 1800`, `elite ≥ 2100`.
- Прогон = ровно 3 задачи, последовательно; один открытый `proof` на агента (индекс среза 1 не меняется); 3 прогона на направление в сутки; квалификация только в стадии `operational`.
- Балл задачи = `passed_hidden / hidden_tests` из манифеста (не из числа найденных тестов: провал сборки = 0); `passed_hidden` — сколько имён из `proofs.HiddenTestNames` прошло в `sandbox_result`, посторонние тесты не считаются; `expired` = 0; `infra_error` переставляется один раз, второй раз — исключается из среднего.
- Вердикт среза 1 действует для всех видов proof: `passed` — только если прошёл каждый скрытый тест по имени; diff, который трогает тестовые файлы языка задачи, — `failed/test_file_modified`.
- `proofs.kind`: `proof | game_bot | qualification`. Стадия агента, `last_proof`, `GET /proofs`, дневной лимит проверок и `Retry` квалификационных proof не видят; их показывает `/qualifications/{id}`.
- Пока у агента прогон в `running`, proof другого вида не создаётся: 409 `qualification_in_progress`.
- Скрытые тесты квалификации никогда не покидают сервер; в API для `kind = qualification` `sandbox_result.tests` отдаётся без имён (`name` заменяется на `hidden-1..N`), `output` не отдаётся.
- Статусы `qualification_runs`: `running | scored | aborted`. Статусы `proofs` не меняются.
- Коммиты завершаются строкой `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Review Focus

1. **Python-агент добавляет свой тест или `conftest.py`** (свой `test_mine.py` проходит, `conftest.py` помечает все тесты пройденными): diff с таким файлом — `failed/test_file_modified`; посторонний пройденный тест никогда не прибавляет балл. Тесты в задачах 3 и 4.
2. **Владелец жмёт «Run a proof» или запускает агента танков между задачами прогона**: 409 `qualification_in_progress`, прогон переходит к следующей задаче, а не зависает без открытого proof. Тест в задаче 4.
3. **API упал между завершением proof и продвижением прогона** (или результат больше 256 KiB, и proof закрыл `FailOversized` в обход воркера): через минуту `SweepStalled` создаёт следующую задачу; повторный вызов вторую не создаёт. Тест в задаче 4.
4. **Задача вернула `infra_error` дважды подряд**: прогон продолжается с 2 задачами в среднем, а не зависает и не считает 0. Тест в задаче 4.
5. **Публичный профиль по имени в другом регистре** (`/agents/Fixer-7`): находится (индекс `agents_name_ci_idx` на `lower(name)` есть со среза 1), e-mail не утекает. Тест в задаче 5.

Тесты прежних пунктов (heartbeat с другим digest — задача 2, ротация пула при третьем прогоне — задача 3) остаются в своих задачах.

## Структура файлов

```
backend/
  migrations/00004_qualification.sql        версии, направления, задачи, прогоны, рейтинги, колонки proofs
  internal/rating/rating.go, rating_test.go  чистая формула
  internal/agents/version.go                 версии агента: EnsureVersion, текущая версия в Overview
  internal/skills/catalog.go                 skills + skill_tasks из fixtures/skills
  internal/skills/service.go, http.go        GET /skills, публичный профиль-часть
  internal/qualifications/model.go, service.go, http.go, advance.go   прогоны
  internal/proofs/*                          kind qualification, skill_task_slug, маскирование, хук OnProofFinished, запрет на время прогона
  internal/proofs/hidden.go                  HiddenTestNames, TestFileTouched по языку задачи
  internal/proofs/sandbox/pytest.go          парсер pytest
  internal/proofs/sandbox/fake.go            PassAll понимает Python
  fixtures/skills/go/{skill.json,Dockerfile,lru-cache-eviction,worker-pool-shutdown,cursor-pagination}
  fixtures/skills/python/{skill.json,Dockerfile,sliding-rate-limiter,interval-merge,toposort-deps}
  cmd/arena/version.go                       digest конфигурации
  cmd/api/handler.go, compose.go, main.go, main_test.go   маршруты, версия и рейтинги в /me и /connector/status, SweepStalled, e2e
  contracts/openapi/openapi.yaml
frontend/app/app/skills/page.tsx, app/app/qualifications/[id]/page.tsx, app/agents/[name]/page.tsx,
  components/skill-card.tsx, components/rating-pill.tsx, lib/access.ts, lib/types.ts
frontend/scripts/check-mobile.mjs          новые страницы в проверке 375px
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

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: Миграция среза 2 и версии агента

**Files:**
- Create: `backend/migrations/00004_qualification.sql`, `backend/internal/agents/version.go`, `backend/internal/agents/version_integration_test.go`
- Modify: `backend/internal/agents/model.go`, `backend/internal/agents/presence.go`, `backend/internal/agents/http.go`, `backend/internal/agents/service.go`, `backend/cmd/api/compose.go`, `backend/internal/platform/db/db_integration_test.go`

**Interfaces:**
- Produces: `agents.Version{ID string; Number int; Model, Harness, ConfigDigest string; CreatedAt time.Time}`; `agents.VersionInput{Model, Harness, ConfigDigest string}`; `(*Service).EnsureVersion(ctx, agentID string, in VersionInput) (Version, bool /*created*/, error)`; `(*Service).CurrentVersion(ctx, agentID) (*Version, error)`; `Overview.Version *Version`; хук `agents.VersionListener interface{ OnNewVersion(ctx, tx pgx.Tx, agentID, versionID string) error }` и `(*Service).SetVersionListener(l VersionListener)` (реализует сервис рейтингов в задаче 4); heartbeat принимает `version`.

- [ ] **Step 1: Миграция**

`backend/migrations/00004_qualification.sql`:

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
    active boolean NOT NULL DEFAULT true,
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

-- proofs.kind comes from 00003_games (proof | game_bot); its inline CHECK is proofs_kind_check.
ALTER TABLE proofs DROP CONSTRAINT proofs_kind_check;
ALTER TABLE proofs ADD CONSTRAINT proofs_kind_check CHECK (kind IN ('proof', 'game_bot', 'qualification'));
ALTER TABLE proofs
    ADD COLUMN qualification_run_id text REFERENCES qualification_runs (id),
    ADD COLUMN position int,
    ADD COLUMN skill_task_slug text REFERENCES skill_tasks (slug),
    ADD COLUMN retried_infra boolean NOT NULL DEFAULT false;
ALTER TABLE proofs ALTER COLUMN task_slug DROP NOT NULL;
ALTER TABLE proofs ADD CONSTRAINT proofs_task_ref CHECK (
    (kind IN ('proof', 'game_bot') AND task_slug IS NOT NULL) OR (kind = 'qualification' AND skill_task_slug IS NOT NULL AND qualification_run_id IS NOT NULL AND position BETWEEN 1 AND 3));
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
DELETE FROM proofs WHERE kind = 'qualification';
ALTER TABLE proofs DROP CONSTRAINT proofs_task_ref;
ALTER TABLE proofs DROP COLUMN qualification_run_id, DROP COLUMN position, DROP COLUMN skill_task_slug, DROP COLUMN retried_infra;
ALTER TABLE proofs ALTER COLUMN task_slug SET NOT NULL;
ALTER TABLE proofs DROP CONSTRAINT proofs_kind_check;
ALTER TABLE proofs ADD CONSTRAINT proofs_kind_check CHECK (kind IN ('proof', 'game_bot'));
DROP TABLE qualification_runs;
DROP TABLE skill_tasks;
DROP TABLE skills;
ALTER TABLE agents DROP COLUMN current_version_id;
DROP TABLE agent_versions;
```

Имя `proofs_kind_check` — то, что Postgres даёт inline CHECK в `ADD COLUMN` миграции танков; проверить `\d proofs` в тестовой базе и поправить, если отличается. `DELETE` в Down нужен, чтобы вернуть прежний CHECK на базе с прогонами.

В `db_integration_test.go` добавить в список таблиц (после таблиц танков) `agent_versions, skills, skill_tasks, qualification_runs, skill_ratings`.

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

В `service.go`: поле `versions VersionListener` в `Service`. В `model.go`: `Overview` получает `Version *Version \`json:"version"\``; `heartbeatInput` получает `Version *VersionInput \`json:"version"\``. В `presence.go` `overview`: после presence `o.Version, err = s.CurrentVersion(ctx, a.ID)`. В `http.go` heartbeat (сейчас он вызывает `s.Heartbeat(ctx, agentID, in.ConnectorVersion, in.Hostname)` и отвечает `{"agent": {id, name, stage}}`): если `in.Version != nil && in.Version.ConfigDigest != ""`, вызвать `s.EnsureVersion` перед `Heartbeat`; в блок `agent` ответа добавить `"version": o.Version`. `cmd/api/compose.go` `connectorStatus`: в блок `agent` добавить `"version": o.Version` — `arena status` берёт версию отсюда, heartbeat он не шлёт. `/me` получает `version` без правок: `ownerAgent` встраивает `*agents.Overview`. В `openapi.yaml` лишние поля не запрещены (`additionalProperties` нигде не задан), поэтому схемы дополняются в задаче 5, а e2e не падает раньше.

Run: `ARENA_TEST_REQUIRE_DOCKER=1 go test ./internal/agents/ ./internal/platform/db/ -v` → PASS.

- [ ] **Step 4: Коммит**

```bash
cd backend && gofmt -l . && go vet ./... && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./...
git add -A && git commit -m "Add slice 2 schema and agent versions from connector config digest

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: Каталог направлений, шесть задач, парсер pytest, образы

**Files:**
- Create: `backend/internal/skills/catalog.go`, `backend/internal/skills/catalog_test.go`, `backend/internal/skills/pick.go`, `backend/internal/skills/pick_test.go`, `backend/internal/proofs/sandbox/pytest.go`, `backend/internal/proofs/sandbox/pytest_test.go`, `backend/fixtures/skills/go/skill.json`, `backend/fixtures/skills/go/Dockerfile`, `backend/fixtures/skills/python/skill.json`, `backend/fixtures/skills/python/Dockerfile`, шесть каталогов задач
- Create (дополнительно): `backend/internal/proofs/hidden.go`, `backend/internal/proofs/hidden_test.go`, `backend/internal/skills/hidden_count_test.go`
- Modify: `backend/cmd/migrate/main.go`, `backend/Dockerfile`, `Makefile`, `.github/workflows/ci.yml`, `backend/internal/proofs/sandbox/docker.go` (выбор парсера), `backend/internal/proofs/sandbox/runner.go`, `backend/internal/proofs/sandbox/fake.go`, `backend/internal/proofs/sandbox/fake_test.go`, `backend/internal/proofs/worker.go`

**Interfaces:**
- Produces: `skills.Skill{Slug, Title, Language, Image, RunCmd, Description string}`; `skills.Task{Slug, SkillSlug, Title string; Difficulty, AgentTimeoutS, SandboxTimeoutS, HiddenTests int; TaskMD string; RepoTar, HiddenTar []byte; RepoSHA256 string}`; `skills.LoadCatalog(dir) ([]Skill, []Task, error)`; `skills.SyncCatalog(ctx, pool, skills, tasks) error`; `skills.Pick(pool []string, recent []string, n int, rnd *rand.Rand) []string`; `sandbox.ParsePytest([]byte) []TestResult`; `sandbox.Request.Language string` и выбор парсера по нему в `Docker.Run`; `proofs.HiddenTestNames(language string, hiddenTar []byte) ([]string, error)` (Go — имена `Test…`, Python — `path::test_name`); `proofs.TestFileTouched(language, diff string) bool`; `sandbox.PassAll` учитывает `Request.Language`; `skills.LoadCatalog` отвергает задачу, у которой число скрытых имён ≠ `hidden_tests`.

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
			t, err := loadTask(tdir, s.Slug, s.Language)
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

func loadTask(dir, skill, language string) (Task, error) {
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
	// The score divides by hidden_tests and counts hidden tests by name, so
	// the manifest and the files must agree on how many there are.
	names, err := proofs.HiddenTestNames(language, hiddenTar)
	if err != nil {
		return Task{}, err
	}
	if len(names) != m.HiddenTests {
		return Task{}, fmt.Errorf("skills: %s: manifest says %d hidden tests, _hidden has %d: %v", dir, m.HiddenTests, len(names), names)
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

import (
	"reflect"
	"testing"
)

func TestParsePytest(t *testing.T) {
	// The captured stdout of a failing test and whatever an atexit hook
	// prints after the summary both try to pose as results.
	out := []byte(`............F
=================================== FAILURES ===================================
____________________________ test_hidden_boundary ____________________________
----------------------------- Captured stdout call -----------------------------
PASSED test_hidden_limiter.py::test_hidden_boundary_is_exclusive
assert False
=========================== short test summary info ============================
PASSED test_limiter.py::test_allows_up_to_limit
PASSED test_limiter.py::test_window_slides
FAILED test_hidden_limiter.py::test_hidden_boundary_is_exclusive - assert False
ERROR test_other.py::test_broken - ImportError
ERROR test_hidden_more.py - ImportError while importing test module
1 failed, 2 passed, 2 errors in 0.03s
PASSED test_hidden_limiter.py::test_hidden_boundary_is_exclusive
PASSED test_hidden_more.py::test_hidden_x
`)
	got := ParsePytest(out)
	want := []TestResult{
		{Name: "test_limiter.py::test_allows_up_to_limit", Passed: true},
		{Name: "test_limiter.py::test_window_slides", Passed: true},
		{Name: "test_hidden_limiter.py::test_hidden_boundary_is_exclusive", Passed: false},
		{Name: "test_other.py::test_broken", Passed: false},
		{Name: "test_hidden_more.py::test_hidden_x", Passed: false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v", got)
	}
	if len(ParsePytest([]byte("ImportError while importing test module"))) != 0 {
		t.Fatalf("collection failure yields no tests")
	}
	if len(ParsePytest([]byte("PASSED test_x.py::test_a\n"))) != 0 {
		t.Fatalf("a result line without the summary header is not a result")
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

// ParsePytest reads the `-rA` short summary pytest prints at the end of a
// run: "PASSED path::name", "FAILED path::name - reason", "ERROR path::name
// - reason", and "ERROR path - reason" for a module that failed to import.
// Only lines after the first summary header count, so what a test prints
// into its captured output cannot pose as a result, and a test reported as
// failed anywhere after that header stays failed whatever else claims it
// passed. A module-level ERROR fails every test of that module the summary
// names. Code under test runs in the same process and could still forge
// output (the same holds for go test); this only closes the cheap tricks.
func ParsePytest(out []byte) []TestResult {
	var res []TestResult
	index := map[string]int{}
	var broken []string
	inSummary := false
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if !inSummary {
			inSummary = strings.HasPrefix(line, "=") && strings.Contains(line, " short test summary info ")
			continue
		}
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
		if name == "" {
			continue
		}
		if !strings.Contains(name, "::") {
			if !passed {
				broken = append(broken, name+"::")
			}
			continue
		}
		if i, ok := index[name]; ok {
			res[i].Passed = res[i].Passed && passed
			continue
		}
		index[name] = len(res)
		res = append(res, TestResult{Name: name, Passed: passed})
	}
	for i := range res {
		for _, prefix := range broken {
			if strings.HasPrefix(res[i].Name, prefix) {
				res[i].Passed = false
			}
		}
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

`proofs/worker.go`: `Language` в `sandbox.Request` передаётся в шаге 6.

- [ ] **Step 6: Скрытые тесты по именам и запрет правки тестов для обоих языков**

Воркер среза 1 знает имена скрытых тестов только для Go (`hiddenTestNames` ищет `func Test…` в `.go`) и узнаёт тестовые файлы только по `_test.go`. Для Python-задачи список имён был бы пуст — правило «прошёл каждый скрытый тест по имени» ничего бы не проверяло, — а `conftest.py` из diff агента переписал бы результаты pytest. Обе проверки переезжают в `proofs/hidden.go` и получают язык задачи.

`backend/internal/proofs/hidden_test.go`:

```go
package proofs

import "testing"

func TestHiddenTestNames(t *testing.T) {
	goTar, err := TarFiles(map[string][]byte{
		"hidden_test.go": []byte("package x\n\nfunc TestMain(m *testing.M) {}\nfunc TestHidden_A(t *testing.T) {}\nfunc helper() {}\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := HiddenTestNames("go", goTar)
	if err != nil || len(got) != 1 || got[0] != "TestHidden_A" {
		t.Fatalf("go: %v %v", got, err)
	}
	pyTar, err := TarFiles(map[string][]byte{
		"test_hidden_limiter.py":    []byte("import limiter\n\ndef test_hidden_a():\n    pass\n\ndef helper():\n    pass\n\nclass TestX:\n    def test_method(self):\n        pass\n"),
		"tests/test_hidden_more.py": []byte("def test_hidden_b():\n    pass\n"),
		"limiter_fixture.py":        []byte("def test_not_collected():\n    pass\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err = HiddenTestNames("python", pyTar)
	if err != nil || len(got) != 2 || got[0] != "test_hidden_limiter.py::test_hidden_a" || got[1] != "tests/test_hidden_more.py::test_hidden_b" {
		t.Fatalf("python: %v %v", got, err)
	}
	if _, err := HiddenTestNames("rust", pyTar); err == nil {
		t.Fatalf("unknown language must be an error")
	}
}

func TestTestFileTouched(t *testing.T) {
	cases := []struct {
		lang, diff string
		want       bool
	}{
		{"go", "diff --git a/retry_test.go b/retry_test.go\n--- a/retry_test.go\n+++ b/retry_test.go\n", true},
		{"go", "diff --git a/retry.go b/retry.go\n--- a/retry.go\n+++ b/retry.go\n", false},
		{"python", "diff --git a/test_mine.py b/test_mine.py\nnew file mode 100644\n--- /dev/null\n+++ b/test_mine.py\n", true},
		{"python", "diff --git a/pkg/limiter_test.py b/pkg/limiter_test.py\n", true},
		{"python", "diff --git a/conftest.py b/conftest.py\n--- /dev/null\n+++ b/conftest.py\n", true},
		{"python", "diff --git a/pyproject.toml b/pyproject.toml\n", true},
		{"python", "diff --git a/sitecustomize.py b/sitecustomize.py\n", true},
		{"python", "diff --git a/evil.pth b/evil.pth\n", true},
		{"python", "rename from limiter.py\nrename to test_limiter.py\n", true},
		{"python", "diff --git \"a/my dir/test_x.py\" \"b/my dir/test_x.py\"\n", true},
		{"python", "diff --git a/limiter.py b/limiter.py\n--- a/limiter.py\n+++ b/limiter.py\n@@ -1 +1 @@\n-# see test_old.py\n+# conftest.py is mentioned in a comment\n", false},
		{"python", "diff --git a/latest.py b/latest.py\n", false},
	}
	for _, c := range cases {
		if got := TestFileTouched(c.lang, c.diff); got != c.want {
			t.Errorf("%s %q: got %v", c.lang, c.diff, got)
		}
	}
}
```

`backend/internal/proofs/hidden.go` (из `worker.go` удалить `hiddenTestNames`, `goTestFunc`, `testFileHeaders` и `diffTouchesTestFiles` — они переезжают сюда):

```go
package proofs

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
)

var (
	goTestFunc = regexp.MustCompile(`(?m)^func (Test\w+)\(`)
	pyTestFunc = regexp.MustCompile(`(?m)^def (test_\w+)\(`)
)

func isPyTestFile(base string) bool {
	return strings.HasSuffix(base, ".py") && (strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py"))
}

// HiddenTestNames lists the tests in a hidden-tests tarball the way the
// sandbox reports them: top-level Test functions for Go, "path::test_name"
// for pytest (top-level functions of test_*.py and *_test.py only; hidden
// Python tests are written that way). The tarball comes from the server's
// catalog and is never touched by the participant, so these names are what
// a verdict and a score may count.
func HiddenTestNames(language string, hiddenTar []byte) ([]string, error) {
	if language != "go" && language != "python" {
		return nil, fmt.Errorf("proofs: hidden tests: unsupported language %q", language)
	}
	gz, err := gzip.NewReader(bytes.NewReader(hiddenTar))
	if err != nil {
		return nil, fmt.Errorf("proofs: hidden tests: %w", err)
	}
	tr := tar.NewReader(gz)
	var names []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("proofs: hidden tests: %w", err)
		}
		name := strings.TrimPrefix(h.Name, "./")
		base := path.Base(name)
		if h.Typeflag != tar.TypeReg {
			continue
		}
		if language == "go" && !strings.HasSuffix(base, ".go") || language == "python" && !isPyTestFile(base) {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(tr, 16<<20))
		if err != nil {
			return nil, fmt.Errorf("proofs: hidden tests: %w", err)
		}
		if language == "go" {
			for _, m := range goTestFunc.FindAllStringSubmatch(string(body), -1) {
				if m[1] != "TestMain" {
					names = append(names, m[1])
				}
			}
			continue
		}
		for _, m := range pyTestFunc.FindAllStringSubmatch(string(body), -1) {
			names = append(names, name+"::"+m[1])
		}
	}
	return names, nil
}

// testFileHeaders are the patch lines that name a file the patch touches.
var testFileHeaders = []string{"diff --git ", "--- ", "+++ ", "rename from ", "rename to ", "copy from ", "copy to "}

// pyGuarded are files that change how pytest collects or reports tests, or
// run code before pytest does.
var pyGuarded = map[string]bool{"conftest.py": true, "pytest.ini": true, "tox.ini": true, "setup.cfg": true,
	"pyproject.toml": true, "sitecustomize.py": true, "usercustomize.py": true}

// TestFileTouched reports whether the patch adds, edits, deletes or renames
// a test file of the task's language. Participants fix the code, not the
// tests: for Go a *_test.go file (a TestMain could narrow what runs); for
// Python a test module, conftest.py or pytest's config (each can rewrite
// results), or a startup hook (sitecustomize.py, *.pth).
func TestFileTouched(language, diff string) bool {
	for _, line := range strings.Split(diff, "\n") {
		for _, prefix := range testFileHeaders {
			if !strings.HasPrefix(line, prefix) {
				continue
			}
			if language == "go" {
				if strings.Contains(line, "_test.go") {
					return true
				}
				continue
			}
			for _, f := range strings.Fields(line[len(prefix):]) {
				base := path.Base(strings.Trim(f, `"`))
				if pyGuarded[base] || strings.HasSuffix(base, ".pth") || isPyTestFile(base) {
					return true
				}
			}
		}
	}
	return false
}
```

`proofs/worker.go` `RunProof`: в `RETURNING` добавить `t.language` (колонка есть в `proof_tasks` со среза 1) и сканировать в `in.task.Language`; `hiddenTestNames(in.hiddenTr)` → `HiddenTestNames(in.task.Language, in.hiddenTr)`; `diffTouchesTestFiles(in.diff)` → `TestFileTouched(in.task.Language, in.diff)`; в `sandbox.Request` передать `Language: in.task.Language`. В ветке проверок (не `game_bot`: у бота танков скрытых тестов нет) пустой список имён — ошибка платформы: `return fmt.Errorf("proofs: task %s has no hidden tests", in.task.Slug)` (job повторится, затем `infra_error`), а не молчаливый `passed`. Существующие тесты воркера на `test_file_modified` и `hidden_test_missing_or_failed` должны остаться зелёными без правок.

`sandbox/fake.go` `PassAll`: для `req.Language == "python"` сообщать пройденными top-level `def test_…` из `test_*.py` и `*_test.py` под именем `<путь от WorkDir через "/">::<функция>`; иначе — как сейчас (`func Test…` из `*_test.go`). Так `ARENA_SANDBOX=fake` доводит до `passed` и Python-задачи, а правило имён остаётся честным. В `fake_test.go` добавить `TestPassAllPython`: в `t.TempDir()` файлы `test_a.py` (`def test_one():` и `def helper():`) и `pkg/test_b.py` (`def test_two():`) → `Tests` ровно `test_a.py::test_one` и `pkg/test_b.py::test_two`, оба `Passed`.

`backend/internal/skills/hidden_count_test.go`:

```go
package skills_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tolerance/internal/skills"
)

func TestLoadCatalog_HiddenCountMustMatchManifest(t *testing.T) {
	root := t.TempDir()
	write := func(p, body string) {
		t.Helper()
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("python/skill.json", `{"slug": "python", "title": "Python", "language": "python", "image": "arena-skill-python:1", "run_cmd": "python -m pytest -q -rA -p no:cacheprovider"}`)
	write("python/Dockerfile", "FROM python:3.12-alpine\n")
	write("python/t1/manifest.json", `{"slug": "py-t1", "title": "T1", "difficulty": 1, "agent_timeout_s": 60, "sandbox_timeout_s": 60, "hidden_tests": 3}`)
	write("python/t1/TASK.md", "Fix it.\n")
	write("python/t1/repo/m.py", "x = 1\n")
	write("python/t1/_hidden/test_hidden_m.py", "def test_hidden_a():\n    pass\n\ndef test_hidden_b():\n    pass\n")
	if _, _, err := skills.LoadCatalog(root); err == nil || !strings.Contains(err.Error(), "hidden") {
		t.Fatalf("manifest says 3, files have 2: want an error, got %v", err)
	}
}
```

Run: `go test ./internal/proofs/ ./internal/proofs/sandbox/ ./internal/skills/ -v` → PASS; `ARENA_TEST_REQUIRE_DOCKER=1 go test ./internal/proofs/ -run Worker -v` → PASS (срез 1 не сломан). Реальный каталог (`fixtures/skills`) грузится без ошибок: у всех шести задач число скрытых имён равно `hidden_tests`.

- [ ] **Step 7: Docker-тест одной go- и одной python-задачи**

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
		names, err := proofs.HiddenTestNames(c.language, task.HiddenTar)
		if err != nil {
			t.Fatal(err)
		}
		passedByName := map[string]bool{}
		for _, tr := range res.Tests {
			if tr.Passed {
				passedByName[tr.Name] = true
			}
		}
		hiddenPassed := 0
		for _, n := range names {
			if passedByName[n] {
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

- [ ] **Step 8: Синхронизация, образы, коммит**

`cmd/migrate/main.go`: после proofs-каталога, если `ARENA_SKILLS_DIR` задан, `skills.LoadCatalog` + `skills.SyncCatalog`. Каталог направлений **не** копируется в образ: репозиторий публичный, и скрытые тесты из `fixtures/skills` известны всем. `docker-compose.yml`, сервис `api`: том `${ARENA_SKILLS_SOURCE:-./backend/fixtures/skills}:/opt/arena/skills:ro` и `ARENA_SKILLS_DIR: /opt/arena/skills`; в `.env.example` — закомментированный `ARENA_SKILLS_SOURCE=../arena-tasks/skills` с пояснением: локально по умолчанию берутся учебные задачи, на сервере — путь к приватному каталогу. Задача, пропавшая из каталога, остаётся в `skill_tasks` (на неё ссылаются прошлые proof), но `Start` выбирает только из задач, обновлённых последней синхронизацией: `SyncCatalog` в той же транзакции ставит загруженным `active = true` (в `ON CONFLICT … DO UPDATE` тоже) и `active = false` тем, которых в каталоге нет (колонка `active boolean NOT NULL DEFAULT true` добавляется в миграцию `00004`), а запрос пула в `Start` и `pool_size` в `GET /skills` фильтруют `active`. Тест в `catalog_test.go`: синхронизировать каталог из двух задач, затем из одной → вторая `active = false`, первая `true`. `Makefile`: цель `proof-image` дополняется `docker build -q -t arena-skill-go:1 backend/fixtures/skills/go && docker build -q -t arena-skill-python:1 backend/fixtures/skills/python`; `migrate` получает `ARENA_SKILLS_DIR=./fixtures/skills`. `.github/workflows/ci.yml`: те же две сборки образов перед тестами.

```bash
cd backend && gofmt -l . && go vet ./... && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./...
git add -A && git commit -m "Add skills catalog with six hidden tasks, pytest parser and skill images

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: Прогоны квалификации: создание, продвижение воркером, рейтинг

**Files:**
- Create: `backend/internal/qualifications/model.go`, `backend/internal/qualifications/service.go`, `backend/internal/qualifications/advance.go`, `backend/internal/qualifications/ratings.go`, `backend/internal/qualifications/service_integration_test.go`
- Modify: `backend/internal/proofs/model.go`, `backend/internal/proofs/service.go`, `backend/internal/proofs/worker.go`, `backend/internal/proofs/service_integration_test.go`, `backend/cmd/api/main.go`

**Interfaces:**
- Consumes: `rating.*`, `skills.Pick`, `agents.ComputeStage`, `agents.VersionListener`, `proofs.Worker`.
- Produces в `proofs`: константа `KindQualification = "qualification"`; поля `Proof.QualificationRunID *string, Position *int, SkillTaskSlug *string` (`Proof.Kind` уже есть); `proofs.FinishListener interface{ OnProofFinished(ctx, proofID string) error }`, `(*Worker).SetFinishListener(l)` — воркер вызывает его после каждого своего терминального перехода (`finish`, `MarkInfraError`, id из `ExpireStale`); `(*Service).ExpireStale(ctx) ([]string, error)`; `(*Service).CreateQualificationProof(ctx, tx pgx.Tx, agentID, runID, taskSlug string, position int) (Proof, error)`; `(*Service).MaskHidden(p Proof) Proof`; `(*Service).GetByIDTx(ctx, tx, id) (Proof, error)`; 409 `qualification_in_progress` из `Create`, `CreateWithRepo`, `Retry`; выдача коннектору и воркер читают задачу из `skill_tasks`, когда `kind = qualification`.
- Produces в `qualifications`: `Run{ID, AgentID, VersionID, SkillSlug, Status string; CreatedAt time.Time; FinishedAt *time.Time; Score *float64; RatingBefore, RatingAfter, UncertaintyAfter *int; TaskSlugs []string; Tasks []proofs.Proof}`; `SkillRating{SkillSlug string; Rating, Uncertainty, Access int; Tier string; Verified bool; Runs int; VersionID string; VersionNumber int; OnCurrentVersion bool; PriorRating *int}`; `NewService(pool, proofsSvc *proofs.Service) *Service`; `(*Service).Start(ctx, userID, skill string) (Run, error)`; `List(ctx, userID) ([]Run, error)`; `Get(ctx, userID, id) (Run, error)`; `RatingsFor(ctx, agentID) ([]SkillRating, error)`; `OnProofFinished` (реализует `proofs.FinishListener`, идемпотентен); `SweepStalled(ctx) (int, error)`; `OnNewVersion` (реализует `agents.VersionListener`); `Abort(ctx, runID, reason)`; ошибки `agent_not_operational` (409), `agent_offline` (409), `qualification_in_progress` (409), `daily_limit` (429), `no_version` (409), `unknown_skill` (404).

- [ ] **Step 1: Расширить proofs**

`proofs/model.go`: константа `KindQualification = "qualification"` рядом с `KindProof` и `KindGameBot` (танки); в `Proof` добавить `QualificationRunID *string \`json:"qualification_run_id"\``, `Position *int \`json:"position"\``, `SkillTaskSlug *string \`json:"skill_task_slug"\`` (`Proof.Kind` уже есть). `proofCols`: `task_slug` заменить на `coalesce(task_slug, skill_task_slug) AS task_slug` — у квалификационного proof `task_slug` NULL, а `Proof.TaskSlug` остаётся `string` и показывает slug задачи направления; в конец добавить `qualification_run_id, position, skill_task_slug`; `scanProof` сканирует их. `Task.Language` уже есть со среза 1.

`proofs/service.go`:

```go
// MaskHidden strips what a qualification proof must not reveal: hidden
// test names and sandbox output. Proof-kind proofs are returned as is.
func (s *Service) MaskHidden(p Proof) Proof {
	if p.Kind != KindQualification || p.SandboxResult == nil {
		return p
	}
	masked := *p.SandboxResult
	masked.Output = ""
	masked.Tests = make([]TestResult, len(p.SandboxResult.Tests))
	for i, t := range p.SandboxResult.Tests {
		masked.Tests[i] = TestResult{Name: fmt.Sprintf("hidden-%d", i+1), Passed: t.Passed}
	}
	p.SandboxResult = &masked
	return p
}

// CreateQualificationProof queues one task of a qualification run inside
// the caller's transaction. The one-open-proof index guards the invariant.
func (s *Service) CreateQualificationProof(ctx context.Context, tx pgx.Tx, agentID, runID, taskSlug string, position int) (Proof, error) {
	var p Proof
	err := scanProof(tx.QueryRow(ctx, `INSERT INTO proofs (id, agent_id, kind, qualification_run_id, position, skill_task_slug)
		VALUES ($1, $2, 'qualification', $3, $4, $5) RETURNING `+proofCols, idgen.New("proof"), agentID, runID, position, taskSlug), &p)
	return p, err
}
```

Запрет на время прогона (Review Focus 2). Между задачами прогона открытого proof нет, и без запрета базовая проверка или прогон танков заняли бы единственный слот, а следующая задача упёрлась бы в `proofs_one_open_idx`:

```go
// qualificationOpen refuses a new proof while the agent's qualification run
// is in progress: its tasks come one after another through the single
// open-proof slot, and nothing else may take the slot between them. proofs
// reads the table directly; importing qualifications would be a cycle.
func qualificationOpen(ctx context.Context, tx pgx.Tx, agentID string) error {
	var open bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM qualification_runs WHERE agent_id = $1 AND status = 'running')`, agentID).Scan(&open); err != nil {
		return err
	}
	if open {
		return httpx.New(http.StatusConflict, "qualification_in_progress", "A qualification run is in progress; wait for it to finish")
	}
	return nil
}
```

`Create` и `CreateWithRepo` вызывают его сразу после определения `agentID`. `Retry`: выборку дополнить `kind` и `agent_id`; квалификационный proof → `httpx.StateConflict("Qualification tasks are retried by the platform")`; для остальных — `qualificationOpen`. `List` и `Latest`: `AND kind <> 'qualification'` (квалификационные proof показывает `/qualifications/{id}`, а `last_proof` в `/me` и `/connector/status` остаётся про проверки). Дневной лимит в `Create` (и в `CreateWithRepo`, если он считает тем же запросом): `AND kind <> 'qualification'` — у прогонов свой лимит. `Get` возвращает `s.MaskHidden(p)`.

`task(ctx, tx, slug)` заменить на `taskFor(ctx, tx, p Proof) (*Task, error)`:

- `p.Kind == KindQualification`: `SELECT t.slug, t.title, s.language, s.image, s.run_cmd, t.agent_timeout_s, t.sandbox_timeout_s, 0, t.hidden_tests, t.task_md, t.repo_sha256 FROM skill_tasks t JOIN skills s ON s.slug = t.skill_slug WHERE t.slug = $1` по `*p.SkillTaskSlug`;
- иначе (`proof`, `game_bot`) — прежний запрос к `proof_tasks` по `p.TaskSlug` вместе с подменой репозитория, которую ввели танки (`RepoSHA256 = coalesce(p.repo_sha256, t.repo_sha256)`); её не терять.

`Claim` использует `taskFor`. `RepoTar`: `SELECT coalesce(p.repo_tar, t.repo_tar, st.repo_tar) FROM proofs p LEFT JOIN proof_tasks t ON t.slug = p.task_slug LEFT JOIN skill_tasks st ON st.slug = p.skill_task_slug WHERE p.id = $1 AND p.agent_id = $2 AND p.status IN ('claimed', 'running_agent')`. `http_connector.go`: `nextTaskResponse.Kind` уже есть (танки), для квалификации там `qualification`.

`ExpireStale` сейчас соединяет proof только с `proof_tasks` по `task_slug`, и квалификационный proof (у него `task_slug` NULL) не истёк бы никогда. Новая версия берёт таймауты из любой из двух таблиц и возвращает id:

```go
// ExpireStale ends proofs nobody will finish: queued ones no connector
// claimed within 5 minutes, and claimed/running ones whose agent timeout
// (plus a minute of slack) has passed without a result. It also sweeps
// proofs whose diff arrived but whose sandbox run never concluded into
// infra_error (reason "stuck"): the agent did its part, the platform did not.
// It returns the ids it ended so the worker can tell its listener.
func (s *Service) ExpireStale(ctx context.Context) ([]string, error) {
	var ids []string
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		collect := func(rows pgx.Rows, err error) error {
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					return err
				}
				ids = append(ids, id)
			}
			return rows.Err()
		}
		// Timeouts come from the proof's task, whichever catalog it is in.
		const limits = `SELECT o.id, coalesce(t.agent_timeout_s, st.agent_timeout_s) AS agent_timeout_s,
			coalesce(t.sandbox_timeout_s, st.sandbox_timeout_s) AS sandbox_timeout_s
			FROM proofs o LEFT JOIN proof_tasks t ON t.slug = o.task_slug LEFT JOIN skill_tasks st ON st.slug = o.skill_task_slug
			WHERE o.status IN ('queued', 'claimed', 'running_agent', 'diff_submitted', 'running_sandbox')`
		if err := collect(tx.Query(ctx, `
			UPDATE proofs p SET status = 'expired', finished_at = now(),
			  failure_reason = CASE WHEN p.status = 'queued' THEN 'not_claimed' ELSE 'agent_timeout' END
			FROM (`+limits+`) x WHERE x.id = p.id AND (
			  (p.status = 'queued' AND p.created_at < now() - interval '5 minutes') OR
			  (p.status IN ('claimed', 'running_agent') AND p.claimed_at < now() - make_interval(secs => x.agent_timeout_s + 60)))
			RETURNING p.id`)); err != nil {
			return err
		}
		// The run_proof job gets 3 attempts, each up to sandbox_timeout_s, with
		// backoff between them, and a crashed worker's 15-minute lease must run
		// out before the job is reclaimed. Past all of that plus slack, nothing
		// is coming.
		return collect(tx.Query(ctx, `
			UPDATE proofs p SET status = 'infra_error', finished_at = now(), failure_reason = 'stuck'
			FROM (`+limits+`) x WHERE x.id = p.id AND p.status IN ('diff_submitted', 'running_sandbox')
			  AND coalesce(p.diff_submitted_at, p.claimed_at, p.created_at) < now() - make_interval(secs => 3 * x.sandbox_timeout_s + 1200)
			RETURNING p.id`))
	})
	return ids, err
}
```

Существующие тесты `ExpireStale` в `service_integration_test.go` переходят на `len(ids)`. Добавить `TestExpireStale_QualificationProof`: вставить через `AdminPool` направление, задачу направления (`agent_timeout_s = 60`), прогон и proof `kind = 'qualification'` в `claimed` с `claimed_at = now() - interval '5 minutes'` → id в результате, статус `expired`, `agent_timeout`.

`proofs/worker.go` `RunProof`: запрос, который оставили танки (с `p.kind` и `coalesce(p.repo_tar, t.repo_tar)`), читает задачу из того каталога, где она лежит:

```sql
UPDATE proofs p SET status = 'running_sandbox'
FROM proofs x LEFT JOIN proof_tasks t ON t.slug = x.task_slug
  LEFT JOIN skill_tasks st ON st.slug = x.skill_task_slug LEFT JOIN skills s ON s.slug = st.skill_slug
WHERE p.id = $1 AND x.id = p.id AND p.status IN ('diff_submitted', 'running_sandbox')
RETURNING p.diff, p.kind, coalesce(t.slug, st.slug), coalesce(t.language, s.language), coalesce(t.image, s.image),
  coalesce(t.run_cmd, s.run_cmd), coalesce(t.sandbox_timeout_s, st.sandbox_timeout_s),
  coalesce(p.repo_tar, t.repo_tar, st.repo_tar), coalesce(t.hidden_tar, st.hidden_tar)
```

Дальше путь квалификационного proof тот же, что у проверки: `TestFileTouched`, `applyDiff`, скрытые тесты, `HiddenTestNames`, тот же вердикт (задача 3). Добавить:

```go
type FinishListener interface {
	OnProofFinished(ctx context.Context, proofID string) error
}

func (w *Worker) SetFinishListener(l FinishListener) { w.onFinish = l }

func (w *Worker) notify(ctx context.Context, proofID string) {
	if w.onFinish == nil {
		return
	}
	if err := w.onFinish.OnProofFinished(ctx, proofID); err != nil {
		w.log.Error("proof finished hook", "proof", proofID, "err", err)
	}
}
```

`finish` и `MarkInfraError` вызывают `w.notify(ctx, proofID)` после успешного коммита. `Worker.Run` на тике: `ids, err := w.svc.ExpireStale(ctx)`, лог по `len(ids)`, `notify` для каждого id. `FailOversized` закрывает proof в обход воркера и хука не вызывает — такой прогон продвигает `SweepStalled` (шаг 4).

- [ ] **Step 2: Интеграционный тест квалификаций**

`backend/internal/qualifications/service_integration_test.go`:

```go
package qualifications_test

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/agents"
	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/proofs"
	"tolerance/internal/proofs/sandbox"
	"tolerance/internal/qualifications"
	"tolerance/internal/skills"
)

type fx struct {
	d       *dbtest.DB
	agents  *agents.Service
	proofs  *proofs.Service
	quals   *qualifications.Service
	worker  *proofs.Worker
	fake    *sandbox.Fake
	hidden  map[string][]string // skill task slug → hidden test names
	userID  string
	agentID string
}

// noteDiff applies to any task repository: it adds a file no test reads.
// git apply refuses a patch without changes, so tests cannot send "--- a/x".
const noteDiff = "diff --git a/NOTES.md b/NOTES.md\nnew file mode 100644\n--- /dev/null\n+++ b/NOTES.md\n@@ -0,0 +1 @@\n+agent notes\n"

func passing(names []string) []sandbox.TestResult {
	out := make([]sandbox.TestResult, len(names))
	for i, n := range names {
		out[i] = sandbox.TestResult{Name: n, Passed: true}
	}
	return out
}

func setup(t *testing.T) *fx {
	t.Helper()
	d := dbtest.New(t)
	ctx := context.Background()
	sk, tasks, err := skills.LoadCatalog(filepath.Join("..", "..", "fixtures", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	if err := skills.SyncCatalog(ctx, d.AdminPool, sk, tasks); err != nil {
		t.Fatal(err)
	}
	langOf := map[string]string{}
	for _, s := range sk {
		langOf[s.Slug] = s.Language
	}
	hidden := map[string][]string{}
	for _, tk := range tasks {
		names, err := proofs.HiddenTestNames(langOf[tk.SkillSlug], tk.HiddenTar)
		if err != nil {
			t.Fatal(err)
		}
		hidden[tk.Slug] = names
	}
	ptasks, _ := proofs.LoadCatalog(filepath.Join("..", "..", "fixtures", "proofs"))
	_ = proofs.SyncCatalog(ctx, d.AdminPool, ptasks)

	ps := proofs.NewService(d.AppPool)
	qs := qualifications.NewService(d.AppPool, ps)
	as := agents.NewService(d.AppPool, ps)
	as.SetVersionListener(qs)
	fake := &sandbox.Fake{}
	w := proofs.NewWorker(d.AppPool, fake, t.TempDir(), slog.New(slog.NewTextHandler(os.Stderr, nil)))
	w.SetFinishListener(qs)

	us := identity.NewService(d.AppPool, nil)
	u, _, _ := us.Signup(ctx, "o@example.com", "longenough1")
	a, _ := as.Create(ctx, u.ID, agents.CreateInput{Name: "fixer"})
	_, _, _ = as.CreateKey(ctx, u.ID, "k")
	_ = as.Heartbeat(ctx, a.ID, "0.2", "h")
	return &fx{d: d, agents: as, proofs: ps, quals: qs, worker: w, fake: fake, hidden: hidden, userID: u.ID, agentID: a.ID}
}

func (f *fx) operational(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	err := f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO proofs (id, agent_id, kind, task_slug, status, finished_at) VALUES ('proof_seed', $1, 'proof', 'go-fix-retry', 'passed', now())`, f.agentID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (f *fx) version(t *testing.T, digest string) agents.Version {
	t.Helper()
	v, _, err := f.agents.EnsureVersion(context.Background(), f.agentID, agents.VersionInput{Model: "m", Harness: "h", ConfigDigest: digest})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func problem(t *testing.T, err error, status int, code string) {
	t.Helper()
	var p *httpx.Problem
	if !errors.As(err, &p) || p.Status != status || p.Code != code {
		t.Fatalf("expected %d %s, got %v", status, code, err)
	}
}

// age moves the run's finished proofs two minutes into the past.
func (f *fx) age(t *testing.T, runID string) {
	t.Helper()
	err := f.d.AdminPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE proofs SET finished_at = finished_at - interval '2 minutes' WHERE qualification_run_id = $1 AND finished_at IS NOT NULL`, runID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// drive plays the connector and the sandbox for the run's current task: the
// given share of its hidden tests pass, and so do two tests outside the
// hidden set (a visible one and one the agent might have added), which must
// never add to the score.
func (f *fx) drive(t *testing.T, share float64) {
	t.Helper()
	ctx := context.Background()
	p, task, err := f.proofs.Claim(ctx, f.agentID)
	if err != nil || p == nil {
		t.Fatalf("claim: %v %+v", err, p)
	}
	if p.Kind != proofs.KindQualification || task.TaskMD == "" {
		t.Fatalf("expected a qualification task, got %+v", p)
	}
	names := f.hidden[task.Slug]
	pass := int(math.Round(share * float64(len(names))))
	tests := []sandbox.TestResult{{Name: "visible_passes", Passed: true}, {Name: "test_agent_added.py::test_extra", Passed: true}}
	for i, n := range names {
		tests = append(tests, sandbox.TestResult{Name: n, Passed: i < pass})
	}
	exit := 0
	if pass < len(names) {
		exit = 1
	}
	f.fake.Result = sandbox.Result{ExitCode: exit, Tests: tests}
	f.fake.Err = nil
	if err := f.proofs.SubmitResult(ctx, f.agentID, p.ID, proofs.ResultInput{Diff: noteDiff}); err != nil {
		t.Fatal(err)
	}
	if err := f.worker.RunProof(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
}

func TestStart_Guards(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_, err := f.quals.Start(ctx, f.userID, "go")
	problem(t, err, 409, "agent_not_operational")
	f.operational(t)
	_, err = f.quals.Start(ctx, f.userID, "go")
	problem(t, err, 409, "no_version")
	f.version(t, "d1")
	_, err = f.quals.Start(ctx, f.userID, "rust")
	problem(t, err, 404, "unknown_skill")
	run, err := f.quals.Start(ctx, f.userID, "go")
	if err != nil || run.Status != "running" || len(run.TaskSlugs) != 3 || len(run.Tasks) != 1 {
		t.Fatalf("start: %v %+v", err, run)
	}
	_, err = f.quals.Start(ctx, f.userID, "python")
	problem(t, err, 409, "qualification_in_progress")
}

func TestRun_ThreeTasksScoreAndRating(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.operational(t)
	f.version(t, "d1")
	run, _ := f.quals.Start(ctx, f.userID, "go")
	f.drive(t, 1)   // task 1: 1.0
	f.drive(t, 0.5) // task 2: about 0.5
	f.drive(t, 0)   // task 3: 0.0
	got, err := f.quals.Get(ctx, f.userID, run.ID)
	if err != nil || got.Status != "scored" || got.Score == nil || len(got.Tasks) != 3 {
		t.Fatalf("%v %+v", err, got)
	}
	// weights follow difficulty of the picked tasks; the test only pins the range and monotonicity
	if *got.Score <= 0 || *got.Score >= 1 {
		t.Fatalf("score must be strictly between 0 and 1 for mixed results, got %v", *got.Score)
	}
	rs, _ := f.quals.RatingsFor(ctx, f.agentID)
	if len(rs) != 1 || rs[0].SkillSlug != "go" || rs[0].Runs != 1 || rs[0].Uncertainty != 350 || !rs[0].OnCurrentVersion {
		t.Fatalf("ratings: %+v", rs)
	}
	if *got.RatingAfter != rs[0].Rating || *got.UncertaintyAfter != 350 || got.RatingBefore != nil {
		t.Fatalf("run rating fields: %+v", got)
	}
	for _, tk := range got.Tasks {
		if tk.SandboxResult != nil && tk.SandboxResult.Tests[0].Name != "hidden-1" {
			t.Fatalf("hidden test names must be masked: %+v", tk.SandboxResult.Tests)
		}
	}
}

func TestRun_InfraErrorRequeuesOnceThenExcludes(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.operational(t)
	f.version(t, "d1")
	run, _ := f.quals.Start(ctx, f.userID, "python")
	// task 1: infra error twice → excluded
	for i := 0; i < 2; i++ {
		p, _, _ := f.proofs.Claim(ctx, f.agentID)
		_ = f.proofs.SubmitResult(ctx, f.agentID, p.ID, proofs.ResultInput{Diff: noteDiff})
		f.fake.Err = errors.New("no docker")
		_ = f.worker.RunProof(ctx, p.ID)
		_ = f.worker.MarkInfraError(ctx, p.ID, "no docker")
	}
	f.drive(t, 1)
	f.drive(t, 1)
	got, _ := f.quals.Get(ctx, f.userID, run.ID)
	if got.Status != "scored" || got.Score == nil || *got.Score != 1 || len(got.Tasks) != 4 {
		t.Fatalf("two clean tasks must average to 1.0 with the infra-errored one excluded: %+v", got)
	}
}

func TestRun_AbortWhenTaskExpiresAndDailyLimit(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.operational(t)
	f.version(t, "d1")
	run, _ := f.quals.Start(ctx, f.userID, "go")
	_ = f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE proofs SET created_at = now() - interval '6 minutes' WHERE qualification_run_id = $1`, run.ID)
		return err
	})
	ids, err := f.proofs.ExpireStale(ctx)
	if err != nil || len(ids) != 1 {
		t.Fatalf("expire: %v %v", ids, err)
	}
	for _, id := range ids {
		_ = f.quals.OnProofFinished(ctx, id)
	}
	got, _ := f.quals.Get(ctx, f.userID, run.ID)
	if got.Status != "aborted" {
		t.Fatalf("expired task must abort the run: %+v", got)
	}
	if rs, _ := f.quals.RatingsFor(ctx, f.agentID); len(rs) != 0 {
		t.Fatalf("aborted run must not create a rating")
	}
	for i := 0; i < 3; i++ {
		r, err := f.quals.Start(ctx, f.userID, "go")
		if err != nil {
			t.Fatalf("start %d: %v", i, err)
		}
		_ = f.quals.Abort(ctx, r.ID, "test")
	}
	_, err = f.quals.Start(ctx, f.userID, "go")
	problem(t, err, 429, "daily_limit")
}

func TestStart_NeedsOnlineConnector(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.operational(t)
	f.version(t, "d1")
	err := f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE agent_presence SET last_seen_at = now() - interval '3 minutes' WHERE agent_id = $1`, f.agentID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.quals.Start(ctx, f.userID, "go")
	problem(t, err, 409, "agent_offline")
}

func TestScore_CountsOnlyHiddenTestsByName(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.operational(t)
	f.version(t, "d1")
	run, _ := f.quals.Start(ctx, f.userID, "python")
	for i := 0; i < 3; i++ {
		f.drive(t, 0) // every hidden test fails; the two tests outside the hidden set pass
	}
	got, _ := f.quals.Get(ctx, f.userID, run.ID)
	if got.Status != "scored" || got.Score == nil || *got.Score != 0 || *got.RatingAfter != 1000 {
		t.Fatalf("tests outside the hidden set must not score: %+v", got)
	}
}

func TestRun_HoldsTheSlotAndSweepsStalled(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.operational(t)
	f.version(t, "d1")
	run, _ := f.quals.Start(ctx, f.userID, "go")

	// Task 1 finishes on a worker without the hook, as if the API stopped
	// between the verdict and the advance.
	deaf := proofs.NewWorker(f.d.AppPool, f.fake, t.TempDir(), slog.New(slog.NewTextHandler(os.Stderr, nil)))
	p1, task, _ := f.proofs.Claim(ctx, f.agentID)
	f.fake.Result = sandbox.Result{Tests: passing(f.hidden[task.Slug])}
	f.fake.Err = nil
	if err := f.proofs.SubmitResult(ctx, f.agentID, p1.ID, proofs.ResultInput{Diff: noteDiff}); err != nil {
		t.Fatal(err)
	}
	if err := deaf.RunProof(ctx, p1.ID); err != nil {
		t.Fatal(err)
	}

	// No proof is open, yet the run keeps the slot.
	_, err := f.proofs.Create(ctx, f.userID, "go-fix-retry")
	problem(t, err, 409, "qualification_in_progress")
	if last, _ := f.proofs.Latest(ctx, f.agentID); last == nil || last.ID != "proof_seed" {
		t.Fatalf("last_proof must stay the basic proof: %+v", last)
	}

	if n, _ := f.quals.SweepStalled(ctx); n != 0 {
		t.Fatalf("a proof finished seconds ago is not stalled yet")
	}
	f.age(t, run.ID)
	if n, err := f.quals.SweepStalled(ctx); err != nil || n != 1 {
		t.Fatalf("sweep: %d %v", n, err)
	}
	if err := f.quals.OnProofFinished(ctx, p1.ID); err != nil { // the late hook is a no-op
		t.Fatal(err)
	}
	got, _ := f.quals.Get(ctx, f.userID, run.ID)
	if len(got.Tasks) != 2 || *got.Tasks[1].Position != 2 {
		t.Fatalf("exactly one next task: %+v", got.Tasks)
	}

	// Task 2's result is too big: FailOversized closes it outside the worker.
	p2, _, _ := f.proofs.Claim(ctx, f.agentID)
	err = f.proofs.SubmitResult(ctx, f.agentID, p2.ID, proofs.ResultInput{Diff: strings.Repeat("+x\n", 100_000)})
	problem(t, err, 413, "diff_too_large")
	_, err = f.proofs.Retry(ctx, f.userID, p2.ID)
	problem(t, err, 409, "state_conflict")
	f.age(t, run.ID)
	if n, err := f.quals.SweepStalled(ctx); err != nil || n != 1 {
		t.Fatalf("sweep after oversized: %d %v", n, err)
	}
	got, _ = f.quals.Get(ctx, f.userID, run.ID)
	if len(got.Tasks) != 3 || *got.Tasks[2].Position != 3 {
		t.Fatalf("an oversized task 2 must still move the run on: %+v", got.Tasks)
	}
}

func TestNewVersion_ResetsConfidenceKeepsPrior(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.operational(t)
	f.version(t, "d1")
	run, _ := f.quals.Start(ctx, f.userID, "go")
	f.drive(t, 1)
	f.drive(t, 1)
	f.drive(t, 1)
	if got, _ := f.quals.Get(ctx, f.userID, run.ID); *got.RatingAfter != 2400 {
		t.Fatalf("perfect run must rate 2400, got %+v", got)
	}
	f.version(t, "d2")
	rs, _ := f.quals.RatingsFor(ctx, f.agentID)
	if rs[0].Rating != 2400 || rs[0].Uncertainty != 350 || rs[0].Runs != 0 || rs[0].OnCurrentVersion || rs[0].PriorRating == nil {
		t.Fatalf("after version change: %+v", rs[0])
	}
	if rs[0].Verified {
		t.Fatalf("2400-350 = 2050 is verified by access, but the rating is not on the current version and must not count as verified")
	}
}
```

- [ ] **Step 3: Модель и сервис**

`backend/internal/qualifications/model.go`:

```go
package qualifications

import (
	"time"

	"tolerance/internal/proofs"
)

const (
	StatusRunning = "running"
	StatusScored  = "scored"
	StatusAborted = "aborted"

	tasksPerRun = 3
	dailyLimit  = 3
	recentRuns  = 2
)

type Run struct {
	ID               string         `json:"id"`
	AgentID          string         `json:"agent_id"`
	VersionID        string         `json:"version_id"`
	SkillSlug        string         `json:"skill_slug"`
	Status           string         `json:"status"`
	CreatedAt        time.Time      `json:"created_at"`
	FinishedAt       *time.Time     `json:"finished_at"`
	Score            *float64       `json:"score"`
	RatingBefore     *int           `json:"rating_before"`
	RatingAfter      *int           `json:"rating_after"`
	UncertaintyAfter *int           `json:"uncertainty_after"`
	TaskSlugs        []string       `json:"task_slugs"`
	Tasks            []proofs.Proof `json:"tasks"`
}

type SkillRating struct {
	SkillSlug        string `json:"skill_slug"`
	Rating           int    `json:"rating"`
	Uncertainty      int    `json:"uncertainty"`
	Access           int    `json:"access"`
	Tier             string `json:"tier"`
	Verified         bool   `json:"verified"`
	Runs             int    `json:"runs"`
	VersionID        string `json:"version_id"`
	VersionNumber    int    `json:"version_number"`
	OnCurrentVersion bool   `json:"on_current_version"`
	PriorRating      *int   `json:"prior_rating"`
}
```

`backend/internal/qualifications/service.go`:

```go
package qualifications

import (
	"context"
	"errors"
	"math/rand"
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
	"tolerance/internal/proofs"
	"tolerance/internal/skills"
)

type Service struct {
	pool   *db.Pool
	proofs *proofs.Service
	rnd    *rand.Rand
}

func NewService(pool *db.Pool, ps *proofs.Service) *Service {
	return &Service{pool: pool, proofs: ps, rnd: rand.New(rand.NewSource(time.Now().UnixNano()))}
}

const runCols = `id, agent_id, version_id, skill_slug, status, created_at, finished_at, score, rating_before, rating_after, uncertainty_after, task_slugs`

func scanRun(row interface{ Scan(...any) error }, r *Run) error {
	if err := row.Scan(&r.ID, &r.AgentID, &r.VersionID, &r.SkillSlug, &r.Status, &r.CreatedAt, &r.FinishedAt, &r.Score, &r.RatingBefore, &r.RatingAfter, &r.UncertaintyAfter, &r.TaskSlugs); err != nil {
		return err
	}
	r.CreatedAt = r.CreatedAt.UTC()
	if r.FinishedAt != nil {
		u := r.FinishedAt.UTC()
		r.FinishedAt = &u
	}
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

// Start opens a run: picks three tasks, queues the first one.
func (s *Service) Start(ctx context.Context, userID, skill string) (Run, error) {
	var run Run
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		agentID, err := s.agentOf(ctx, tx, userID)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT 1 FROM agents WHERE id = $1 FOR UPDATE`, agentID); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM skills WHERE slug = $1)`, skill).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return httpx.New(http.StatusNotFound, "unknown_skill", "No such skill")
		}
		var passed bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM proofs WHERE agent_id = $1 AND kind = 'proof' AND status = 'passed')`, agentID).Scan(&passed); err != nil {
			return err
		}
		if !passed {
			return httpx.New(http.StatusConflict, "agent_not_operational", "Pass the basic proof first")
		}
		// Same rule as proofs.Create: an offline connector would let the first
		// task expire unclaimed and burn one of the day's runs.
		var online bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM agent_presence WHERE agent_id = $1 AND last_seen_at > now() - interval '2 minutes')`, agentID).Scan(&online); err != nil {
			return err
		}
		if !online {
			return httpx.New(http.StatusConflict, "agent_offline", "The connector is not online; run `arena connect` first")
		}
		var versionID *string
		if err := tx.QueryRow(ctx, `SELECT current_version_id FROM agents WHERE id = $1`, agentID).Scan(&versionID); err != nil {
			return err
		}
		if versionID == nil {
			return httpx.New(http.StatusConflict, "no_version", "The connector has not reported the agent version yet; update it and run `arena connect`")
		}
		var today int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM qualification_runs WHERE agent_id = $1 AND skill_slug = $2 AND created_at > now() - interval '24 hours'`, agentID, skill).Scan(&today); err != nil {
			return err
		}
		if today >= dailyLimit {
			return httpx.New(http.StatusTooManyRequests, "daily_limit", "At most 3 qualification runs per skill per day")
		}
		rows, err := tx.Query(ctx, `SELECT slug FROM skill_tasks WHERE skill_slug = $1 AND active ORDER BY slug`, skill)
		if err != nil {
			return err
		}
		var pool []string
		for rows.Next() {
			var slug string
			if err := rows.Scan(&slug); err != nil {
				rows.Close()
				return err
			}
			pool = append(pool, slug)
		}
		rows.Close()
		if len(pool) == 0 {
			return httpx.New(http.StatusConflict, "no_tasks", "This skill has no tasks yet")
		}
		var recent []string
		if err := tx.QueryRow(ctx, `SELECT coalesce(array_agg(t), '{}') FROM (SELECT unnest(task_slugs) t FROM qualification_runs
			WHERE agent_id = $1 AND skill_slug = $2 ORDER BY created_at DESC LIMIT $3) x`, agentID, skill, recentRuns).Scan(&recent); err != nil {
			return err
		}
		picked := skills.Pick(pool, recent, tasksPerRun, s.rnd)
		if err := scanRun(tx.QueryRow(ctx, `INSERT INTO qualification_runs (id, agent_id, version_id, skill_slug, task_slugs) VALUES ($1, $2, $3, $4, $5) RETURNING `+runCols,
			idgen.New("qrun"), agentID, *versionID, skill, picked), &run); err != nil {
			return err
		}
		first, err := s.proofs.CreateQualificationProof(ctx, tx, agentID, run.ID, picked[0], 1)
		if err != nil {
			return err
		}
		run.Tasks = []proofs.Proof{first}
		return audit.Record(ctx, tx, audit.Event{ActorID: userID, Action: "qualification.started", AggregateKind: "qualification_run", AggregateID: run.ID,
			Payload: map[string]any{"skill": skill, "tasks": picked}, RequestID: httpx.RequestID(ctx)})
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && (strings.Contains(pgErr.ConstraintName, "qualification_runs_one_open_idx") || strings.Contains(pgErr.ConstraintName, "proofs_one_open_idx")) {
		return Run{}, httpx.New(http.StatusConflict, "qualification_in_progress", "A qualification or proof is already in progress")
	}
	return run, err
}

func (s *Service) tasksOf(ctx context.Context, tx pgx.Tx, runID string) ([]proofs.Proof, error) {
	rows, err := tx.Query(ctx, `SELECT id FROM proofs WHERE qualification_run_id = $1 ORDER BY position, created_at`, runID)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	out := make([]proofs.Proof, 0, len(ids))
	for _, id := range ids {
		p, err := s.proofs.GetByIDTx(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, s.proofs.MaskHidden(p))
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, userID, id string) (Run, error) {
	var run Run
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := scanRun(tx.QueryRow(ctx, `SELECT `+runCols+` FROM qualification_runs WHERE id = $1 AND agent_id = (SELECT id FROM agents WHERE owner_user_id = $2)`, id, userID), &run); err != nil {
			return err
		}
		var err error
		run.Tasks, err = s.tasksOf(ctx, tx, run.ID)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, httpx.NotFound()
	}
	return run, err
}

func (s *Service) List(ctx context.Context, userID string) ([]Run, error) {
	out := []Run{}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		agentID, err := s.agentOf(ctx, tx, userID)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT `+runCols+` FROM qualification_runs WHERE agent_id = $1 ORDER BY created_at DESC LIMIT 50`, agentID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r Run
			if err := scanRun(rows, &r); err != nil {
				return err
			}
			r.Tasks = []proofs.Proof{}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

// Abort ends a run without touching the rating.
func (s *Service) Abort(ctx context.Context, runID, reason string) error {
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE qualification_runs SET status = 'aborted', finished_at = now() WHERE id = $1 AND status = 'running'`, runID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE proofs SET status = 'expired', finished_at = now(), failure_reason = $2
			WHERE qualification_run_id = $1 AND status IN ('queued', 'claimed', 'running_agent', 'diff_submitted', 'running_sandbox')`, runID, "run_aborted: "+reason)
		return err
	})
}
```

В `proofs/service.go` добавить `GetByIDTx(ctx, tx, id) (Proof, error)` (без проверки владельца; используется только внутри сервера).

- [ ] **Step 4: Продвижение и рейтинг**

`backend/internal/qualifications/advance.go`:

```go
package qualifications

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/proofs"
	"tolerance/internal/rating"
)

// OnProofFinished implements proofs.FinishListener: it moves the run to
// the next task, re-queues a task that hit an infra error once, and scores
// the run after the last one. Non-qualification proofs are ignored. It is
// idempotent: only the run's newest proof, once finished, moves the run, so
// the worker's hook and SweepStalled may both fire for the same proof.
func (s *Service) OnProofFinished(ctx context.Context, proofID string) error {
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		p, err := s.proofs.GetByIDTx(ctx, tx, proofID)
		if err != nil {
			return err
		}
		if p.Kind != proofs.KindQualification || p.QualificationRunID == nil {
			return nil
		}
		var run Run
		if err := scanRun(tx.QueryRow(ctx, `SELECT `+runCols+` FROM qualification_runs WHERE id = $1 FOR UPDATE`, *p.QualificationRunID), &run); err != nil {
			return err
		}
		if run.Status != StatusRunning || p.FinishedAt == nil {
			return nil
		}
		var newest string
		if err := tx.QueryRow(ctx, `SELECT id FROM proofs WHERE qualification_run_id = $1 ORDER BY created_at DESC, id DESC LIMIT 1`, run.ID).Scan(&newest); err != nil {
			return err
		}
		if newest != p.ID {
			return nil // already moved on
		}
		switch p.Status {
		case proofs.StatusExpired:
			return s.abortTx(ctx, tx, run.ID)
		case proofs.StatusInfraError:
			var retried bool
			if err := tx.QueryRow(ctx, `SELECT retried_infra FROM proofs WHERE id = $1`, p.ID).Scan(&retried); err != nil {
				return err
			}
			if !retried {
				if _, err := tx.Exec(ctx, `UPDATE proofs SET retried_infra = true WHERE id = $1`, p.ID); err != nil {
					return err
				}
				_, err := s.proofs.CreateQualificationProof(ctx, tx, run.AgentID, run.ID, *p.SkillTaskSlug, *p.Position)
				return err
			}
		}
		if *p.Position < tasksPerRun {
			next := *p.Position + 1
			_, err := s.proofs.CreateQualificationProof(ctx, tx, run.AgentID, run.ID, run.TaskSlugs[next-1], next)
			return err
		}
		return s.scoreTx(ctx, tx, run)
	})
}

// SweepStalled moves on runs whose newest proof finished over a minute ago
// with nothing after it: the worker's hook never ran for it (the API stopped
// between the two transactions) or the proof was closed outside the worker
// (FailOversized). cmd/api calls it every 30 seconds.
func (s *Service) SweepStalled(ctx context.Context) (int, error) {
	var ids []string
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT newest.id FROM qualification_runs q
			CROSS JOIN LATERAL (SELECT id, finished_at FROM proofs WHERE qualification_run_id = q.id ORDER BY created_at DESC, id DESC LIMIT 1) newest
			WHERE q.status = 'running' AND newest.finished_at < now() - interval '1 minute'`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return rows.Err()
	})
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		if err := s.OnProofFinished(ctx, id); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

func (s *Service) abortTx(ctx context.Context, tx pgx.Tx, runID string) error {
	_, err := tx.Exec(ctx, `UPDATE qualification_runs SET status = 'aborted', finished_at = now() WHERE id = $1`, runID)
	return err
}

// scoreTx computes the run score from the last terminal proof per position
// and folds it into the skill rating. A task scores the share of its hidden
// tests, by name, that passed; a test outside the hidden set (a visible one,
// or one the agent wrote) never counts, and a name reported both passed and
// failed counts as failed.
func (s *Service) scoreTx(ctx context.Context, tx pgx.Tx, run Run) error {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT ON (p.position) p.position, p.status, p.sandbox_result, t.hidden_tests, t.difficulty, t.hidden_tar, s.language
		FROM proofs p JOIN skill_tasks t ON t.slug = p.skill_task_slug JOIN skills s ON s.slug = t.skill_slug
		WHERE p.qualification_run_id = $1 AND p.finished_at IS NOT NULL
		ORDER BY p.position, p.finished_at DESC`, run.ID)
	if err != nil {
		return err
	}
	var weighted, weights float64
	for rows.Next() {
		var position, hidden, difficulty int
		var status, language string
		var hiddenTar []byte
		var sr *proofs.SandboxResult
		if err := rows.Scan(&position, &status, &sr, &hidden, &difficulty, &hiddenTar, &language); err != nil {
			rows.Close()
			return err
		}
		if status == proofs.StatusInfraError {
			continue // excluded: the platform failed, not the agent
		}
		names, err := proofs.HiddenTestNames(language, hiddenTar)
		if err != nil {
			rows.Close()
			return err
		}
		passed := 0
		if sr != nil {
			ok := map[string]bool{}
			for _, t := range sr.Tests {
				prev, seen := ok[t.Name]
				ok[t.Name] = t.Passed && (!seen || prev)
			}
			for _, n := range names {
				if ok[n] {
					passed++
				}
			}
		}
		if passed > hidden {
			passed = hidden
		}
		w := float64(difficulty)
		weighted += w * float64(passed) / float64(hidden)
		weights += w
	}
	rows.Close()
	if weights == 0 {
		return s.abortTx(ctx, tx, run.ID)
	}
	score := weighted / weights

	st, before, err := s.loadState(ctx, tx, run.AgentID, run.SkillSlug, run.VersionID)
	if err != nil {
		return err
	}
	next := rating.Apply(st, score)
	if err := s.saveState(ctx, tx, run.AgentID, run.SkillSlug, run.VersionID, next); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE qualification_runs SET status = 'scored', finished_at = now(), score = $2, rating_before = $3, rating_after = $4, uncertainty_after = $5 WHERE id = $1`,
		run.ID, score, before, next.Rating, next.Uncertainty)
	return err
}
```

`backend/internal/qualifications/ratings.go`:

```go
package qualifications

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/rating"
)

// loadState returns the rating state for (agent, skill) on versionID. A
// row on an older version means the agent changed version without the
// listener having run (should not happen) and is treated as a fresh one
// with the old rating as prior. before is the displayed rating before this
// run, nil when there was none.
func (s *Service) loadState(ctx context.Context, tx pgx.Tx, agentID, skill, versionID string) (rating.State, *int, error) {
	var st rating.State
	var rowVersion string
	err := tx.QueryRow(ctx, `SELECT version_id, rating, uncertainty, runs, sum_targets, prior_rating FROM skill_ratings WHERE agent_id = $1 AND skill_slug = $2 FOR UPDATE`,
		agentID, skill).Scan(&rowVersion, &st.Rating, &st.Uncertainty, &st.Runs, &st.SumTargets, &st.Prior)
	if errors.Is(err, pgx.ErrNoRows) {
		return rating.State{}, nil, nil
	}
	if err != nil {
		return rating.State{}, nil, err
	}
	before := st.Rating
	if rowVersion != versionID {
		st = rating.NewVersion(st)
	}
	return st, &before, nil
}

func (s *Service) saveState(ctx context.Context, tx pgx.Tx, agentID, skill, versionID string, st rating.State) error {
	_, err := tx.Exec(ctx, `INSERT INTO skill_ratings (agent_id, skill_slug, version_id, rating, uncertainty, runs, sum_targets, prior_rating, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
		ON CONFLICT (agent_id, skill_slug) DO UPDATE SET version_id = $3, rating = $4, uncertainty = $5, runs = $6, sum_targets = $7, prior_rating = $8, updated_at = now()`,
		agentID, skill, versionID, st.Rating, st.Uncertainty, st.Runs, st.SumTargets, st.Prior)
	return err
}

// OnNewVersion implements agents.VersionListener: every skill rating moves
// to the new version with confidence reset and the old rating as prior.
func (s *Service) OnNewVersion(ctx context.Context, tx pgx.Tx, agentID, versionID string) error {
	rows, err := tx.Query(ctx, `SELECT skill_slug, rating, uncertainty, runs, sum_targets, prior_rating FROM skill_ratings WHERE agent_id = $1 FOR UPDATE`, agentID)
	if err != nil {
		return err
	}
	type row struct {
		skill string
		st    rating.State
	}
	var all []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.skill, &r.st.Rating, &r.st.Uncertainty, &r.st.Runs, &r.st.SumTargets, &r.st.Prior); err != nil {
			rows.Close()
			return err
		}
		all = append(all, r)
	}
	rows.Close()
	for _, r := range all {
		if err := s.saveState(ctx, tx, agentID, r.skill, versionID, rating.NewVersion(r.st)); err != nil {
			return err
		}
	}
	return nil
}

// RatingsFor lists the agent's skill ratings with derived fields. Verified
// requires the rating to be on the agent's current version with at least
// one run there.
func (s *Service) RatingsFor(ctx context.Context, agentID string) ([]SkillRating, error) {
	out := []SkillRating{}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT r.skill_slug, r.rating, r.uncertainty, r.runs, r.version_id, v.number, r.prior_rating,
			(a.current_version_id = r.version_id)
			FROM skill_ratings r JOIN agent_versions v ON v.id = r.version_id JOIN agents a ON a.id = r.agent_id
			WHERE r.agent_id = $1 ORDER BY r.skill_slug`, agentID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r SkillRating
			if err := rows.Scan(&r.SkillSlug, &r.Rating, &r.Uncertainty, &r.Runs, &r.VersionID, &r.VersionNumber, &r.PriorRating, &r.OnCurrentVersion); err != nil {
				return err
			}
			r.Access = rating.Access(r.Rating, r.Uncertainty)
			r.Tier = rating.Tier(r.Access)
			r.Verified = r.OnCurrentVersion && r.Runs > 0 && rating.Verified(r.Rating, r.Uncertainty)
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}
```

В `cmd/api/main.go` (после того как танки подключили `worker.SetGameBotJudge(gs)`): `qs := qualifications.NewService(pool, ps); agentsSvc.SetVersionListener(qs); worker.SetFinishListener(qs)`; `qs` в `deps`; рядом с `go worker.Run(ctx)` — горутина, которая каждые 30 с вызывает `qs.SweepStalled(ctx)` и пишет в лог ошибку или число продвинутых прогонов (`time.NewTicker`, выход по `ctx.Done()`).

Run: `ARENA_TEST_REQUIRE_DOCKER=1 go test ./internal/qualifications/ ./internal/proofs/... -v` → PASS.

- [ ] **Step 5: Коммит**

```bash
cd backend && gofmt -l . && go vet ./... && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./...
git add -A && git commit -m "Add qualification runs: three hidden tasks, worker-driven advance, skill ratings

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: API кабинета, публичный профиль, OpenAPI, e2e

**Files:**
- Create: `backend/internal/qualifications/http.go`, `backend/internal/skills/http.go`, `backend/internal/agents/public.go`
- Modify: `backend/cmd/api/handler.go`, `backend/cmd/api/compose.go`, `backend/cmd/api/main.go`, `backend/cmd/api/main_test.go`, `backend/contracts/openapi/openapi.yaml`, `backend/internal/agents/model.go`, `backend/internal/agents/presence.go`, `backend/internal/agents/service.go`

**Interfaces:**
- Produces: `GET /api/v1/skills`, `POST /api/v1/qualifications`, `GET /api/v1/qualifications`, `GET /api/v1/qualifications/{id}` (cookie); `GET /api/v1/agents/{name}` (без auth); `Overview.Skills []qualifications.SkillRating` в `/me`; `agents.SkillsSource interface{ RatingsFor(ctx, agentID) ([]qualifications.SkillRating, error) }` — чтобы `agents` не импортировал `qualifications`, тип рейтинга переезжает в отдельный пакет `internal/rating` как `rating.SkillRating` (тот же набор полей), а `qualifications.SkillRating = rating.SkillRating` через alias.

- [ ] **Step 1: Маршруты**

`backend/internal/skills/http.go`:

```go
package skills

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/identity"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/rating"
)

type SkillView struct {
	Skill
	PoolSize      int                 `json:"pool_size"`
	Rating        *rating.SkillRating `json:"rating"`
	RunsToday     int                 `json:"runs_today"`
	CanStart      bool                `json:"can_start"`
	BlockedReason string              `json:"blocked_reason"`
}

type RatingsSource interface {
	RatingsFor(ctx context.Context, agentID string) ([]rating.SkillRating, error)
}

// RegisterOwnerRoutes mounts GET /skills: the catalog with the caller's
// rating and whether a run can start right now.
func RegisterOwnerRoutes(mux *http.ServeMux, pool *db.Pool, ratings RatingsSource, stageOf func(ctx context.Context, userID string) (agentID, stage string, hasVersion bool, err error)) {
	mux.HandleFunc("GET /api/v1/skills", func(w http.ResponseWriter, r *http.Request) {
		userID := identity.MustFromContext(r.Context()).UserID
		agentID, stage, hasVersion, err := stageOf(r.Context(), userID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var items []SkillView
		err = pool.Tx(r.Context(), func(ctx context.Context, tx pgx.Tx) error {
			rows, err := tx.Query(ctx, `SELECT s.slug, s.title, s.language, s.description, (SELECT count(*) FROM skill_tasks t WHERE t.skill_slug = s.slug AND t.active),
				(SELECT count(*) FROM qualification_runs q WHERE q.skill_slug = s.slug AND q.agent_id = $1 AND q.created_at > now() - interval '24 hours')
				FROM skills s ORDER BY s.slug`, agentID)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var v SkillView
				if err := rows.Scan(&v.Slug, &v.Title, &v.Language, &v.Description, &v.PoolSize, &v.RunsToday); err != nil {
					return err
				}
				items = append(items, v)
			}
			return rows.Err()
		})
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var rs []rating.SkillRating
		if agentID != "" {
			if rs, err = ratings.RatingsFor(r.Context(), agentID); err != nil {
				httpx.WriteError(w, r, err)
				return
			}
		}
		for i := range items {
			for j := range rs {
				if rs[j].SkillSlug == items[i].Slug {
					items[i].Rating = &rs[j]
				}
			}
			switch {
			case agentID == "":
				items[i].BlockedReason = "no_agent"
			case stage == "checking":
				items[i].BlockedReason = "in_progress"
			case stage != "operational":
				items[i].BlockedReason = "not_operational"
			case !hasVersion:
				items[i].BlockedReason = "no_version"
			case items[i].RunsToday >= 3:
				items[i].BlockedReason = "daily_limit"
			case items[i].PoolSize == 0:
				items[i].BlockedReason = "no_tasks"
			default:
				items[i].CanStart = true
			}
		}
		if items == nil {
			items = []SkillView{}
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items})
	})
}
```

Функция `stageOf` реализуется в `agents`: `(*Service).StageOf(ctx, userID) (agentID, stage string, hasVersion bool, err error)` через `Overview` (пустой `agentID` когда агента нет).

`backend/internal/qualifications/http.go`:

```go
package qualifications

import (
	"net/http"

	"tolerance/internal/identity"
	"tolerance/internal/platform/httpx"
)

type startInput struct {
	Skill string `json:"skill"`
}

func RegisterOwnerRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("POST /api/v1/qualifications", func(w http.ResponseWriter, r *http.Request) {
		raw, err := httpx.ReadBody(w, r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var in startInput
		if err := httpx.Decode(raw, &in); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		run, err := s.Start(r.Context(), identity.MustFromContext(r.Context()).UserID, in.Skill)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusCreated, run)
	})
	mux.HandleFunc("GET /api/v1/qualifications", func(w http.ResponseWriter, r *http.Request) {
		items, err := s.List(r.Context(), identity.MustFromContext(r.Context()).UserID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items})
	})
	mux.HandleFunc("GET /api/v1/qualifications/{id}", func(w http.ResponseWriter, r *http.Request) {
		run, err := s.Get(r.Context(), identity.MustFromContext(r.Context()).UserID, r.PathValue("id"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, run)
	})
}
```

`backend/internal/agents/public.go`:

```go
package agents

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/httpx"
	"tolerance/internal/rating"
)

type PublicVersion struct {
	Number  int    `json:"number"`
	Model   string `json:"model"`
	Harness string `json:"harness"`
}

type PublicProfile struct {
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Joined      time.Time            `json:"joined"`
	Stage       string               `json:"stage"`
	Version     *PublicVersion       `json:"version"`
	Skills      []rating.SkillRating `json:"skills"`
}

type SkillsSource interface {
	RatingsFor(ctx context.Context, agentID string) ([]rating.SkillRating, error)
}

// PublicByName is the profile anyone can see: no email, no keys.
func (s *Service) PublicByName(ctx context.Context, name string, skills SkillsSource) (PublicProfile, error) {
	var a Agent
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return scanAgent(tx.QueryRow(ctx, `SELECT `+agentCols+` FROM agents WHERE lower(name) = lower($1)`, name), &a)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return PublicProfile{}, httpx.NotFound()
	}
	if err != nil {
		return PublicProfile{}, err
	}
	o, err := s.overview(ctx, a)
	if err != nil {
		return PublicProfile{}, err
	}
	rs, err := skills.RatingsFor(ctx, a.ID)
	if err != nil {
		return PublicProfile{}, err
	}
	p := PublicProfile{Name: a.Name, Description: a.Description, Joined: a.CreatedAt, Stage: o.Stage, Skills: rs}
	if o.Version != nil {
		p.Version = &PublicVersion{Number: o.Version.Number, Model: o.Version.Model, Harness: o.Version.Harness}
	}
	return p, nil
}

func RegisterPublicRoutes(mux *http.ServeMux, s *Service, skills SkillsSource) {
	mux.HandleFunc("GET /api/v1/agents/{name}", func(w http.ResponseWriter, r *http.Request) {
		p, err := s.PublicByName(r.Context(), r.PathValue("name"), skills)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, p)
	})
}
```

`rating.SkillRating` — перенести структуру из `qualifications/model.go` в `rating/rating.go` (те же поля и json-теги); в `qualifications`: `type SkillRating = rating.SkillRating`.

`agents/model.go`: `Overview.Skills []rating.SkillRating \`json:"skills"\``; `presence.go` `overview` заполняет через `s.skills.RatingsFor`, где `s.skills SkillsSource` задаётся `SetSkillsSource` (без него — пустой срез `[]rating.SkillRating{}`, не `nil`: схема требует массив). `cmd/api/compose.go` `connectorStatus`: в блок `agent` добавить `"skills": o.Skills` рядом с `"version"` из задачи 2 — `arena status` печатает рейтинги отсюда. `cmd/api/main.go`: `agentsSvc.SetSkillsSource(qs)`.

`cmd/api/handler.go`:

```go
	skills.RegisterOwnerRoutes(owner, d.pool, d.quals, d.agents.StageOf)
	qualifications.RegisterOwnerRoutes(owner, d.quals)
	api.Handle("/api/v1/skills", session(owner))
	api.Handle("/api/v1/qualifications", session(owner))
	api.Handle("/api/v1/qualifications/", session(owner))

	public := http.NewServeMux()
	agents.RegisterPublicRoutes(public, d.agents, d.quals)
	api.Handle("/api/v1/agents/", public)
```

- [ ] **Step 2: OpenAPI**

Добавить в `openapi.yaml` пути `/skills`, `/qualifications`, `/qualifications/{id}`, `/agents/{name}` (security `[]`), схемы `SkillRating` (`skill_slug, rating, uncertainty, access, tier{none,verified,strong,elite}, verified, runs, version_id, version_number, on_current_version, prior_rating nullable`), `SkillView`, `QualificationRun` (все поля `Run`, `tasks: array of Proof`), `PublicProfile`, `AgentVersion` (`id, number, model, harness, config_digest, created_at`). Enum `Proof.kind` (танки ввели `[proof, game_bot]`) дополняется значением `qualification`; `Proof` получает `qualification_run_id nullable`, `position nullable`, `skill_task_slug nullable`. `AgentOverview` дополняется `version nullable AgentVersion` и `skills array SkillRating` (это и `/me`: `ownerAgent` встраивает overview). Блок `agent` ответа `/connector/heartbeat` дополняется `version` (nullable), блок `agent` ответа `/connector/status` — `version` и `skills`. `kind` в ответе `/connector/tasks/next` уже есть (танки), его enum дополняется `qualification`.

- [ ] **Step 3: e2e**

`newE2E` в `cmd/api/main_test.go` дополнить, не трогая проводку танков: после `proofs.SyncCatalog` — `skills.LoadCatalog(filepath.Join("..", "..", "fixtures", "skills"))` и `skills.SyncCatalog(ctx, d.AdminPool, sk, stasks)`; `qs := qualifications.NewService(d.AppPool, ps)`; сервис агентов создать отдельной переменной и вызвать `SetVersionListener(qs)` и `SetSkillsSource(qs)`; `quals: qs` в `deps`; воркер, который возвращается в `e.worker`, получает `SetFinishListener(qs)`. Добавить хелперы:

```go
// noteDiff applies to any task repository: it adds a file no test reads.
const noteDiff = "diff --git a/NOTES.md b/NOTES.md\nnew file mode 100644\n--- /dev/null\n+++ b/NOTES.md\n@@ -0,0 +1 @@\n+agent notes\n"

// hiddenNames maps each skill task to its hidden test names, read from the
// catalog newE2E syncs.
func hiddenNames(t *testing.T) map[string][]string {
	t.Helper()
	sk, tasks, err := skills.LoadCatalog(filepath.Join("..", "..", "fixtures", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	lang := map[string]string{}
	for _, s := range sk {
		lang[s.Slug] = s.Language
	}
	out := map[string][]string{}
	for _, tk := range tasks {
		names, err := proofs.HiddenTestNames(lang[tk.SkillSlug], tk.HiddenTar)
		if err != nil {
			t.Fatal(err)
		}
		out[tk.Slug] = names
	}
	return out
}
```

Тест:

```go
func TestEndToEnd_Qualification(t *testing.T) {
	e := newE2E(t)
	owner := e.browser(t)
	plain := &http.Client{}
	e.call(t, owner, "POST", "/api/v1/auth/signup", "", map[string]string{"email": "q@example.com", "password": "longenough1"}, nil)
	e.call(t, owner, "POST", "/api/v1/agent", "", map[string]string{"name": "Fixer-7"}, nil)
	var keyResp struct{ Key string `json:"key"` }
	e.call(t, owner, "POST", "/api/v1/agent/keys", "", map[string]string{"name": "k"}, &keyResp)
	hb := map[string]any{"connector_version": "0.2.0", "hostname": "h", "version": map[string]string{"model": "claude-opus-5-5", "harness": "claude-code", "config_digest": "abc"}}
	e.call(t, plain, "POST", "/api/v1/connector/heartbeat", keyResp.Key, hb, nil)

	// skills are blocked until the basic proof passes
	var sk struct{ Items []struct {
		Slug string `json:"slug"`; CanStart bool `json:"can_start"`; BlockedReason string `json:"blocked_reason"`
	} `json:"items"` }
	e.call(t, owner, "GET", "/api/v1/skills", "", nil, &sk)
	if len(sk.Items) != 2 || sk.Items[0].CanStart || sk.Items[0].BlockedReason != "not_operational" {
		t.Fatalf("skills before proof: %+v", sk.Items)
	}
	if code := e.call(t, owner, "POST", "/api/v1/qualifications", "", map[string]string{"skill": "go"}, nil); code != 409 {
		t.Fatalf("qualification before proof: %d", code)
	}

	// basic proof via the same path as slice 1
	var proof proofs.Proof
	e.call(t, owner, "POST", "/api/v1/proofs", "", map[string]string{"task_slug": "go-fix-retry"}, &proof)
	e.call(t, plain, "GET", "/api/v1/connector/tasks/next?wait=1", keyResp.Key, nil, nil)
	e.call(t, plain, "POST", "/api/v1/connector/proofs/"+proof.ID+"/result", keyResp.Key, map[string]any{"diff": noteDiff}, nil)
	_ = e.worker.RunProof(context.Background(), proof.ID)

	e.call(t, owner, "GET", "/api/v1/skills", "", nil, &sk)
	if !sk.Items[0].CanStart {
		t.Fatalf("skills after proof: %+v", sk.Items)
	}
	var run qualifications.Run
	if code := e.call(t, owner, "POST", "/api/v1/qualifications", "", map[string]string{"skill": "go"}, &run); code != 201 || len(run.Tasks) != 1 {
		t.Fatalf("start: %d %+v", code, run)
	}
	hidden := hiddenNames(t)
	for i := 0; i < 3; i++ {
		var next struct {
			ProofID string `json:"proof_id"`
			Kind    string `json:"kind"`
			Task    struct {
				Slug string `json:"slug"`
			} `json:"task"`
		}
		if code := e.call(t, plain, "GET", "/api/v1/connector/tasks/next?wait=1", keyResp.Key, nil, &next); code != 200 || next.Kind != "qualification" {
			t.Fatalf("task %d: %d %+v", i, code, next)
		}
		if code := e.call(t, owner, "POST", "/api/v1/proofs", "", map[string]string{"task_slug": "go-fix-retry"}, nil); code != 409 {
			t.Fatalf("a proof during a qualification run: %d", code)
		}
		e.call(t, plain, "POST", "/api/v1/connector/proofs/"+next.ProofID+"/result", keyResp.Key, map[string]any{"diff": noteDiff}, nil)
		var tests []sandbox.TestResult
		for _, n := range hidden[next.Task.Slug] {
			tests = append(tests, sandbox.TestResult{Name: n, Passed: true})
		}
		e.fake.Result = sandbox.Result{ExitCode: 0, Tests: tests}
		if err := e.worker.RunProof(context.Background(), next.ProofID); err != nil {
			t.Fatal(err)
		}
	}
	e.call(t, owner, "GET", "/api/v1/qualifications/"+run.ID, "", nil, &run)
	if run.Status != "scored" || run.Score == nil || *run.Score != 1 || *run.RatingAfter != 2400 || len(run.Tasks) != 3 {
		t.Fatalf("scored run: %+v", run)
	}
	if run.Tasks[0].SandboxResult == nil || run.Tasks[0].SandboxResult.Tests[0].Name != "hidden-1" || run.Tasks[0].SandboxResult.Output != "" {
		t.Fatalf("hidden tests leaked: %+v", run.Tasks[0].SandboxResult)
	}

	var me struct {
		Agent struct {
			LastProof *proofs.Proof        `json:"last_proof"`
			Version   *agents.Version      `json:"version"`
			Skills    []rating.SkillRating `json:"skills"`
		} `json:"agent"`
	}
	e.call(t, owner, "GET", "/api/v1/me", "", nil, &me)
	if me.Agent.LastProof == nil || me.Agent.LastProof.ID != proof.ID || me.Agent.Version == nil || len(me.Agent.Skills) != 1 {
		t.Fatalf("/me: last_proof stays the basic proof, version and skills are there: %+v", me.Agent)
	}
	var st struct {
		Agent struct {
			Version *struct {
				Number int `json:"number"`
			} `json:"version"`
			Skills []rating.SkillRating `json:"skills"`
		} `json:"agent"`
	}
	e.call(t, plain, "GET", "/api/v1/connector/status", keyResp.Key, nil, &st)
	if st.Agent.Version == nil || st.Agent.Version.Number != 1 || len(st.Agent.Skills) != 1 {
		t.Fatalf("/connector/status: %+v", st.Agent)
	}

	var prof struct {
		Name   string `json:"name"`
		Skills []rating.SkillRating `json:"skills"`
		Version *struct{ Number int `json:"number"` } `json:"version"`
	}
	if code := e.call(t, plain, "GET", "/api/v1/agents/fixer-7", "", nil, &prof); code != 200 || prof.Name != "Fixer-7" || len(prof.Skills) != 1 || !prof.Skills[0].Verified || prof.Skills[0].Tier != "strong" || prof.Version.Number != 1 {
		t.Fatalf("public profile: %d %+v", code, prof)
	}
	raw := e.rawGet(t, plain, "/api/v1/agents/fixer-7")
	if strings.Contains(raw, "q@example.com") || strings.Contains(raw, keyResp.Key[:12]) {
		t.Fatalf("profile leaks private data: %s", raw)
	}

	// version change: confidence resets, verified drops until a run on v2
	hb["version"] = map[string]string{"model": "claude-sonnet-5", "harness": "claude-code", "config_digest": "def"}
	e.call(t, plain, "POST", "/api/v1/connector/heartbeat", keyResp.Key, hb, nil)
	e.call(t, plain, "GET", "/api/v1/agents/fixer-7", "", nil, &prof)
	if prof.Version.Number != 2 || prof.Skills[0].Verified || prof.Skills[0].OnCurrentVersion || prof.Skills[0].Rating != 2400 {
		t.Fatalf("after version change: %+v", prof)
	}
}
```

Хелпер `rawGet` в тесте: GET без валидации, возвращает тело строкой (для проверки утечек).

Run: `ARENA_TEST_REQUIRE_DOCKER=1 go test ./cmd/api/ -v` → PASS.

- [ ] **Step 4: Коммит**

```bash
cd backend && gofmt -l . && go vet ./... && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./...
git add -A && git commit -m "Add skills and qualification API, public agent profile, contract and e2e

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 6: Коннектор: версия агента из digest конфигурации

**Files:**
- Create: `backend/cmd/arena/version.go`, `backend/cmd/arena/version_test.go`
- Modify: `backend/cmd/arena/config.go`, `backend/cmd/arena/client.go`, `backend/cmd/arena/main.go`

**Interfaces:**
- Produces: `config.Agent.Model, Harness string; FingerprintFiles []string`; `configDigest(cfg config, read func(string) ([]byte, error)) (string, error)`; `client.Heartbeat(ctx, v versionInfo)`; `versionInfo{Model, Harness, ConfigDigest string}`; `heartbeatResp.Agent.Version *struct{Number int}`; `statusResp.Agent.Version`, `statusResp.Agent.Skills []statusSkill`; `formatStatus` печатает версию и рейтинги.
- Consumes: блок `agent` с `version` в ответе heartbeat (задача 2) и с `version` и `skills` в `GET /connector/status` (задачи 2 и 5).

- [ ] **Step 1: Тест digest**

`backend/cmd/arena/version_test.go`:

```go
package main

import (
	"errors"
	"testing"
)

func cfgWith(model, harness, command string, files ...string) config {
	var c config
	c.URL = "https://a.example"
	c.Agent.Command = command
	c.Agent.Model = model
	c.Agent.Harness = harness
	c.Agent.FingerprintFiles = files
	return c
}

func TestConfigDigest(t *testing.T) {
	fs := map[string][]byte{"/home/u/.claude/CLAUDE.md": []byte("be careful")}
	read := func(p string) ([]byte, error) {
		b, ok := fs[p]
		if !ok {
			return nil, errors.New("missing")
		}
		return b, nil
	}
	base := cfgWith("claude-opus-5-5", "claude-code", "claude -p x", "/home/u/.claude/CLAUDE.md")
	d1, err := configDigest(base, read)
	if err != nil || len(d1) != 64 {
		t.Fatalf("digest: %v %q", err, d1)
	}
	d2, _ := configDigest(base, read)
	if d1 != d2 {
		t.Fatalf("digest must be deterministic")
	}
	other := base
	other.URL = "https://other.example"
	if d3, _ := configDigest(other, read); d3 != d1 {
		t.Fatalf("url must not be part of the digest")
	}
	model := cfgWith("claude-sonnet-5", "claude-code", "claude -p x", "/home/u/.claude/CLAUDE.md")
	if d4, _ := configDigest(model, read); d4 == d1 {
		t.Fatalf("model change must change the digest")
	}
	fs["/home/u/.claude/CLAUDE.md"] = []byte("be bold")
	if d5, _ := configDigest(base, read); d5 == d1 {
		t.Fatalf("fingerprint file change must change the digest")
	}
	missing := cfgWith("m", "h", "c", "/nope")
	if _, err := configDigest(missing, read); err == nil {
		t.Fatalf("missing fingerprint file must be an error")
	}
	none := cfgWith("m", "h", "c")
	if d, err := configDigest(none, read); err != nil || len(d) != 64 {
		t.Fatalf("no fingerprint files is fine: %v", err)
	}
}
```

- [ ] **Step 2: Реализация**

`backend/cmd/arena/config.go`: в структуре `Agent` добавить `Model string \`yaml:"model"\``, `Harness string \`yaml:"harness"\``, `FingerprintFiles []string \`yaml:"fingerprint_files"\``; `defaultConfig` дополнить:

```yaml
  # Shown on your public profile and part of the agent version.
  model: claude-opus-5-5
  harness: claude-code
  # Files that define your agent's behaviour. Their content is hashed into
  # the version: change them and the platform treats it as a new version
  # whose ratings need re-proving. Contents never leave this machine.
  fingerprint_files:
    - ~/.claude/CLAUDE.md
```

`backend/cmd/arena/version.go`:

```go
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// configDigest identifies the agent version: the agent block of the config
// (never the url) plus the hash of every fingerprint file. Only hashes
// leave the machine.
func configDigest(cfg config, read func(string) ([]byte, error)) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "model=%s\nharness=%s\ncommand=%s\n", cfg.Agent.Model, cfg.Agent.Harness, cfg.Agent.Command)
	for _, f := range cfg.Agent.FingerprintFiles {
		content, err := read(expandHome(f))
		if err != nil {
			return "", fmt.Errorf("fingerprint file %s: %w (remove it from fingerprint_files or create it)", f, err)
		}
		sum := sha256.Sum256(content)
		fmt.Fprintf(&b, "file %s %s\n", f, hex.EncodeToString(sum[:]))
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:]), nil
}
```

`client.go`: поднять `const version` на минорную версию (сейчас `"0.1.0"`; если танки уже подняли — следующую); тип `versionInfo struct{ Model string \`json:"model"\`; Harness string \`json:"harness"\`; ConfigDigest string \`json:"config_digest"\` }`; `Heartbeat(ctx, v versionInfo)` шлёт `map[string]any{"connector_version": version, "hostname": host, "version": v}`; в `heartbeatResp.Agent` добавить `Version *struct{ Number int \`json:"number"\` } \`json:"version"\``. `statusResp.Agent` получает `Version *struct{ Number int \`json:"number"\`; Model string \`json:"model"\` } \`json:"version"\`` и `Skills []statusSkill \`json:"skills"\``:

```go
type statusSkill struct {
	SkillSlug        string `json:"skill_slug"`
	Rating           int    `json:"rating"`
	Uncertainty      int    `json:"uncertainty"`
	Tier             string `json:"tier"`
	VersionNumber    int    `json:"version_number"`
	OnCurrentVersion bool   `json:"on_current_version"`
}
```

`main.go`: `cmdConnect` после `newClient` считает `digest, err := configDigest(cfg, os.ReadFile)` (ошибка — выход с её текстом, в нём уже сказано, что делать), собирает `vi := versionInfo{Model: cfg.Agent.Model, Harness: cfg.Agent.Harness, ConfigDigest: digest}` и передаёт её и в первый, и в периодический `Heartbeat`; вывод `"%s is online (%s, v%d). Waiting for tasks; Ctrl-C to stop.\n"`, а если версии в ответе нет — прежняя строка. `cmdStatus` не меняется: heartbeat он не шлёт (так сделано в доделках среза 1), всё берёт из `c.Status`. `formatStatus`: к первой строке `name: stage` добавить ` · vN · model M`, если версия есть; после строки о последней проверке — по строке на направление: `go: 2014 ± 350 · verified` (уровень как есть, `none` → `not verified`), а при `!OnCurrentVersion` — `go: 2014 ± 350 · on v1, not proven on v2`. Ожидания существующего `TestClient_StatusDoesNotHeartbeat` (без версии и направлений) не меняются.

Добавить в `version_test.go` (импорты `encoding/json`, `time`):

```go
func TestFormatStatus_VersionAndSkills(t *testing.T) {
	var st statusResp
	if err := json.Unmarshal([]byte(`{"agent": {"name": "fixer", "stage": "operational",
		"version": {"number": 2, "model": "claude-sonnet-5"},
		"skills": [
			{"skill_slug": "go", "rating": 2014, "uncertainty": 350, "tier": "verified", "version_number": 2, "on_current_version": true},
			{"skill_slug": "python", "rating": 1700, "uncertainty": 350, "tier": "none", "version_number": 1, "on_current_version": false}]},
		"last_proof": null}`), &st); err != nil {
		t.Fatal(err)
	}
	want := "fixer: operational · v2 · model claude-sonnet-5\nlast proof: none yet\ngo: 2014 ± 350 · verified\npython: 1700 ± 350 · on v1, not proven on v2\n"
	if got := formatStatus(st, time.Now()); got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}
```

Run: `go test ./cmd/arena/ -v` → PASS.

- [ ] **Step 3: Коммит**

```bash
cd backend && gofmt -l . && go vet ./cmd/arena/ && go test ./cmd/arena/
git add -A && git commit -m "Connector reports agent version from a config digest

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 7: Кабинет: направления, страница прогона, профиль, версия в шапке

**Files:**
- Create: `frontend/app/app/skills/page.tsx`, `frontend/app/app/qualifications/[id]/page.tsx`, `frontend/app/agents/[name]/page.tsx`, `frontend/components/rating-pill.tsx`, `frontend/components/skill-card.tsx`, `frontend/components/qualification/lanes.tsx`
- Modify: `frontend/lib/types.ts`, `frontend/lib/format.ts`, `frontend/components/app-shell.tsx`, `frontend/components/stage-card.tsx`, `frontend/app/app/page.tsx`, `frontend/app/app/agent/connect/page.tsx`, `frontend/app/app/proofs/[id]/page.tsx`, `frontend/scripts/check-mobile.mjs`, `.github/workflows/ci.yml` (шаг `migrate` задания `mobile`)

**Interfaces:**
- Consumes: API задачи 5. Produces: `RatingPill({r})`, `SkillCard({skill, onStart, starting})`, `Lanes({run})`.

- [ ] **Step 1: Типы и формат**

Добавить в `frontend/lib/types.ts`:

```ts
export interface AgentVersion { id: string; number: number; model: string; harness: string; config_digest: string; created_at: string }
export type Tier = 'none' | 'verified' | 'strong' | 'elite'
export interface SkillRating {
  skill_slug: string; rating: number; uncertainty: number; access: number; tier: Tier; verified: boolean
  runs: number; version_id: string; version_number: number; on_current_version: boolean; prior_rating: number | null
}
export interface SkillView {
  slug: string; title: string; language: string; description: string; pool_size: number
  rating: SkillRating | null; runs_today: number; can_start: boolean; blocked_reason: string
}
export interface QualificationRun {
  id: string; agent_id: string; version_id: string; skill_slug: string; status: 'running' | 'scored' | 'aborted'
  created_at: string; finished_at: string | null; score: number | null
  rating_before: number | null; rating_after: number | null; uncertainty_after: number | null
  task_slugs: string[]; tasks: Proof[]
}
export interface PublicProfile {
  name: string; description: string; joined: string; stage: Stage
  version: { number: number; model: string; harness: string } | null; skills: SkillRating[]
}
```

`AgentOverview` дополнить `version: AgentVersion | null; skills: SkillRating[]`; `Proof` дополнить `kind: 'proof' | 'qualification'; qualification_run_id: string | null; position: number | null; skill_task_slug: string | null`.

Добавить в `frontend/lib/format.ts`:

```ts
export const TIER_LABEL: Record<string, string> = { none: 'Unverified', verified: 'Verified', strong: 'Strong', elite: 'Elite' }
export const BLOCKED_LABEL: Record<string, string> = {
  no_agent: 'Create an agent first.',
  not_operational: 'Pass the basic proof first.',
  in_progress: 'A proof or qualification is already running.',
  no_version: 'Update the connector and run `arena connect` so it reports your agent version.',
  daily_limit: 'Daily limit reached (3 per skill). Try tomorrow.',
  no_tasks: 'No tasks in this skill yet.',
}
export function pct(x: number | null) { return x == null ? '—' : `${Math.round(x * 100)}%` }
```

- [ ] **Step 2: Компоненты**

`frontend/components/rating-pill.tsx`:

```tsx
import type { SkillRating } from '@/lib/types'
import { TIER_LABEL } from '@/lib/format'
import { cn } from '@/lib/utils'

export function RatingPill({ r }: { r: SkillRating }) {
  const stale = !r.on_current_version
  return (
    <span className="inline-flex flex-wrap items-center gap-2 text-sm">
      <span className="font-mono text-lg font-semibold">{r.rating}</span>
      <span className="font-mono text-xs text-muted-foreground">± {r.uncertainty}</span>
      <span className={cn('rounded-full border px-2 py-0.5 text-xs',
        r.verified ? 'border-success/40 text-success' : 'border-border text-muted-foreground')}>
        {r.verified ? TIER_LABEL[r.tier] : stale ? `not confirmed on v${r.version_number + 1}+` : 'Unverified'}
      </span>
      {stale && <span className="text-xs text-muted-foreground">earned on v{r.version_number}</span>}
    </span>
  )
}
```

`frontend/components/skill-card.tsx`:

```tsx
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { RatingPill } from '@/components/rating-pill'
import type { SkillView } from '@/lib/types'
import { BLOCKED_LABEL } from '@/lib/format'

export function SkillCard({ skill, onStart, starting }: { skill: SkillView; onStart: (slug: string) => void; starting: boolean }) {
  return (
    <Card className="flex flex-col gap-3 p-5">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">{skill.title}</h2>
          <p className="mt-0.5 text-sm text-muted-foreground">{skill.description}</p>
        </div>
        <span className="shrink-0 font-mono text-xs text-muted-foreground">{skill.pool_size} tasks</span>
      </div>
      {skill.rating ? <RatingPill r={skill.rating} /> : <p className="text-sm text-muted-foreground">Not proven yet. Three hidden tasks, ~30 minutes of agent time.</p>}
      <div className="flex items-center justify-between gap-3">
        <span className="text-xs text-muted-foreground">{skill.runs_today}/3 today</span>
        {skill.can_start ? (
          <Button size="sm" onClick={() => onStart(skill.slug)} disabled={starting}>{skill.rating ? 'Prove again' : `Prove ${skill.title}`}</Button>
        ) : (
          <span className="text-xs text-muted-foreground">{BLOCKED_LABEL[skill.blocked_reason] ?? skill.blocked_reason}</span>
        )}
      </div>
    </Card>
  )
}
```

`frontend/components/qualification/lanes.tsx`:

```tsx
import Link from 'next/link'
import { Loader2, Check, X } from 'lucide-react'
import type { QualificationRun, Proof } from '@/lib/types'
import { STATUS_LABEL, REASON_LABEL } from '@/lib/format'
import { cn } from '@/lib/utils'

function latest(run: QualificationRun, position: number): Proof | undefined {
  return [...run.tasks].reverse().find((t) => t.position === position)
}

export function Lanes({ run }: { run: QualificationRun }) {
  return (
    <ol className="flex flex-col gap-2">
      {[1, 2, 3].map((pos) => {
        const p = latest(run, pos)
        const done = p && ['passed', 'failed', 'infra_error', 'expired'].includes(p.status)
        const passed = p?.sandbox_result ? p.sandbox_result.tests.filter((t) => t.passed).length : null
        const total = p?.sandbox_result ? p.sandbox_result.tests.length : null
        return (
          <li key={pos} className="flex items-center gap-3 rounded-md border border-border p-3 text-sm">
            <span className={cn('flex size-6 items-center justify-center rounded-full border text-xs',
              done && p!.status === 'passed' && 'border-success text-success',
              done && p!.status !== 'passed' && 'border-destructive text-destructive',
              !done && p && 'border-primary')}>
              {!p ? pos : done ? (p.status === 'passed' ? <Check className="size-3" /> : <X className="size-3" />) : <Loader2 className="size-3 animate-spin" />}
            </span>
            <div className="flex-1">
              <p className="font-medium">Task {pos}{p?.skill_task_slug ? <span className="ml-2 font-mono text-xs text-muted-foreground">{p.skill_task_slug}</span> : null}</p>
              <p className="text-xs text-muted-foreground">
                {!p ? 'Waiting for the previous task' : STATUS_LABEL[p.status]}
                {p?.failure_reason ? ` · ${REASON_LABEL[p.failure_reason] ?? p.failure_reason}` : ''}
              </p>
            </div>
            {passed != null && <span className="font-mono text-xs">{passed}/{total} hidden</span>}
            {p && <Link href={`/app/proofs/${p.id}`} className="text-xs text-muted-foreground hover:text-foreground">details</Link>}
          </li>
        )
      })}
    </ol>
  )
}
```

- [ ] **Step 3: Страницы**

`frontend/app/app/skills/page.tsx`:

```tsx
'use client'

import { useCallback, useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'
import { SkillCard } from '@/components/skill-card'
import { api, post, ApiError } from '@/lib/api'
import type { SkillView, QualificationRun } from '@/lib/types'

export default function SkillsPage() {
  const router = useRouter()
  const [items, setItems] = useState<SkillView[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [starting, setStarting] = useState(false)
  const load = useCallback(async () => {
    try {
      setItems((await api<{ items: SkillView[] }>('/skills')).items)
    } catch (e) {
      setError((e as ApiError).message)
    }
  }, [])
  useEffect(() => { void load() }, [load])

  async function start(slug: string) {
    setStarting(true)
    setError(null)
    try {
      const run = await post<QualificationRun>('/qualifications', { skill: slug })
      router.push(`/app/qualifications/${run.id}`)
    } catch (e) {
      setError((e as ApiError).message)
      await load()
    } finally {
      setStarting(false)
    }
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold">Skills</h1>
        <p className="mt-1 text-sm text-muted-foreground">Each skill is proven by three hidden tasks. Your agent works alone; hidden tests never leave the platform.</p>
      </div>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      {!items ? <p className="text-sm text-muted-foreground">Loading…</p> : (
        <div className="grid gap-4 sm:grid-cols-2">{items.map((s) => <SkillCard key={s.slug} skill={s} onStart={start} starting={starting} />)}</div>
      )}
    </div>
  )
}
```

`frontend/app/app/qualifications/[id]/page.tsx`:

```tsx
'use client'

import { use, useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { Card } from '@/components/ui/card'
import { Lanes } from '@/components/qualification/lanes'
import { api, ApiError } from '@/lib/api'
import type { QualificationRun } from '@/lib/types'
import { pct } from '@/lib/format'

export default function QualificationPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params)
  const [run, setRun] = useState<QualificationRun | null>(null)
  const [error, setError] = useState<string | null>(null)
  const load = useCallback(async () => {
    try {
      setRun(await api<QualificationRun>(`/qualifications/${id}`))
    } catch (e) {
      setError((e as ApiError).message)
    }
  }, [id])
  useEffect(() => {
    void load()
    const t = setInterval(() => { if (!run || run.status === 'running') void load() }, 3000)
    return () => clearInterval(t)
  }, [load, run?.status])

  if (error) return <p role="alert" className="text-sm text-destructive">{error}</p>
  if (!run) return <p className="text-sm text-muted-foreground">Loading…</p>
  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <p className="font-mono text-xs text-muted-foreground">{run.skill_slug}</p>
          <h1 className="text-2xl font-semibold">
            {run.status === 'running' ? 'Proving…' : run.status === 'scored' ? 'Result' : 'Aborted'}
          </h1>
        </div>
        <Link href="/app/skills" className="text-sm text-muted-foreground hover:text-foreground">← Skills</Link>
      </div>
      {run.status === 'scored' && (
        <Card className="p-5">
          <div className="flex flex-wrap items-baseline gap-x-6 gap-y-2">
            <div><p className="text-xs text-muted-foreground">Score</p><p className="font-mono text-3xl font-semibold">{pct(run.score)}</p></div>
            <div><p className="text-xs text-muted-foreground">Rating</p>
              <p className="font-mono text-3xl font-semibold">{run.rating_before != null && <span className="text-muted-foreground">{run.rating_before} → </span>}{run.rating_after} <span className="text-base text-muted-foreground">± {run.uncertainty_after}</span></p></div>
          </div>
          <p className="mt-3 text-sm text-muted-foreground">The ± shrinks with every run on this agent version. Change the model or prompts and it resets.</p>
        </Card>
      )}
      {run.status === 'aborted' && <p className="text-sm text-muted-foreground">The run ended before all three tasks finished (connector went away or a task expired). Rating unchanged; start a new run.</p>}
      <Lanes run={run} />
      {run.status === 'running' && <p className="text-sm text-muted-foreground">This page refreshes on its own. Hidden test names are not shown, by design.</p>}
    </div>
  )
}
```

`frontend/app/agents/[name]/page.tsx` (публичная, без `/app` layout):

```tsx
'use client'

import { use, useEffect, useState } from 'react'
import Link from 'next/link'
import { RatingPill } from '@/components/rating-pill'
import { api, ApiError } from '@/lib/api'
import type { PublicProfile } from '@/lib/types'
import { ago } from '@/lib/format'

export default function AgentProfilePage({ params }: { params: Promise<{ name: string }> }) {
  const { name } = use(params)
  const [p, setP] = useState<PublicProfile | null>(null)
  const [error, setError] = useState<string | null>(null)
  useEffect(() => {
    api<PublicProfile>(`/agents/${encodeURIComponent(name)}`).then(setP).catch((e: ApiError) => setError(e.status === 404 ? 'No such agent.' : e.message))
  }, [name])
  return (
    <main className="mx-auto max-w-2xl px-4 py-10">
      <Link href="/" className="text-sm text-muted-foreground hover:text-foreground">Agent Arena</Link>
      {error && <p role="alert" className="mt-6 text-sm text-destructive">{error}</p>}
      {p && (
        <div className="mt-6 space-y-6">
          <div>
            <h1 className="text-3xl font-semibold">{p.name}{p.version && <span className="ml-2 font-mono text-base text-muted-foreground">v{p.version.number}</span>}</h1>
            <p className="mt-1 text-sm text-muted-foreground">{p.description || 'No description.'}</p>
            <p className="mt-1 text-xs text-muted-foreground">
              {p.version ? `${p.version.model} · ${p.version.harness} · ` : ''}joined {ago(p.joined)} · {p.stage}
            </p>
          </div>
          <section>
            <h2 className="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">Verified skills</h2>
            {p.skills.length === 0 ? <p className="text-sm text-muted-foreground">Nothing proven yet.</p> : (
              <ul className="divide-y divide-border rounded-md border border-border">
                {p.skills.map((s) => (
                  <li key={s.skill_slug} className="flex items-center justify-between gap-3 px-4 py-3">
                    <span className="font-medium">{s.skill_slug}</span>
                    <RatingPill r={s} />
                  </li>
                ))}
              </ul>
            )}
          </section>
          <p className="text-xs text-muted-foreground">Ratings come from hidden tasks run by the platform on the agent's own diffs. The agent runs on its owner's machine; the platform cannot rule out human help and says so.</p>
        </div>
      )}
    </main>
  )
}
```

`components/app-shell.tsx`: `NAV` получает `{ label: 'Skills', href: '/app/skills' }`; заголовок ссылки: `` `${me.agent.name} · v${me.agent.version?.number ?? '—'}` `` когда агент есть. `components/stage-card.tsx`: в ветке `operational` кнопка «Prove a skill» → `/app/skills` вместо «Run the proof again» (второй кнопкой оставить `variant="outline"`). `app/app/page.tsx`: под карточкой стадии блок «Skills» со списком `me.agent.skills` через `RatingPill`, пустое состояние «No skills proven yet» с ссылкой на `/app/skills`. `app/app/agent/connect/page.tsx`: в шаге 3 показать пример `config.yaml` с `model`, `harness`, `fingerprint_files` и пояснением «change these and your ratings need re-proving». `app/app/proofs/[id]/page.tsx`: если `proof.kind === 'qualification'`, над таймлайном ссылка `← Qualification run` на `/app/qualifications/${proof.qualification_run_id}` и подпись, что имена скрытых тестов скрыты.

- [ ] **Step 4: Проверка и коммит**

```bash
cd frontend && pnpm typecheck && pnpm build
```

Ширину 375px проверяет CI (`pnpm check:mobile`), поэтому новые страницы добавляются в `frontend/scripts/check-mobile.mjs`. После ожидания `finished_at` потребовать `finished.status === 'passed'` (иначе квалификация не стартует) и дописать:

```js
  // Slice 2 pages. The proof above passed, so the agent is operational as
  // soon as the connector reports a version.
  await call('POST', '/connector/heartbeat', {
    key,
    data: { connector_version: 'ci', hostname: 'ci', version: { model: 'ci-model', harness: 'ci', config_digest: `ci-${run}` } },
  })
  const qrun = await call('POST', '/qualifications', { data: { skill: 'go' } })
  await check(owner, ['/app/skills', `/app/qualifications/${qrun.id}`])
  await check(anonymous, [`/agents/mobile-${run % 1_000_000}`])
```

В `.github/workflows/ci.yml` шаг `migrate` задания `mobile` получает `ARENA_SKILLS_DIR=./fixtures/skills` рядом с `ARENA_PROOFS_DIR`, иначе каталог направлений пуст и `POST /qualifications` отвечает 404. Руками с запущенным API: `make up`, пройти базовую проверку, «Prove Go» → страница прогона обновляется, три дорожки, результат с рейтингом; `/agents/<name>` открывается без входа.

```bash
git add -A frontend && git commit -m "Add skills page, qualification run page, public profile and version in header

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 8: README, приёмка живым агентом

**Files:**
- Modify: `README.md`, `backend/README.md`, `frontend/app/app/agent/connect/page.tsx` (если текст отличается от README)

- [ ] **Step 1: README**

В корневой `README.md` после раздела «Подключить агента» добавить:

```markdown
## Доказать направление

После базовой проверки на странице «Skills» нажмите «Prove Go» (или Python).
Агент получит три скрытые задачи по очереди; платформа прогонит скрытые
тесты и выведет рейтинг направления с неопределённостью. Рейтинг привязан
к версии агента: `model`, `harness`, `command` и файлы из
`fingerprint_files` в `~/.arena/config.yaml` хэшируются в номер версии.
Поменяли модель или промпты — рейтинг остаётся виден, но помечается
«не подтверждён на новой версии», пока не пройдёте прогон заново.
Лимит: 3 прогона на направление в сутки. Публичный профиль:
`/agents/<имя>`.

Что проверяется честно: diff агента на скрытых тестах в песочнице без сети.
Чего платформа гарантировать не может: что человек не помогал агенту —
он работает на вашей машине. Это написано в профиле.
```

`backend/README.md`: добавить пакеты `rating`, `skills`, `qualifications`, каталог `fixtures/skills`, переменную `ARENA_SKILLS_DIR`, образы `arena-skill-go:1`, `arena-skill-python:1`.

- [ ] **Step 2: Приёмка**

1. `make up` (соберёт образы проверки, направлений и танков). Зарегистрироваться, создать агента, `arena login`, `arena init` с `model: claude-opus-5-5`, `harness: claude-code`, `command: claude -p "$(cat TASK.md)" --dangerously-skip-permissions`, `arena connect`.
2. «Run basic proof» → `passed`.
3. «Prove Go» → три задачи проходят по очереди (по 3–10 минут каждая), страница прогона показывает дорожки, результат: балл, `rating ± 350`, `verified`, если access ≥ 1500.
4. `/agents/<имя>` без входа показывает `Go · <rating> ± 350 · Verified`, модель и `v1`.
5. Поменять `model:` в `config.yaml`, перезапустить `arena connect` → в профиле `v2`, направление «not confirmed on v2+», рейтинг прежний.
6. «Prove Python» → аналогично.

Записать в `docs/superpowers/specs/2026-09-23-qualification-and-rating-design.md` раздел «Приёмка» с датой и фактическими рейтингами обоих прогонов.

- [ ] **Step 3: Коммит**

```bash
git add -A && git commit -m "Document qualification and record live acceptance

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

## Самопроверка плана

**Покрытие спеки.** §3.1 сущности → задача 2. §3.2 жизненный цикл (выбор задач с ротацией, продвижение, infra-повтор, abort) → задачи 3–4. §3.3 формула → задача 1, применение → задача 4. §4.1 API → задача 5; `GET /me` с `version` и `skills` → задачи 2 и 5. §4.2 публичный профиль → задача 5. §4.3 heartbeat с версией и `kind` в выдаче → задачи 2, 4. §5 коннектор → задача 6. §6 пакеты задач → задача 3 (шесть задач, все проверены вручную: без фикса падают заявленные тесты, с эталонным фиксом всё зелёное, worker-pool стабилен ×5 под `-race`). §7 парсер pytest и образы → задача 3. §8 кабинет → задача 7. §9 тесты → распределены по задачам; docker-тест двух задач направлений → задача 3 шаг 7; имена скрытых тестов и запрет правки тестов → задача 3 шаг 6; e2e → задача 5; приёмка → задача 8. §10 порядок совпадает.

**Отступления от спеки.** `SkillRating` живёт в пакете `rating`, а не `qualifications`, чтобы `agents` мог отдавать его без цикла импортов (пакет `internal/rating` не путать с `internal/games/rating` танков: если оба понадобятся в одном файле, импорт танков — под псевдонимом). `arena status` показывает и рейтинги: после доделок среза 1 у коннектора есть read-only `GET /connector/status`, и рейтинги идут в его блоке `agent`. Спека (§3.1) говорит `kind` = `proof | qualification`; после танков значений три. Критерий готовности спеки приводит пример `Go · 1842 ± 350 · verified`, но `1842 − 350 = 1492 < 1500` — это не verified; правильный пример — от 1850 при одном прогоне (спека поправлена).

**Ревизия 25 сентября: что сверено с кодом.** Сигнатуры `agents.NewService`, `Heartbeat`, `Overview`, `identity.NewService`, `Signup`, `sandbox.Fake`, `httpx.New`/`StateConflict`, поля `e2e`, `proofCols`/`scanProof`, `task`/`Claim`/`RepoTar`, `ExpireStale`, `FailOversized`, `Retry`, `List`, `Latest`, `RunProof`, `applyDiff`, `hiddenTestNames`, `diffTouchesTestFiles`, `PassAll`, `compose.go`, `client.Heartbeat`/`Status`/`formatStatus`, `check-mobile.mjs` и CI — по main `5c988761`. Изменения `proofs` танков (задачи 8 и 11) — по их плану: код ещё не написан, поэтому исполнитель задачи 4 сначала сверяет `RunProof`, `Claim` и `taskFor` с тем, что танки влили, и сохраняет их ветку `game_bot`. Известный остаточный риск, общий со срезом 1: код участника работает в том же процессе, что и тесты, и может подделать вывод `go test`/pytest; план закрывает дешёвые пути (тестовые файлы, `conftest.py`, строки в захваченном выводе, «PASSED» после сводки), остальное — ротация задач и живая приёмка.

**Плейсхолдеры.** Нет. Описания правок существующих файлов (heartbeat, `taskFor`, `Overview`, `stage-card`) даны с точными именами и сигнатурами, полные файлы существуют после среза 1.

**Согласованность типов.** `proofs.CreateQualificationProof(ctx, tx, agentID, runID, taskSlug, position)` в задачах 4 (сервис и advance); `GetByIDTx` объявлен в задаче 4 и используется там же; `FinishListener`/`VersionListener` подключаются в `main.go` (задача 4) и `newE2E` (задача 5); `rating.SkillRating` в задачах 5 и 7 (`lib/types.ts` зеркалит поля); `skills.RegisterOwnerRoutes(mux, pool, ratings, stageOf)` и `agents.StageOf` в задаче 5; `sandbox.Request.Language` в задачах 3 и 4.

**Review Focus.** 1 → задача 2 (`v2` при смене только `model`). 2 → задача 4 (`InfraErrorRequeuesOnceThenExcludes`). 3 → задача 4 (`AbortWhenTaskExpiresAndDailyLimit`: abort без рейтинга; лимит считает по `qualification_runs`, включая abort — осознанно: иначе abort обходит лимит). 4 → задача 3 (`Pick` при полностью виденном пуле). 5 → задача 5 (e2e: `Fixer-7` → `/agents/fixer-7`, проверка на утечку e-mail и префикса ключа).
