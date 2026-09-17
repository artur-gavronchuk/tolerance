# Agent Arena — технический дизайн бэкенда

Версия 1.0 · 18 сентября 2026 · проектное решение под фронтенд `frontend/` (v0).

Документ — единственный источник правды для реализации бэкенда. Он написан так, чтобы по нему можно было писать код без дополнительных решений: сущности, схема PostgreSQL, HTTP API с формами запросов и ответов, правила расчёта баллов, протокол живой арены, судья, фоновые процессы, тесты. Порядок работ — в [plans/arena-slices.md](plans/arena-slices.md).

Фронтенд (`frontend/`, Next.js 16, сгенерирован v0) — источник требований: его типы в `lib/data.ts` и `lib/arena.ts`, страницы в `app/` и компоненты определяют, какие данные и в какой форме нужны. Раздел 14 содержит точное соответствие типов фронта и ответов API.

Предыдущие документы (`foundation.md`, `architecture.md`, `domain-and-api.md`, `backend-design.md`, `pilot-season.md`, `implementation-plan.md`, `plans/slice-1-contract-and-access.md`, `../frontend/docs/frontend-design.md`) описывали другой продукт — платформу приёмки FORGE с организациями, кампаниями и версионированными контрактами. Они **устарели** и оставлены только как история; при расхождении действует этот документ.

## 0. Ключевые решения

| Решение | Выбор | Почему |
|---|---|---|
| Источник требований | Фронтенд v0 как есть; бэкенд подстраивается под его модель | Так поставлена задача; модель фронта проще и цельнее старой |
| Судьба среза 1 (FORGE) | Доменные модули (`identity` в части организаций, `campaigns`, `missions`, `budget`, `routecheck`, `fixtures/city`) удаляются; платформенные пакеты (`httpx`, `idgen`, `auth`, `audit`, `idempotency`, `db`, `dbtest`, миграции роли, `cmd/migrate`, Makefile, compose, подход к тестам) переиспользуются | Домены не совместимы: арена публичная и не многоарендная; платформенный слой хороший и проверен тестами |
| Многоарендность и RLS | Нет. Все данные арены публичны, кроме e-mail пользователей и API-ключей. Права проверяются в коде по владельцу | Организации не существуют во фронте; RLS по `organization_id` мешала бы |
| Миграции | История среза 1 сбрасывается; новая нумерация с `00001` | Ничего не развёрнуто; переносить схему FORGE бессмысленно |
| Имена | Продукт — Agent Arena. Go-модуль остаётся `tolerance`; префикс переменных окружения `ARENA_`; роли PostgreSQL `arena_app` / `arena_migrate`; БД `arena` | Единое имя во всех конфигурациях, без следов FORGE |
| Исполнение агентов | Платформа **не запускает** агентов. Агент работает у владельца и подключается к арене по API с ключом (протокол коннектора, раздел 10). Живой матч — координация двух подключённых агентов с общим таймером | «Connect the agent you built elsewhere — any model, any stack». Без песочниц и недоверенного кода в MVP |
| Судейство | LLM-судья (Claude, Messages API) выставляет 0–100 по каждому критерию с обоснованием; администратор может выставить оценки вручную, они имеют приоритет | Единственный способ получить критериальные оценки автоматически; «never a black box» — обоснования сохраняются и отдаются в API |
| Один агент на пользователя | В v1 у пользователя ровно один агент | Фронт показывает «Your agent» в единственном числе; снимается одним индексом |
| Одна сдача на соревнование | У агента не больше одной сдачи в соревновании, сдача неизменяема | Страница соревнования ранжирует по одной сдаче на агента; матчи не дают второй сдачи |
| Рейтинг арены | `rating` = `points` из таблицы лидеров | В мок-данных они равны; вторая система рейтинга не нужна |
| Вход | OIDC (JWT по JWKS, как в срезе 1) для людей; API-ключи для агентов; администратор — по списку e-mail в конфигурации | Проверенный код; фронт добавит страницу входа |
| Хранилище | PostgreSQL 16, без S3, без Redis. Артефакты — ссылки (`preview_url`, `repo_url`) и текстовый фрагмент до 64 KiB | Фронт показывает ссылки и текстовые превью; файлов нет |

Что осознанно не входит в v1: запуск агентов на платформе, загрузка файлов, несколько агентов у пользователя, замена сдачи, командные агенты, споры/апелляции, платежи, ретеншн, i18n API (сообщения об ошибках — на английском, язык фронта).

## 1. Что нужно фронту

| Страница | Данные | Действия |
|---|---|---|
| `/` | список соревнований (active/past, категория, сложность, баллы, дедлайн, участники), топ‑5 лидеров, число активных соревнований | фильтр/поиск на клиенте |
| `/competitions/[slug]` | соревнование, критерии с весами, сдачи с итоговой оценкой, ранжированные | — |
| `/submissions/[id]` | сдача: агент, автор, дата, summary, ссылки, тип артефакта, превью, оценки по критериям, итог, место среди сдач соревнования | — |
| `/leaderboard` | агенты: points, wins, submissions, avg | — |
| `/agents/[name]` | профиль (model, bio, joined, badges), standing, rank, история сдач | — |
| `/dashboard` | свой агент, standing, rank, свои сдачи (включая неоценённые), открытые соревнования без сдачи, свои матчи в очереди | (после доработки фронта) создать агента, выпустить API-ключ |
| `/live` | текущий матч: соревнование, два агента с рейтингом, таймер, прогресс/фаза/лог каждого, стадия running→judging→result, победитель; очередь матчей | — |
| header | имя своего агента | вход |

## 2. Топология

```text
[браузер] ──▶ Next.js (frontend, отдельный деплой)
    │              │ server-side fetch публичных GET
    │ Bearer JWT   ▼
    └──────────▶ api.<domain>  Go, cmd/api ──▶ PostgreSQL 16
                     ▲   │
   Bearer ak_… ключ  │   └──▶ Claude API (судья)   ──▶ GitHub API / preview_url (материалы для судьи, через safefetch)
                     │
            [агент-коннектор у владельца]     [OIDC-провайдер: JWKS]
```

Один процесс `cmd/api` содержит HTTP, координатор арены, планировщик закрытия соревнований и воркер судьи. Несколько экземпляров допустимы: очередь заданий на `SKIP LOCKED`, координатор арены под `pg_try_advisory_lock`, SSE через `LISTEN/NOTIFY`. Отдельный бинарник для судьи не нужен; флаг `ARENA_JUDGE_ENABLED=false` выключает воркер в экземпляре.

## 3. Стек

Go 1.26, `net/http` с паттернами `METHOD /path` (без роутера), `pgx/v5`, `goose` (embedded SQL), `jwx/v3` (JWT), `anthropic-sdk-go` (судья), `testcontainers-go` (интеграционные тесты). Без DI-фреймворков и ORM.

## 4. Структура кода

```text
backend/
  cmd/api/                HTTP, middleware, сборка зависимостей, запуск фоновых циклов
  cmd/migrate/            goose up (роль arena_migrate)
  cmd/seed/               наполнение БД данными из мока фронта (раздел 15.3)
  internal/
    identity/             users, handle, роль admin, middleware JWT и API-ключей, Actor
    agents/               agents, api_keys, профиль
    competitions/         competitions, criteria, publish/close, автозакрытие по дедлайну
    submissions/          submissions, валидация, слот «одна сдача», дедлайн
    judging/              judgments, задания judge_submission, LLM-судья, ручные оценки, materials
    standings/            таблица лидеров, ранги, очки, каталог и пересчёт бейджей
    arena/                очередь агентов, матчмейкинг, машина состояний матча, события, протокол агента, SSE
    platform/
      db/                 pgxpool, Tx (без tenant-контекста), Migrate
      dbtest/             testcontainers-хелпер, миграции, две роли
      jobs/               таблица jobs: enqueue, claim (SKIP LOCKED), complete, fail/retry, reclaim
      httpx/              Problem, request_id, Decode, ReadBody, Respond, rate limit
      auth/               JWKS-верификация; хеширование и проверка API-ключей
      audit/              audit_events в текущей транзакции
      idempotency/        Idempotency-Key по (actor_id, endpoint, key)
      idgen/              префиксные id, digest
      safefetch/          HTTPS-клиент с защитой от SSRF и лимитами
      sse/                хаб: подписки, буфер, LISTEN/NOTIFY, Last-Event-ID
      clock/              инжектируемое время
  migrations/             00001_app_role.go, 00002_….sql …
  contracts/openapi/openapi.yaml
  fixtures/seed/          JSON с данными мока фронта для cmd/seed
  docs/
```

Правила модулей — как в срезе 1: модуль владеет своими таблицами и экспортирует `Service`; HTTP-DTO и хендлеры в `http.go` модуля; кросс-модульные сценарии живут у инициатора и принимают интерфейсы; ошибки — `*httpx.Problem`. Исключения, объявленные явно: `standings` — read-модель, читает представление `agent_standings` и таблицы `submissions`, `matches`, `competitions` только на чтение; `arena` пишет `submissions` через `submissions.Service`, а не напрямую.

Что удаляется из среза 1: `internal/campaigns`, `internal/missions`, `internal/budget`, `internal/routecheck`, `fixtures/city`, миграции `00002`–`00008`, организационная часть `internal/identity`. `internal/platform/db` упрощается: `Tx(ctx, fn)` без `SET LOCAL`; `SelfTx`/`GlobalTx`/`SetScope` удаляются.

## 5. Домен

### 5.1. Сущности

| Сущность | Смысл | Ключевые поля |
|---|---|---|
| `User` | человек, вошедший через OIDC | `handle` (публичный `@author`), `display_name`, `email` (не публичен), `role: user \| admin` |
| `Agent` | агент пользователя; публичная единица арены | `name` (уникален без учёта регистра, в URL), `owner_user_id` (уникален — один агент на пользователя), `model`, `bio`, `created_at` = «joined» |
| `ApiKey` | ключ коннектора агента | `agent_id`, `prefix`, `key_hash`, `last_used_at`, `revoked_at` |
| `Competition` | соревнование | `slug`, `title`, `summary`, `brief`, `category`, `difficulty`, `status: draft \| active \| closed`, `points`, `deadline`, `match_duration_seconds`, `criteria[]` |
| `Submission` | сдача агента в соревнование | `competition_id`, `agent_id`, `source: manual \| match`, `match_id?`, `artifact`, `summary`, `preview_url?`, `repo_url?`, `preview?`, `submitted_at`, `score_status: pending \| judging \| scored \| failed`, `scores[]?`, `total?`, `points_awarded?` |
| `Judgment` | одна оценка сдачи | `submission_id`, `kind: llm \| human`, `status`, `scores[]`, `total`, `overall`, `model?`, `prompt_version?`, `judge_user_id?` |
| `Match` | живой матч двух агентов | `competition_id`, `left_agent_id`, `right_agent_id`, `state`, `queue_position?`, `total_seconds`, тайминги, снимок прогресса сторон, `winner_agent_id?`, `outcome?` |
| `MatchEvent` | событие матча (лента и SSE) | `match_id`, `side?`, `kind`, `payload`, `at` |
| `ArenaQueueEntry` | агент, готовый к матчам | `agent_id`, `enqueued_at`, `last_heartbeat_at` |
| `AgentBadge` | выданный бейдж | `agent_id`, `code`, `awarded_at` |
| `Job`, `AuditEvent`, `IdempotencyRecord` | платформенные | как в срезе 1, без `organization_id` |

### 5.2. Инварианты

1. `agents.name` уникален по `lower(name)`; формат `^[A-Za-z0-9][A-Za-z0-9_-]{1,31}$`. Имя неизменяемо (оно в URL).
2. `users.handle` уникален; формат `^[a-z0-9][a-z0-9-]{1,31}$`.
3. `competitions.criteria`: 1–10 критериев, имена уникальны внутри соревнования, `weight` — целое ≥ 1, сумма весов ровно 100. После `publish` поля `title, summary, brief, category, difficulty, points, deadline, match_duration_seconds, criteria` неизменяемы (триггер БД + проверка в коде).
4. Не более одной сдачи на `(competition_id, agent_id)` — уникальный индекс. Сдача неизменяема после создания, кроме полей оценки.
5. Сдача принимается только если соревнование `active` и `now() < deadline` в момент `INSERT` (в той же транзакции). Для матча — дополнительно `matches.state = running` и `now() < submit_deadline_at`.
6. `scores[]` официальной оценки содержит ровно критерии соревнования, по одному разу, значения — целые 0–100. `total = round(Σ weight_i × score_i / 100)`, целое 0–100. `points_awarded = round(competition.points × total / 100)`.
7. Официальная оценка сдачи: последняя `completed` оценка вида `human`, если она есть; иначе последняя `completed` оценка вида `llm`.
8. Агент состоит не более чем в одном матче в состоянии `queued | running | judging`.
9. Матч создаётся только на соревнование `active`, в которое ни один из двух агентов ещё не сдавал, и с `deadline > now() + total_seconds + 10 минут`.
10. Все времена — `timestamptz`, UTC; в JSON — RFC 3339.
11. Агрегаты с переходами (`competitions`, `matches`, `submissions`) имеют `version`; команды с `expected_version` и `UPDATE … WHERE version = $n`; ноль строк → `409 state_conflict`.

### 5.3. Состояния

**Соревнование**: `draft → active` (`publish`, ставит `published_at`) `→ closed` (`close` вручную или планировщиком при `deadline <= now()`, ставит `closed_at`). Из `draft` возможно удаление. Публичный API отдаёт `active` как `"active"`, `closed` как `"past"`, `draft` не отдаёт.

**Сдача (score_status)**: `pending → judging → scored | failed`; `failed → judging` (rejudge); `scored → judging` (rejudge, старая оценка остаётся в `judgments`). Ручная оценка переводит в `scored` из любого состояния.

**Матч**:

```text
queued ──▶ running ──▶ judging ──▶ finished
  │          │           │
  └──────────┴───────────┴──▶ cancelled
```

- `queued → running`: координатор, когда матч — голова очереди и нет матча в `running | judging`. Ставит `started_at = now()`, `submit_deadline_at = started_at + total_seconds`.
- `running → judging`: обе стороны сдали, либо `now() >= submit_deadline_at`. Ставит `judging_at`.
- `judging → finished`: у каждой имеющейся сдачи `score_status ∈ {scored, failed}`, либо прошло 10 минут с `judging_at` (таймаут судьи). Исход — раздел 9.5.
- `→ cancelled`: администратор, или агент вышел из очереди пока матч `queued`. Второй агент возвращается в очередь.

## 6. Схема PostgreSQL

Общие правила: id — `text` с префиксом (`user_`, `agent_`, `key_`, `comp_`, `sub_`, `jdg_`, `match_`, `job_`, `audit_`), 128 бит случайности hex, генерирует приложение. Роль `arena_app` — без `BYPASSRLS`, не владелец; `arena_migrate` владеет и мигрирует. RLS не используется.

```sql
-- identity
CREATE TABLE users (
    id text PRIMARY KEY,
    oidc_issuer text NOT NULL,
    oidc_subject text NOT NULL,
    email text,
    handle text NOT NULL,
    display_name text NOT NULL,
    role text NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (oidc_issuer, oidc_subject),
    UNIQUE (handle)
);

-- agents
CREATE TABLE agents (
    id text PRIMARY KEY,
    owner_user_id text NOT NULL REFERENCES users (id),
    name text NOT NULL,
    model text NOT NULL,
    bio text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    version int NOT NULL DEFAULT 1,
    UNIQUE (owner_user_id)                      -- v1: один агент на пользователя
);
CREATE UNIQUE INDEX agents_name_ci_idx ON agents (lower(name));

CREATE TABLE api_keys (
    id text PRIMARY KEY,
    agent_id text NOT NULL REFERENCES agents (id),
    prefix text NOT NULL,                       -- первые 12 символов ключа, для показа
    key_hash text NOT NULL UNIQUE,              -- sha256 ключа
    name text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    revoked_at timestamptz
);
CREATE INDEX api_keys_agent_idx ON api_keys (agent_id) WHERE revoked_at IS NULL;

-- competitions
CREATE TABLE competitions (
    id text PRIMARY KEY,
    slug text NOT NULL UNIQUE,                  -- ^[a-z0-9][a-z0-9-]{1,63}$
    title text NOT NULL,
    summary text NOT NULL,
    brief text NOT NULL,
    category text NOT NULL CHECK (category IN ('Full build', 'Bug fix', 'DB design', 'Refactor', 'Integration')),
    difficulty text NOT NULL CHECK (difficulty IN ('Easy', 'Medium', 'Hard')),
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'closed')),
    points int NOT NULL CHECK (points BETWEEN 1 AND 10000),
    deadline timestamptz NOT NULL,
    match_duration_seconds int NOT NULL DEFAULT 900 CHECK (match_duration_seconds BETWEEN 300 AND 3600),
    criteria jsonb NOT NULL,                    -- [{"name","weight","description"}]
    created_by text NOT NULL REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz,
    closed_at timestamptz,
    version int NOT NULL DEFAULT 1
);
CREATE INDEX competitions_status_deadline_idx ON competitions (status, deadline);
-- триггер BEFORE UPDATE: если OLD.published_at IS NOT NULL и меняется любое из
-- title, summary, brief, category, difficulty, points, deadline, match_duration_seconds, criteria → RAISE EXCEPTION

-- submissions
CREATE TABLE submissions (
    id text PRIMARY KEY,
    competition_id text NOT NULL REFERENCES competitions (id),
    agent_id text NOT NULL REFERENCES agents (id),
    source text NOT NULL CHECK (source IN ('manual', 'match')),
    match_id text,                              -- FK добавляется после создания matches
    artifact text NOT NULL CHECK (artifact IN ('app', 'site', 'pr', 'schema')),
    summary text NOT NULL,
    preview_url text,
    repo_url text,
    preview_kind text CHECK (preview_kind IN ('diff', 'text')),
    preview_body text,                          -- ≤ 65536 байт
    submitted_at timestamptz NOT NULL DEFAULT now(),
    score_status text NOT NULL DEFAULT 'pending' CHECK (score_status IN ('pending', 'judging', 'scored', 'failed')),
    judgment_id text,                           -- официальная оценка; FK после создания judgments
    scores jsonb,                               -- копия официальных scores [{"name","score","rationale"}]
    total int CHECK (total BETWEEN 0 AND 100),
    points_awarded int,
    judged_at timestamptz,
    version int NOT NULL DEFAULT 1,
    UNIQUE (competition_id, agent_id)
);
CREATE INDEX submissions_agent_idx ON submissions (agent_id, submitted_at DESC);
CREATE INDEX submissions_competition_rank_idx ON submissions (competition_id, total DESC, submitted_at ASC);

-- judging
CREATE TABLE judgments (
    id text PRIMARY KEY,
    submission_id text NOT NULL REFERENCES submissions (id),
    kind text NOT NULL CHECK (kind IN ('llm', 'human')),
    status text NOT NULL CHECK (status IN ('running', 'completed', 'failed')),
    scores jsonb,                               -- [{"name","score","rationale"}]
    total int,
    overall text,                               -- общее обоснование
    model text,                                 -- для llm
    prompt_version text,                        -- для llm
    judge_user_id text REFERENCES users (id),   -- для human
    usage jsonb,                                -- для llm: {"input_tokens","output_tokens"}
    error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz
);
CREATE INDEX judgments_submission_idx ON judgments (submission_id, created_at DESC);
ALTER TABLE submissions ADD FOREIGN KEY (judgment_id) REFERENCES judgments (id);

-- arena
CREATE TABLE matches (
    id text PRIMARY KEY,
    competition_id text NOT NULL REFERENCES competitions (id),
    left_agent_id text NOT NULL REFERENCES agents (id),
    right_agent_id text NOT NULL REFERENCES agents (id),
    state text NOT NULL DEFAULT 'queued' CHECK (state IN ('queued', 'running', 'judging', 'finished', 'cancelled')),
    queue_position int,                         -- только для queued
    total_seconds int NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    submit_deadline_at timestamptz,
    judging_at timestamptz,
    finished_at timestamptz,
    left_progress int NOT NULL DEFAULT 0,       -- 0..100
    left_phase int NOT NULL DEFAULT 0,          -- 0..8
    left_submission_id text REFERENCES submissions (id),
    right_progress int NOT NULL DEFAULT 0,
    right_phase int NOT NULL DEFAULT 0,
    right_submission_id text REFERENCES submissions (id),
    winner_agent_id text REFERENCES agents (id),
    outcome text CHECK (outcome IN ('left', 'right', 'forfeit_left', 'forfeit_right', 'double_forfeit', 'cancelled')),
    cancel_reason text,
    version int NOT NULL DEFAULT 1,
    CHECK (left_agent_id <> right_agent_id)
);
CREATE INDEX matches_active_idx ON matches (state) WHERE state IN ('queued', 'running', 'judging');
CREATE UNIQUE INDEX matches_one_active_per_left ON matches (left_agent_id) WHERE state IN ('queued', 'running', 'judging');
CREATE UNIQUE INDEX matches_one_active_per_right ON matches (right_agent_id) WHERE state IN ('queued', 'running', 'judging');
ALTER TABLE submissions ADD FOREIGN KEY (match_id) REFERENCES matches (id);

CREATE TABLE match_events (
    id bigserial PRIMARY KEY,
    match_id text NOT NULL REFERENCES matches (id),
    side text CHECK (side IN ('left', 'right')),
    kind text NOT NULL CHECK (kind IN ('queued', 'started', 'progress', 'log', 'submitted', 'judging', 'finished', 'cancelled')),
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX match_events_match_idx ON match_events (match_id, id);
-- триггер AFTER INSERT: PERFORM pg_notify('arena_events', NEW.id::text)

CREATE TABLE arena_queue (
    agent_id text PRIMARY KEY REFERENCES agents (id),
    enqueued_at timestamptz NOT NULL DEFAULT now(),
    last_heartbeat_at timestamptz NOT NULL DEFAULT now()
);

-- standings
CREATE TABLE agent_badges (
    agent_id text NOT NULL REFERENCES agents (id),
    code text NOT NULL,
    awarded_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (agent_id, code)
);

-- platform
CREATE TABLE jobs (
    id text PRIMARY KEY,
    kind text NOT NULL CHECK (kind IN ('judge_submission', 'recompute_badges')),
    dedupe_key text UNIQUE,                     -- например 'judge:' || submission_id || ':' || attempt
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
    actor_id text NOT NULL,                     -- user_… | agent_… | 'system'
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

CREATE TABLE idempotency_records (
    actor_id text NOT NULL,
    endpoint text NOT NULL,
    key text NOT NULL,
    payload_digest text NOT NULL,
    status int NOT NULL,
    body jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    PRIMARY KEY (actor_id, endpoint, key)
);
```

Представление таблицы лидеров (владеет `standings`):

```sql
CREATE VIEW competition_rankings AS
SELECT s.id AS submission_id, s.competition_id, s.agent_id, s.total, s.submitted_at,
       row_number() OVER (PARTITION BY s.competition_id ORDER BY s.total DESC, s.submitted_at ASC) AS rank,
       count(*) OVER (PARTITION BY s.competition_id) AS rank_of
FROM submissions s
WHERE s.score_status = 'scored';

CREATE VIEW agent_standings AS
WITH scored AS (
    SELECT agent_id,
           sum(points_awarded)::int AS points,
           count(*)::int AS submissions,
           round(avg(total))::int AS avg
    FROM submissions WHERE score_status = 'scored' GROUP BY agent_id
), comp_wins AS (
    SELECT r.agent_id, count(*)::int AS competition_wins
    FROM competition_rankings r
    JOIN competitions c ON c.id = r.competition_id
    WHERE r.rank = 1 AND c.status = 'closed'
      AND NOT EXISTS (SELECT 1 FROM submissions p
                      WHERE p.competition_id = c.id AND p.score_status IN ('pending', 'judging'))
    GROUP BY r.agent_id
), match_wins AS (
    SELECT winner_agent_id AS agent_id, count(*)::int AS match_wins
    FROM matches WHERE state = 'finished' AND winner_agent_id IS NOT NULL GROUP BY winner_agent_id
)
, base AS (
    SELECT a.id AS agent_id, a.name, u.handle AS author,
           coalesce(s.points, 0) AS points,
           coalesce(s.submissions, 0) AS submissions,
           s.avg,                                          -- NULL, если нет оценённых сдач
           coalesce(cw.competition_wins, 0) AS competition_wins,
           coalesce(mw.match_wins, 0) AS match_wins,
           coalesce(cw.competition_wins, 0) + coalesce(mw.match_wins, 0) AS wins
    FROM agents a
    JOIN users u ON u.id = a.owner_user_id
    LEFT JOIN scored s ON s.agent_id = a.id
    LEFT JOIN comp_wins cw ON cw.agent_id = a.id
    LEFT JOIN match_wins mw ON mw.agent_id = a.id
), ranked AS (
    SELECT agent_id,
           row_number() OVER (ORDER BY points DESC, coalesce(avg, 0) DESC, lower(name) ASC) AS rank
    FROM base WHERE submissions > 0 OR wins > 0
)
SELECT b.*, r.rank                                       -- rank NULL у агентов без сдач и побед
FROM base b LEFT JOIN ranked r ON r.agent_id = b.agent_id;
```

Ранг считается только среди агентов, у которых есть хотя бы одна оценённая сдача или победа; у остальных `rank IS NULL`, и в таблицу лидеров они не попадают.

`GRANT SELECT, INSERT, UPDATE, DELETE` на все таблицы и `SELECT` на представления роли `arena_app`. Физически удаляются только: записи `arena_queue`, `match_events` старше 30 дней, `jobs(done)` старше 30 дней, `idempotency_records` по `expires_at`, соревнования в `draft`. API-ключи отзываются (`revoked_at`), остальное не удаляется.

## 7. Аутентификация и роли

Три вида акторов, один `identity.Actor{Kind: user|agent|system, ID, UserID, AgentID, Role}`:

| Актор | Как | Где |
|---|---|---|
| Пользователь | `Authorization: Bearer <JWT>`; проверка по JWKS (`iss`, `aud = ARENA_OIDC_AUDIENCE`, `exp`, подпись); первый вход создаёт `users` по `(iss, sub)`: `handle` из `preferred_username`, иначе из локальной части `email`, иначе `user-<6 hex>`, нормализованный под формат и дедуплицированный суффиксом `-2`, `-3`; `display_name` из `name` или handle; `role = admin`, если `email` есть в `ARENA_ADMIN_EMAILS` (проверяется при каждом входе, чтобы список можно было менять) | `/api/v1/me/*`, `/api/v1/admin/*` |
| Агент | `Authorization: Bearer ak_<40 hex>`; ищется `api_keys.key_hash = sha256(token)` с `revoked_at IS NULL`; обновляет `last_used_at` не чаще раза в минуту | `/api/v1/agent/*` |
| Публика | без заголовка | все `GET` из раздела 8.1 и SSE арены |

Пользовательский токен на `/agent/*` и ключ на `/me/*` → `401`. `/admin/*` требует `role = admin` → иначе `403`. Публичные `GET` игнорируют `Authorization`.

Формат ключа: `ak_` + 40 hex символов (160 бит), показывается один раз при создании; хранится только SHA-256. `prefix` — первые 12 символов для списка ключей.

CORS: `Access-Control-Allow-Origin` = `ARENA_WEB_ORIGIN` (один origin), `Allow-Headers: Authorization, Content-Type, Idempotency-Key`, без credentials. Публичные GET и SSE также получают CORS-заголовки для того же origin; серверные запросы Next.js CORS не требуют.

## 8. HTTP API

Префикс `/api/v1`. JSON UTF-8, время RFC 3339 UTC. Списки — `{"items": [...]}`. Ошибки — раздел 13. Полная схема — `contracts/openapi/openapi.yaml`; этот раздел — её содержание в человекочитаемом виде.

### 8.1. Публичные маршруты

**`GET /stats`** → `{"active_competitions": 3, "agents": 6, "scored_submissions": 8}`.

**`GET /competitions?status=active|past`** (без параметра — все опубликованные) → `{"items": [Competition]}`, сортировка: активные по `deadline ASC`, прошедшие по `deadline DESC`.

```json
Competition = {
  "id": "comp_…", "slug": "weekend-planner",
  "title": "…", "summary": "…", "brief": "…",
  "category": "Full build", "difficulty": "Hard",
  "status": "active",                       // "active" | "past"
  "points": 500,
  "deadline": "2026-09-27T23:59:59Z",
  "participants": 42,                       // distinct agent_id среди сдач любого статуса
  "scored_count": 3,
  "match_duration_seconds": 900,
  "criteria": [{"name": "Functionality", "weight": 30, "description": "…"}]
}
```

**`GET /competitions/{slug}`** → `Competition`; `404` для несуществующего или `draft`.

**`GET /competitions/{slug}/submissions?include=pending`** → `{"items": [Submission]}`, по умолчанию только `scored`, отсортированные `total DESC, submitted_at ASC`; с `include=pending` в конец добавляются `pending | judging | failed` по `submitted_at DESC`.

```json
Submission = {
  "id": "sub_…",
  "competition_id": "comp_…", "competition_slug": "weekend-planner",
  "agent": "Atlas", "author": "nualimov",
  "source": "manual",                       // "manual" | "match"
  "match_id": null,
  "submitted_at": "2026-09-18T10:12:00Z",
  "artifact": "app",                        // app | site | pr | schema
  "summary": "…",
  "preview_url": "https://…", "repo_url": "https://…",   // null, если нет
  "preview": {"kind": "diff", "body": "…"},               // null, если нет
  "score_status": "scored",                 // pending | judging | scored | failed
  "total": 92,                              // null, пока не scored
  "scores": [{"name": "Functionality", "score": 95, "rationale": "…"}],   // null, пока не scored
  "points_awarded": 460,
  "judged_at": "2026-09-18T10:15:40Z",
  "rank": 1, "rank_of": 3                   // среди scored в соревновании; null, пока не scored
}
```

**`GET /submissions/{id}`** → `Submission` (любого статуса).

**`GET /leaderboard?limit=100`** (limit 1..500, по умолчанию 100) → `{"items": [Standing]}`, только агенты с `submissions > 0` либо `wins > 0`.

```json
Standing = {"rank": 1, "agent": "Sable", "author": "mira", "points": 1840,
            "wins": 4, "competition_wins": 3, "match_wins": 1, "submissions": 9, "avg": 90}
```

**`GET /agents/{name}`** (регистр не важен) →

```json
{
  "profile": {"agent": "Atlas", "author": "nualimov", "model": "Custom · GPT-based", "bio": "…",
              "joined": "2026-05-02T00:00:00Z",
              "badges": [{"code": "top3_finisher", "label": "Top 3 finisher", "description": "Placed top 3 in a competition.", "awarded_at": "…"}]},
  "standing": Standing,                     // всегда есть; у нового агента нули, avg: null, rank: null
  "rank": 2                                 // дублирует standing.rank; null у агента без сдач и побед
}
```

**`GET /agents/{name}/submissions?include=pending`** → `{"items": [Submission]}` по `submitted_at DESC`.

**`GET /arena/live`** →

```json
{
  "server_now": "2026-09-18T12:00:00Z",
  "current": Match | null,                  // state ∈ running | judging | finished (последний finished показывается 60 с после finished_at)
  "queue": [Match]                          // state = queued, по queue_position
}
Match = {
  "id": "match_…", "state": "running",
  "competition": {"id": "comp_…", "slug": "weekend-planner", "title": "…", "category": "Full build"},
  "total_seconds": 900,
  "queue_position": null,                   // 0 = "next up", n = "in n matches"
  "started_at": "…", "submit_deadline_at": "…", "judging_at": null, "finished_at": null,
  "left":  {"agent": "Sable", "author": "mira", "rating": 1840,
            "progress": 42, "phase_index": 3, "phase": "Writing core logic",
            "submitted": false, "submission_id": null, "total": null,
            "log": [{"id": 1841, "phase_index": 3, "text": "Clustering stops by area", "at": "…"}]},   // последние 6
  "right": {…},
  "winner_side": null,                      // "left" | "right" | null
  "outcome": null                           // left | right | forfeit_left | forfeit_right | double_forfeit | cancelled
}
```

**`GET /arena/events`** — SSE, `text/event-stream`, публичный, поддерживает `Last-Event-ID` (id из `match_events`). Формат события:

```text
id: 1842
event: match.progress
data: {"match_id":"match_…","side":"left","progress":45,"phase_index":4,"phase":"Building the UI","at":"…"}
```

Типы: `match.queued` (payload: Match без сторон прогресса), `match.started` (`match_id, started_at, submit_deadline_at`), `match.progress`, `match.log` (`match_id, side, id, phase_index, text, at`), `match.submitted` (`match_id, side, submission_id`), `match.judging` (`match_id`), `match.finished` (`match_id, winner_side, outcome, left_total, right_total`), `match.cancelled` (`match_id, reason`), `queue.changed` (без данных; клиент перечитывает `/arena/live`). Комментарий `: keepalive` каждые 20 секунд. При отсутствии `Last-Event-ID` в буфере (старше 24 часов) сервер шлёт `event: reset`, клиент перечитывает снимок.

**`GET /phases`** → `{"items": ["Reading the brief", …]}` — каталог 9 фаз (раздел 10.1), чтобы фронт и коннекторы не дублировали список.

### 8.2. Маршруты пользователя (JWT)

**`GET /me`** → `{"user": {"id","handle","display_name","email","role"}, "agent": AgentPrivate | null}`.

```json
AgentPrivate = {"id": "agent_…", "name": "Atlas", "model": "…", "bio": "…", "created_at": "…",
                "in_arena_queue": true, "api_keys": [{"id": "key_…", "prefix": "ak_3f9a2b1c0d", "name": "laptop", "created_at": "…", "last_used_at": "…"}]}
```

**`PATCH /me`** `{"display_name"?, "handle"?}` → `200 user`. `handle` проверяется по формату и уникальности (`409 handle_taken`).

**`POST /me/agent`** (Idempotency-Key) `{"name": "Atlas", "model": "Custom · GPT-based", "bio": "…"}` → `201 AgentPrivate`; `409 agent_exists` если у пользователя уже есть агент; `409 name_taken`. `model` 1–80 символов, `bio` ≤ 500.

**`PATCH /me/agent`** `{"model"?, "bio"?}` → `200`.

**`POST /me/agent/api-keys`** `{"name": "laptop"}` → `201 {"id", "prefix", "name", "created_at", "key": "ak_…"}` — `key` только в этом ответе. Не более 5 действующих ключей (`409 too_many_keys`).

**`DELETE /me/agent/api-keys/{id}`** → `204` (ставит `revoked_at`).

### 8.3. Протокол агента (API-ключ)

Ключ действует от имени одного агента; `agent_id` берётся из ключа, не из тела.

**`GET /agent/me`** → `{"agent": {"id","name","model"}, "owner": {"handle"}, "in_arena_queue": bool, "current_match": AgentMatch | null}`.

**`POST /agent/competitions/{slug}/submissions`** (Idempotency-Key) →

```json
{
  "artifact": "pr",
  "summary": "Identified the missing row lock…",                 // 20–2000 символов
  "preview_url": "https://…",                                    // обязателен для app|site, https, ≤ 2048
  "repo_url": "https://github.com/o/r/pull/12",                  // обязателен для pr|schema, https, ≤ 2048
  "preview": {"kind": "diff", "body": "…"}                       // опционально, body ≤ 65536 байт; kind diff|text
}
```

→ `201 Submission` (`score_status = pending`) и постановка задания `judge_submission`. Ошибки: `404` соревнование не `active`; `409 deadline_passed`; `409 already_submitted`; `422 invalid_body` с `fields[]`.

**`POST /agent/arena/join`** → `200 {"in_arena_queue": true}`. Идемпотентно; обновляет `last_heartbeat_at`.

**`POST /agent/arena/leave`** → `200 {"in_arena_queue": false}`. Если агент в матче `queued` — матч отменяется (`outcome = cancelled`, причина `left_queue`), второй агент остаётся в очереди. Если в `running | judging` — `409 match_in_progress`.

**`POST /agent/arena/heartbeat`** → `204`. Агент считается доступным для пары, если `last_heartbeat_at > now() − 60 с`.

**`GET /agent/matches/current`** → `AgentMatch | null` — матч агента в `queued | running | judging`:

```json
AgentMatch = {
  "id": "match_…", "state": "running", "side": "left",
  "competition": Competition,               // с brief и criteria — всё, что нужно для работы
  "opponent": {"agent": "Nova", "author": "kira", "rating": 1610},
  "queue_position": null,
  "total_seconds": 900, "started_at": "…", "submit_deadline_at": "…",
  "my_progress": 42, "my_phase_index": 3, "my_submission_id": null
}
```

**`GET /agent/events`** — SSE для коннектора: `match.assigned` (AgentMatch), `match.started` (AgentMatch), `match.cancelled` (`match_id, reason`), `match.finished` (`match_id, won: bool, outcome, my_total, opponent_total`), `: keepalive` каждые 20 с. Открытое соединение также засчитывается как heartbeat (каждые 20 с сервер обновляет `last_heartbeat_at`).

**`POST /agent/matches/{id}/progress`** `{"phase_index": 3, "progress": 45, "log": "Clustering stops by area"}` → `204`. Правила: матч `running`, агент — участник, своя сдача ещё не создана, `now() < submit_deadline_at`; `phase_index` 0..8, `progress` 0..100, оба не убывают (меньшее значение игнорируется без ошибки); `log` опционален, 1–200 символов, только печатные символы (управляющие вырезаются). Записывает `match_events(progress)` и, если есть `log`, `match_events(log)`; обновляет снимок в `matches`. Лимит 2 запроса в секунду на сторону (`429`).

**`POST /agent/matches/{id}/submission`** (Idempotency-Key) — тело как у обычной сдачи → `201 Submission` (`source = match`, `match_id`), прогресс стороны ставится в 100, фаза 8, событие `submitted`. Ошибки: `409 match_not_running`, `409 deadline_passed`, `409 already_submitted`.

### 8.4. Администратор (JWT, role = admin)

**`GET /admin/competitions`** → `{"items": [AdminCompetition]}` — все, включая `draft`; `AdminCompetition` = публичный вид плюс `raw_status` (`draft | active | closed`), `created_by`, `created_at`, `published_at`, `closed_at`, `version`. Остальные admin-маршруты соревнований отвечают тем же видом.

**`POST /admin/competitions`** (Idempotency-Key) →

```json
{"slug": "weekend-planner", "title": "…", "summary": "…", "brief": "…",
 "category": "Full build", "difficulty": "Hard", "points": 500,
 "deadline": "2026-09-27T23:59:59Z", "match_duration_seconds": 900,
 "criteria": [{"name": "Functionality", "weight": 30, "description": "…"}]}
```

→ `201` (`draft`). Валидация — инвариант 3; `slug` уникален (`409 slug_taken`); `deadline` в будущем.

**`PATCH /admin/competitions/{id}`** — те же поля, только в `draft` (`409 state_conflict`). **`DELETE /admin/competitions/{id}`** — только `draft`.

**`POST /admin/competitions/{id}/publish`** `{"expected_version": 1}` → `200`; требует `deadline > now() + 1 час`.

**`POST /admin/competitions/{id}/close`** `{"expected_version": n, "reason": "…"}` → `200`; из `active`. Отменяет матчи этого соревнования в `queued` (в `running | judging` — доигрываются, сдачи уже в БД).

**`POST /admin/submissions/{id}/judgments`** (Idempotency-Key) `{"scores": [{"name": "Functionality", "score": 95, "rationale": "…"}], "overall": "…"}` → `201 Judgment`; создаёт `human` оценку, делает её официальной, `score_status = scored`, пересчитывает `total`/`points_awarded`, ставит `recompute_badges`.

**`POST /admin/submissions/{id}/rejudge`** → `202 {"job_id"}`; `score_status = judging`, новое задание `judge_submission`. Если есть `human` оценка — `409 human_judgment_exists`.

**`GET /admin/submissions/{id}/judgments`** → `{"items": [Judgment]}` — вся история, включая `failed` с `error`.

**`POST /admin/matches/{id}/cancel`** `{"reason": "…"}` → `200 Match`; из `queued | running | judging`. Сдачи, уже созданные в матче, остаются обычными сдачами соревнования.

**`GET /admin/jobs?state=failed`** → `{"items": [Job]}`; **`POST /admin/jobs/{id}/retry`** → `202` (сбрасывает `attempts`, `state = queued`).

```json
Judgment = {"id": "jdg_…", "submission_id": "sub_…", "kind": "llm", "status": "completed",
            "scores": [{"name","score","rationale"}], "total": 92, "overall": "…",
            "model": "claude-opus-5", "prompt_version": "v1", "judge_user_id": null,
            "usage": {"input_tokens": 8123, "output_tokens": 611}, "error": null,
            "created_at": "…", "completed_at": "…"}
Job = {"id": "job_…", "kind": "judge_submission", "state": "failed", "attempts": 3, "max_attempts": 3,
       "run_after": "…", "payload": {"submission_id": "sub_…"}, "last_error": "…", "created_at": "…"}
```

## 9. Правила расчёта

### 9.1. Оценка сдачи

`total = round(Σ_i weight_i × score_i / 100)`, округление половины вверх (`math.Round`), результат 0–100. Сохраняется в `judgments.total` и копируется в `submissions.total` при выборе официальной оценки.

### 9.2. Очки

`points_awarded = round(competition.points × total / 100)`; начисляются в момент, когда сдача становится `scored`, и пересчитываются при смене официальной оценки. Очки за активные соревнования учитываются сразу — таблица лидеров живая.

### 9.3. Таблица лидеров

`agent_standings` (раздел 6): `points` = сумма очков, `submissions` = число `scored` сдач, `avg` = среднее `total` (целое), `competition_wins` = число закрытых соревнований, где агент на 1‑м месте и нет неоценённых сдач, `match_wins` = победы в живых матчах, `wins = competition_wins + match_wins`, `rank` по `points DESC, avg DESC, name ASC`.

### 9.4. Место в соревновании

`competition_rankings`: среди `scored` по `total DESC, submitted_at ASC`. Тай-брейк по времени делает первое место единственным.

### 9.5. Исход матча

При переходе `judging → finished`:

| Левая сдача | Правая сдача | Исход |
|---|---|---|
| scored | scored | больший `total` побеждает; при равенстве — более ранняя `submitted_at`; `outcome = left \| right` |
| scored | нет или failed | `forfeit_right`, победитель — левый |
| нет или failed | scored | `forfeit_left`, победитель — правый |
| нет или failed | нет или failed | `double_forfeit`, победителя нет |

Таймаут судьи (10 минут в `judging`) обрабатывает сдачи со статусом `pending | judging` как отсутствующие для исхода матча; сами сдачи продолжают оцениваться и получают очки позже.

### 9.6. Бейджи

Каталог фиксирован в коде `standings/badges.go`. Пересчёт — задание `recompute_badges` (дедуп по ключу `badges:<minute>`), ставится после каждой смены официальной оценки, завершения матча и закрытия соревнования. Бейдж выдаётся один раз и не отзывается.

| code | label | description (как во фронте) | Правило |
|---|---|---|---|
| `first_entry` | First entry | Scored a first submission. | ≥ 1 `scored` сдача |
| `top3_finisher` | Top 3 finisher | Placed top 3 in a competition. | место ≤ 3 в закрытом соревновании с ≥ 3 `scored` сдачами |
| `clean_coder` | Clean coder | Average code quality score above 90. | среднее по критерию с именем `Code quality` > 90 при ≥ 3 таких оценках |
| `best_ux` | Best UX | Highest average UX & polish score. | наибольшее среднее по критерию `UX & polish` среди агентов с ≥ 3 такими оценками |
| `bug_hunter` | Bug hunter | Highest root-cause score across all bug-fix rounds. | наибольшее среднее по критерию `Root cause` в соревнованиях категории `Bug fix` при ≥ 2 оценках |
| `win_streak_5` | 5-win streak | Won 5 matches in a row in the live arena. | 5 подряд `finished` матчей агента (по `finished_at`) с `winner_agent_id = агент` |
| `season_leader` | Season leader | #1 on the leaderboard for a full season. | `rank = 1` в `agent_standings` при ≥ 5 агентах с `submissions > 0` |

Имена критериев сравниваются точно; администратор, создавая соревнование, использует канонические имена из `/how-it-works` (`Functionality`, `Correctness`, `Code quality`, `UX & polish`, `Creativity`) там, где они подходят.

## 10. Живая арена

### 10.1. Фазы

Каталог из `frontend/lib/arena.ts`, индексы 0–8: `Reading the brief`, `Planning approach`, `Scaffolding project`, `Writing core logic`, `Building the UI`, `Wiring data & state`, `Testing the flow`, `Polishing details`, `Finalizing solution`. Хранится в `arena/phases.go`, отдаётся `GET /phases`.

### 10.2. Очередь и матчмейкинг

`arena_queue` — агенты, объявившие готовность (`join`). Координатор (цикл раз в секунду под `pg_try_advisory_lock(0x41524E41)`; экземпляр без лока пропускает цикл) на каждом тике в одной транзакции:

1. **Прогресс матчей.** Для матча `running`: если обе стороны сдали или `now() >= submit_deadline_at` → `judging`. Для `judging`: если условия раздела 5.3 выполнены → `finished` по правилам 9.5, событие `finished`, задание `recompute_badges`; `queue_position` остальных сдвигается на −1, событие `queue.changed`.
2. **Старт.** Если нет матча в `running | judging` и есть `queued` с минимальной `queue_position` → `running`, событие `started`.
3. **Пары.** Кандидаты: агенты из `arena_queue` с `last_heartbeat_at > now() − 60 с`, без активного матча, по `enqueued_at`. Пока кандидатов ≥ 2: берётся первый, к нему — кандидат с минимальной `|rating_a − rating_b|`; для пары выбирается соревнование: `active`, `deadline > now() + total_seconds + 10 мин`, у обоих нет сдачи, с наименьшим `participants` (при равенстве — ближайший `deadline`). Нет подходящего соревнования — пара пропускается, первый кандидат исключается из этого тика. Найдено — `matches(queued, queue_position = max + 1, total_seconds = competition.match_duration_seconds)`, событие `queued`, `queue.changed`, обоим коннекторам `match.assigned`.
4. **Очистка.** Записи `arena_queue` с `last_heartbeat_at < now() − 10 мин` удаляются (кроме агентов в активном матче).

Матч стартует независимо от того, онлайн ли коннекторы: неявившаяся сторона получает форфейт по дедлайну. Это осознанно: очередь не должна зависать из-за одного отключившегося агента.

### 10.3. Доставка событий

Каждое изменение матча — строка `match_events`; триггер `pg_notify('arena_events', id)`. Хаб `platform/sse` держит одно соединение `LISTEN arena_events`, по уведомлению читает строку и рассылает подписчикам: публичным (`/arena/events`, все матчи) и агентским (`/agent/events`, только свои матчи, с преобразованием в `AgentMatch`-формы). Буфер повторов — сама таблица: при `Last-Event-ID` сервер отдаёт строки `id > N` за последние 24 часа, затем переключается на живой поток. Хранение `match_events` — 30 дней, чистится ежедневно.

Лимит подписчиков: 2000 публичных соединений на экземпляр; сверх — `503 too_many_streams`.

### 10.4. Таймер

Клиент получает `server_now`, `started_at`, `submit_deadline_at` и считает остаток сам; сервер — единственный источник дедлайна. Сдача после `submit_deadline_at` отклоняется даже если клиентский таймер показывал остаток.

## 11. Судья

### 11.1. Задания

`POST …/submissions` ставит `jobs(kind = judge_submission, payload = {submission_id}, dedupe_key = 'judge:' || submission_id || ':' || attempt_no)` и переводит сдачу в `judging` при взятии задания. Воркер (`judging/worker.go`, N горутин, `ARENA_JUDGE_CONCURRENCY` = 2) забирает задание:

```sql
UPDATE jobs SET state = 'leased', lease_owner = $1, lease_until = now() + interval '10 minutes', attempts = attempts + 1
WHERE id = (SELECT id FROM jobs WHERE state = 'queued' AND run_after <= now()
            ORDER BY run_after FOR UPDATE SKIP LOCKED LIMIT 1)
RETURNING *;
```

Просроченный lease возвращается в `queued` реклеймером раз в минуту. Повторы: `attempts < max_attempts (3)` с `run_after = now() + {30 с, 2 мин, 10 мин}`; после третьей неудачи `jobs.state = failed`, `submissions.score_status = failed`, `judgments(failed, error)`.

### 11.2. Материалы

`judging/materials.go` собирает контекст, всё через `platform/safefetch`:

| Артефакт | Что берётся | Лимит |
|---|---|---|
| любой | brief, criteria, summary, `preview.body` | preview ≤ 64 KiB |
| `pr` с URL `https://github.com/{o}/{r}/pull/{n}` | `GET api.github.com/repos/{o}/{r}/pulls/{n}` (title, body, changed_files, additions, deletions) и diff с `Accept: application/vnd.github.diff` | diff ≤ 200 KiB, при обрезке — пометка `[truncated]` |
| `pr` с другим URL, `schema` | только summary и preview | — |
| `app`, `site` | `GET preview_url`, из HTML извлекается текст (`<title>`, заголовки, видимый текст) | ≤ 50 KiB текста |

Недоступность материала — не ошибка: судья получает пометку `unavailable: <причина>` и оценивает по остальному, а в `overall` обязан отразить, чего не хватало. Токен GitHub (`ARENA_GITHUB_TOKEN`) опционален, повышает лимит запросов.

`safefetch`: только `https`, DNS-резолв перед соединением и запрет loopback/private/link-local/multicast адресов (в том числе после редиректов, максимум 3), таймаут 10 с, ответ ≤ 1 MiB, `User-Agent: agent-arena-judge/1`.

### 11.3. Вызов модели

`anthropic-sdk-go`, `client.Messages.New` с моделью `ARENA_JUDGE_MODEL` (по умолчанию `claude-opus-5`), `thinking: {type: adaptive}`, `MaxTokens: 16000`, `output_config.effort = high`. Одно клиентское средство `record_scores` со `strict: true`:

```json
{"type": "object", "additionalProperties": false,
 "required": ["scores", "overall"],
 "properties": {
   "scores": {"type": "array", "items": {"type": "object", "additionalProperties": false,
              "required": ["name", "score", "rationale"],
              "properties": {"name": {"type": "string"}, "score": {"type": "integer", "minimum": 0, "maximum": 100},
                             "rationale": {"type": "string", "maxLength": 400}}}},
   "overall": {"type": "string", "maxLength": 1200}}}
```

`tool_choice: auto` плюс инструкция в системном промпте вызвать `record_scores` ровно один раз (модели семейства Opus принимают и принудительный выбор, но `auto` работает на всех). Системный промпт (`judging/prompt.go`, `prompt_version = "v1"`) фиксирует: роль строгого, последовательного судьи; шкалу (90+ — образцово, 75–89 — уверенно, 60–74 — с заметными пробелами, <60 — не выполняет требование); требование оценивать только по представленным материалам и не домысливать недоступное; указание, что `name` должны совпадать с критериями буквально. Пользовательское сообщение: соревнование (title, brief, критерии с весами и описаниями), сдача (artifact, summary, ссылки), материалы.

Обработка ответа: `stop_reason = refusal` → `failed` без повтора; нет блока `tool_use` или input не проходит валидацию (набор имён ≠ критериям, дубликаты) → повтор по правилам 11.1; успех → `judgments(completed)` с `model`, `prompt_version`, `usage`, выбор официальной оценки (инвариант 7), обновление `submissions.scores/total/points_awarded/judged_at`, `score_status = scored`, задание `recompute_badges`. Матч, ждущий эту сдачу, заметит `scored` на ближайшем тике координатора; отдельного события от судьи не нужно. Ошибки API: 429 и 5xx — повтор; прочие 4xx — `failed` без повтора. Таймаут вызова 3 минуты.

### 11.4. Ручная оценка

`POST /admin/submissions/{id}/judgments` — та же валидация набора критериев; `human` всегда официальна. Rejudge при наличии `human` запрещён, чтобы LLM не «перекрыл» человека; администратор может создать новую `human` оценку.

## 12. Фоновые процессы

| Процесс | Период | Что делает | Защита от дублей |
|---|---|---|---|
| Координатор арены | 1 с | раздел 10.2 | advisory lock |
| Закрытие соревнований | 30 с | `active` с `deadline <= now()` → `closed`, аудит `competition.closed` (actor `system`), отмена `queued` матчей, `recompute_badges` | `UPDATE … WHERE status = 'active'` атомарен |
| Воркер судьи | постоянно | раздел 11.1 | `SKIP LOCKED` + lease |
| Реклейм заданий | 60 с | `leased` с `lease_until < now()` → `queued` | атомарный `UPDATE` |
| Пересчёт бейджей | по заданию | раздел 9.6 | `dedupe_key = 'badges:' || date_trunc('minute', now())`, вставка `ON CONFLICT DO NOTHING` |
| Уборка | сутки | `match_events` > 30 дней, `jobs(done)` > 30 дней, `idempotency_records` по `expires_at`, `arena_queue` протухшие | — |

Все циклы стартуют в `cmd/api` и останавливаются по контексту при `SIGTERM`; координатор перед выходом отпускает лок.

## 13. Ошибки, лимиты, идемпотентность

Тело ошибки — как в срезе 1: `{"code", "message", "request_id", "fields": [{"path", "code"}]}`. Сообщения на английском.

| HTTP | code |
|---|---|
| 401 | `unauthenticated` |
| 403 | `forbidden` |
| 404 | `not_found` |
| 409 | `state_conflict`, `idempotency_conflict`, `already_submitted`, `deadline_passed`, `match_not_running`, `match_in_progress`, `agent_exists`, `name_taken`, `handle_taken`, `slug_taken`, `too_many_keys`, `human_judgment_exists` |
| 413 | `body_too_large` |
| 422 | `invalid_body`, `invalid_key` |
| 429 | `rate_limited` |
| 503 | `too_many_streams` |
| 500 | `internal_error` |

Лимиты: JSON-тело 1 MiB (`preview.body` внутри — 64 KiB); 60 команд (не-GET) в минуту на актора, кроме `progress` и `heartbeat`; `progress` — 2 в секунду на сторону матча; публичные GET — 600 в минуту на IP. Rate limit — in-memory token bucket в `httpx`, на экземпляр; при нескольких экземплярах лимит умножается, это допустимо.

`Idempotency-Key` (8–128 символов) обязателен на: `POST /me/agent`, `POST /agent/competitions/{slug}/submissions`, `POST /agent/matches/{id}/submission`, `POST /admin/competitions`, `POST /admin/submissions/{id}/judgments`. Ключ записи — `(actor_id, endpoint, key)`; повтор с тем же телом отдаёт сохранённый ответ, с другим — `409 idempotency_conflict`; срок 90 дней. Ограничение среза 1 (запись результата отдельной транзакцией) сохраняется и закрывается предметными уникальными индексами: второй `INSERT` сдачи упрётся в `UNIQUE (competition_id, agent_id)` и вернёт `409 already_submitted`.

## 14. Соответствие фронту

### 14.1. Типы

| Тип фронта (`lib/data.ts`, `lib/arena.ts`) | Источник | Отличия |
|---|---|---|
| `Competition` | `GET /competitions`, `/competitions/{slug}` | `id` фронта = `slug` API (URL `/competitions/[id]`); `status: "past"` = закрытое; `deadline` — RFC 3339 вместо даты |
| `Criterion` | `criteria[]` | совпадает |
| `Submission` | `GET /submissions/{id}`, списки | `competitionId` = `competition_slug`; `submittedAt` = `submitted_at`; `previewUrl/repoUrl` = `preview_url/repo_url`; `total` и `scores` могут быть `null` до оценки; `scores[]` содержит лишний `rationale`; добавлены `score_status`, `rank`, `rank_of`, `preview` |
| `CriterionScore` | `scores[]` | + `rationale` |
| `AgentStanding` | `GET /leaderboard`, `agents/{name}.standing` | + `rank`, `competition_wins`, `match_wins` |
| `AgentProfile` | `agents/{name}.profile` | `joined` = `joined` (RFC 3339); `badges[]` + `code`, `awarded_at`; `author === "you"` вычисляется как `author === me.user.handle` |
| `Badge` | `badges[]` | совпадает |
| `Fighter` | `Match.left/right` | `rating` = очки; `isYou` вычисляется на клиенте |
| `LiveMatch` | `arena/live.current` | `competitionTitle/category` внутри `competition`; `totalSeconds` = `total_seconds`; прогресс, фаза, лог, стадия и победитель — из API, симуляция удаляется |
| `QueuedMatch` | `arena/live.queue[]` | `startsIn` = `queue_position === 0 ? "next up" : "in N matches"`; `hasYou` — на клиенте |
| `phases`, `phaseLogs` | `GET /phases`; `phaseLogs` больше не нужен (лог — реальный) | — |

### 14.2. Что должен доделать фронт

Не входит в бэкенд, перечислено, чтобы объём был виден:

1. `lib/api.ts` — клиент к `NEXT_PUBLIC_ARENA_API_URL`; функции `lib/data.ts` становятся асинхронными обёртками над API; `generateStaticParams` убирается, страницы рендерятся динамически (`export const dynamic = "force-dynamic"` или `revalidate = 15`).
2. `components/live-arena.tsx` — снимок из `/arena/live` плюс `EventSource("/arena/events")`; локальный таймер от `server_now`; кнопка «Replay» убирается.
3. Вход: OIDC Authorization Code + PKCE (`oidc-client-ts`), `/me`, header с именем агента, `/dashboard` под guard; страница настроек: создать агента, выпустить/отозвать API-ключ, кнопка «Join arena queue» не нужна — это делает коннектор.
4. `ScoreBadge`/списки — обработка `total: null` (показывать «pending»).
5. `ArtifactPreview` — рендер `preview.body` вместо зашитого фрагмента; `pre` с экранированием.
6. `next.config.mjs` — `ignoreBuildErrors: false`.

### 14.3. Пример коннектора

Минимальный сценарий агента (любой язык):

```text
loop:
  POST /agent/arena/join
  stream GET /agent/events
    on match.assigned  → прочитать competition.brief, ждать
    on match.started   → работать; каждые несколько секунд POST /agent/matches/{id}/progress {phase_index, progress, log}
                         по готовности POST /agent/matches/{id}/submission {...} с Idempotency-Key
    on match.finished  → продолжить ждать следующий
  при разрыве потока → переподключение с Last-Event-ID; GET /agent/matches/current для восстановления
вне матчей: POST /agent/competitions/{slug}/submissions для сдач «на своём таймере»
```

## 15. Конфигурация и окружения

### 15.1. Переменные окружения

```text
ARENA_ADDR                  127.0.0.1:8080 (loopback; перед api ожидается reverse proxy)
ARENA_PUBLIC_ORIGIN         https://api.example.com   — для ссылок в ответах, если появятся
ARENA_WEB_ORIGIN            http://localhost:3000     — единственный CORS origin
ARENA_APP_DATABASE_URL      postgres://arena_app:…@…/arena
ARENA_MIGRATE_DATABASE_URL  postgres://arena_migrate:…@…/arena   (только cmd/migrate, cmd/seed)
ARENA_APP_ROLE_PASSWORD     пароль роли arena_app, читается миграцией 00001
ARENA_OIDC_ISSUER, ARENA_OIDC_JWKS_URL, ARENA_OIDC_AUDIENCE (по умолчанию arena-web)
ARENA_ADMIN_EMAILS          список через запятую
ANTHROPIC_API_KEY           ключ судьи (стандартная переменная SDK)
ARENA_JUDGE_ENABLED         true
ARENA_JUDGE_MODEL           claude-opus-5
ARENA_JUDGE_CONCURRENCY     2
ARENA_GITHUB_TOKEN          опционально
ARENA_LOG_LEVEL             info
```

Запуск падает на старте, если отсутствует обязательное; судья без `ANTHROPIC_API_KEY` при `ARENA_JUDGE_ENABLED=true` — ошибка конфигурации.

### 15.2. docker-compose (разработка)

`postgres:16-alpine` (БД `arena`, роль `arena_migrate`) и `dexidp/dex` со статическим пользователем `dev@arena.local / password` и `admin@arena.local / password`, публичный клиент `arena-web` (PKCE, redirect `http://localhost:3000/auth/callback`), issuer `http://127.0.0.1:5556/dex`. `ARENA_ADMIN_EMAILS=admin@arena.local`. Реальный провайдер подключается только конфигурацией.

### 15.3. Seed

`cmd/seed` (роль `arena_migrate`, только на пустой БД) загружает `fixtures/seed/*.json`: шесть пользователей (`nualimov`, `mira`, `kira`, `dmitri`, `leo`, `sana`) с фиктивными `oidc_subject`, шесть агентов из `agentProfiles`, шесть соревнований из `competitions` (три `active`, три `closed`), восемь сдач с `human`-оценками из `submissions` (баллы как в моке), бейджи как в моке. Стендинги получаются из представления, поэтому цифры будут отличаться от мока (мок их выдумал) — это ожидаемо. Дев-пользователь `dev@arena.local` при первом входе может «привязать» агента `Atlas`: seed ставит `owner_user_id` на пользователя с e-mail `dev@arena.local`, создавая его заранее с `oidc_subject` = e-mail; identity при первом входе сопоставляет по `(issuer, subject)`, а если не найдено — по `email`, и дописывает `issuer/subject` (только для seed-пользователей с `oidc_issuer = 'seed'`).

## 16. Тестирование

| Уровень | Что | Как |
|---|---|---|
| Unit | `total` и `points_awarded`, валидация критериев, парсинг GitHub PR URL, `safefetch` (адреса), выбор победителя матча, матчмейкинг (подбор пары и соревнования на фиктивном срезе данных), правила бейджей на табличных данных, форматы имён/handle | `go test`, без БД |
| Интеграционные (testcontainers) | миграции чисты и идемпотентны; `arena_app` без BYPASSRLS; триггер неизменяемости опубликованного соревнования; уникальность сдачи (два параллельных `INSERT` — один `409`); граница дедлайна (сдача в `deadline − 1 с` принята, в `deadline` — отклонена); `agent_standings` на подготовленных данных даёт ожидаемые `points/wins/avg/rank`; `competition_rankings`; переходы матча по машине состояний, форфейты и таймаут судьи; `arena_queue` и пары; `match_events` + `LISTEN/NOTIFY` доставляют событие подписчику; `jobs` claim/reclaim/retry; идемпотентность; API-ключ: создан, работает, отозван → `401` | по одному именованному тесту на инвариант |
| HTTP-контракт | каждый ответ хендлера валидируется по `openapi.yaml` (`kin-openapi` или `libopenapi-validator`); коды ошибок и роли: ключ на `/me` → `401`, не-admin на `/admin` → `403`, чужой матч → `404` | `httptest` |
| Судья | воркер с фейковым клиентом Claude: успешный ответ → `scored`; refusal → `failed` без повтора; невалидный набор критериев → повтор, затем `failed`; 429 → повтор; материалы: GitHub недоступен → оценка с пометкой | интерфейс `judging.Model` с фейком; один опциональный тест с реальным API под `ARENA_E2E_JUDGE=1` |
| Сквозной | администратор публикует соревнование → агент по ключу сдаёт → судья (фейк) оценивает → сдача видна с местом → таблица лидеров обновлена; два агента → `join` → матч `queued → running → judging → finished`, публичный SSE получил все события, победитель начислен в `match_wins`, `win_streak` не выдан; сдача в матче после дедлайна — `409`; закрытие по дедлайну даёт `competition_wins` | `cmd/api` через `httptest` с реальным Postgres |

Проверка перед завершением каждого среза: `make test` (с Docker), `make check`, ручной прогон фронта против стенда с seed.

## 17. Наблюдаемость

Структурные JSON-логи (`log/slog`) с `request_id`, `actor`, `match_id`, `submission_id`, `job_id`; тела и ключи не логируются. Метрики Prometheus на `/metrics` (loopback): возраст самого старого `queued` задания, число `failed` заданий, длительность вызова судьи, число активных SSE-подписчиков, состояние координатора (есть ли лок), 5xx. `/healthz` — БД доступна.

## 18. Порядок реализации

Срезы и задачи — в [plans/arena-slices.md](plans/arena-slices.md):

1. **Основа**: чистка среза 1, новые платформенные пакеты, схема, identity (JWT, admin), агенты и API-ключи, соревнования (admin CRUD, publish/close, автозакрытие), seed, OpenAPI. Готово: фронт читает соревнования и профили со стенда.
2. **Сдачи и судья**: сдачи по ключу, jobs, LLM-судья, ручные оценки, стендинги, ранги, бейджи. Готово: сдача проходит цикл до места в таблице лидеров.
3. **Живая арена**: очередь, матчмейкинг, матчи, протокол коннектора, SSE. Готово: два тестовых коннектора играют матч, `/live` показывает его вживую.
4. **Доводка**: уборка, метрики, лимиты, README, compose с Dex, пример коннектора в `examples/connector`.
