# Этап 1 — продукт и бэкенд (T1–T10)

Общие решения и контракты — в [00-overview.md](00-overview.md). Все пути ниже — от корня репозитория. Команды бэкенда выполняются из `backend/`.

**Образцы стиля, которые надо читать перед написанием нового модуля:** сервис — `backend/internal/agents/service.go`; HTTP — `backend/internal/competitions/http.go`; интеграционный тест — `backend/internal/agents/service_integration_test.go` (хелпер `problem(t, err)`, `dbtest.New(t)`); сквозной тест — `backend/cmd/api/main_test.go` (`newTestServer`, `call` с проверкой ответа по OpenAPI). Каждая временная метка после `Scan` нормализуется `.UTC()` (см. `competitions/service.go: scan`).

---

### Task 1: База — ветка, инструменты, зелёный старт

**Files:** Modify: `frontend/package.json` (скрипты). Ничего больше.

- [ ] **Step 1.** `git status` — если есть незакоммиченные чужие изменения, не трогать их и не откатывать; `git switch -c feat/mvp-first-competition`.
- [ ] **Step 2.** Поднять Docker: `docker info` → если демон недоступен: `colima start`, затем `export DOCKER_HOST=unix://$HOME/.colima/default/docker.sock TESTCONTAINERS_RYUK_DISABLED=true` (см. `backend/README.md`, раздел Colima). Если Docker поднять нельзя — записать это в «Отклонения» и во всех отчётах называть интеграционные тесты SKIPPED.
- [ ] **Step 3.** `cd backend && make test && make check`. Ожидание: все пакеты `ok`, `gofmt -l` ничего не печатает. В выводе `go test -v ./internal/agents/ 2>&1 | grep -c SKIP` должно быть `0`.
- [ ] **Step 4.** `cd frontend && corepack enable pnpm && pnpm install --frozen-lockfile`. Добавить в `package.json` скрипты `"typecheck": "tsc --noEmit"`. Запустить `pnpm typecheck` и `pnpm build`; **записать** список ошибок typecheck (они сейчас спрятаны `ignoreBuildErrors`) в описание коммита — чинятся в T17.
- [ ] **Step 5.** Commit: `Add frontend typecheck script`.

---

### Task 2: Пакет задания первого соревнования + обновление дизайна

**Files:**
- Create: `backend/fixtures/tasks/city-day-planner/places.json`, `task.json`, `checks.json`, `travel-vectors.json`
- Create: `backend/fixtures/tasks/tasks.go` (embed + загрузка), `backend/internal/tasks/cityplanner/travel.go`, `travel_test.go`, `dataset_test.go`
- Create: `backend/docs/competitions/city-day-planner.md` (условия для людей, на английском — их читает участник)
- Modify: `backend/docs/arena-backend-design.md` (версия 1.1)

**Interfaces — Produces:**
- `cityplanner.TravelMinutes(a, b Point) int`, `cityplanner.Point{Lat, Lng float64}`
- `tasks.Load(slug string) (Bundle, error)`; `Bundle{Task json.RawMessage; Places json.RawMessage; Checks []CheckDef}`; `CheckDef{ID, Title, Requirement string; Weight int; Required bool}`

- [ ] **Step 1: `places.json`** — перенести как есть:

```json
{
  "city": "Alderhaven",
  "currency": "EUR",
  "start": { "id": "start", "name": "Central Station", "lat": 44.4100, "lng": 8.9300 },
  "places": [
    {"id":"p01","name":"Old Harbor Promenade","category":"sight","lat":44.4062,"lng":8.9262,"visit_minutes":40,"opens":"00:00","closes":"23:59","cost_eur":0},
    {"id":"p02","name":"Maritime Museum","category":"museum","lat":44.4078,"lng":8.9221,"visit_minutes":90,"opens":"10:00","closes":"18:00","cost_eur":12},
    {"id":"p03","name":"Lantern Tower","category":"viewpoint","lat":44.4039,"lng":8.9185,"visit_minutes":45,"opens":"09:00","closes":"17:00","cost_eur":8},
    {"id":"p04","name":"Cathedral of St. Alda","category":"sight","lat":44.4091,"lng":8.9315,"visit_minutes":35,"opens":"08:00","closes":"19:00","cost_eur":0},
    {"id":"p05","name":"Market Hall","category":"food","lat":44.4108,"lng":8.9342,"visit_minutes":50,"opens":"07:30","closes":"15:00","cost_eur":14},
    {"id":"p06","name":"Botanical Garden","category":"park","lat":44.4163,"lng":8.9278,"visit_minutes":60,"opens":"09:00","closes":"18:30","cost_eur":5},
    {"id":"p07","name":"City Art Gallery","category":"museum","lat":44.4121,"lng":8.9366,"visit_minutes":80,"opens":"11:00","closes":"19:00","cost_eur":10},
    {"id":"p08","name":"Ropewalk Street","category":"shop","lat":44.4097,"lng":8.9289,"visit_minutes":30,"opens":"10:00","closes":"20:00","cost_eur":0},
    {"id":"p09","name":"Hilltop Fortress","category":"sight","lat":44.4214,"lng":8.9331,"visit_minutes":75,"opens":"09:30","closes":"17:30","cost_eur":9},
    {"id":"p10","name":"Funicular Ride","category":"viewpoint","lat":44.4152,"lng":8.9312,"visit_minutes":30,"opens":"08:00","closes":"22:00","cost_eur":4},
    {"id":"p11","name":"Trattoria da Bruna","category":"food","lat":44.4085,"lng":8.9301,"visit_minutes":60,"opens":"12:00","closes":"15:00","cost_eur":22},
    {"id":"p12","name":"Alder Park","category":"park","lat":44.4135,"lng":8.9221,"visit_minutes":45,"opens":"00:00","closes":"23:59","cost_eur":0},
    {"id":"p13","name":"Science Centre","category":"museum","lat":44.4049,"lng":8.9408,"visit_minutes":100,"opens":"10:00","closes":"17:00","cost_eur":15},
    {"id":"p14","name":"Glassblowers' Quarter","category":"shop","lat":44.4072,"lng":8.9377,"visit_minutes":40,"opens":"10:30","closes":"18:30","cost_eur":0},
    {"id":"p15","name":"Lighthouse Pier","category":"viewpoint","lat":44.4011,"lng":8.9249,"visit_minutes":30,"opens":"00:00","closes":"23:59","cost_eur":0},
    {"id":"p16","name":"Harbor Fish Shack","category":"food","lat":44.4055,"lng":8.9243,"visit_minutes":40,"opens":"11:30","closes":"21:00","cost_eur":16},
    {"id":"p17","name":"Clerk's Palace","category":"sight","lat":44.4102,"lng":8.9259,"visit_minutes":55,"opens":"09:00","closes":"16:00","cost_eur":7},
    {"id":"p18","name":"Night Observatory","category":"viewpoint","lat":44.4239,"lng":8.9402,"visit_minutes":60,"opens":"18:00","closes":"23:00","cost_eur":11},
    {"id":"p19","name":"Tram Depot Museum","category":"museum","lat":44.4180,"lng":8.9455,"visit_minutes":50,"opens":"10:00","closes":"16:00","cost_eur":6},
    {"id":"p20","name":"Cafe Meridiana","category":"food","lat":44.4116,"lng":8.9297,"visit_minutes":30,"opens":"07:00","closes":"19:00","cost_eur":6},
    {"id":"p21","name":"Old Town Walls Walk","category":"sight","lat":44.4142,"lng":8.9385,"visit_minutes":50,"opens":"00:00","closes":"23:59","cost_eur":0},
    {"id":"p22","name":"Aquarium","category":"museum","lat":44.4033,"lng":8.9301,"visit_minutes":110,"opens":"09:00","closes":"19:00","cost_eur":24},
    {"id":"p23","name":"Antiques Arcade","category":"shop","lat":44.4089,"lng":8.9238,"visit_minutes":35,"opens":"09:00","closes":"13:00","cost_eur":0},
    {"id":"p24","name":"Riverside Wine Cellar","category":"food","lat":44.4191,"lng":8.9246,"visit_minutes":70,"opens":"16:00","closes":"22:00","cost_eur":28}
  ]
}
```

- [ ] **Step 2: `travel.go`** — эталон правила D2:

```go
// Package cityplanner holds the published rules of the city-day-planner
// task. The checker (TypeScript) implements the same formula; both are
// pinned by fixtures/tasks/city-day-planner/travel-vectors.json.
package cityplanner

import "math"

type Point struct{ Lat, Lng float64 }

const (
	earthRadiusKm = 6371.0088
	detourFactor  = 1.3
	walkKmPerHour = 4.8
)

func TravelMinutes(a, b Point) int {
	if a == b {
		return 0
	}
	rad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat, dLng := rad(b.Lat-a.Lat), rad(b.Lng-a.Lng)
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(rad(a.Lat))*math.Cos(rad(b.Lat))*math.Sin(dLng/2)*math.Sin(dLng/2)
	km := 2 * earthRadiusKm * math.Asin(math.Sqrt(h))
	return int(math.Ceil(km * detourFactor / walkKmPerHour * 60))
}
```

- [ ] **Step 3: тесты (сначала написать, увидеть падение, потом Step 2).**
  - `TestTravelMinutes_Properties`: `TravelMinutes(p,p)==0`; симметричность для всех пар; `start→p08` ∈ [1,3]; `start→p09` ∈ [18,24].
  - `TestDataset_Shape`: 24 места, id уникальны и `^p\d{2}$`, категории из `sight|museum|viewpoint|food|park|shop`, `opens < closes`, формат `HH:MM`, `visit_minutes ≥ 30`, `cost_eur ≥ 0` целое, ≥ 8 бесплатных, ≥ 4 `food`, все координаты в 44.40–44.43 / 8.91–8.95.
  - `TestDataset_InfeasibleCases`: для (`09:00`, 30 мин, бюджет 1000) и (`23:30`, 120 мин, бюджет 0) **ни одно** место не посещаемо (перебор 24 мест: `arrive = max(start+travel, opens)`, `arrive+visit ≤ min(closes, start+window)`). Эти два случая станут скрытыми случаями проверки `no-plan`.
  - `TestDataset_FeasibleCases`: для (`09:00`, 480 мин, бюджет 40) жадный перебор находит план ≥ 3 мест; для (`09:00`, 240, бюджет 0) — ≥ 2 бесплатных мест.
  - `TestTravelVectors_Golden`: с флагом `-update` пишет `travel-vectors.json` — массив `{"from":"start","to":"p01","minutes":N}` для всех пар `start`×24 и 40 фиксированных пар мест (первые 40 по лексикографическому порядку `(from,to)`, `from<to`); без флага сверяет. Сгенерировать один раз, закоммитить.
- [ ] **Step 4: `checks.json`** — публичный манифест (сумма весов = 100):

```json
{
  "suite": "city-day-planner",
  "version": "1",
  "checks": [
    {"id":"build-plan","title":"Builds a plan from the provided places","requirement":"R1,R2","weight":15,"required":true},
    {"id":"time-constraint","title":"Plan respects time window, opening hours and travel rule","requirement":"R3","weight":15,"required":true},
    {"id":"budget-constraint","title":"Plan respects the budget and shows the correct total","requirement":"R3","weight":15,"required":true},
    {"id":"remove-stop","title":"Removing a stop recomputes a valid plan","requirement":"R4","weight":10,"required":true},
    {"id":"replace-stop","title":"Replacing a stop recomputes a valid plan","requirement":"R4","weight":10,"required":true},
    {"id":"persistence","title":"Plan and inputs survive a page reload","requirement":"R5","weight":10,"required":true},
    {"id":"no-plan","title":"Explains when no plan is possible","requirement":"R6","weight":10,"required":true},
    {"id":"mobile","title":"Main flow works at 375px width","requirement":"R7","weight":15,"required":true}
  ]
}
```

- [ ] **Step 5: `task.json`** — структурированные условия, отдаются в `Competition.task`. Поля (все обязательны): `version: "1"`; `what_to_build` (абзац); `where_it_runs` («Your agent runs on your own machine via arena-connector; the platform never executes your code; model credentials never leave your machine»); `what_to_submit` (описание `arena-result.json` из overview); `hosting` («any public HTTPS static hosting; the URL must stay reachable until results are final; optional `/arena-build.json` = `{"commit":"<sha>"}`»); `allowed` (`["localStorage","any frontend framework or none","bundling the dataset into the app"]`); `forbidden` (`["paid map tiles or geocoding","external travel/places APIs","server-side state","requests to third-party origins during the main flow other than fonts/CDN assets"]`); `travel_rule` (текст D2 + формула строкой); `requirements` — массив R1–R7 `{id, text}` ровно по списку раздела 1 ТЗ (R1 ввод времени и бюджета; R2 план из набора мест; R3 ограничения времени и бюджета; R4 удалить/заменить + пересчёт; R5 сохранение после перезагрузки; R6 объяснение отсутствия плана; R7 мобильная ширина 375px); `extras` — `["plan quality: more of the day well used","clear timeline or map-like visualisation without paid tiles","explains why places were chosen or skipped","meal stop near midday","keyboard and screen-reader accessibility","readable, maintainable source"]`; `ui_contract`:

```json
{
  "note": "Automated checks locate elements ONLY by these data-testid values and read ONLY these data-* attributes. Visible text, layout and extra elements are up to you.",
  "inputs": {
    "input-hours": "number input, available time in hours, accepts decimals such as 0.5",
    "input-budget": "number input, budget in EUR, integer",
    "input-start-time": "time input HH:MM, default 09:00",
    "build-plan": "button; builds or rebuilds the plan from the current inputs"
  },
  "plan": {
    "plan": "container, present only when a plan exists",
    "plan-stop": "one per stop, in visiting order; attributes data-place-id, data-arrive=HH:MM, data-leave=HH:MM",
    "remove-stop": "button inside each plan-stop; removes that place and recomputes",
    "replace-stop": "button inside each plan-stop; swaps that place for another one not in the plan and recomputes",
    "plan-total-cost": "element with data-value=<integer EUR>",
    "plan-end-time": "element with data-value=HH:MM (leave time of the last stop)"
  },
  "empty": {
    "no-plan": "visible only when no valid plan exists for the inputs; contains a human-readable explanation of at least 20 characters"
  },
  "timing": "After a click the UI must settle within 3 seconds."
}
```

  и `evaluation`: текст про 60/20/10/10, четыре статуса проверки, `unverifiable`, официальную и тренировочные попытки, `self_reported`, `commit_link` без значения verified.
- [ ] **Step 6: `tasks.go`** — `//go:embed tasks/*/*.json` рядом (`package tasks`, путь `backend/fixtures/tasks/tasks.go`), `Load(slug)` валидирует: `checks` веса = 100, id уникальны, `task.json` — валидный JSON с ключами из Step 5. Тест `TestLoad_CityDayPlanner`.
- [ ] **Step 7: `docs/competitions/city-day-planner.md`** — человекочитаемая версия условий на английском: те же разделы, таблица мест не дублируется (ссылка на `GET /api/v1/competitions/city-day-planner/dataset`), минимальный пример HTML-разметки контракта, пример `arena-result.json`, пример `arena-build.json`.
- [ ] **Step 8: дизайн-документ v1.1.** В `arena-backend-design.md`: шапка «Версия 1.1 · 21 сентября 2026»; в §0 заменить строки «Источник требований», «Судейство», «Одна сдача на соревнование», «Рейтинг арены» на решения D4–D9, D12, D15; добавить абзац «Макет фронтенда больше не источник доменных ограничений»; §5–§11 не переписывать целиком — добавить в начало каждого затронутого раздела врезку «Изменено в 1.1: см. §19», а в конец новый §19 «MVP первого соревнования» со ссылкой на `plans/mvp/00-overview.md` и кратким перечнем: попытки, снимок конфигурации, конвейер проверок, статусы, evidence, checker, опрос вместо SSE, отложенное (очередь, матчмейкинг, SSE, safefetch в Go). В `plans/arena-slices.md` — строка вверху: срезы 2–4 заменены планом `plans/mvp/`.
- [ ] **Step 9.** `go test ./fixtures/... ./internal/tasks/...` → PASS. Commit: `Add city-day-planner task package and design v1.1`.

---

### Task 3: Миграция 00003 — попытки, проверки, доказательства

**Files:** Create: `backend/migrations/00003_mvp.sql`. Test: `backend/internal/platform/db/db_integration_test.go` (дополнить).

- [ ] **Step 1.** Узнать реальное имя уникального ограничения: после `make up && make migrate` выполнить `docker compose exec postgres psql -U arena_migrate -d arena -c '\d submissions'`. Ожидается `submissions_competition_id_agent_id_key`; если иное — подставить. Тем же способом сверить имена `submissions_score_status_check` и `jobs_kind_check` (`\d jobs`) — миграция ниже удаляет их по имени.
- [ ] **Step 2.** Миграция (перенести как есть; `-- +goose Down` — обратные операции в обратном порядке, представления в Down восстанавливаются текстом из `00002`):

```sql
-- +goose Up
ALTER TABLE competitions ADD COLUMN task jsonb, ADD COLUMN check_suite text;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION competitions_immutable_after_publish() RETURNS trigger AS $$
BEGIN
    IF OLD.published_at IS NOT NULL AND (
        NEW.title IS DISTINCT FROM OLD.title OR NEW.summary IS DISTINCT FROM OLD.summary OR
        NEW.brief IS DISTINCT FROM OLD.brief OR NEW.category IS DISTINCT FROM OLD.category OR
        NEW.difficulty IS DISTINCT FROM OLD.difficulty OR NEW.points IS DISTINCT FROM OLD.points OR
        NEW.deadline IS DISTINCT FROM OLD.deadline OR
        NEW.match_duration_seconds IS DISTINCT FROM OLD.match_duration_seconds OR
        NEW.criteria IS DISTINCT FROM OLD.criteria OR NEW.slug IS DISTINCT FROM OLD.slug OR
        NEW.task IS DISTINCT FROM OLD.task OR NEW.check_suite IS DISTINCT FROM OLD.check_suite) THEN
        RAISE EXCEPTION 'competition % is immutable after publish', OLD.id;
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TABLE attempts (
    id text PRIMARY KEY,
    competition_id text NOT NULL REFERENCES competitions (id),
    agent_id text NOT NULL REFERENCES agents (id),
    kind text NOT NULL CHECK (kind IN ('official', 'practice')),
    attempt_no int NOT NULL CHECK (attempt_no >= 1),
    status text NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'submitted', 'abandoned')),
    agent_snapshot jsonb NOT NULL,
    reported_cost jsonb,
    match_id text REFERENCES matches (id),
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    voided_at timestamptz,
    void_reason text,
    UNIQUE (competition_id, agent_id, attempt_no)
);
CREATE UNIQUE INDEX attempts_one_official ON attempts (competition_id, agent_id)
    WHERE kind = 'official' AND voided_at IS NULL;
CREATE UNIQUE INDEX attempts_one_running ON attempts (competition_id, agent_id)
    WHERE status = 'running' AND voided_at IS NULL;

CREATE TABLE attempt_events (
    id bigserial PRIMARY KEY,
    attempt_id text NOT NULL REFERENCES attempts (id),
    kind text NOT NULL CHECK (kind IN ('started', 'phase', 'log', 'preview_available', 'build_finished',
                                       'submitted', 'check_started', 'check_finished', 'result', 'abandoned')),
    phase_index int CHECK (phase_index BETWEEN 0 AND 8),
    text text,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX attempt_events_attempt_idx ON attempt_events (attempt_id, id);

ALTER TABLE submissions DROP CONSTRAINT submissions_competition_id_agent_id_key;
ALTER TABLE submissions DROP CONSTRAINT submissions_score_status_check;
ALTER TABLE submissions ADD CONSTRAINT submissions_score_status_check
    CHECK (score_status IN ('pending', 'judging', 'scored', 'unverifiable', 'failed'));
ALTER TABLE submissions
    ADD COLUMN attempt_id text REFERENCES attempts (id),
    ADD COLUMN attempt_kind text NOT NULL DEFAULT 'official' CHECK (attempt_kind IN ('official', 'practice')),
    ADD COLUMN attempt_no int NOT NULL DEFAULT 1,
    ADD COLUMN agent_snapshot jsonb,
    ADD COLUMN commit_sha text,
    ADD COLUMN notes text,
    ADD COLUMN reported_cost jsonb,
    ADD COLUMN verification text NOT NULL DEFAULT 'self_reported' CHECK (verification IN ('self_reported')),
    ADD COLUMN commit_link text NOT NULL DEFAULT 'not_provided'
        CHECK (commit_link IN ('not_provided', 'unverified', 'declared_match', 'mismatch')),
    ADD COLUMN check_run_id text,
    ADD COLUMN unscored_reason text;
CREATE UNIQUE INDEX submissions_one_official ON submissions (competition_id, agent_id) WHERE attempt_kind = 'official';
CREATE UNIQUE INDEX submissions_one_per_attempt ON submissions (attempt_id) WHERE attempt_id IS NOT NULL;

CREATE TABLE check_runs (
    id text PRIMARY KEY,
    submission_id text NOT NULL REFERENCES submissions (id),
    suite text NOT NULL,
    suite_version text NOT NULL,
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'completed', 'unreachable', 'infra_error')),
    checker_version text,
    browser text,
    functional_score int CHECK (functional_score BETWEEN 0 AND 100),
    unreachable_reason text,
    error text,
    build_info jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    finished_at timestamptz
);
CREATE INDEX check_runs_submission_idx ON check_runs (submission_id, created_at DESC);
ALTER TABLE submissions ADD FOREIGN KEY (check_run_id) REFERENCES check_runs (id);

CREATE TABLE check_results (
    id text PRIMARY KEY,
    check_run_id text NOT NULL REFERENCES check_runs (id),
    check_id text NOT NULL,
    title text NOT NULL,
    requirement text NOT NULL,
    required boolean NOT NULL,
    weight int NOT NULL,
    status text NOT NULL CHECK (status IN ('passed', 'failed', 'insufficient_data', 'infra_error')),
    expected text NOT NULL DEFAULT '',
    actual text NOT NULL DEFAULT '',
    diagnostics jsonb NOT NULL DEFAULT '{}'::jsonb,
    duration_ms int,
    override_status text CHECK (override_status IN ('passed', 'failed', 'insufficient_data')),
    override_reason text,
    override_by text REFERENCES users (id),
    override_at timestamptz,
    UNIQUE (check_run_id, check_id)
);

CREATE TABLE evidence_blobs (
    id text PRIMARY KEY,
    check_run_id text NOT NULL REFERENCES check_runs (id),
    check_id text,
    kind text NOT NULL CHECK (kind IN ('screenshot', 'page_text', 'console', 'network')),
    label text NOT NULL,
    content_type text NOT NULL CHECK (content_type IN ('image/png', 'image/jpeg', 'text/plain', 'application/json')),
    bytes bytea NOT NULL,
    size int NOT NULL CHECK (size BETWEEN 1 AND 1048576),
    sha256 text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX evidence_blobs_run_idx ON evidence_blobs (check_run_id, check_id);

ALTER TABLE judgments ADD COLUMN check_run_id text REFERENCES check_runs (id), ADD COLUMN reason text;

ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check CHECK (kind IN ('check_submission', 'judge_submission', 'recompute_badges'));

DROP VIEW agent_standings;
DROP VIEW competition_rankings;
CREATE VIEW competition_rankings AS
SELECT s.id AS submission_id, s.competition_id, s.agent_id, s.total, s.submitted_at,
       row_number() OVER (PARTITION BY s.competition_id ORDER BY s.total DESC, s.submitted_at ASC) AS rank,
       count(*) OVER (PARTITION BY s.competition_id) AS rank_of
FROM submissions s
WHERE s.score_status = 'scored' AND s.attempt_kind = 'official';
-- agent_standings: скопировать определение из 00002 дословно, заменив в CTE scored условие на
--   WHERE score_status = 'scored' AND attempt_kind = 'official'
GRANT SELECT, INSERT, UPDATE, DELETE ON attempts, attempt_events, check_runs, check_results, evidence_blobs TO arena_app;
GRANT USAGE, SELECT ON SEQUENCE attempt_events_id_seq TO arena_app;
GRANT SELECT ON competition_rankings, agent_standings TO arena_app;
```

- [ ] **Step 3: тесты** (в `db_integration_test.go`): `TestMigrations_UpDownUp` (миграции чисто откатываются до 2 и накатываются снова — посмотреть, как это уже сделано для 00002, и расширить); `TestSchema_OneOfficialSubmission` (второй `INSERT` официальной сдачи того же агента → unique violation, а `practice` — проходит); `TestSchema_TaskImmutableAfterPublish` (`UPDATE competitions SET task = '{}'` у опубликованного → ошибка триггера).
- [ ] **Step 4.** `go test ./internal/platform/db/ ./cmd/seed/` → PASS (demo-seed вставляет сдачи без `attempt_id` — должно работать за счёт DEFAULT). Commit: `Add migration for attempts, check runs and evidence`.

---

### Task 4: `platform/jobs`

**Files:** Create: `backend/internal/platform/jobs/jobs.go`, `jobs_integration_test.go`.

**Interfaces — Produces:**

```go
type Job struct { ID, Kind, State string; Attempts, MaxAttempts int; Payload json.RawMessage; LastError string; RunAfter, CreatedAt time.Time }
func Enqueue(ctx context.Context, tx pgx.Tx, kind string, payload any, dedupeKey string) (id string, err error) // ON CONFLICT (dedupe_key) DO NOTHING → возвращает id существующего
func (q *Queue) Claim(ctx context.Context, owner string, kinds []string, lease time.Duration) (*Job, error)     // nil, nil если пусто
func (q *Queue) Complete(ctx context.Context, id string) error
func (q *Queue) Fail(ctx context.Context, id string, cause error) (final bool, err error)                       // повтор через 30s/2m/10m; final=true когда attempts >= max_attempts
func (q *Queue) Reclaim(ctx context.Context) (int, error)                                                        // leased с lease_until < now() → queued
func (q *Queue) List(ctx context.Context, state string) ([]Job, error)
func (q *Queue) Retry(ctx context.Context, id string) error                                                      // attempts=0, state=queued, run_after=now()
func New(pool *db.Pool) *Queue
```

SQL захвата — дословно из `arena-backend-design.md` §11.1 (`FOR UPDATE SKIP LOCKED`), с добавлением `AND kind = ANY($2)`.

- [ ] **Step 1: тесты** — `TestJobs_TwoWorkersNeverShareAJob` (20 заданий, 2 горутины, множество id не пересекается); `TestJobs_DedupeKey` (два `Enqueue` с одним ключом → одна строка); `TestJobs_FailSchedule` (после 1-го `Fail` `run_after ≈ now+30s`, после 3-го `final=true`, `state='failed'`); `TestJobs_ReclaimExpiredLease` (lease 1 мс → `Reclaim` = 1 → снова `Claim`).
- [ ] **Step 2.** Реализация. **Step 3.** `go test -race ./internal/platform/jobs/` → PASS. Commit: `Add job queue on SKIP LOCKED`.

---

### Task 5: Соревнование несёт задание; `seed-task`; датасет по HTTP

**Files:** Modify: `internal/competitions/model.go`, `service.go`, `validate.go`, `http.go`, `validate_test.go`, `service_integration_test.go`; `cmd/seed/main.go`; `fixtures/seed/seed.go`; `Makefile`; `contracts/openapi/openapi.yaml`.

- [ ] **Step 1.** `Criterion` получает `Source string \`json:"source,omitempty"\``; `Validate`: пусто → `"llm"`, иначе только `checks|llm`, не более одного критерия `checks`. Тест `TestValidate_CriterionSource`.
- [ ] **Step 2.** `Competition`/`Input`/`PublicView` получают `Task json.RawMessage \`json:"task"\`` (может быть `null`) и `CheckSuite string \`json:"check_suite"\``; `cols`, `scan`, `INSERT`, `UPDATE` расширяются. Валидация: если `check_suite != ""`, то `tasks.Load(check_suite)` должен существовать, а `task` — не пустой; среди критериев обязан быть ровно один `source: checks`. Счётчик `participants` в `cols` — заменить на `count(DISTINCT agent_id) … WHERE attempt_kind = 'official'`.
- [ ] **Step 3.** `GET /api/v1/competitions/{slug}/dataset` → `places.json` из `tasks.Load` (`Content-Type: application/json`, `Cache-Control: public, max-age=300` — исключение из общего `no-store`, выставить в хендлере после middleware). `404`, если у соревнования нет `check_suite`.
- [ ] **Step 4.** `cmd/seed`: флаги `-demo` (старое поведение `seed.Load`) и `-task <slug>`: создаёт пользователя `system-seed` (если нет) и соревнование из `tasks.Load(slug)` в статусе `draft` с критериями D5, `points 500`, `deadline = now + 14 дней`, `brief` = `what_to_build`. Идемпотентно: существующий `slug` → сообщение и exit 0. `Makefile`: `seed-demo` (`go run ./cmd/seed -demo`), `seed-task` (`go run ./cmd/seed -task city-day-planner`), старая цель `seed` удаляется; README обновится в T27.
- [ ] **Step 5.** OpenAPI: `Competition.task` (object, nullable), `check_suite`, `Criterion.source`, путь `/competitions/{slug}/dataset`. `go test ./contracts/... ./internal/competitions/... ./cmd/...` → PASS. Commit: `Carry the task bundle on competitions and add seed-task`.

---

### Task 6: Модуль `attempts`

**Files:** Create: `backend/internal/attempts/model.go`, `service.go`, `http.go`, `sanitize.go`, `sanitize_test.go`, `service_integration_test.go`. Modify: `cmd/api/handler.go`, `main.go`, `main_test.go` (deps), OpenAPI.

**Interfaces — Produces:**

```go
type AgentConfig struct { Adapter, AdapterModel, ConnectorVersion, OS string }   // json: adapter, adapter_model, connector_version, os; каждое ≤ 80 символов
type Attempt struct { ID, CompetitionID, CompetitionSlug, AgentID, Kind, Status string; AttemptNo int; AgentSnapshot json.RawMessage; MatchID *string; StartedAt time.Time; FinishedAt *time.Time }
type EventInput struct { Kind string; PhaseIndex *int; Text string }
func (s *Service) Start(ctx, actor identity.Actor, slug, kind string, cfg AgentConfig) (a Attempt, resumed bool, err error)
func (s *Service) AddEvents(ctx, actor identity.Actor, attemptID string, in []EventInput) error
func (s *Service) Abandon(ctx, actor identity.Actor, attemptID string) error
func (s *Service) Void(ctx, admin identity.Actor, attemptID, reason string) error
func (s *Service) RecordSystemEvent(ctx, tx pgx.Tx, attemptID, kind string, payload any) error  // для submissions/checks
func (s *Service) MarkSubmitted(ctx, tx pgx.Tx, attemptID string) error
func (s *Service) ListForAgent(ctx, agentID, competitionID string) ([]Attempt, error)
func (s *Service) Events(ctx, attemptID string, afterID int64, limit int) ([]Event, error)
```

**Правила `Start`** (одна транзакция, `SELECT … FOR UPDATE` строки соревнования):
1. Соревнование `active` и `now() < deadline`, иначе `404` / `409 deadline_passed`.
2. Есть незавершённая попытка (`status='running'`, не voided) → вернуть её, `resumed=true`, HTTP `200`.
3. `kind=official`, и официальная невоиденная попытка уже есть → `409 official_attempt_used`.
4. `attempt_no = coalesce(max(attempt_no),0)+1` по паре; снимок = `{name, model, bio}` из `agents` + `cfg`; событие `started`; аудит `attempt.started`. HTTP `201`.
5. Если у агента есть матч `queued|running` на это соревнование без привязанной попытки для его стороны — проставить `match_id` (заработает в T25; сейчас запрос просто ничего не находит).

**Маршруты (API-ключ):**
- `GET /agent/competitions/{slug}/task` → `{"competition": PublicView, "dataset_url": "/api/v1/competitions/{slug}/dataset" (относительный; коннектор разрешает его от своего `--api`), "attempts": [Attempt], "official_available": bool}`
- `POST /agent/competitions/{slug}/attempts` `{"kind","agent_config"}` → `201|200 {"attempt": Attempt, "resumed": bool}`
- `POST /agent/attempts/{id}/events` `{"events":[{kind, phase_index?, text?}]}` → `204`. Только `started|phase|log|preview_available|build_finished`; ≤ 20 событий на запрос; попытка своя и `running`, иначе `404` / `409 attempt_not_running`; in-memory лимит 4 запроса/с на попытку → `429 rate_limited`.
- `POST /agent/attempts/{id}/abandon` → `204`.
- Admin: `POST /admin/attempts/{id}/void` `{"reason"}` (причина 10–500 символов; `409 has_submission`, если сдача есть).

**`sanitize.go`** (сервер повторяет защиту коннектора — не доверяем клиенту):

```go
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(sk|pk|ak|ghp|gho|ghs|github_pat|xox[abprs]|AKIA)[-_][A-Za-z0-9_\-]{8,}`),
	regexp.MustCompile(`(?i)\b(bearer|token|secret|password|passwd|api[_-]?key)\b\s*[:=]\s*\S+`),
	regexp.MustCompile(`\b[A-Fa-f0-9]{32,}\b`),
	regexp.MustCompile(`\b[A-Za-z0-9+/_\-]{40,}={0,2}`),
	regexp.MustCompile(`-----BEGIN [A-Z ]+-----`),
}

// CleanText strips control characters, redacts anything secret-shaped and
// truncates to 200 runes. Empty result means "drop the text".
func CleanText(s string) string
```

Тесты `sanitize_test.go` (таблица): `"export ANTHROPIC_API_KEY=sk-ant-abc123def456"` → содержит `[redacted]`, не содержит `sk-ant`; `"Authorization: Bearer eyJhbGciOi…"` → redacted; 40 hex SHA → redacted (SHA в событиях не нужен); `"Editing src/plan.ts"` — без изменений; `\x1b[31mred\x1b[0m` → `red`; 500 символов → 200.

- [ ] **Step 1: интеграционные тесты** — `TestAttempts_OfficialOnlyOnce`, `TestAttempts_StartResumesRunning` (второй `Start` → тот же id, `resumed`), `TestAttempts_PracticeAfterOfficial` (`attempt_no = 2`), `TestAttempts_SnapshotSurvivesProfileEdit` (после `agents.Patch(model)` снимок старой попытки не изменился), `TestAttempts_VoidAllowsNewOfficial`, `TestAttempts_EventsRejectServerKinds` (`kind: "result"` от агента → `422`), `TestAttempts_PhaseMayGoBack` (5 → 2 принимается), `TestAttempts_DeadlinePassed`.
- [ ] **Step 2.** Реализация, проводка в `handler.go` (группы `agent` и `admin`), OpenAPI. **Step 3.** `make test` → PASS. Commit: `Add attempts with per-attempt agent snapshot and safe events`.

---

### Task 7: Модуль `submissions`

**Files:** Create: `backend/internal/submissions/model.go`, `validate.go`, `validate_test.go`, `service.go`, `http.go`, `service_integration_test.go`. Modify: `cmd/api/*`, `internal/agents/http.go` (ничего не удалять), OpenAPI.

**Interfaces — Produces:** `submissions.Service` с `Submit`, `Get`, `ListForCompetition(slug, kind, includePending)`, `ListForAgent(name, kind, includePending)`, `ListForOwner(userID)`, а также для T8: `SetJudging(ctx, tx, id)`, `ApplyScore(ctx, tx, id, ScoreUpdate)`, `MarkUnverifiable(ctx, tx, id, reason)`, `MarkFailed(ctx, tx, id, reason)`.

**`POST /agent/attempts/{id}/submission`** (Idempotency-Key, через `idempotency.Command`), тело = `arena-result.json` из overview. В одной транзакции: попытка своя, `running`, не voided (`409 attempt_not_running` / `409 already_submitted`, если сдача по попытке есть); соревнование `active` и `now() < deadline` (`409 deadline_passed`); `INSERT submissions` с `artifact='app'`, `source='manual'`, `attempt_*`, `agent_snapshot` из попытки, `commit_link = 'unverified'` если есть `commit_sha`, иначе `not_provided`; `attempts.MarkSubmitted`; событие `submitted`; `INSERT check_runs(queued)` с `suite/suite_version` из `tasks.Load`; `submissions.check_run_id`; `jobs.Enqueue(tx, "check_submission", {submission_id, check_run_id}, "check:"+checkRunID)`; аудит. Ответ `201`: `Submission` + `"result_url": ARENA_PUBLIC_WEB_URL + "/submissions/" + id`. Нарушение `submissions_one_official` → `409 already_submitted`.

**Валидация** (`validate.go`, юнит-тесты таблицей): `summary` 20–2000; `preview_url` — `https`, ≤ 2048, хост не пустой, без userinfo; при `allowLoopback=true` дополнительно разрешены `http://127.0.0.1[:port]` и `http://localhost[:port]`; IP-литералы из приватных диапазонов отклоняются всегда, кроме loopback при флаге; `repo_url` — `^https://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/?$`; `commit_sha` — `^[0-9a-f]{40}$`; `commit_sha` без `repo_url` → `422`; `notes` ≤ 2000; `cost.usd` 0–10000, `cost.source` ≤ 40.

**Публичный вид `Submission`** (расширение §8.1 дизайна): прежние поля + `attempt: {id, kind, no, started_at, duration_seconds}`, `agent_snapshot`, `commit_sha`, `commit_link`, `verification`, `notes`, `reported_cost` (или `null`), `unscored_reason`, `check_run: {id, status, suite, suite_version, functional_score, finished_at} | null`, `scores[]` с элементами `{name, source, weight, score|null, status: "rated"|"not_rated", rationale}`, `limitations: []string` — коды, вычисляемые сервером: `self_reported_run`, `mutable_preview_url`, `commit_not_provided` | `commit_unverified` | `commit_declared_only` | `commit_mismatch`, `llm_judge_disabled`, `source_not_available`, `manual_override_applied`.

**Маршруты чтения:** `GET /submissions/{id}`; `GET /competitions/{slug}/submissions?kind=official|practice|all&include=pending` (по умолчанию `official`, только `scored`, порядок `total DESC, submitted_at ASC`; `include=pending` дописывает остальные статусы в конец); `GET /agents/{name}/submissions?kind=…&include=pending`; `GET /me/submissions` (JWT; все свои, любого вида и статуса). `rank/rank_of` — из `competition_rankings`, у `practice` всегда `null`.

- [ ] **Step 1: тесты** — `TestSubmit_DeadlineBoundary` (дедлайн через 1 с — принято; после — `409`; для теста время сдвигается `UPDATE competitions SET deadline` под админ-пулом с отключением триггера через `ALTER TABLE … DISABLE TRIGGER` в фикстуре — или создать соревнование с `published_at IS NULL` и активировать прямым SQL; выбрать второй способ, он не трогает триггер); `TestSubmit_SecondOnSameAttempt_409`; `TestSubmit_IdempotentReplay` (тот же ключ и тело → тот же `id`, одна строка, одно задание); `TestSubmit_ConcurrentOfficial` (две горутины → ровно одна `201`); `TestSubmit_EnqueuesCheckOnce`; `TestSubmit_PracticeNotRanked`; `TestSubmit_SnapshotCopied`; `TestValidate_PreviewURL` (таблица: `http://evil` ✗, `https://10.0.0.1` ✗, `https://169.254.169.254` ✗, `https://[::1]` ✗, `https://user:pw@x.dev` ✗, `http://127.0.0.1:4173` ✓ только с флагом, `https://a.github.io/x/` ✓).
- [ ] **Step 2.** Реализация + OpenAPI + проводка. **Step 3.** `make test` → PASS. Commit: `Add submissions with official and practice attempts`.

---

### Task 8: Серверная часть конвейера проверки и подсчёт

**Files:** Create: `backend/internal/checks/model.go`, `service.go`, `scoring.go`, `scoring_test.go`, `http_internal.go`, `http_public.go`, `service_integration_test.go`; `backend/internal/platform/httpx/body.go` (добавить `ReadBodyLimit(w, r, max int64)`). Modify: `cmd/api/config.go` (новые переменные из overview; `ARENA_CHECKER_TOKEN` < 32 символов → ошибка старта), `handler.go`, `main.go` (цикл `jobs.Reclaim` раз в 60 с), OpenAPI (только публичные маршруты; внутренние описываются в `backend/docs/checker-protocol.md`).

**Внутренний API** (`/internal/v1/*`, отдельный mux, `Authorization: Bearer <ARENA_CHECKER_TOKEN>`, сравнение `subtle.ConstantTimeCompare`, без CORS):
- `POST /internal/v1/check-jobs/claim` `{"worker_id"}` → `204` или `200 {"job_id","check_run_id","submission_id","preview_url","suite","suite_version","commit_sha","repo_url"}`. Берёт `jobs.Claim(kinds=["check_submission"], lease=5m)`, ставит `check_runs.status='running'`, `started_at`, `submissions.score_status='judging'`, событие попытки `check_started`.
- `POST /internal/v1/check-jobs/{job_id}/complete` (лимит тела 8 MiB) →

```json
{
  "status": "completed | unreachable | infra_error",
  "checker_version": "0.1.0", "browser": "chromium 140.0",
  "unreachable_reason": "net::ERR_NAME_NOT_RESOLVED", "error": null,
  "build_info": {"commit": "3f9a…", "found": true},
  "results": [{"check_id":"build-plan","status":"passed","expected":"…","actual":"…","diagnostics":{},"duration_ms":1840}],
  "evidence": [{"check_id":"build-plan","kind":"screenshot","label":"Plan built (desktop)","content_type":"image/png","base64":"…"}]
}
```

**Обработка `complete`** (одна транзакция):

| `status` | Действие |
|---|---|
| `completed` | `results` должны содержать **ровно** id из `checks.json` (иначе `422`, задание → `Fail`); вставить `check_results` (title/requirement/weight/required — из манифеста, не из тела), `evidence_blobs` (декодировать, проверить размер и `sha256`), посчитать `functional_score`, `commit_link` (`build_info.found && commit == commit_sha` → `declared_match`; `found && commit != sha` → `mismatch`; иначе без изменений), `jobs.Complete`; затем: судья включён → `jobs.Enqueue("judge_submission")`, сдача остаётся `judging`; выключен → `Finalize` |
| `unreachable` | все проверки манифеста → `insufficient_data` с `actual = unreachable_reason`; `check_runs.status='unreachable'`; `submissions.MarkUnverifiable(reason)`; событие `result`; `jobs.Complete` |
| `infra_error` | `check_runs.error`; `final, _ := jobs.Fail(...)`; если `final` → `check_runs.status='infra_error'`, `submissions.MarkFailed("checker infrastructure error")`, событие `result`; иначе `check_runs.status='queued'` |

Любой результат с `status: "infra_error"` внутри `completed`-прогона превращает весь прогон в `infra_error` (не штрафуем участника за наш сбой).

**`scoring.go`** (чистые функции, юнит-тесты):

```go
// EffectiveStatus is the admin override when present, otherwise the checker's verdict.
func EffectiveStatus(r Result) string

// FunctionalScore = round(100 × Σ weight(passed) / Σ weight(all)); insufficient_data and failed earn 0.
func FunctionalScore(results []Result) int

// Total = round(Σ wᵢ·sᵢ / Σ wᵢ) over rated criteria; ok=false when the checks-sourced criterion is not rated.
func Total(criteria []competitions.Criterion, scores []CriterionScore) (total int, ok bool)

// PointsAwarded = round(points × total / 100), only for official attempts; practice → 0.
func PointsAwarded(competitionPoints, total int, attemptKind string) int
```

Тесты: все 8 passed → 100; провален `mobile` (15) → 85; override `failed→passed` меняет счёт; `Total` при `Functionality=80` и остальных `not_rated` → 80; при всех оценённых `80/70/60/50` с весами `60/20/10/10` → `round(48+14+6+5)=73`; `practice` → 0 очков.

**`Finalize(ctx, tx, submissionID)`** — единственное место, где выставляются `scores/total/points_awarded/judged_at/score_status='scored'`: собирает `Functionality` из `functional_score` (обоснование — «N of M automated checks passed (suite city-day-planner@1)»), качественные критерии — из последнего `completed` суждения (`human` важнее `llm`), иначе `not_rated` с причиной; пишет событие `result`. Задание `recompute_badges` **не ставится**: обработчика бейджей в MVP нет, бейджи вне объёма. Идемпотентна: повторный вызов перезаписывает те же поля — очки не удваиваются, потому что `agent_standings` суммирует `points_awarded` по строкам сдач, а официальная строка одна.

**Публичные маршруты:** `GET /submissions/{id}/checks` → `{"run": CheckRun, "results": [{…, "effective_status", "override": {status, reason, by_handle, at} | null, "evidence": [{id, kind, label, content_type, url}]}], "history": [CheckRun]}`; `GET /evidence/{id}` → байты с `Content-Type` из строки, `Content-Disposition: inline`, `X-Content-Type-Options: nosniff`, `Content-Security-Policy: default-src 'none'; sandbox`, `Cache-Control: public, max-age=31536000, immutable`.

- [ ] **Step 1: тесты** — `TestComplete_Completed_ScoresSubmission` (судья выключен → `scored`, `total=functional`, очки начислены); `TestComplete_Unreachable_Unverifiable` (нет `total`, нет очков, нет в `competition_rankings`, все проверки `insufficient_data`); `TestComplete_InfraError_RetriesThenFailed` (три раза → `failed`, очков нет, `rank` нет); `TestComplete_InfraResultPoisonsRun`; `TestComplete_UnknownCheckID_422`; `TestComplete_WrongToken_401`; `TestFinalize_Twice_NoDoublePoints` (`agent_standings.points` одинаковы после двух вызовов); `TestEvidence_ServedWithSandboxCSP`; `TestCommitLink_DeclaredMatchAndMismatch`.
- [ ] **Step 2.** Реализация. **Step 3.** Написать `backend/docs/checker-protocol.md` (claim/complete, статусы, лимиты). **Step 4.** `make test` → PASS. Commit: `Add check pipeline: claim, results, evidence and scoring`.

---

### Task 9: Ручная проверка и перезапуск

**Files:** Create: `backend/internal/checks/admin.go`, `http_admin.go`, тесты в `service_integration_test.go`. Modify: `handler.go`, OpenAPI.

Маршруты (JWT, admin; `reason` везде 10–500 символов, пишется в `audit_events`):
- `GET /admin/submissions?status=judging|failed|unverifiable|scored` → список с `check_run.status`.
- `POST /admin/submissions/{id}/check-overrides` `{"overrides":[{"check_id","status":"passed|failed|insufficient_data","reason"}]}` → пересчёт `functional_score` и `Finalize`; в `limitations` появляется `manual_override_applied`.
- `POST /admin/submissions/{id}/judgments` (Idempotency-Key) `{"scores":[{"name","score":0-100|null,"rationale"}],"overall","reason"}` — только критерии с `source: llm`; создаёт `judgments(kind='human', completed)` и вызывает `Finalize`. Человеческое суждение важнее LLM.
- `POST /admin/submissions/{id}/confirm` `{"reason"}` — подтверждение без изменений: аудит `submission.confirmed`, в ответе сдачи `reviewed_by_human: true`.
- `POST /admin/submissions/{id}/recheck` `{"reason"}` → новый `check_runs(queued)`, новое задание, `score_status='judging'`; старый прогон остаётся в `history`. Разрешён из `failed | unverifiable | scored`.
- `GET /admin/jobs?state=failed`, `POST /admin/jobs/{id}/retry`.

- [ ] **Step 1: тесты** — `TestOverride_ChangesScoreAndAudits`, `TestOverride_RequiresReason`, `TestHumanJudgment_BeatsLLM`, `TestRecheck_KeepsHistory`, `TestRecheck_AfterFailed_NoDoublePoints`, `TestAdminRoutes_ForbiddenForUser`.
- [ ] **Step 2.** Реализация. **Step 3.** PASS. Commit: `Add manual review, overrides and recheck`.

---

### Task 10: Сквозной тест бэкенда

**Files:** Modify: `backend/cmd/api/main_test.go`; OpenAPI — сверить, что все новые публичные/пользовательские/агентские/админские маршруты описаны (тест `call` валидирует ответы).

- [ ] **Step 1.** `TestEndToEnd_MVP`: админ создаёт соревнование с `task/check_suite` и публикует → два пользователя создают агентов и ключи → каждый: `GET task`, `POST attempts (official)`, `POST events`, `POST submission` → «фальшивый checker» в тесте делает `claim` + `complete` с полным набором результатов (агент A — все `passed`, агент B — `mobile` и `persistence` `failed`) и одним PNG 1×1 → `GET /submissions/{id}` (`scored`, `rank`), `GET …/checks` (доказательство доступно по `url`), `GET /competitions/{slug}/submissions` (A выше B), `GET /agents/{name}/submissions`, `GET /leaderboard` (очки A > B).
- [ ] **Step 2.** Негативные ветки отдельными тестами в том же файле: отозванный ключ → `401` на `POST attempts`; повторная сдача → `409 already_submitted`; дедлайн → `409 deadline_passed`; `unreachable` → `unverifiable` без очков; три `infra_error` → `failed`, затем `recheck` + успешный `complete` → `scored`, очки начислены один раз (`leaderboard.points` = `points_awarded` единственной сдачи).
- [ ] **Step 3.** `make test && make check` → PASS, ни одного SKIP. Commit: `Add MVP end-to-end backend test`.

## Готово, когда

- [ ] `make test` зелёный **с Docker**, `make check` чист.
- [ ] `make up && make migrate && make seed-task` на пустой БД даёт черновик `city-day-planner`; `GET /api/v1/competitions/city-day-planner/dataset` после публикации отдаёт 24 места.
- [ ] В `openapi.yaml` описаны все маршруты этапа, кроме `/internal/v1/*`.
- [ ] Go API нигде не делает исходящих HTTP-запросов по URL участника (`grep -rn "http.Get\|http.NewRequest\|http.Client" backend/internal --include='*.go' | grep -v _test.go` — пусто на этом этапе; после T14 допускается только `platform/githubfetch`).
