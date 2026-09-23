# Этап 5 — live-экран и приёмка (T25–T27)

Решение D12 (опрос вместо SSE, один назначенный матч) — в [00-overview.md](00-overview.md). Этот этап начинается **только** после того, как этапы 1–4 закрыты по своим разделам «Готово, когда». Если время поджимает — T25–T26 можно отложить целиком, выполнив лишь T26 Step 1 (убрать симуляцию из рабочего режима, показать пустое состояние) и перейти к T27: основной сценарий важнее трансляции.

---

### Task 25: Назначенный матч на бэкенде

**Files:** Create: `backend/migrations/00004_assigned_matches.sql`, `backend/internal/arena/model.go`, `service.go`, `http.go`, `outcome.go`, `outcome_test.go`, `service_integration_test.go`. Modify: `internal/arena/phases.go` (не трогать каталог, только соседство), `internal/attempts/service.go` (привязка попытки к матчу — пункт 5 правил `Start` из T6), `internal/checks/service.go` (`Finalize`/`MarkUnverifiable`/`MarkFailed` вызывают `arena.OnSubmissionSettled(ctx, tx, submissionID)` через интерфейс, переданный в конструктор, — без импорта `arena` из `checks`), `cmd/api/handler.go`, `main.go` (тикер раз в 5 с), OpenAPI.

- [ ] **Step 1: миграция.**

```sql
-- +goose Up
ALTER TABLE matches
    DROP COLUMN left_progress, DROP COLUMN left_phase, DROP COLUMN right_progress, DROP COLUMN right_phase,
    ADD COLUMN left_attempt_id text REFERENCES attempts (id),
    ADD COLUMN right_attempt_id text REFERENCES attempts (id),
    ADD COLUMN created_by text REFERENCES users (id),
    ADD COLUMN note text;
-- outcome: добавить 'draw' и 'no_result' в CHECK (пересоздать ограничение matches_outcome_check)
```

  Таблицы `match_events` и `arena_queue` остаются пустыми и неиспользуемыми (удаление — вместе с будущей переработкой арены; отметить в дизайне §19).
- [ ] **Step 2: правила.**
  - `POST /admin/matches` `{"competition_slug","left_agent","right_agent","total_seconds"?,"note"?}` → `201 Match` в `queued`. Проверки: соревнование `active`; агенты различны и существуют; ни у одного нет активного матча (существующие частичные уникальные индексы → `409 agent_busy`); `total_seconds` по умолчанию `competition.match_duration_seconds`, допускается 300–7200 (снять старый верхний предел в коде, не в БД — в `matches` ограничения нет).
  - Привязка: `attempts.Start` для агента с матчем `queued | running` на это соревнование и пустым `*_attempt_id` своей стороны → проставляет `attempts.match_id` и `matches.<side>_attempt_id`. Первая привязка: `queued → running`, `started_at = now()`, `submit_deadline_at = started_at + total_seconds`. Вид попытки не меняется матчем: у кого официальная ещё не использована и он её запускает — она официальная; иначе тренировочная. Матч сравнивает результаты именно привязанных попыток.
  - Тикер и `OnSubmissionSettled`: `running → judging`, когда обе стороны сдали **или** `now() ≥ submit_deadline_at` (сторона без сдачи считается не сдавшей; её попытка не трогается — агент может досдать в общий зачёт соревнования, но не в матч); `judging → finished`, когда у каждой имеющейся сдачи `score_status ∈ {scored, unverifiable, failed}` или прошло 15 минут.
  - **`outcome.go`** (чистая функция, таблица-тест): оба `scored` → больший `total`; равенство → больший `functional_score`; снова равенство → `draw`, победителя нет (никакой случайности и никакого «кто раньше»); один `scored`, другой нет сдачи/`unverifiable` → `forfeit_*`; у любой стороны `failed` (ошибка платформы) → `no_result`, победителя нет — сбой проверяльщика не даёт сопернику победу; обе без результата → `double_forfeit`.
  - `POST /admin/matches/{id}/cancel` `{"reason"}` из любого незавершённого состояния.
- [ ] **Step 3: чтение.**
  - `GET /arena/live?after=<event_id>` → `{"server_now", "current": Match | null, "last_finished": Match | null, "upcoming": [Match], "events": [Event], "last_event_id"}`. `current` — матч в `running | judging`; если его нет — `last_finished` (последний `finished`), чтобы экран не был пустым; `upcoming` — `queued`. `events` — события обеих привязанных попыток с `id > after` (первый запрос без `after` отдаёт последние 50), каждое: `{id, side, kind, phase_index, phase, text, at}`.
  - `Match = {id, state, competition:{slug,title,category}, total_seconds, started_at, submit_deadline_at, judging_at, finished_at, left: Side, right: Side, winner_side, outcome, note}`; `Side = {agent, author, season_points, attempt: {id, kind, no} | null, phase_index | null, phase | null, submitted: bool, submission_id | null, score_status | null, total | null, functional_score | null}`. **Нет** полей `progress` и `rating`. `phase_index` — из последнего события `phase` попытки.
  - `GET /arena/matches/{id}` → `Match` + **все** события обеих попыток по порядку — для воспроизведения.
  - `GET /arena/matches?state=finished&limit=20` → список для перехода к повторам.
- [ ] **Step 4: тесты** — `TestOutcome_Table` (все строки правил выше, включая `draw` и `no_result`); `TestMatch_AttachOnAttemptStart`; `TestMatch_RunningToJudgingOnBothSubmitted`; `TestMatch_DeadlineForfeit`; `TestMatch_PlatformFailureIsNoResult`; `TestMatch_AgentBusy_409`; `TestLive_AfterCursorReturnsOnlyNewEvents`; `TestLive_FallsBackToLastFinished`; `TestMatchWins_CountedInStandings` (представление `agent_standings.match_wins` растёт только при `winner_agent_id`).
- [ ] **Step 5.** `make test && make check` → PASS. Commit: `Add admin-assigned matches driven by real attempt events`.

---

### Task 26: Live-экран на реальных событиях

**Files:** Rewrite: `frontend/components/live-arena.tsx`. Create: `frontend/components/live/use-live.ts`, `fighter-panel.tsx`, `event-feed.tsx`, `match-replay.tsx`, `frontend/app/live/demo/page.tsx`, `frontend/components/live/demo-arena.tsx`, `frontend/app/live/[id]/page.tsx`. Modify: `frontend/app/live/page.tsx`, `frontend/components/admin/*` (форма «Assign match»).

- [ ] **Step 1: вынести симуляцию.** Текущий `live-arena.tsx` целиком переезжает в `components/live/demo-arena.tsx` (импорты — из `lib/demo/arena.ts`), страница `/live/demo` рендерит его под жёлтой плашкой «Demo — simulated match, random winner, not real agents». Подпись `elo` удалить и там. С `/live` — маленькая ссылка «See a simulated demo» только в пустом состоянии.
- [ ] **Step 2: `use-live.ts`.** Опрос `GET /arena/live?after=<last_event_id>` каждые 2 с (пауза при `document.hidden`), накопление событий в состоянии (последние 200), таймер — от `server_now` и `submit_deadline_at` (локальные часы не используются как источник истины), ошибка сети → ненавязчивая строка «Reconnecting…», данные не сбрасываются.
- [ ] **Step 3: вёрстка сохраняется** — два агента по центру, VS и таймер между ними, очередь снизу. `FighterPanel`: аватар, агент, `@author · N season points`, `attemptLabel`; **вместо полосы процента** — «рельс» из 9 фаз: текущая подсвечена, уже посещённые помечены точкой, возврат к ранней фазе просто перемещает подсветку (никаких «−%»); под ним лента последних 6 событий стороны (`kind`-иконка, текст, относительное время); бейджи состояния: `Working` → `Preview available` → `Build finished` → `Submitted` → `Checking` → итог (`total` и `functional N/8`, либо `Could not be verified`, либо `Platform error`). Центр: стадия (`Live now` / `Checking` / `Match complete`), таймер, после завершения — победитель **из API** (`winner_side`) или «Draw» / «No result — platform error» / «Both forfeited». Кнопки «Open result» у каждой стороны и «Compare» (→ `/compare`), когда есть обе сдачи.
- [ ] **Step 4: пустое состояние и повторы.** Нет `current` → показать `last_finished` с пометкой «Last finished match · <date>» и кнопкой «Replay»; нет ни того ни другого → «No match is scheduled. Matches are assigned by the organisers during the pilot.» + ссылка на демо. `/live/[id]` (`MatchReplay`): `GET /arena/matches/{id}`, воспроизведение событий по их реальным меткам времени со скоростью ×1 / ×10 / ×60, пауза, перемотка ползунком; итог показывается только в конце дорожки.
- [ ] **Step 5: админка.** Форма «Assign match»: соревнование, два агента, длительность, заметка; список матчей с Cancel.
- [ ] **Step 6.** Браузер: матч двух тестовых исполнителей (две консоли с `arena-connector run … --adapter command`) виден вживую; повтор воспроизводится; desktop + 375px. `pnpm typecheck && pnpm build`. Commit: `Drive the live screen from real attempt events and add replay`.

---

### Task 27: Сквозная приёмка, документация, отчёт

**Files:** Create: `scripts/e2e-local.sh`, `backend/docs/mvp-acceptance-report.md`. Modify: `README.md`, `backend/README.md`, `frontend/README.md` (создать, если нет), `checker/README.md`, `backend/docs/arena-backend-design.md` (финальная сверка §19 с фактом), `backend/docs/plans/arena-slices.md`.

Перед началом загрузить скилл `superpowers:verification-before-completion`: в отчёт попадает только то, что подтверждено выводом команд в этой сессии.

- [ ] **Step 1: стенд с нуля.** `docker compose down -v` (только локальные dev-тома этого проекта) → `make up migrate seed-task` → API (`ARENA_ALLOW_LOOPBACK_PREVIEW=true`, `ARENA_CHECKER_TOKEN=$(openssl rand -hex 32)`, `ARENA_JUDGE_ENABLED=false`) → `make checker-local` → `pnpm dev`. Каждая команда и её фактический вывод — в отчёт.
- [ ] **Step 2: основной прогон по чек-листу ТЗ (в браузере, со скриншотами desktop + 375px):** 1) админ входит, публикует соревнование в `/admin`; 2) `dev@arena.local` входит, создаёт агента; 3) выпускает ключ, `arena-connector check`, индикатор «Connected»; 4) `arena-connector run` с тестовым исполнителем (официальная попытка) → ссылка на результат; 5) страница результата: 8 проверок, скриншоты, объяснения; 6) работа видна в соревновании и профиле; 7) `dev2@arena.local` проходит тот же путь, но с фикстурой `no-persistence` (добавить второй скрипт `examples/agents/copy-variant.sh <variant>`) → 90/100; 8) `/compare` показывает разницу по `persistence`.
- [ ] **Step 3: отдельные проверки.** Отзыв ключа → `check` и `run` дают `401`, понятное сообщение коннектора. Повторная отправка → `409 already_submitted`; повтор с тем же `Idempotency-Key` → тот же ответ. Дедлайн: соревнование с дедлайном через 2 минуты (создать через API) → сдача после → `409 deadline_passed`; автозакрытие переводит в `past`. Недоступный preview: сдача с `--preview-url https://arena-mvp-unreachable.invalid/` → `unverifiable`, без очков, не в рейтинге, причина на странице. Сбой проверяльщика: остановить checker посреди прогона (`kill -9`) → после истечения lease задание подхвачено, результат один; затем смоделировать три `infra_error` (переменная `CHECKER_FORCE_INFRA_ERROR=1` — добавить в `worker.ts` только для этого теста, задокументировать) → `failed`, подпись «platform error», очков нет → admin «Re-run checks» → `scored`. **Двойное начисление:** после recheck и после override `SELECT points FROM agent_standings` равен `points_awarded` единственной официальной сдачи; тренировочная сдача того же агента очков не добавила.
- [ ] **Step 4: реальный coding agent.** Проверить `claude --version` и что CLI авторизован (`claude -p "say ok" --max-budget-usd 0.05`). Если да: `arena-connector run --competition city-day-planner --practice --adapter claude-code --serve-local . --max-budget 5 --timeout 40m --yes` третьим пользователем (или тренировочной попыткой `dev`). Это расходует квоту владельца — один прогон, лимит 5 USD, без повторов «до победы». В отчёт: версия CLI, фактическая команда, длительность, заявленная стоимость, результат проверок **как есть** (низкий балл — это результат агента, а не дефект интеграции; дефект интеграции — если коннектор не довёл до сдачи). Если CLI не авторизован или недоступен — раздел отчёта «Проверено: инфраструктура на тестовом исполнителе. НЕ проверено: интеграция с реальным Claude Code» с точной причиной.
- [ ] **Step 5: полный набор автоматических проверок.** `cd backend && make test && make check` (приложить итог и `grep -c SKIP`); `cd checker && pnpm test`; `cd frontend && pnpm typecheck && pnpm test && pnpm build`. Любой SKIP интеграционных тестов из-за Docker — отдельной строкой «SKIPPED, не пройдено».
- [ ] **Step 6: README по факту.** Корневой: что это, карта каталогов (`backend`, `frontend`, `checker`), «стенд за 10 минут» (порядок команд Step 1), ссылка на `docs/connector.md`. `backend/README.md`: статус реализации (срез 1 + MVP), переменные окружения, цели Makefile (`seed-demo` / `seed-task`, `connector`, `checker-local`, `checker-up`), Dex и тестовые учётные записи с пометкой dev-only, раздел «Что использует тестовые данные». Убрать утверждения, переставшие быть правдой (SSE, матчмейкинг, «фронтенд генерирует клиент из OpenAPI»).
- [ ] **Step 7: `mvp-acceptance-report.md`** — ровно разделы из ТЗ: **Что работает** · **Как поднять стенд и подключить агента** · **Результаты проверок** (таблица: проверка → команда → итог → дата) · **Что использует тестовые данные** (Dex-пользователи, `cases.dev.json` в репозитории, эталонные фикстуры, `seed-demo`, демо-режим фронта, `/live/demo`) · **Реальные ограничения** (минимум: локальный запуск не исключает помощь человека; preview URL изменяем, оценка привязана к снимкам на момент проверки; `commit_link` — заявление, не доказательство; DNS rebinding и egress-политика checker; скрытые случаи лежат в репозитории до выноса; evidence в PostgreSQL; нет бинарных релизов коннектора; LLM-оценка — с реальным API проверена/не проверена; лимиты не распределённые; один агент на пользователя) · **Следующий минимальный шаг до первого внешнего участника** (публичный стенд за TLS с настоящим OIDC-провайдером и egress-фильтром для checker; вынос `cases` из репозитория; собранные бинарники коннектора; один прогон с внешним человеком по `docs/connector.md` без подсказок).
- [ ] **Step 8.** Сверить `00-overview.md → Отклонения` с фактом; обновить память проекта не требуется. Commit: `Add MVP acceptance report and update READMEs`.

## Готово, когда

- [ ] Все восемь пунктов «Проверка готовности» из [SPEC.md](SPEC.md) отмечены в отчёте с доказательством (команда + вывод или скриншот).
- [ ] Отчёт честно разделяет «проверено» и «не проверено»; в нём нет слов «должно работать».
- [ ] В рабочем режиме нет случайного победителя, процентов прогресса, подписи Elo и выдуманной стоимости: `grep -rn "Math.random" frontend/app frontend/components | grep -v "components/live/demo-arena"` → пусто.
- [ ] Критерий успеха ТЗ: человек, не видевший проект, по корневому README и `docs/connector.md` проходит от входа до проверенной работы и по странице результата может объяснить, за что сняты баллы.
