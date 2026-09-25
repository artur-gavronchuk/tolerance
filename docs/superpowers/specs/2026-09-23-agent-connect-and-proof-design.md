# Срез 1: подключение агента и базовая проверка

23 сентября 2026. Одобрено владельцем в обсуждении.

## 1. Что строим

Платформа становится рынком труда для автономных AI-агентов: владелец подключает своего агента, агент доказывает, что умеет работать, и потом получает доступ к оплачиваемым заданиям. Этот срез делает только первый шаг цикла, от начала до конца и без имитаций:

**регистрация → кабинет → создать агента и ключ → коннектор держит агента online → агент выполняет одну проверочную задачу у владельца → платформа проверяет diff в своей песочнице → результат в кабинете.**

Вне среза: рейтинг, категории навыков, квалификация, заказы, деньги, версии агента, автопилот, сторона заказчика, публичные страницы, подтверждение почты, вход через GitHub.

## 2. Решения

| Вопрос | Решение | Почему |
|---|---|---|
| Как агент подключается | Коннектор `arena` (CLI на Go) у владельца: long-poll задач, запуск агента локально, отправка diff | Ключи модели и код агента не покидают машину; подходит для Claude Code, Codex и любого скрипта; совпадает с текущим бэкендом (API-ключи, санитизация событий) |
| Где проверяется результат | Песочница на платформе: Docker-контейнер без сети на VPS, скрытые тесты | Результат нельзя подделать, скрытые тесты не утекают; уходим от `self_reported` |
| Вход | Email и пароль в своём бэкенде, сессия в HttpOnly-cookie | Ноль внешних сервисов, работает на VPS сразу; Dex и OIDC удаляются — заменено 25 сентября: `specs/2026-09-25-oauth-login-design.md` |
| Фронт | Кабинет владельца из архива `proofwork-frontend-development`, обрезанный до нужных экранов, все данные из API | Лучшие экраны прогона проверки из трёх макетов; текущий `frontend/` целиком на моках |
| Старый домен арены | Удаляется | Соревнования, сдачи, стендинги, live, бейджи, Dex не нужны новому продукту; история в git |
| Один агент на пользователя | Остаётся | Ограничение уже в схеме, срезу достаточно |

## 3. Домен

### 3.1. Сущности

- `users`: `id`, `email` (уникальный, нижний регистр), `password_hash` (argon2id), `created_at`. Роль admin по списку e-mail из конфигурации остаётся.
- `sessions`: `id` (случайные 256 бит, в cookie только его хэш), `user_id`, `created_at`, `expires_at` (30 дней), `last_seen_at`.
- `agents`, `api_keys`: как сейчас (имя, описание, ключ показывается один раз, в базе SHA-256, до 5 активных).
- `agent_presence`: `agent_id` (PK), `last_seen_at`, `connector_version`, `hostname`. Обновляется heartbeat'ом коннектора не чаще раза в 10 секунд.
- `proof_tasks`: каталог проверочных задач, загружается из `backend/fixtures/proofs/` при миграции или старте: `slug`, `title`, `language`, `image` (Docker-образ для прогона), `agent_timeout_s`, `sandbox_timeout_s`, `visible_tests` (число), `hidden_tests` (число), `repo_sha256`.
- `proofs`: один прогон проверки. `id`, `agent_id`, `task_slug`, `status`, `created_at`, `claimed_at`, `diff_submitted_at`, `finished_at`, `diff` (text, до 256 KiB), `agent_log_tail` (text, санитизированный, до 32 KiB), `agent_duration_ms`, `sandbox_result` (jsonb: список тестов с passed/failed, stdout-хвост, код выхода), `failure_reason` (text).

### 3.2. Статусы `proofs`

`queued → claimed → running_agent → diff_submitted → running_sandbox → passed | failed | infra_error | expired`

- `expired`: коннектор не забрал задачу за 5 минут или не прислал diff за `agent_timeout_s` + 60 секунд.
- `infra_error`: песочница не запустилась или упала не из-за diff (нет образа, таймаут docker, сбой платформы). Никогда не засчитывается как провал агента; можно перезапустить.
- `failed`: diff не применился, тесты не собрались или хотя бы один скрытый тест упал. В `sandbox_result` видно, какой.

Одновременно у агента не больше одной незавершённой проверки. Лимит: 10 запусков в сутки на агента. Переводы в `expired` делает периодическая задача воркера раз в 30 секунд.

### 3.3. Стадия агента

Не хранится, вычисляется:

- `registered`: нет активного ключа или presence никогда не было.
- `offline`: presence есть, но `last_seen_at` старше 2 минут.
- `connected`: presence свежая, нет ни одной `passed` проверки, нет незавершённой.
- `checking`: есть незавершённая проверка.
- `operational`: есть хотя бы одна `passed`.
- `check_failed`: последняя завершённая проверка `failed`, нет ни одной `passed`.

## 4. HTTP API

Префикс `/api/v1`. Ответы и ошибки валидируются по `openapi.yaml` в e2e-тестах, как сейчас.

### 4.1. Сессии (cookie)

- `POST /auth/signup` `{email, password}` → 201, ставит cookie. Пароль от 10 символов. Дубликат e-mail → 409.
- `POST /auth/login` `{email, password}` → 200, cookie. Неверные данные → 401 с одинаковым сообщением. Лимит: 10 попыток в минуту на e-mail и на IP.
- `POST /auth/logout` → 204, удаляет сессию.
- `GET /me` → пользователь, агент (или null), стадия агента, presence, последняя проверка.

### 4.2. Кабинет (cookie)

- `POST /agent` `{name, description}` → агент. Второй → 409.
- `PATCH /agent` `{name?, description?}`.
- `POST /agent/keys` → `{id, prefix, key}`; `key` только в этом ответе. `DELETE /agent/keys/{id}`.
- `GET /proof-tasks` → каталог (без скрытого).
- `POST /proofs` `{task_slug}` → `queued` проверка. 409 если уже есть незавершённая или агент offline. 429 при лимите.
- `GET /proofs` (свои), `GET /proofs/{id}` (полный: diff, лог, результат песочницы).
- `POST /proofs/{id}/retry` для `infra_error` и `expired`.

### 4.3. Коннектор (заголовок `Authorization: Bearer <api key>`)

- `POST /connector/heartbeat` `{connector_version, hostname}` → `{agent: {name, stage}}`.
- `GET /connector/tasks/next?wait=25` long-poll до 25 секунд → 204 или `{proof_id, task: {slug, title, agent_timeout_s, repo_url, task_md}}`. Атомарно переводит `queued → claimed`. `repo_url` — подписанная ссылка на tarball, живёт 10 минут.
- `POST /connector/proofs/{id}/started` → `running_agent`.
- `POST /connector/proofs/{id}/result` `{diff, log_tail, duration_ms, exit_code}` → `diff_submitted`, ставит job `run_proof`. Diff больше 256 KiB → 413. Повторная отправка → 409.

Санитизация `log_tail` на сервере повторяется независимо от коннектора (ANSI, секреты по шаблонам, как в `attempts/sanitize.go`).

## 5. Коннектор `cmd/arena`

Один статический бинарник, `go install tolerance/cmd/arena@latest` или скачать со страницы подключения.

- `arena login` читает ключ из stdin, пишет в `~/.arena/key` с правами 0600. Ключ не принимается аргументом командной строки.
- `arena init` создаёт `~/.arena/config.yaml`:

```yaml
agent:
  command: claude -p "$(cat TASK.md)" --dangerously-skip-permissions
  # любая команда; запускается в корне репозитория задачи, TASK.md лежит рядом
```

- `arena connect`: heartbeat каждые 30 секунд, long-poll задач. На задачу: временная папка, скачать и распаковать tarball, проверить `sha256`, `git init` и коммит исходного состояния, записать `TASK.md`, запустить `agent.command` через `sh -c` с таймаутом `agent_timeout_s`, stdout и stderr в локальный файл, после завершения `git add -A && git diff --cached` (без `.git`, без бинарных файлов больше 1 MiB), санитизировать хвост лога, отправить результат, удалить папку. Всё, что агент пишет в консоль, на платформу не уходит, кроме санитизированного хвоста в 32 KiB.
- `arena status`: стадия агента и последняя проверка.
- При потере сети: повтор с backoff, задача в работе не теряется до `agent_timeout_s`.

## 6. Проверочные задачи

`backend/fixtures/proofs/<slug>/`:

```
manifest.json     slug, title, language, image, agent_timeout_s, sandbox_timeout_s, run_cmd
TASK.md           что видит агент
repo/             проект с видимыми тестами; один или несколько падают
hidden/           тесты, которых нет в repo/; копируются поверх перед прогоном
Dockerfile        образ с тулчейном; собирается при деплое, тег из manifest
```

Первая задача `go-fix-retry`: небольшой Go-пакет с функцией повтора запроса, баг в расчёте задержки, три видимых теста (один падает), пять скрытых. `run_cmd`: `go test ./... -json`. Вторая задача (Python, pytest) после запуска первой.

Каталог грузится в `proof_tasks` командой `cmd/migrate` (идемпотентно по `slug`, обновляет поля при изменении).

## 7. Песочница

Воркер очереди `jobs` запускается в `cmd/api` (сейчас не запускается вообще, это чинится в этом срезе). Job `run_proof`:

1. Прочитать `proofs` и `proof_tasks`, перевести в `running_sandbox`.
2. Временная папка: копия `repo/`, `git apply --index` diff'а (отказ → `failed`, причина `diff_not_applicable`), копия `hidden/` поверх.
3. `docker run --rm --network none --memory 1g --cpus 1 --pids-limit 256 --read-only --tmpfs /tmp -v <dir>:/work:ro -w /work <image> sh -c "<run_cmd>"` с таймаутом `sandbox_timeout_s`. Копия рабочей папки монтируется в контейнер, запись только в tmpfs.
4. Разбор вывода: для Go `go test -json`, для Python `pytest --junitxml`. Результат в `sandbox_result`. Все скрытые тесты прошли → `passed`, иначе `failed`. Таймаут тестов → `failed` с причиной `timeout`. Ошибка docker или отсутствие образа → `infra_error`, повтор через backoff до 3 раз (механизм очереди уже есть).

Интерфейс `sandbox.Runner` с двумя реализациями: `docker` и `fake` для тестов. В compose контейнеру api монтируется `/var/run/docker.sock`; это осознанный компромисс первого среза, отдельный runner-сервис позже.

## 8. Кабинет

Заменяет `frontend/` целиком. База: `proofwork-frontend-development` (shadcn, токены, компоненты `proof/*`, `home/*`, `app-shell/*`). Переносятся только нужные экраны, моки и `preview-state` удаляются.

| Маршрут | Что показывает | Действия |
|---|---|---|
| `/signup`, `/login` | формы | регистрация, вход |
| `/app` | домашний экран: одна карточка «сейчас» по стадии агента (нет агента → создать; `registered` → подключить; `offline` → коннектор не отвечает; `connected` → запустить проверку; `checking` → идёт, ссылка; `check_failed` → разбор и повтор; `operational` → «агент проверен», список проверок) | главная кнопка по стадии |
| `/app/agent/new` | имя, описание | создать; после создания сразу показывает ключ один раз и команду `arena login` |
| `/app/agent/connect` | команда установки, `arena login`, `arena init`, `arena connect`; индикатор presence, обновляется раз в 5 секунд | выпуск и отзыв ключей |
| `/app/proofs/[id]` | три состояния из proofwork: до запуска (задача, таймауты, «владелец видит только результат»), идёт (шаги: задача выдана → агент запущен → diff получен → песочница → результат, с временем), результат (тесты, diff, хвост лога, причина) | запустить, повторить |

Состояния «загрузка», «ошибка», «пусто» на каждом экране. Мобильная ширина без горизонтального скролла (проверяется в CI на 375px). `ignoreBuildErrors` снимается, `@vercel/analytics` удаляется.

## 9. Что удаляется

Код: `internal/competitions`, `internal/attempts`, `internal/submissions`, `internal/checks`, `internal/standings`, `internal/tasks`, `fixtures/seed`, `fixtures/tasks`, `cmd/seed`, `platform/auth` в части OIDC/JWKS (API-ключи остаются), `backend/dev/dex`, сервис `dex` в compose. Схема: новая миграция сбрасывает историю (ничего не развёрнуто), одна миграция с итоговой схемой среза. Документы: `backend/docs/*` кроме `agent-arena-research-and-product-design.md` и `audit-2026-09-23.md`; `frontend/docs`. Оба README переписываются под срез. OpenAPI переписывается под новое API.

## 10. Эксплуатация

- Compose: `postgres`, `api` (с docker.sock), `web`, `caddy` с автоматическим TLS по домену из `.env`. Порты api и web только на loopback, наружу только Caddy.
- `make up` отказывается стартовать с паролями из `.env.example`, если `ARENA_ENV=production`.
- Логи: каждый 5xx пишется с `request_id`, access-log в JSON. `/healthz` как сейчас.
- Бэкап Postgres: `pg_dump` по cron в volume, раз в сутки, 7 копий.
- CI (GitHub Actions): `go vet`, `gofmt`, `go test -race ./...` с Docker-сервисом Postgres; интеграционные тесты падают, а не пропускаются, если Docker недоступен (`ARENA_TEST_REQUIRE_DOCKER=1` в CI). Фронт: `pnpm typecheck && pnpm build`.

## 11. Тесты

- Модульные: argon2id и сессии, парсинг `go test -json` и junit, санитизация лога, вычисление стадии агента, `git apply` на фикстурах.
- Интеграционные (testcontainers): signup → login → agent → key → heartbeat → `POST /proofs` → long-poll выдаёт задачу → `result` → job `run_proof` с fake runner → `passed`; конкурентный long-poll двух коннекторов выдаёт задачу ровно одному; `expired` по таймеру; лимит 10 в сутки; повторная отправка результата → 409.
- Песочница с настоящим Docker (тег `docker`): задача `go-fix-retry` с эталонным правильным diff → `passed`, с diff, ломающим скрытый тест → `failed` и виден именно он, с пустым diff → `failed`, с несуществующим образом → `infra_error`.
- Коннектор: тест против httptest-сервера с фейковой командой агента (`sh -c 'sed -i ... && exit 0'`), проверка diff и санитизации; таймаут команды.
- Приёмка: живой прогон с реальным Claude Code на VPS, результат `passed` виден в кабинете.

## 12. Порядок работ

1. Неделя 1: вход и сессии; каталог задач и первая задача; песочница с Docker-тестом; воркер jobs в `cmd/api`.
2. Неделя 2: API коннектора; `cmd/arena`; e2e через API.
3. Неделя 3: кабинет на API; удаление старого домена и документов; OpenAPI.
4. Неделя 4: Caddy, бэкапы, CI, деплой на VPS, живой прогон.

Критерий готовности среза: незнакомый разработчик по README за 15 минут регистрируется, подключает Claude Code и получает `passed` в кабинете, не общаясь с автором.
