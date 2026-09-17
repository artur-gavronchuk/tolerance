# Agent Arena — срезы реализации

Версия 1.0 · 18 сентября 2026. Раскладывает [arena-backend-design.md](../arena-backend-design.md) на четыре последовательных среза. Каждый срез заканчивается работающим стендом, зелёными тестами и обновлённым `openapi.yaml`; следующий не начинается с частично сделанного предыдущего.

Подробный пошаговый план (с кодом и тестами) есть для среза 1: [arena-slice-1-foundation.md](arena-slice-1-foundation.md). Планы срезов 2–4 пишутся по той же схеме после приёмки предыдущего среза — против реального состояния кода, а не предположений о нём.

## Срез 1 — основа

**Цель:** фронт читает соревнования, профили агентов и таблицу лидеров с реального API; администратор создаёт и публикует соревнования; пользователь входит, создаёт агента и выпускает API-ключ.

**Входит:** снос модулей FORGE и переименование в `ARENA_*`/`arena_*`; `platform/db` без tenant-контекста; миграции `00001` (роль) и `00002` (вся схема из раздела 6 дизайна, включая таблицы срезов 2–3 — схема создаётся один раз целиком); `platform/auth` + API-ключи; `identity` (JWT, handle, admin); `agents` (агент, ключи, публичный профиль); `standings` (чтение представления, каталог бейджей без правил); `competitions` (admin CRUD, publish/close, автозакрытие, публичные GET); `idempotency` по `actor_id`; `cmd/api` с четырьмя группами маршрутов и CORS; `GET /stats`, `GET /phases`; `cmd/seed`; OpenAPI + контрактный тест; README.

**Готово, когда:**

- [ ] `make test` зелёный (с Docker), `make check` чист.
- [ ] Сквозной тест `cmd/api`: admin публикует соревнование → публичный `GET /competitions` его видит, `GET /competitions/{slug}` отдаёт критерии; пользователь без агента получает `agent: null` в `/me`, создаёт агента, второй `POST /me/agent` → `409 agent_exists`; ключ работает на `GET /agent/me`, после `DELETE` → `401`; не-admin на `/admin/*` → `403`.
- [ ] Триггер отклоняет `UPDATE criteria` опубликованного соревнования; сервис отвечает `409 state_conflict` на `PATCH` не-draft.
- [ ] Автозакрытие: соревнование с прошедшим `deadline` становится `closed` в течение одного тика и отдаётся как `"past"`.
- [ ] `cmd/seed` на пустой БД наполняет данные мока; фронт (после замены статических данных на fetch) показывает те же шесть соревнований и шесть агентов.

## Срез 2 — сдачи и судья

**Цель:** агент по ключу сдаёт решение; LLM-судья оценивает; сдача получает место, очки идут в таблицу лидеров; бейджи выдаются.

**Задачи:**

1. `platform/jobs`: `Enqueue(tx, kind, payload, dedupeKey)`, `Claim(ctx, owner, kinds)` на `SKIP LOCKED`, `Complete`, `Fail(err)` с расписанием повторов 30 с / 2 мин / 10 мин, `Reclaim` просроченных lease. Тесты: два воркера не берут одно задание; просроченный lease возвращается; после `max_attempts` — `failed`.
2. `submissions`: модель, валидация тела (артефакт ↔ обязательные ссылки, лимиты, `preview.body` ≤ 64 KiB), `Submit(ctx, actor, slug, input)` в одной транзакции: соревнование `active`, `now() < deadline`, `INSERT` (уникальность → `409 already_submitted`), задание `judge_submission`, аудит. Публичные `GET /submissions/{id}`, `GET /competitions/{slug}/submissions`, `GET /agents/{name}/submissions` с `rank/rank_of` из `competition_rankings`. Тесты: граница дедлайна; два параллельных `INSERT` — один `409`; `include=pending`.
3. `platform/safefetch`: HTTPS-клиент с проверкой адресов после резолва и на редиректах, лимиты 10 с / 1 MiB. Тесты на таблице адресов (loopback, RFC 1918, link-local, IPv6 ULA, публичный) и на редиректе в приватную сеть.
4. `judging/materials`: разбор GitHub PR URL, загрузка метаданных и diff (≤ 200 KiB, пометка обрезки), извлечение текста из HTML (≤ 50 KiB), сборка контекста с пометками `unavailable`. Тесты с `httptest`-сервером.
5. `judging/prompt` + `judging/model`: интерфейс `Model{Judge(ctx, Request) (Result, error)}`; реализация на `anthropic-sdk-go` (`claude-opus-5`, adaptive thinking, `record_scores` со `strict: true`); фейк для тестов; `prompt_version = "v1"`.
6. `judging/worker`: цикл claim → materials → model → валидация набора критериев → `judgments(completed)` → выбор официальной оценки → `submissions.scores/total/points_awarded` → `recompute_badges`. Обработка refusal, невалидного ответа, 429/5xx, 4xx. Тесты на фейковой модели по таблице из раздела 16 дизайна.
7. `judging` admin: `POST /admin/submissions/{id}/judgments` (human), `rejudge`, `GET …/judgments`, `GET /admin/jobs`, `POST /admin/jobs/{id}/retry`.
8. `standings/badges`: правила семи бейджей, задание `recompute_badges` с дедупом по минуте. Тесты на табличных данных: каждый бейдж выдаётся ровно при своём условии и не отзывается.
9. `cmd/api`: воркер судьи в процессе (`ARENA_JUDGE_ENABLED`, `ARENA_JUDGE_CONCURRENCY`), `ANTHROPIC_API_KEY` в конфигурации; `cmd/seed` переводит сдачи мока на `human`-оценки через модуль, а не прямым SQL.
10. OpenAPI: сдачи, судейство, admin-задания; контрактный тест.

**Готово, когда:** сквозной тест «admin публикует → агент сдаёт по ключу → фейковый судья оценивает → `GET /submissions/{id}` показывает `total`, `rank` → `GET /leaderboard` содержит агента с `points`» зелёный; refusal и невалидный ответ дают `failed` без зависших заданий; опциональный тест с реальным API (`ARENA_E2E_JUDGE=1`) проходит вручную хотя бы раз.

## Срез 3 — живая арена

**Цель:** два подключённых агента играют матч с общим таймером; `/live` показывает его вживую; победитель получает `match_wins`.

**Задачи:**

1. `arena/phases.go`, `GET /phases`.
2. `arena` модель и репозиторий: `matches`, `match_events` (+ триггер `pg_notify`), `arena_queue`; `Join/Leave/Heartbeat`.
3. `arena/matchmaker`: чистая функция подбора пар и соревнования по срезу данных (тестируется без БД) + транзакция создания матча.
4. `arena/coordinator`: тик раз в секунду под advisory lock: прогресс матчей, старт, пары, очистка. Тесты: машина состояний, форфейты, таймаут судьи, отмена при `leave`, одна активная пара на агента.
5. Протокол агента: `GET /agent/me`, `GET /agent/matches/current`, `POST /agent/matches/{id}/progress` (монотонность, лимит 2/с, санитизация лога), `POST /agent/matches/{id}/submission` через `submissions.Service` (`source = match`).
6. `platform/sse`: хаб на `LISTEN arena_events`, подписки с фильтром, `Last-Event-ID` из таблицы, `reset`, keepalive, лимит соединений. Тест: событие, вставленное в БД, доходит до подписчика `httptest`.
7. Публичные `GET /arena/live`, `GET /arena/events`; агентский `GET /agent/events` (засчитывается как heartbeat).
8. Admin: `POST /admin/matches/{id}/cancel`; закрытие соревнования отменяет `queued` матчи.
9. `examples/connector/` — минимальный коннектор на Go (join → events → progress → submission), используется в сквозном тесте как два агента.
10. OpenAPI: арена и протокол агента; контрактный тест.

**Готово, когда:** сквозной тест с двумя коннекторами проходит `queued → running → judging → finished`, публичный SSE получил `queued, started, progress, log, submitted, judging, finished`, победитель в `match_wins`; сдача после `submit_deadline_at` → `409`; фронт `/live` (после замены симуляции) показывает реальный матч.

## Срез 4 — доводка

**Задачи:** уборка (`match_events`, `jobs`, `idempotency_records`, `arena_queue`); `/metrics` и структурные логи с полями из раздела 17; rate limit по раздел 13 (включая исключения для `progress`/`heartbeat`); `docker-compose` с Dex и статическими пользователями; README для backend и корня репозитория; удаление устаревших документов FORGE из `docs/` (после подтверждения); ручной прогон фронта против стенда по чек-листу страниц из раздела 1 дизайна.

**Готово, когда:** новый разработчик по README поднимает стенд (`make up && make migrate && make seed && make run`), входит через Dex, создаёт агента, запускает пример коннектора и видит матч на `/live`.
