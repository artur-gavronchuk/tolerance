> **Устарело (18.09.2026).** Этот документ описывал платформу приёмки FORGE и заменён дизайном Agent Arena: см. [arena-backend-design.md](arena-backend-design.md). Оставлен как история; при расхождении действует новый документ.

# Технический дизайн бэкенда

Версия 0.1 · 17 сентября 2026 · проектное решение для срезов 1–5.

Документ конкретизирует `architecture.md` и `domain-and-api.md` из `tolerance-back/docs` до уровня, с которого можно писать код: структура репозитория, схема PostgreSQL, транзакционные сценарии, HTTP API, внутренний API worker'ов, оценщик, исполнитель, тестирование и эксплуатация. Сущности, состояния и правила доверия не переопределяются; при расхождении приоритет у этого документа как у более позднего, расхождение считается ошибкой и исправляется в обоих.

Фронт — отдельное приложение с отдельным деплоем; его дизайн в [frontend-design.md](frontend-design.md).

## 1. Топология

Три деплоя, три сетевых контура.

```text
[браузер] ──HTTPS──▶ app.<domain>   SPA, статика за CDN/nginx
    │
    └──HTTPS + Bearer JWT──▶ api.<domain>   Go, cmd/api
                                │
                     ┌──────────┼──────────────┐
                     ▼          ▼              ▼
                PostgreSQL   S3 (4 bucket'а)  OIDC-провайдер (JWKS)
                     ▲
        /internal/v1 │ сервисные токены + fencing token
           ┌─────────┴──────────┐
        cmd/runner          cmd/evaluator
           │                     │
     sandbox-провайдер     изолированная среда приложения
     (одноразовая VM)      + браузер Playwright
```

| Контур | Кто внутри | Что видит |
|---|---|---|
| Управление | `cmd/api`, PostgreSQL, S3 | Всё. Только `cmd/api` пишет в БД |
| Исполнение | `cmd/runner`, `cmd/evaluator` | `api.<domain>/internal/v1`, S3 по pre-signed URL с scope на свой attempt или evaluation |
| Недоверенный код | агент, приложение участника, браузер оценщика | Ничего из двух контуров выше; наружу только через шлюз разрешённых API |
| Демо | приложения финалистов для зрителей | Отдельный домен `demo-<slug>.<demo-domain>`, без общих cookie с платформой |

Bucket'ы S3 и политики доступа:

| Bucket | Содержимое | Пишет | Читает |
|---|---|---|---|
| `artifacts-in` | сданные пакеты и манифесты | участник по pre-signed PUT | evaluator по pre-signed GET на конкретный digest |
| `scenarios-private` | закрытые сценарии, seeds, скрытые входы | организатор через api | только evaluator, только пакеты назначенной оценки |
| `evidence-private` | trace, снимки, логи, состояние | evaluator по pre-signed PUT на свой evaluation | владелец сдачи/организатор через краткоживущий GET от api |
| `public-snapshots` | очищенные отчёты, обезличенные свидетельства | `cmd/api` при публикации | CDN, без авторизации |

Фронт не получает прямых ссылок на приватные bucket'ы. Любая ссылка на скачивание — pre-signed URL со сроком жизни 5 минут, выданный api после проверки прав.

## 2. Структура кода

Один Go-модуль `tolerance`, Go 1.24+, три бинарника.

```text
cmd/api/            HTTP, диспетчер заданий, reconciler, публикация
cmd/runner/         worker исполнения агентов через sandbox-адаптер
cmd/evaluator/      worker независимой проверки
internal/
  identity/         организации, пользователи, членство, роли; OIDC-claims → Actor
  campaigns/        кампания, режим, политика публикации, slug
  missions/         миссия, версии контракта, требования, пакеты сценариев, калибровка
  entries/          участие, проект, версии агента, comparison_group
  artifacts/        загрузки, digest, retention, pre-signed URL
  submissions/      сдача, манифест, родительская сдача, official-слоты
  execution/        Run, RunAttempt, lease, fencing, сверка, вмешательства
  evaluation/       Evaluation, CheckResult, Evidence, назначение сценариев
  acceptance/       ReviewDecision, Appeal, AcceptedRelease, ReleaseEvent, финализация
  budget/           Budget, Reservation, UsageEntry
  usersessions/     слепые пользовательские сессии, рубрика, назначения
  publication/      ReportRevision, очищенный снимок, публичная проекция
  platform/
    db/             pgx-пул, Tx, RLS-контекст, выполнение миграций
    jobs/           таблица jobs: claim, heartbeat, complete, fencing
    blob/           S3-клиент, pre-signed URL, bucket-политики
    auth/           JWKS-верификация, сервисные токены, scope
    httpx/          problem-ответы, Idempotency-Key, лимиты тела, request_id, CORS
    audit/          запись событий в текущей транзакции
    events/         SSE-шина: fan-out изменений по campaign_id
    clock/          инжектируемое время
contracts/openapi/  openapi.yaml, источник для клиента фронта
contracts/packages/ JSON Schema: манифест сдачи, пакет свидетельств, пакет передачи, манифест агента
migrations/         SQL-миграции, только вперёд, нумерованные
workers/evaluator/  сценарии Playwright, валидатор маршрута, stub API v1/v2, миграционные проверки
workers/runner/     адаптеры sandbox, шлюз API и бюджета
fixtures/city/      публичный snapshot, эталонное приложение, намеренные поломки
docs/               проектные решения
```

### 2.1. Правила модулей

- Модуль владеет своими таблицами. Другие модули не читают их SQL напрямую.
- Модуль экспортирует `Service` (прикладные сценарии) и `Repository` (интерфейс над pgx). Реализация репозитория лежит в том же модуле, файл `postgres.go`.
- Сценарий, затрагивающий несколько модулей, живёт в модуле-инициаторе и принимает интерфейсы остальных. Пример: `submissions.Service.Submit` вызывает `artifacts.Confirm`, `budget.Reserve`, `evaluation.Schedule`, `audit.Record` внутри одной `db.Tx`.
- Модули не импортируют друг друга по пакетам, только по интерфейсам, которые объявлены у потребителя и реализованы у владельца. Сборка зависимостей вручную в `cmd/api/main.go`, без DI-фреймворка.
- Доменные типы и состояния — в модуле, HTTP-DTO — в `internal/<module>/http.go`. Хендлер: декодировать, проверить форму, вызвать сервис, отдать ответ. Без бизнес-логики.
- Ошибки доменных правил — типизированные `problem` с кодом из раздела 5.5; хендлер их только передаёт.

### 2.2. Что берётся из прототипа

Из локального прототипа переносятся решения, не код:

- `contract_digest` версии миссии считается по каноническому JSON только контрактных полей; операционные поля в digest не входят.
- Идемпотентность: ключ `(organization_id, actor_id, endpoint, key)`, тело сравнивается по digest, повтор возвращает сохранённый ответ.
- Артефакт отдаётся точными сохранёнными байтами; перекодирование JSON недопустимо.
- Валидатор маршрута `ValidateRoute` переезжает в `workers/evaluator/route` как один из сценариев B-G2.

## 3. Схема PostgreSQL

Общие правила:

- Идентификаторы `text` с префиксом (`mission_…`, `sub_…`), генерирует приложение: 128 бит случайности, hex.
- Все таблицы частных данных содержат `organization_id text not null`. Внешние ключи между tenant-таблицами составные: `(id, organization_id)`, чтобы ссылка не могла пересечь организацию.
- Время `timestamptz`, хранится UTC. Деньги `bigint` минимальных единиц, валюта `text` ISO 4217, без float.
- Агрегаты с переходами состояний имеют `version int not null default 1`. Команда передаёт ожидаемую версию; `UPDATE … WHERE id = $1 AND version = $2` с нулём строк даёт `409 state_conflict`.
- Опубликованные версии контракта неизменяемы: триггер `BEFORE UPDATE` на `mission_versions` и `requirements` отклоняет любое изменение строки, у которой `published_at IS NOT NULL`. Операционное состояние живёт в `missions`, не в версии.
- Удаление физическое только по retention-политике через отдельную процедуру; для остального `deleted_at` или события.

### 3.1. Таблицы

**identity**

```sql
organizations(id, name, created_at)
users(id, oidc_issuer, oidc_subject, email, display_name, created_at)          unique(oidc_issuer, oidc_subject)
memberships(id, organization_id, user_id, role, status, invited_by, created_at) unique(organization_id, user_id)
  role in (owner, organizer, participant, evaluator, arbiter, viewer)
invitations(id, organization_id, email, role, token_digest, expires_at, accepted_at)
```

**campaigns**

```sql
campaigns(id, organization_id, name, slug, mode, publication_policy jsonb, currency, budget_id, state, version, created_at)
  mode in (public_season, private_trial); unique(slug) where slug is not null
  state in (draft, active, finalizing, completed, cancelled)
```

**missions**

```sql
missions(id, organization_id, campaign_id, stage, ordinal, state, active_version_id, version)
  stage in (qualification, build, adapt, handoff); unique(campaign_id, ordinal)
  state in (draft, calibrating, open, submission_closed, evaluating, provisional, appeals_open, finalized, cancelled, invalidated)
mission_versions(id, organization_id, mission_id, number, parent_version_id, contract jsonb, contract_digest,
                 published_at, submission_deadline, appeal_deadline, policies jsonb, version)
  unique(mission_id, number)
requirements(id, organization_id, mission_version_id, stable_key, revision, gate bool, weight int, category, text)
  unique(mission_version_id, stable_key)
scenario_bundles(id, organization_id, mission_version_id, digest, evaluator_digest, dataset_digest,
                 visibility, storage_key, calibration_id, calibrated_at)
  visibility in (private, public_example)
requirement_scenarios(requirement_id, scenario_bundle_id, scenario_id, aggregation)
  aggregation in (all, any)
calibrations(id, organization_id, scenario_bundle_id, reference_submission_id, broken_submission_ids jsonb, result jsonb, passed bool, at)
```

`contract jsonb` — полный снимок: бриф, требования, правила ресурсов, приёмки, публикации, лимиты. `contract_digest` — SHA-256 канонического JSON этого снимка плюс `evaluator_digest`. Клиент видит digest и может пересчитать его по опубликованному контракту.

**entries**

```sql
teams(id, organization_id, name)
team_members(team_id, user_id, role)
entries(id, organization_id, campaign_id, team_id, project_id, mode, qualification_status, comparison_group_id, version)
  mode in (external, managed); unique(campaign_id, project_id)
projects(id, organization_id, entry_id, owner_id, name, maintenance_until)
agent_versions(id, organization_id, team_id, manifest jsonb, manifest_digest, model_ids jsonb, tools jsonb,
               context_digest, human_policy, created_at)
  unique(organization_id, manifest_digest)
```

**artifacts**

```sql
uploads(id, organization_id, owner_user_id, purpose, state, expected_size, expected_digest, storage_key,
        put_url_expires_at, completed_at, artifact_id)
  state in (pending, completed, aborted, expired)
artifacts(id, organization_id, digest, storage_key, size, media_type, retention_until, deleted_at)
  unique(organization_id, digest)
```

Физическая дедупликация по digest допустима внутри организации, между организациями — нет: одинаковые байты у двух организаций получают две строки и два ключа хранения.

**submissions**

```sql
submissions(id, organization_id, project_id, mission_version_id, parent_submission_id, run_attempt_id,
            replicate_index, official bool, artifact_id, artifact_digest, manifest jsonb, manifest_digest,
            provenance jsonb, submitted_at, version)
  unique(project_id, mission_version_id, replicate_index) where official
  fk (parent_submission_id, project_id) -> submissions(id, project_id)
  fk (artifact_id, organization_id) -> artifacts(id, organization_id)
official_replacements(id, organization_id, superseded_submission_id, replacement_submission_id, reason, actor_id, at)
```

`submitted_at` — время успешного `uploads/{id}/complete`, его сравнивают с дедлайном.

**execution**

```sql
runs(id, organization_id, entry_id, mission_version_id, agent_version_id, purpose, replicate_index, state, budget_reservation_id, version)
  purpose in (build, adapt, handoff)
run_attempts(id, organization_id, run_id, attempt_no, execution_key, environment_id, state, retry_reason,
             lease_owner, lease_until, fencing_token bigint, heartbeat_at, started_at, finished_at,
             result jsonb, reconciliation jsonb, version)
  unique(run_id, attempt_no); unique(execution_key)
  state in (queued, provisioning, running, collecting, succeeded, failed, budget_exhausted, timed_out,
            stopping, cancelled, reconciliation_required)
environment_snapshots(id, organization_id, image_digest, dataset_digest, resource_limits jsonb, network_policy jsonb, clock, seed)
interventions(id, organization_id, run_attempt_id, actor_id, kind, duration_seconds, reason, evidence_id, at)
```

**evaluation**

```sql
evaluations(id, organization_id, submission_id, scenario_bundle_id, scenario_bundle_digest, environment_id,
            replicate_index, state, official bool, supersedes_id, lease_owner, lease_until, fencing_token,
            started_at, finished_at, version)
  state in (queued, running, completed, infra_error, cancelled)
check_results(id, organization_id, evaluation_id, scenario_id, requirement_id, status, observed jsonb,
              failure_origin, decided_by, at)
  status in (pass, fail, error, skipped); failure_origin in (product, platform, scenario, unknown)
  unique(evaluation_id, scenario_id)
evidence(id, organization_id, check_result_id, artifact_id, kind, visibility, redaction_state, created_at)
  kind in (trace, screenshot, state_snapshot, log, manual_observation)
  visibility in (private, public_after_redaction, public); redaction_state in (raw, redacted, approved)
```

**acceptance**

```sql
review_decisions(id, organization_id, evaluation_id, revision, supersedes_id, eligibility, acceptance,
                 reasons jsonb, considered_check_ids jsonb, considered_evidence_ids jsonb, actor_id, state, created_at)
  eligibility in (eligible, ineligible, pending); acceptance in (accepted, rejected, pending)
  state in (pending, provisional, final); unique(evaluation_id, revision)
appeals(id, organization_id, decision_id, requirement_id, submitted_by, evidence_ids jsonb, reason, state,
        arbiter_id, resolution jsonb, resolution_id, created_at, resolved_at)
  state in (open, reviewing, upheld, rejected, needs_recheck)
accepted_releases(id, organization_id, project_id, submission_id, decision_id, previous_release_id, accepted_at)
  unique(submission_id); fk (previous_release_id, project_id) -> accepted_releases(id, project_id)
release_events(id, organization_id, release_id, kind, reason, decision_id, actor_id, at)
  kind in (accepted, revoked, superseded, note)
```

**budget**

```sql
budgets(id, organization_id, scope_kind, scope_id, currency, limit_minor, version)
  scope_kind in (campaign, entry, organizer_reserve)
reservations(id, organization_id, budget_id, holder_kind, holder_id, category, amount_minor, state, created_at, settled_at)
  holder_kind in (run_attempt, evaluation, build, user_session); state in (open, awaiting_settlement, settled, released)
usage_entries(id, organization_id, budget_id, holder_kind, holder_id, category, source, coverage,
              amount_minor, provider, external_usage_id, dedupe_key, observed_at)
  source in (measured, declared, estimated); coverage in (complete, partial, unknown)
  unique(provider, external_usage_id) where external_usage_id is not null; unique(dedupe_key)
```

**usersessions**

```sql
testers(id, organization_id, contact jsonb, consent_at)                      -- личные данные, не попадают в проекции
session_assignments(id, organization_id, campaign_id, tester_id, order jsonb, rubric_version, created_at)
user_sessions(id, organization_id, assignment_id, entry_id, submission_id, pseudonym, task_scores jsonb,
              observations jsonb, hints jsonb, valid bool, invalid_reason, moderator_id, at)
```

**publication**

```sql
report_revisions(id, organization_id, campaign_id, number, decision_ids jsonb, rubric_digest, snapshot jsonb,
                 supersedes_id, created_by, created_at, published_at, published_by)
  unique(campaign_id, number)
public_projection(campaign_slug, revision_id, body jsonb, published_at)   -- денормализованный ответ /public
```

**platform**

```sql
jobs(id, kind, aggregate_kind, aggregate_id, organization_id, state, run_after, lease_owner, lease_until,
     fencing_token bigint, attempts int, max_attempts int, payload jsonb, last_error, created_at)
  kind in (run_attempt, evaluation, publication, retention, reconcile)
  state in (queued, leased, done, failed, dead)
audit_events(id, organization_id, actor_id, actor_kind, action, aggregate_kind, aggregate_id,
             before_version, after_version, reason, payload jsonb, request_id, at)
idempotency_records(organization_id, actor_id, endpoint, key, payload_digest, status, body jsonb, created_at, expires_at)
  primary key(organization_id, actor_id, endpoint, key)
```

### 3.2. RLS

Второй барьер поверх проверки роли в коде.

- Роль `forge_app`: без `BYPASSRLS`, не владелец таблиц. На всех tenant-таблицах `ENABLE ROW LEVEL SECURITY` и `FORCE ROW LEVEL SECURITY`; политика `organization_id = current_setting('app.organization_id', true)`.
- Каждая транзакция начинается с `SET LOCAL app.organization_id = $1`. Обёртка `db.Tx(ctx, actor, fn)` делает это сама; без актора транзакцию открыть нельзя.
- Роль `forge_worker` для внутренних маршрутов: политики дополнительно проверяют `current_setting('app.attempt_id')` или `app.evaluation_id`.
- Роль `forge_migrate` владеет таблицами и запускает миграции. Роль `forge_publisher` читает все организации только для сборки снимка публикации и только в `cmd/api` при выполнении job `publication`.

### 3.3. Индексы

Помимо уникальных выше: `jobs(state, run_after)`, `run_attempts(lease_until) where state in active`, `evaluations(lease_until) where state = 'running'`, `submissions(project_id, submitted_at desc)`, `check_results(evaluation_id)`, `audit_events(organization_id, at desc)`, `usage_entries(budget_id)`, `reservations(budget_id, state)`.

## 4. Транзакционные сценарии

Каждый сценарий — одна функция сервиса, одна транзакция, один аудит-событие в конце.

| Сценарий | Шаги в транзакции | Ответ |
|---|---|---|
| Открыть миссию | проверить роль organizer; `missions.state = calibrating`; последняя калибровка `passed`; посчитать `contract_digest`; поставить `published_at`; `missions.state = open`, `active_version_id`; аудит | `200` версия |
| Начать загрузку | проверить квоту участника (по умолчанию 20 незавершённых загрузок и 2 GiB в сутки); создать `uploads(pending)`; выдать pre-signed PUT на `artifacts-in` со сроком 15 минут и ограничением размера | `201` upload с URL |
| Завершить загрузку | `HEAD` объекта в S3; сравнить размер; прочитать объект потоком и посчитать SHA-256; сравнить с `expected_digest`; создать `artifacts`; `uploads.state = completed` | `200` artifact |
| Сдать | владелец entry; upload `completed`; миссия `open`, `now < submission_deadline`; манифест валиден по JSON Schema; `parent_submission_id` того же проекта; слот `(project, version, replicate)` свободен либо `official = false`; резерв оценки из бюджета кампании; создать `submissions`, `evaluations(queued, official = official сдачи)`, `jobs(evaluation)`; аудит | `201` сдача с id оценки |
| Запросить запуск | участник в окне или организатор; `entries.mode = managed`; `agent_versions` принадлежит команде; слот повторения свободен; `FOR UPDATE` бюджета; резерв верхней границы; создать `runs`, `run_attempts(attempt_no = 1, execution_key = attempt.id)`, `jobs(run_attempt)`; аудит | `202` run с attempt и резервом |
| Отменить запуск | владелец run или организатор; попытка активна; `state = stopping`; `jobs(reconcile)` для подтверждения; аудит | `202` |
| Принять решение | назначенный владелец; оценка `completed`; собрать `considered_check_ids`; если `acceptance = accepted` и есть обязательное требование без `pass` — `409 gate_failed`; создать ревизию `review_decisions(provisional)` с `supersedes_id`; аудит | `201` решение |
| Подать спор | окно споров открыто; решение `provisional`; требование принадлежит версии оценки; создать `appeals(open)`; аудит | `201` |
| Разрешить спор | назначенный арбитр, не автор и не спонсор; исход `upheld`, `rejected` или `needs_recheck`; при `needs_recheck` создать новые `evaluations` для всех затронутых сдач с новым `scenario_bundle_digest`, резерв из `organizer_reserve` или `409 budget_unavailable`; аудит | `200` |
| Финализировать миссию | `appeals_open`, `now > appeal_deadline`, нет `open/reviewing/needs_recheck`; каждое `provisional` решение становится `final`; для `accepted` создать `accepted_releases` с `previous_release_id` последнего выпуска проекта и `release_events(accepted)`; `missions.state = finalized`; аудит | `200` |
| Собрать отчёт | все миссии кампании `finalized` или явно исключены с причиной; собрать `snapshot` из финальных решений, баллов, сессий, расходов, ограничений; `report_revisions` | `201` |
| Опубликовать отчёт | политика публикации кампании; очистка по правилам раздела 8; записать `public_snapshots`; `public_projection`; `published_at`; аудит | `200` |

Бюджетная проверка внутри «Сдать» и «Запросить запуск»:

```sql
SELECT limit_minor FROM budgets WHERE id = $1 FOR UPDATE;
-- available = limit − Σ usage(confirmed) − Σ reservations(open, awaiting_settlement)
-- если available < upper_bound → откат, 409 budget_unavailable
INSERT INTO reservations(..., state = 'open', amount_minor = upper_bound);
```

`upper_bound` считается из контракта версии миссии: максимальная стоимость агентной сессии, сборки, оценки, пользовательской сессии. Клиент не передаёт лимит.

## 5. HTTP API

Префикс `/api/v1`. JSON, UTF-8, UTC-время в RFC 3339, cursor pagination `?cursor=&limit=` с ответом `{items, next_cursor}`. OpenAPI 3.1 в `contracts/openapi/openapi.yaml` — единственный источник форм запросов и ответов; фронт генерирует клиент из него, api проверяет запросы по нему в тестах.

### 5.1. Аутентификация и авторизация

- Пользователь: `Authorization: Bearer <access_token>` от OIDC-провайдера. Api проверяет подпись по JWKS (кэш 10 минут), `iss`, `aud = forge-api`, `exp`. Первый вход создаёт `users` по `(iss, sub)`.
- Активная организация: заголовок `X-Organization-Id`. Api проверяет `memberships(status = active)` и строит `Actor{user_id, organization_id, role}`. Без заголовка доступны только `/me` и `/public/*`.
- Публичные маршруты `/public/*` без токена читают `public_projection`.
- Сервисные маршруты `/internal/v1/*`: worker при старте имеет статический bootstrap-секрет (из окружения) и обменивает его на краткоживущий сервисный токен `POST /internal/v1/token` с ролью `runner` или `evaluator`. `claim` выдаёт задание вместе с токеном scope `attempt:<id>` или `evaluation:<id>`, срок 15 минут, продлевается в `heartbeat`. Пользовательский токен на `/internal` отклоняется по `aud`.
- CORS: `Access-Control-Allow-Origin` только для origin SPA из конфигурации, `Allow-Headers: Authorization, Content-Type, Idempotency-Key, X-Organization-Id, If-Match`, без credentials.

Матрица ролей (роль в организации):

| Маршрут | owner | organizer | participant | evaluator | arbiter | viewer |
|---|---|---|---|---|---|---|
| campaigns, missions: create/open/finalize | ✓ | ✓ | | | | |
| scenario bundles, calibration | | ✓ | | ✓ | | |
| entries, agent-versions, uploads, submissions, runs | | ✓ (от имени) | ✓ (свои) | | | |
| evaluations: read | ✓ | ✓ | ✓ (свои) | ✓ | ✓ (по спору) | |
| evidence: read private | ✓ | ✓ | ✓ (свои) | ✓ | ✓ (по спору) | |
| decisions: create | ✓ | | | | ✓ (по спору) | |
| appeals: create | ✓ | | ✓ (свои) | | | |
| appeals: resolve | | | | | ✓ | |
| report-revisions, publish | | ✓ | | | | |
| public projection, demo | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |

Участник никогда не принимает собственную работу: `decisions` дополнительно проверяют, что актор не член команды сдачи. Арбитр не может быть членом команды и не может быть владельцем кампании.

### 5.2. Маршруты пользователя

| Метод и маршрут | Назначение |
|---|---|
| `GET /me` | пользователь, членства, активная организация |
| `GET /organizations/{id}/members` · `POST /organizations/{id}/invitations` · `POST /invitations/{token}/accept` | членство |
| `GET /campaigns` · `POST /campaigns` · `GET /campaigns/{id}` · `POST /campaigns/{id}/activate` · `POST /campaigns/{id}/complete` · `POST /campaigns/{id}/cancel` | кампания; произвольного изменения `state` нет |
| `GET /campaigns/{id}/budget` · `POST /campaigns/{id}/budget` | лимит и статьи |
| `POST /campaigns/{id}/missions` · `GET /missions/{id}` · `GET /campaigns/{id}/missions` | этапы |
| `POST /missions/{id}/versions` · `GET /missions/{id}/versions/{n}` | версия контракта, тело — полный `contract` |
| `POST /mission-versions/{id}/scenario-bundles` · `POST /scenario-bundles/{id}/calibrate` | закрытые пакеты и калибровка |
| `POST /missions/{id}/open` · `POST /missions/{id}/close-submissions` · `POST /missions/{id}/finalize` · `POST /missions/{id}/cancel` | переходы состояния, тело с `expected_version` и `reason` |
| `POST /campaigns/{id}/entries` · `GET /entries/{id}` · `GET /campaigns/{id}/entries` | участие |
| `POST /agent-versions` · `GET /agent-versions/{id}` | конфигурация агента |
| `POST /uploads` · `POST /uploads/{id}/complete` · `POST /uploads/{id}/abort` | загрузка пакета |
| `POST /entries/{id}/submissions` · `GET /submissions/{id}` · `GET /submissions/{id}/artifact-url` · `GET /entries/{id}/submissions` | сдача |
| `POST /entries/{id}/runs` · `GET /runs/{id}` · `POST /runs/{id}/cancel` · `GET /runs/{id}/attempts/{n}/log-url` | управляемый запуск |
| `POST /run-attempts/{id}/interventions` | заявленная помощь человека |
| `GET /submissions/{id}/evaluations` · `GET /evaluations/{id}` · `GET /evaluations/{id}/matrix` · `GET /evidence/{id}/url` | матрица «требование → результат → свидетельство» |
| `POST /evaluations/{id}/decisions` · `GET /decisions/{id}` | решение и его ревизии |
| `POST /decisions/{id}/appeals` · `GET /appeals/{id}` · `POST /appeals/{id}/resolve` · `GET /campaigns/{id}/appeals` | споры |
| `GET /projects/{id}/releases` · `GET /releases/{id}` · `POST /releases/{id}/events` | история выпусков |
| `GET /projects/{id}/handoff-package-url` | пакет передачи для получателя |
| `POST /campaigns/{id}/session-assignments` · `POST /user-sessions` · `GET /campaigns/{id}/user-sessions` | пользовательские сессии |
| `GET /campaigns/{id}/scoreboard` | баллы по правилам регламента, с `pending` |
| `POST /campaigns/{id}/report-revisions` · `GET /report-revisions/{id}` · `POST /report-revisions/{id}/publish` | отчёт |
| `GET /campaigns/{id}/audit` | журнал |
| `GET /campaigns/{id}/export-url` | экспорт артефактов и свидетельств до истечения retention |
| `GET /campaigns/{id}/events` | SSE, см. 5.4 |
| `GET /public/campaigns/{slug}` · `GET /public/campaigns/{slug}/report` · `GET /public/campaigns/{slug}/compare` | публичная проекция |

`GET /evaluations/{id}/matrix` возвращает готовую матрицу:

```json
{
  "evaluation_id": "eval_…",
  "state": "completed",
  "scope_note": "Проверены сценарии пакета sha256:…; не проверены: …",
  "rows": [
    {"requirement": {"stable_key": "B-G2", "gate": true, "text": "…"},
     "status": "pass", "failure_origin": null,
     "checks": [{"scenario_id": "route-feasible-1", "status": "pass", "observed": {...},
                 "evidence": [{"id": "ev_…", "kind": "trace", "visibility": "private"}]}]}
  ],
  "eligible": true,
  "blocking": []
}
```

### 5.3. Сервисные маршруты

| Маршрут | Кто | Что |
|---|---|---|
| `POST /internal/v1/token` | runner, evaluator | bootstrap-секрет → сервисный токен |
| `POST /internal/v1/jobs/claim` | оба | `{kinds: [...]}` → задание, `fencing_token`, scope-токен, pre-signed URL входов |
| `POST /internal/v1/attempts/{id}/heartbeat` | runner | продлевает lease, принимает `state` и промежуточные метрики |
| `POST /internal/v1/attempts/{id}/usage` | runner | измерения с `external_usage_id`, идемпотентно |
| `POST /internal/v1/attempts/{id}/interventions` | runner | записанная помощь человека |
| `POST /internal/v1/attempts/{id}/complete` | runner | `state`, `result`, digest артефакта; создаёт `submissions` от имени попытки |
| `POST /internal/v1/evaluations/{id}/heartbeat` | evaluator | продление lease |
| `POST /internal/v1/evaluations/{id}/evidence-urls` | evaluator | pre-signed PUT в `evidence-private` |
| `POST /internal/v1/evaluations/{id}/results` | evaluator | `check_results` + `evidence` пакетом, идемпотентно по `(evaluation_id, scenario_id)` |
| `POST /internal/v1/evaluations/{id}/complete` | evaluator | `completed` или `infra_error` с причиной |

Каждый вызов несёт `X-Fencing-Token`; api сравнивает с текущим в строке и отвечает `409 stale_lease` при меньшем значении. Scope-токен ограничивает `id` в пути.

### 5.4. SSE

`GET /campaigns/{id}/events` держит соединение, `text/event-stream`. Токен передаётся в заголовке `Authorization`; клиент использует потоковый `fetch`, не `EventSource`. Поддерживается `Last-Event-ID` для возобновления из буфера последних 1000 событий кампании в памяти api, при недоступности — клиент перечитывает.

Событие несёт только ссылку на изменившийся агрегат:

```text
id: 184
event: change
data: {"kind":"evaluation","id":"eval_…","version":7,"at":"2026-09-17T20:00:00Z"}
```

Ни логов, ни свидетельств, ни данных в событиях нет; клиент перечитывает объект через обычный GET, который проверяет права. Фильтрация событий по видимости актора выполняется на сервере перед отправкой.

### 5.5. Ошибки

```json
{"code": "state_conflict", "message": "Открыть можно только версию в состоянии calibrating.",
 "request_id": "req_…", "fields": [{"path": "contract.deadline", "code": "past"}]}
```

Коды: `401 unauthenticated`, `403 forbidden`, `404 not_found` (и для чужих объектов), `409 state_conflict`, `409 idempotency_conflict`, `409 budget_unavailable`, `409 gate_failed`, `409 official_slot_taken`, `409 stale_lease`, `410 artifact_expired`, `413 body_too_large`, `422 invalid_body`, `422 invalid_contract`, `429 rate_limited`, `500 internal_error`. Сообщение безопасно для показа; внутренние детали только в логах по `request_id`.

Лимиты: тело JSON 1 MiB, манифест 256 KiB, пакет сдачи 512 MiB через S3, rate limit 60 команд в минуту на актора.

## 6. Оценщик

`cmd/evaluator` — отдельный процесс на отдельном узле. Цикл:

1. `claim` заданий вида `evaluation`. В ответе: pre-signed GET артефакта и закрытого пакета сценариев, `environment_snapshot`, seed, ожидаемый `artifact_digest`, `scenario_bundle_digest`.
2. Скачать оба объекта, пересчитать SHA-256. Несовпадение — `complete(infra_error, reason = digest_mismatch)`, ни один сценарий не запускается.
3. Создать изолированную среду приложения по адаптеру `AppSandbox` (для fixtures в разработке — Docker без сети; для внешних участников — тот же VM-провайдер, что у runner). Сборка внутри среды с лимитами: размер распаковки, число файлов, запрет выхода путей, сеть закрыта кроме stub-API.
4. Старт по `startup_contract`: порт 8080, `GET /healthz` за 120 секунд, объявленный volume, команда миграции отдельно. Провал — `check_results` для B-G1 со `status = fail, failure_origin = product`; остальные `skipped` с причиной.
5. Выполнить сценарии пакета. Каждый сценарий — Go-функция или Playwright-скрипт с интерфейсом `Run(ctx, app, inputs) (Observation, error)`. Инфраструктурная ошибка сценария — `error, failure_origin = platform`; наблюдаемое нарушение — `fail, product`; сомнение — `unknown`. Результаты и свидетельства отправляются пакетно после каждого сценария (`results`), чтобы падение оценщика не теряло прошедшее.
6. Для миссии `adapt`: восстановить volume из свидетельства `state_snapshot` родительской сдачи, применить команду миграции новой версии, затем сценарии сохранности данных.
7. `complete(completed)`; среда уничтожается, ключи отзываются. Heartbeat каждые 30 секунд; потеря lease останавливает оценку.

Состав сценариев эталонного сезона лежит в `workers/evaluator/scenarios/city/`: `startup`, `route_feasibility` (перенос `ValidateRoute`, читает маршрут через API приложения), `persistence_restart`, `session_isolation`, `invalid_inputs`, `interest_choice`, `timeline_display`, `replace_and_recompute`, `named_copies`, `migration`, `concurrent_edit`, `api_v2`, `closed_place`, `shared_edit`, `export_json`. Stub API мест v1/v2 — `workers/evaluator/stubapi`, поднимается в сети среды с задержками и `429` по сценарию.

Калибровка — та же машина с `reference_submission_id` и списком намеренных поломок: пакет проходит, если эталон получает `pass` по всем обязательным, а каждая поломка получает `fail` ровно на своём требовании.

Браузер Playwright изолирован: отдельный контейнер или VM без доступа к сети управления, официальный образ, `--no-sandbox` запрещён. Сырые HTML-логи не сохраняются как свидетельства без пометки `raw`; в интерфейс попадают только текстовые извлечения.

## 7. Исполнитель

`cmd/runner` запускает агента в одноразовой среде.

Адаптер sandbox-провайдера:

```go
type Sandbox interface {
    CreateOrGet(ctx, execKey string, spec EnvSpec) (Env, error) // идемпотентно по execKey
    Find(ctx, execKey string) (Env, bool, error)
    Stop(ctx, envID string) error                                // возвращает после подтверждения
    Usage(ctx, envID string) ([]UsageRecord, error)              // с external_usage_id
}
```

Провайдер без `CreateOrGet` и `Find` не принимается: `cmd/runner` отказывается стартовать. Первый адаптер — Docker для доверенных fixtures в разработке; продовый провайдер одноразовых VM выбирается перед срезом 4.

Цикл попытки:

1. `claim(run_attempt)` → scope-токен, fencing token, `EnvSpec` (образ по digest, лимиты, сетевая политика, snapshot данных), манифест агента и адаптер runner'а.
2. `heartbeat(provisioning)`; `CreateOrGet(attempt_id)`. Повторная доставка задания находит ту же среду.
3. Выдать среде краткоживущие учётные данные шлюза с `attempt_id`; шлюз (`workers/runner/gateway`) проксирует только разрешённые API, считает запросы, токены и стоимость, отклоняет вызовы, для которых остаток резерва меньше верхней оценки, пишет `usage` с `external_usage_id`. Ключи в логи не пишет.
4. `heartbeat(running)` каждые 30 секунд с текущими метриками. Таймаут сессии (60 минут в пилоте) → `Stop` → `timed_out`. Исчерпание резерва → `budget_exhausted`.
5. `collecting`: забрать рабочий каталог, упаковать, загрузить в `artifacts-in` по pre-signed PUT, посчитать digest.
6. `complete(succeeded, artifact_digest, manifest)`; api создаёт `submissions` с `run_attempt_id` и ставит оценку.
7. Любая ошибка адаптера с неопределённым ответом → `complete(reconciliation_required)`; дальше работает reconciler в `cmd/api`.

Отмена: api ставит `stopping` и job `reconcile`; runner на следующем heartbeat получает `cancel_requested`, вызывает `Stop`, отзывает ключи, отправляет `complete(cancelled)`. Если runner не отвечает, reconciler сам вызывает `Stop` по `execution_key`.

Reconciler в `cmd/api` каждые 60 секунд: попытки с `lease_until < now()` и активным состоянием → `Find(execution_key)`; найдена и работает → новый lease следующему worker'у; не найдена → `failed(reason = lost)`; провайдер не отвечает → `reconciliation_required`, алерт. Резерв остаётся до `settled`.

## 8. Публикация и хранение

Снимок отчёта собирается из финальных решений и проходит очистку: удаляются закрытые входы, `evidence` с `visibility = private`, любые `storage_key`, личные данные тестировщиков, логи с `redaction_state = raw`. Список исключённых частей включается в снимок явно. Публичная проекция обезличивает команды до раскрытия по политике кампании.

Retention-job раз в сутки: демо через 14 дней, сырые логи через 30, артефакты и очищенные свидетельства через 90, решения и хеши 12 месяцев. Удалённый объект помечается `deleted_at`, отчёт получает пометку «невоспроизводим», экспорт доступен до удаления через `GET /campaigns/{id}/export-url`.

## 9. Конфигурация и окружения

Конфигурация через переменные окружения, валидируется при старте, секреты только из окружения или файла с правами 0600:

```text
FORGE_HTTP_ADDR, FORGE_PUBLIC_ORIGIN, FORGE_SPA_ORIGIN
FORGE_DATABASE_URL (роль forge_app), FORGE_MIGRATE_DATABASE_URL
FORGE_OIDC_ISSUER, FORGE_OIDC_AUDIENCE
FORGE_S3_ENDPOINT, FORGE_S3_REGION, FORGE_S3_BUCKET_*, ключи
FORGE_WORKER_BOOTSTRAP_SECRET_RUNNER, FORGE_WORKER_BOOTSTRAP_SECRET_EVALUATOR
FORGE_SANDBOX_PROVIDER = docker | <vm-provider>
FORGE_DEMO_DOMAIN
```

Разработка: `docker compose` с PostgreSQL 16, MinIO, Dex как OIDC, api, evaluator с Docker-адаптером. Фронт запускается отдельно и указывает на `http://localhost:8080`. Реальные участники в compose не запускаются.

Пилот: один узел api + PostgreSQL с ежедневными резервными копиями и PITR, один узел evaluator, узел runner у sandbox-провайдера. Без Kubernetes.

## 10. Наблюдаемость

- Структурные логи JSON с `request_id`, `organization_id`, `campaign_id`, `run_id`, `attempt_id`, `evaluation_id`. Тела запросов и секреты не логируются.
- Метрики Prometheus: возраст самого старого `queued` job по kind, число активных lease, число `reconciliation_required`, сумма открытых резервов по валюте, расходы за сутки, ошибки 5xx, длительность оценки, время от сдачи до решения.
- Алерты, требующие человека: `reconciliation_required > 0` дольше 10 минут; активная среда после дедлайна миссии; открытые резервы выше лимита кампании; провал резервного копирования.
- Трассировка OpenTelemetry по желанию, экспорт в любой совместимый коллектор; не обязательна для пилота.

## 11. Тестирование

| Уровень | Что | Как |
|---|---|---|
| Доменные правила | переходы состояний, digest контракта, `CanAccept`, расчёт баллов, очистка публикации | табличные unit-тесты в модуле, без БД |
| Репозитории и транзакции | инварианты схемы, RLS, оптимистическая блокировка, идемпотентность | интеграционные тесты на PostgreSQL в контейнере (testcontainers), каждая проверка из таблицы рисков плана реализации — отдельный тест |
| HTTP-контракт | соответствие OpenAPI, коды ошибок, роли | `httptest` + валидатор OpenAPI на каждый ответ |
| Оценщик | сценарии на эталоне и намеренных поломках | тот же пакет, что и калибровка, запускается в CI на fixtures |
| Исполнитель | идемпотентность `CreateOrGet`, потеря lease, stale fencing, отмена | fake-адаптер с управляемыми сбоями |
| Сквозной | «владелец публикует → участник сдаёт → оценщик проверяет → владелец принимает → второй выпуск с миграцией» | compose-стенд, Go-тест через публичный API, плюс Playwright-тест фронта |

Обязательные негативные проверки из плана реализации (подмена требований, чужой доступ, подмена артефакта, двойной запуск, гонка бюджета, потеря worker'а, задержка расходов, граница срока, публичная утечка) оформляются как именованные интеграционные тесты и блокируют merge.

## 12. Порядок реализации по срезам

| Срез | Модули и компоненты | Готово, когда |
|---|---|---|
| 1 | platform/*, identity, campaigns, missions, budget (лимиты без резервов), audit, OpenAPI, миграции, compose | владелец публикует контракт; чужая организация получает 404; опубликованная версия не меняется |
| 2 | artifacts, submissions, evaluation, acceptance (решение, спор), cmd/evaluator с Docker-адаптером, сценарии B-G1…B-G5, SSE | внешний пакет проходит цикл до принятого выпуска; повторная оценка использует те же байты |
| 3 | parent_submission, migration-сценарии, accepted_releases цепочка, release_events, scoreboard, сценарии A-G/A-S | второй выпуск сохраняет данные; регрессия видна; первая приёмка не стирается |
| 4 | execution, cmd/runner, sandbox-адаптер, шлюз, резервы и usage, reconciler, interventions | потеря worker'а не создаёт вторую платную среду; отмена подтверждена; расходы не теряются |
| 5 | usersessions, publication, retention, handoff, calibration, экспорт | эталонный сезон проведён по регламенту, включая случай без победителя |

Каждый срез заканчивается работающим стендом и зелёным набором тестов своего уровня; следующий не начинается с частично сделанного предыдущего.
