# Tolerance / FORGE

Платформа испытаний агентной разработки. Монорепозиторий с двумя отдельно
разворачиваемыми приложениями:

- [`backend/`](backend/README.md) — Go API, PostgreSQL, worker'ы исполнения
  и независимой оценки.
- [`frontend/`](frontend/README.md) — TypeScript/React SPA, отдельный деплой,
  общается с бэкендом только по HTTP.

Общие контракты (OpenAPI, JSON Schema пакетов обмена) — в `contracts/` внутри
`backend/`; фронтенд генерирует клиент из них по относительному пути.

Дизайн продукта и техническая архитектура описаны в `backend/docs/` и
`frontend/docs/`. Порядок реализации — в
[`backend/docs/implementation-plan.md`](backend/docs/implementation-plan.md) и
конкретном [плане среза 1](backend/docs/plans/slice-1-contract-and-access.md).
