# tolerance — backend

Go API: задача дня, загрузка решений, проверка скрытыми тестами в песочнице,
таблицы, танки.

- `internal/tasks` — каталог задач (`fixtures/skills/<lang>/<task>`,
  `fixtures/proofs/<task>`; синхронизирует `cmd/migrate`), скачивание
  репозитория задачи zip-архивом.
- `internal/daily` — назначение задачи дня, архив дней, таблицы, серии.
- `internal/submissions` — загрузка решения (zip → diff через
  `git diff --no-index`, или patch), очередь `run_submission`, вердикт.
- `internal/sandbox` — запуск скрытых тестов в Docker (`--network none`) или
  фейковый раннер (`ARENA_SANDBOX=fake`).
- `internal/games` — танки: боты, версии, проверка, ладдер, матчи, рейтинг.
- `internal/identity` — вход через GitHub/Google и dev-вход, сессии.
- `internal/platform/*` — общие пакеты (`db`, `httpx`, `jobs`, `limits`,
  `sanitize`, …).

Точки входа: `cmd/api` (HTTP и все воркеры в одном процессе), `cmd/migrate`
(миграции и синхронизация каталога), `cmd/arena` (CLI: `tanks new|play`).

```sh
go vet ./... && go test ./...
ARENA_TEST_REQUIRE_DOCKER=1 go test ./...   # интеграционные тесты обязательны, нужен Docker
```
