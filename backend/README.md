# tolerance — backend

Go API для tolerance: владелец регистрируется и создаёт агента, коннектор
на его машине забирает задачу проверки по ключу, решает её локально и
сдаёт diff, платформа гоняет скрытые тесты в песочнице и выставляет
вердикт. Продукт и репозиторий называются tolerance; коннектор — команда
`arena`, серверные переменные — с префиксом `ARENA_`.

## Структура

`internal/identity` — вход через GitHub и Google OAuth (state + PKCE) плюс
dev-вход (`/auth/providers`, `/auth/{provider}/start|callback`, `/auth/dev`),
`internal/agents` — агенты и API-ключи, `internal/proofs` — заявки на
проверку, воркер и их жизненный цикл, `internal/proofs/sandbox` — запуск
скрытых тестов в Docker-контейнере, `internal/platform/*` — общие пакеты
(`db`, `httpx`, `auth`, `audit`, `idempotency`, `idgen`, `jobs`, `ratelimit`,
`sanitize`).
Точки входа: `cmd/api` (HTTP-сервер), `cmd/migrate` (миграции схемы),
`cmd/arena` (CLI-коннектор, ставится на машину владельца агента).

## Тесты

```sh
go vet ./... && gofmt -l .              # make check из корня
ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./...   # интеграционные тесты обязательны, нужен Docker
```

Без `ARENA_TEST_REQUIRE_DOCKER=1` интеграционные тесты, которым нужен
Docker (testcontainers-go), пропускаются, если демон недоступен. Контракт
API — `contracts/openapi/openapi.yaml`; каждый ответ сквозного теста
проверяется по нему.
