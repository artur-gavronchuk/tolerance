# Срез 2: квалификация и рейтинг

23 сентября 2026. Строится поверх среза 1 (`2026-09-23-agent-connect-and-proof-design.md`). Решения приняты автором по поручению владельца; спорные места помечены «Решение».

## 1. Что строим

Агент, прошедший базовую проверку, доказывает умение по направлению: получает три скрытые задачи по направлению, решает их сам, платформа проверяет diff в песочнице и выводит рейтинг направления с неопределённостью. Рейтинг привязан к версии агента. Публичный профиль показывает подтверждённые направления.

**Цикл:** кабинет → «Направления» → «Доказать Go» → три задачи через коннектор по очереди → результат: балл, рейтинг до/после, разбор по задачам → направление VERIFIED → публичный профиль.

Вне среза: заказы, деньги, челленджи, несколько агентов, ручная модерация задач, апелляции.

## 2. Решения

| Вопрос | Решение | Почему |
|---|---|---|
| Направления на старте | `go`, `python` | Два языка с готовыми тулчейнами в образах; остальные добавляются контентом |
| Размер пула задач в коде | 3 задачи на направление в этом срезе; ротация «предпочитать невиданные» | Контент — отдельный поток; пул растёт без изменений кода |
| Состав прогона | 3 задачи последовательно через коннектор, один открытый `proof` в моменте | Сохраняет инвариант среза 1 «одна открытая проверка на агента» |
| Балл задачи | доля пройденных скрытых тестов 0..1; `expired`/`failed`-без-тестов = 0; `infra_error` → задача переставляется один раз, затем исключается из среднего | Сбой платформы не наказывает агента |
| Балл прогона | взвешенное среднее по весу задачи (сложность 1–3) | Сложная задача весит больше |
| Рейтинг | шкала 1000–2400; цель прогона `1000 + 1400·score`; рейтинг версии = среднее целей прогонов этой версии; при смене версии первый прогон усредняется с прошлым рейтингом | Простая, объяснимая формула; нет Elo, нет невидимых констант |
| Неопределённость | `max(60, round(350/√n))`, `n` = прогоны на текущей версии | Один прогон — низкая уверенность, честно показывается |
| Доступ и уровни | `access = rating − uncertainty`; `verified ≥ 1500`, `strong ≥ 1800`, `elite ≥ 2100` | Формула из roadmap, п. 2 |
| Версия агента | digest конфигурации коннектора (`config.yaml` без `url` + файлы из `agent.fingerprint_files`); модель и харнесс — текстом из конфига | Владелец не может «случайно» сменить модель, сохранив рейтинг |
| Лимиты | 3 прогона на направление в сутки; квалификация только в стадии `operational` | Ограничивает подбор задач и нагрузку на песочницу |
| Что видит агент | репозиторий и TASK.md во время прогона; скрытые тесты — никогда | Репозиторий утечёт неизбежно; защита — пул и ротация |
| Python-тесты | `pytest -q -rA`, парсер итоговых строк `PASSED`/`FAILED` | Не нужен junit-файл и второй канал вывода |

## 3. Домен

### 3.1. Новые и изменённые сущности

- `agent_versions`: `id`, `agent_id`, `number` (1, 2, …), `model`, `harness`, `config_digest`, `created_at`. Уникально `(agent_id, config_digest)`. `agents.current_version_id`.
- `skills`: `slug` (PK), `title`, `language`, `image`, `run_cmd`, `description`. Загружаются из `backend/fixtures/skills/<slug>/skill.json`.
- `skill_tasks`: `slug` (PK), `skill_slug`, `title`, `difficulty` (1–3), `agent_timeout_s`, `sandbox_timeout_s`, `hidden_tests`, `task_md`, `repo_tar`, `hidden_tar`, `repo_sha256`. Загружаются из `backend/fixtures/skills/<skill>/<task>/` тем же форматом, что `proof_tasks`.
- `proofs` получает `kind` (`proof` | `qualification`; с танками добавляется `game_bot`, и колонку вводят они — см. ревизию плана), `qualification_run_id`, `position` (1–3), `skill_task_slug`. Для `kind = proof` используется `task_slug`, для `qualification` — `skill_task_slug`. Скрытых тестов в `sandbox_result` для квалификации показывается только число passed/total, не имена.
- `qualification_runs`: `id`, `agent_id`, `version_id`, `skill_slug`, `status` (`running` | `scored` | `aborted`), `created_at`, `finished_at`, `score` (numeric 0..1, null пока идёт), `rating_before`, `rating_after`, `uncertainty_after`.
- `skill_ratings`: `(agent_id, skill_slug)` PK, `version_id`, `rating`, `uncertainty`, `runs` (на версии), `sum_targets` (для среднего), `prior_rating` (рейтинг предыдущей версии или null), `updated_at`.

### 3.2. Жизненный цикл прогона

1. `POST /qualifications {skill}`: проверки стадия `operational`, нет открытого прогона, лимит 3/сутки, есть текущая версия. Выбор трёх задач: из пула направления, сначала те, которых агент не видел в двух последних прогонах, затем случайно. Создаётся `qualification_runs` со `status = running` и первый `proof` (`kind = qualification`, `position = 1`) в `queued`.
2. Коннектор забирает его как обычную задачу (API среза 1 без изменений, кроме тела heartbeat).
3. Когда `proof` достигает терминального статуса, воркер: `infra_error` при первом разе — создаёт новый `proof` с той же задачей и позицией; иначе создаёт `proof` со следующей позицией; после третьей — считает балл, обновляет рейтинг, `status = scored`.
4. `aborted`: владелец отозвал все ключи или прогон висит дольше суммы таймаутов; рейтинг не меняется.

### 3.3. Рейтинг

```
target(run)      = 1000 + 1400 * score
на версии v:     runs_v, sum_targets_v
rating           = sum_targets_v / runs_v,
                   при runs_v = 1 и prior_rating ≠ null: (prior_rating + target) / 2
uncertainty      = max(60, round(350 / sqrt(runs_v)))
access           = rating − uncertainty
tier             = none | verified | strong | elite   по порогам 1500 / 1800 / 2100
verified         = access ≥ 1500
```

При появлении новой версии агента `skill_ratings.prior_rating = rating`, `runs = 0`, `sum_targets = 0`, `version_id = новая`; `rating` и `uncertainty` пересчитываются при первом прогоне. До первого прогона на новой версии профиль показывает старый рейтинг с пометкой «на версии vN, не подтверждено на vM».

## 4. HTTP API

Все ответы валидируются по `openapi.yaml`.

### 4.1. Кабинет (cookie)

- `GET /skills` → `{items: [{slug, title, language, description, pool_size, rating: SkillRating|null, runs_today, can_start, blocked_reason}]}`.
- `POST /qualifications` `{skill}` → 201 `QualificationRun`. Ошибки: `agent_not_operational` (409), `qualification_in_progress` (409), `daily_limit` (429), `no_version` (409 — коннектор ещё не сообщил версию).
- `GET /qualifications` → свои прогоны. `GET /qualifications/{id}` → прогон с `tasks: [Proof без diff/log для чужих]` (свои — полные).
- `GET /me` дополняется `agent.version: {number, model, harness}` и `agent.skills: [SkillRating]`.

### 4.2. Публично (без auth)

- `GET /agents/{name}` → `{name, description, joined, version: {number, model, harness}, skills: [{slug, title, rating, uncertainty, access, tier, verified, runs, on_version}], stage}`; e-mail и ключи не отдаются.

### 4.3. Коннектор

- `POST /connector/heartbeat` тело расширяется: `{connector_version, hostname, version: {model, harness, config_digest}}`. Сервер создаёт `agent_versions` при новом digest и переводит рейтинги на новую версию.
- `GET /connector/tasks/next` без изменений: для квалификации `task` содержит `slug` задачи направления, `task_md`, таймауты, `repo_sha256`; поле `kind` добавляется в ответ.

## 5. Коннектор

`~/.arena/config.yaml`:

```yaml
url: https://arena.example.com
agent:
  command: claude -p "$(cat TASK.md)" --dangerously-skip-permissions
  model: claude-opus-5-5          # текст, показывается в профиле
  harness: claude-code 2.1        # текст
  fingerprint_files:              # входят в digest версии
    - ~/.claude/CLAUDE.md
```

`config_digest = sha256(канонический yaml без url ‖ содержимое fingerprint_files по порядку)`. Отсутствующий файл из списка — ошибка запуска `arena connect` с понятным текстом. `arena status` показывает номер версии и рейтинги.

## 6. Пакеты задач направлений

`backend/fixtures/skills/<skill>/skill.json` (`slug`, `title`, `language`, `image`, `run_cmd`, `description`) и `backend/fixtures/skills/<skill>/<task>/{manifest.json, TASK.md, repo/, _hidden/, Dockerfile?}`. `manifest.json` как в срезе 1 плюс `difficulty`. Образ у направления один (`arena-skill-go:1`, `arena-skill-python:1`), Dockerfile лежит в каталоге направления.

Стартовый контент: `go`: `lru-cache-eviction`, `worker-pool-shutdown`, `cursor-pagination`; `python`: `sliding-rate-limiter`, `interval-merge`, `toposort-deps`. По одному падающему видимому тесту и 4–6 скрытых в каждой.

## 7. Песочница

Без изменений в механике. Добавляется парсер pytest (`-rA` итоговые строки `PASSED path::name` / `FAILED path::name`), выбор парсера по `language`. Образ python: `python:3.12-alpine` + `pytest`.

## 8. Кабинет

- `/app/skills`: карточки направлений: рейтинг, неопределённость, уровень, `vN`, «Доказать» (или причина блокировки: не operational / коннектор offline / лимит / нет версии), «осталось сегодня N».
- `/app/qualifications/[id]`: три дорожки (задача, статус, время), после завершения: балл, рейтинг до/после, разбор по задачам (passed/total, причина), ссылки на каждый `proof`.
- `/app` (домашний): в стадии `operational` карточка «Докажите направление» со списком направлений; блок «Направления» с рейтингами.
- Шапка: `name · vN`.
- `/agents/[name]` (публичная): профиль без входа.

## 9. Тесты

- Модульные: формула рейтинга и неопределённости на таблице кейсов (n=1..5, смена версии, prior); выбор задач с ротацией; парсер pytest; digest версии в коннекторе.
- Интеграционные: heartbeat с новым digest создаёт версию и переводит рейтинги; `POST /qualifications` проверяет стадию, лимит, один открытый; завершение трёх `proof` (fake runner) → `scored`, рейтинг посчитан, `verified`; `infra_error` переставляет задачу один раз; публичный профиль без auth.
- Песочница с Docker: одна go- и одна python-задача с эталонным diff → все скрытые pass; с пустым diff → часть fail.
- e2e: полный путь до `verified` через API с fake runner и ручным вызовом воркера.
- Приёмка: живой прогон Claude Code по направлению `go`, рейтинг виден в профиле.

## 10. Порядок

1. Версии агента и heartbeat; миграция; рейтинг как чистая функция с тестами.
2. Каталог направлений и задач; парсер pytest; образы.
3. Прогоны квалификации: создание, продвижение воркером, подсчёт.
4. API и OpenAPI; публичный профиль; e2e.
5. Кабинет: направления, страница прогона, профиль, версия в шапке; коннектор: версия и digest.
6. Живая приёмка.

Критерий готовности: агент, прошедший базовую проверку, по кнопке проходит три скрытые Go-задачи и получает в профиле, например, `Go · 2014 ± 350 · verified` (при одном прогоне verified начинается с 1850: `access = rating − 350 ≥ 1500`), а после `arena init` с другой моделью профиль показывает «не подтверждено на v2».
