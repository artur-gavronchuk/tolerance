# Вход через GitHub и Google

Дата: 25 сентября 2026. Заменяет строку «Вход» в спеке среза 1
(`2026-09-23-agent-connect-and-proof-design.md`): e-mail и пароль уходят.

## Зачем

Вход по паролю без подтверждения почты не даёт сбросить пароль: забыл пароль —
потерял аккаунт с агентом и историей. Чинить это — рассылка писем, токены сброса,
подтверждение адреса; это больше работы, чем OAuth. Хранить пароли — отдельный
риск. Аудитория — разработчики, у всех есть GitHub или Google.

Вход через ChatGPT и Claude не делаем: у Anthropic нет OAuth для сторонних сайтов
(и условия это запрещают), у OpenAI «Sign in with ChatGPT» закрыт для партнёров.
Схема ниже принимает нового провайдера без переделок.

## Решения

| Вопрос | Решение | Почему |
|---|---|---|
| Провайдеры | GitHub (OAuth App) и Google (OpenID Connect) | Покрывают аудиторию; оба бесплатны; Google со скоупами `openid email profile` не требует проверки приложения |
| Пароли | Удаляются целиком: `password_hash`, argon2id, `/auth/signup`, `/auth/login` | Нечего сбрасывать, нечего утекать |
| Библиотека | `golang.org/x/oauth2` + `golang.org/x/oauth2/endpoints`; без Dex и без `x/oauth2/google` (тянет облачные зависимости) | Два обработчика, никаких сервисов |
| Личность Google | Эндпоинт userinfo по access token, без разбора id_token | Токен получен напрямую по TLS от Google; JWKS не нужен |
| Защита потока | `state` (32 случайных байта) и PKCE S256 для обоих провайдеров; оба живут в HttpOnly-cookie на 10 минут | Стандарт; GitHub поддерживает PKCE |
| Связывание | Ключ — `(provider, subject)`; при первом входе через второго провайдера — к существующему пользователю с тем же подтверждённым e-mail | Один человек входит то через GitHub, то через Google и видит одного агента. Живых пользователей нет, переносить аккаунты с паролями не нужно |
| Неподтверждённая почта | Отказ `email_unverified` | Иначе чужой аккаунт можно захватить, указав его e-mail у провайдера |
| Сессии | Без изменений: `arena_session`, 30 дней, хранится хеш | `RequireSession` не трогаем |
| Локальная разработка и CI | `ARENA_DEV_LOGIN=true` открывает `POST /auth/dev {email}`; конфиг отказывается запускаться с ним при `ARENA_SECURE_COOKIES=true`, Makefile — при `ARENA_ENV=production` | Без реальных OAuth-приложений `make up`, e2e и `check:mobile` должны работать |
| Коннектор | Без изменений | `arena login` принимает API-ключ, не пароль |

## Поток

```
браузер ─ GET /api/v1/auth/{provider}/start?next=/app
        ← 302 на провайдера (client_id, redirect_uri, scope, state, code_challenge S256)
          + Set-Cookie arena_oauth = provider.state.verifier.next (HttpOnly, Lax, Path=/api/v1/auth/, 10 мин)
провайдер ─ 302 на {ARENA_PUBLIC_URL}/api/v1/auth/{provider}/callback?code&state
браузер ─ GET …/callback
        сверка cookie: тот же provider, state совпадает (constant-time); cookie стирается
        обмен code + verifier на токен → личность {subject, email, email_verified, login}
        identity.Service.SignIn → пользователь + сессия
        ← 302 на next + Set-Cookie arena_session
```

`next` принимается, только если начинается с `/` и не с `//` или `/\`; иначе
`/app`. Ошибка на любом шаге колбэка — `302 /login?error=<код>`, а не JSON: это
навигация браузера. Коды: `oauth_denied` (провайдер вернул `error`, например
пользователь нажал «Отмена»), `oauth_state` (нет cookie или state не совпал),
`oauth_failed` (обмен или запрос личности упал), `email_unverified`,
`rate_limited`. Неизвестный провайдер или провайдер без настроенных ключей —
`404 not_found` в обычном формате `httpx.Problem`.

`start` и `callback` ограничены 20 запросами в минуту на IP, `POST /auth/dev` —
10 в минуту на IP и на e-mail (как сейчас логин).

**GitHub.** Скоуп `read:user user:email`. Личность: `GET https://api.github.com/user`
(`id` → subject, `login`), почта — первичная и подтверждённая из
`GET /user/emails`. Нет такой — `email_unverified`.

**Google.** Скоупы `openid email profile`. Личность:
`GET https://openidconnect.googleapis.com/v1/userinfo` (`sub`, `email`,
`email_verified`).

Эндпоинты провайдеров — поля структуры, чтобы тесты подменяли их на httptest-сервер.

## Вход в сервисе

`identity.Service.SignIn(ctx, Identity{Provider, Subject, Email, EmailVerified, Login}) (User, token, error)`,
одна транзакция:

1. Есть строка `user_identities (provider, subject)` — это пользователь; обновить
   `email`, `login`, `last_login_at` у личности.
2. Нет — нужен `EmailVerified`, иначе `email_unverified`. Пользователь с этим
   e-mail есть (вошёл раньше через другого провайдера) — привязать личность, аудит `user.identity_linked`. Нет — создать
   пользователя и личность, аудит `user.signed_up` с провайдером в данных.
   Создание устойчиво к гонке двух первых входов: `INSERT … ON CONFLICT DO NOTHING`
   и повторное чтение.
3. Роль пересчитывается по `ARENA_ADMIN_EMAILS` от `users.email` (как сейчас).
   `users.email` — адрес с первого входа, потом не меняется.
4. Создать сессию, вернуть токен для cookie.

Вход разработчика — тот же `SignIn` с `Identity{Provider: "dev", Subject: email, Email: email, EmailVerified: true}`.

## Схема

`migrations/00004_oauth_identities.sql` (номер 00003 занят `00003_games.sql` в ветке танков):

```sql
CREATE TABLE user_identities (
    provider text NOT NULL CHECK (provider IN ('github', 'google', 'dev')),
    subject text NOT NULL,
    user_id text NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    email text NOT NULL,
    login text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    last_login_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, subject)
);
CREATE INDEX user_identities_user_idx ON user_identities (user_id);
ALTER TABLE users DROP COLUMN password_hash;
GRANT SELECT, INSERT, UPDATE, DELETE ON user_identities TO arena_app;
```

Down возвращает `password_hash text NOT NULL DEFAULT ''` и удаляет таблицу.
Живых пользователей нет (25 сентября), поэтому данные не переносятся; отдельная
миграция нужна только затем, чтобы накатиться на развёрнутую базу без её сброса.

`db.Migrate` вызывает `goose.Up` с `goose.WithAllowMissing()`: продакшн уже
работает, и если `00004` попадёт туда раньше `00003` танков, следующий деплой не
должен упасть на «пропущенной» миграции. Таблицы веток не пересекаются, порядок
им не важен.

## API

Убираются `POST /auth/signup`, `POST /auth/login`, тело `Credentials`, коды
`email_taken` и `invalid_credentials`. Добавляются:

- `GET /auth/providers` → `{providers: ["github", "google"], dev_login: bool}` —
  только настроенные; фронт рисует по нему кнопки.
- `GET /auth/{provider}/start?next=` → 302.
- `GET /auth/{provider}/callback?code&state&error` → 302.
- `POST /auth/dev {email}` → 200 `{user}` + cookie; маршрут существует, только
  если `ARENA_DEV_LOGIN=true`.
- `POST /auth/logout` не меняется.

## Конфигурация

```
ARENA_PUBLIC_URL=https://tolerance.cc        # основа redirect_uri; обязателен, если задан хоть один провайдер
ARENA_GITHUB_CLIENT_ID= / ARENA_GITHUB_CLIENT_SECRET=
ARENA_GOOGLE_CLIENT_ID= / ARENA_GOOGLE_CLIENT_SECRET=
ARENA_DEV_LOGIN=true                         # только локально и в CI
```

Провайдер включён, когда заданы оба его ключа; задан один — ошибка конфига.
Callback-адреса при регистрации приложений:
`https://tolerance.cc/api/v1/auth/github/callback`,
`https://tolerance.cc/api/v1/auth/google/callback`, для разработки то же с
`http://localhost:3000`. Секреты продакшна лежат в `/opt/tolerance/.env` на сервере.
Колбэк идёт через rewrite Next (`/api/*` → Go API), как все запросы; 302 и
`Set-Cookie` проходят насквозь, cookie остаётся first-party.

## Фронт

- `/login` и `/signup` — один компонент, разный текст. Кнопки «Continue with
  GitHub» и «Continue with Google» — обычные ссылки на
  `/api/v1/auth/{provider}/start?next=/app` (полная навигация, не fetch).
  Показываются только провайдеры из `/auth/providers`. При `dev_login` — поле
  e-mail и кнопка «Dev sign in» под чертой, с пометкой, что это только для
  разработки. `?error=` показывается человеческой фразой.
- На `/signup` остаётся согласие с условиями: «Продолжая, вы принимаете…».
- `/terms`: вместо «хеш пароля (argon2id)» — «e-mail и идентификатор аккаунта у
  GitHub или Google».
- `scripts/check-mobile.mjs` входит через `/auth/dev`; CI-джоб mobile запускает
  API с `ARENA_DEV_LOGIN=true`.

## Тесты

- Модульные: провайдеры GitHub и Google против httptest-сервера (обмен кода с
  verifier, личность, нет подтверждённой почты); кодирование cookie состояния;
  проверка `next`; `start` выдаёт 302 с `state`, `code_challenge_method=S256`,
  верным `redirect_uri`; `callback` без cookie, с чужим state, с `error=` —
  нужный `/login?error=`.
- Интеграционные (`dbtest`): `SignIn` — новый пользователь, повторный вход,
  привязка второго провайдера по e-mail, отказ неподтверждённой
  почте, роль admin, две параллельные первые попытки дают одного пользователя.
- e2e (`cmd/api/main_test.go`): полный OAuth-поток против фейкового провайдера
  на httptest (браузер с cookie jar, редиректы вручную), дальше как сейчас —
  агент, ключ, проверка. Отдельно вход разработчика и лимит на `/auth/dev`.
  Все ответы, включая 302, сверяются с `openapi.yaml`.
- Тесты агентов и проверок, создававшие пользователя через `Signup`, переходят
  на `SignIn` с dev-личностью.

## Вне среза

Turnstile на форме входа (ботам теперь нужен аккаунт GitHub или Google), вход по
ссылке на почту, Microsoft, привязка второго провайдера из настроек, смена e-mail,
удаление аккаунта. Реальный IP клиента за Cloudflare даёт общий хелпер из ветки
`gentle-naranja`; до её слияния лимиты берут `identity.clientIP`.
