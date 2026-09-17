# Tolerance / FORGE — backend

Платформа испытаний агентной разработки: команда сдаёт продукт, проходит независимую приёмку, вносит изменение и проверяет передачу следующему исполнителю. Результат — история выпусков с доказательствами выполнения требований.

`tolerance` — имя репозитория; FORGE — рабочее название продукта из исследования. Окончательное название не выбрано. Этот каталог — backend-приложение; фронтенд живёт отдельно в [`../frontend`](../frontend).

## Проектная основа

1. [Продукт и границы MVP](docs/foundation.md) — пользователи, основной цикл, экраны и критерий готовности.
2. [Архитектура](docs/architecture.md) — компоненты, изоляция, выполнение, бюджет и восстановление.
3. [Модель данных и API](docs/domain-and-api.md) — сущности, состояния, инварианты и контракты.
4. [Технический дизайн бэкенда](docs/backend-design.md) — структура кода, схема PostgreSQL, HTTP API, оценщик, исполнитель; конкретизирует пункты 2–3 до уровня реализации.
5. [Эталонный сезон «Незнакомый город»](docs/pilot-season.md) — бриф, сценарии, оценка, сроки и споры.
6. [Порядок реализации](docs/implementation-plan.md) — последовательность законченных поставок и проверки.
7. [План среза 1](docs/plans/slice-1-contract-and-access.md) — контракт и доступ: что именно реализовано сейчас.

Основание: [исследование и проект продукта](docs/agent-arena-research-and-product-design.md). Его рыночные выводы используются как исходные гипотезы; новое исследование спроса не проводилось.

## Статус реализации

Срез 1 (контракт и доступ) в работе: identity, campaigns, missions, платформенные пакеты (`db`, `httpx`, `auth`, `audit`, `idgen`), миграции, docker-compose для разработки. См. [план среза 1](docs/plans/slice-1-contract-and-access.md) за подробным чек-листом.

`internal/routecheck` — независимый от хранилища валидатор маршрута для требования B-G2 эталонного сезона; используется оценщиком со среза 2.

## Локальный запуск

```sh
export FORGE_APP_ROLE_PASSWORD=some-local-dev-password   # forge_app role; never committed
make up        # docker compose up -d postgres
make migrate   # применить миграции докеризованной PostgreSQL под ролью forge_migrate

# cmd/api проверяет JWT по JWKS реального OIDC-провайдера; для разработки
# нужен любой провайдер, отдающий JWKS (Dex, Keycloak, локальный тестовый).
export FORGE_OIDC_ISSUER=https://your-provider/realms/forge
export FORGE_OIDC_JWKS_URL=https://your-provider/realms/forge/protocol/openid-connect/certs
export FORGE_SPA_ORIGIN=http://localhost:5173
make run       # go run ./cmd/api

make test      # go test -race ./... — модульные тесты без сети; интеграционные поднимают testcontainers-go (нужен Docker)
make check     # go vet + gofmt -l
```

Реальный OIDC-провайдер (Dex/Keycloak) в `docker-compose.yml` пока не поднимается: это осознанное решение среза 1, см.
[план среза 1](docs/plans/slice-1-contract-and-access.md#объём). Интеграционные тесты проверяют верификацию JWT по-настоящему,
против тестового JWKS-сервера, а не заглушки.

**Colima**: если Docker работает через Colima, testcontainers-go по умолчанию не находит его сокет и пытается примонтировать
reaper-контейнер способом, который Colima не поддерживает. Перед `make test` экспортируйте:

```sh
export DOCKER_HOST=unix://$HOME/.colima/default/docker.sock
export TESTCONTAINERS_RYUK_DISABLED=true   # тесты сами вызывают Terminate() в t.Cleanup
```

Первый рабочий срез: **одна миссия → одна замороженная сдача → независимая проверка → свидетельства → решение владельца → принятый выпуск**. После него — изменение этого же продукта и сравнение с предыдущей версией.
