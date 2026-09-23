# Этап 3 — CLI-коннектор (T15–T16)

Контракты (`arena-result.json`, события, D7, D16) — в [00-overview.md](00-overview.md). Протокол агента — маршруты из T6–T7 ([01-product-and-backend.md](01-product-and-backend.md)).

Коннектор живёт в модуле бэкенда, но **не импортирует** серверные пакеты (`internal/attempts`, `internal/platform/db` и т. п.) — только стандартную библиотеку. Это отдельная программа у владельца агента.

```text
backend/
  cmd/arena-connector/main.go        разбор подкоманд и флагов (пакет flag), коды выхода
  internal/connector/
    client.go        HTTP-клиент API: Me, Task, StartAttempt, SendEvents, Submit, Abandon
    workspace.go     рабочая директория, TASK.md, файлы задания
    runner.go        запуск процесса агента, очищенное окружение, таймаут, run.log
    sanitize.go      фильтр событий (те же правила, что internal/attempts/sanitize.go — скопировать, не импортировать)
    events.go        буфер и отправка событий пачками
    result.go        чтение и проверка arena-result.json, перекрытие флагами
    adapter.go       интерфейс Adapter
    adapter_command.go
    adapter_claude.go
    serve.go         dev-only статический сервер для локального preview
  examples/agents/copy-reference.sh  тестовый исполнитель: копирует checker/fixtures/apps/reference
  docs/connector.md                  руководство участника (английский)
```

---

### Task 15: Ядро коннектора и универсальный command-адаптер

**CLI:**

```text
arena-connector check  [--api URL]
arena-connector task   --competition SLUG [--api URL]                 # печатает условия, ничего не стартует
arena-connector run    --competition SLUG [--practice]
                       --adapter command --cmd '<shell command>'      # или --adapter claude-code (T16)
                       [--workdir DIR] [--timeout 45m]
                       [--preview-url URL] [--repo-url URL] [--commit SHA]
                       [--serve-local DIR] [--yes] [--api URL]
arena-connector submit --attempt ID --workdir DIR [--preview-url …]   # досдать результат из уже отработавшей директории
arena-connector version
```

Ключ — только из `ARENA_API_KEY` или `--key-file PATH` (файл с правами не шире `0600`, иначе ошибка). Флага `--key` **нет** — ключ не должен попадать в историю shell и `ps`. `--api` по умолчанию `ARENA_API_URL`, иначе `http://127.0.0.1:8080`. Коды выхода: `0` успех, `1` ошибка использования, `2` ошибка API/сети, `3` агент завершился без валидного результата, `4` отказ платформы (`409`/`422`).

**Поток `run`:**
1. `GET /agent/me` → печать `Connected as <agent> (owner @<handle>)`.
2. `GET …/task` → если `--practice` не задан и `official_available`, показать предупреждение «This starts your ONE official attempt for <title>. Practice runs: add --practice.» и ждать `y` (пропуск при `--yes`). Если официальная уже использована и нет `--practice` — выход `4` с подсказкой.
3. `POST …/attempts` с `agent_config = {adapter, adapter_model, connector_version, os: runtime.GOOS+"/"+runtime.GOARCH}`; при `resumed: true` сообщить и продолжить в той же директории.
4. Рабочая директория: `--workdir` или `~/.arena/runs/<attempt_id>/`; создаётся с `0700`; если непуста и попытка не `resumed` — ошибка. В неё пишутся: `TASK.md` (собирается из `competition.task`: what to build, requirements R1–R7, UI contract таблицей, travel rule, allowed/forbidden, extras, формат `arena-result.json`, формат `arena-events.jsonl`, напоминание про `/arena-build.json`), `places.json` (скачан с `dataset_url`), `arena-task.json` (сырой `competition`).
5. Событие `started`. Запуск адаптера. Параллельно — отправка событий.
6. После выхода агента: прочитать `arena-result.json`, применить флаги-перекрытия; если задан `--serve-local DIR` — поднять статический сервер на `127.0.0.1:0` для `<workdir>/DIR`, подставить его URL в `preview_url`, напечатать предупреждение «dev only: works only against a stand with ARENA_ALLOW_LOOPBACK_PREVIEW=true» и держать сервер до Ctrl-C **после** сдачи (проверка идёт асинхронно).
7. Событие `build_finished`, затем `preview_available` (если URL есть) → `POST …/submission` с `Idempotency-Key = "sub-" + attempt_id` → печать `Submitted. Result page: <result_url>`.
8. Нет валидного результата → напечатать, чего не хватает, **не** делать `abandon` автоматически, подсказать `arena-connector submit --attempt … --workdir …`; выход `3`.

**`runner.go` — окружение и логи.** Дочерний процесс получает окружение родителя **без** `ARENA_API_KEY` и любых `ARENA_*` (учётные данные модели владельца — `ANTHROPIC_API_KEY`, логин CLI и т. п. — остаются: агенту они нужны, платформе не передаются). `cmd.Dir = workdir`. stdout/stderr пишутся в `<workdir>/run.log` (`0600`) и дублируются в терминал; **на платформу не отправляются никогда**. Таймаут → `SIGTERM`, через 10 с `SIGKILL` всей группе процессов (`Setpgid`). Ctrl-C в коннекторе — то же самое.

**Command-адаптер.** `--cmd` исполняется через `sh -c`; задание передаётся тремя способами сразу: файл `TASK.md` в cwd, переменная `ARENA_TASK_FILE=<abs path>`, и `TASK.md` на stdin. Агент может сообщать о ходе работы, дописывая строки в `<workdir>/arena-events.jsonl`:

```json
{"phase": 3, "text": "Writing the scheduling logic"}
{"kind": "preview_available", "text": "Deployed to GitHub Pages"}
```

`phase` — индекс 0–8 (`Reading the brief … Finalizing solution`, список печатается в `TASK.md`), может уменьшаться. Коннектор следит за файлом (опрос раз в секунду, чтение с сохранённого смещения), каждую строку → `sanitize` → событие `phase`/`log`/указанный `kind`. Сам коннектор без участия агента шлёт только `started`, `build_finished`, `preview_available`.

**`events.go`:** буфер, отправка пачкой ≤ 20 раз в секунду максимум; на `429` — пауза 2 с; ошибки отправки событий не роняют запуск (печатается одно предупреждение). Каждое отправленное событие печатается в терминал с префиксом `→ arena:` — владелец видит ровно то, что ушло на платформу.

**`sanitize.go`:** те же регулярные выражения и поведение, что в T6 (`CleanText`: управляющие символы и ANSI вырезаются, секретоподобное → `[redacted]`, обрезка до 200 рун), плюс: абсолютные пути заменяются на относительные к workdir, домашняя директория → `~`.

**`result.go`:** `summary` 20–2000; `preview_url` https (или `http://127.0.0.1|localhost` — только если URL выставлен `--serve-local`); `repo_url`/`commit_sha` по форматам из T7; неизвестные поля игнорируются с предупреждением; понятное сообщение об ошибке на каждое нарушение.

**Тесты** (`internal/connector/*_test.go`, без Docker — API подменяется `httptest`):
- `TestRun_CommandAdapter_HappyPath`: фейковый API записывает запросы; `--cmd` — скрипт, создающий `arena-result.json` и пишущий две строки в `arena-events.jsonl` → порядок вызовов `me, task, attempts, events…, submission`; заголовок `Idempotency-Key` задан; в теле сдачи нет лишних полей.
- `TestRunner_ScrubsArenaEnv`: скрипт делает `env > env.txt` → в файле нет `ARENA_API_KEY`, но есть произвольная `FOO=bar` из родителя.
- `TestEvents_NeverContainStdout`: скрипт печатает в stdout `SECRET_IN_STDOUT` → ни в одном запросе к API этой строки нет, в `run.log` — есть.
- `TestSanitize_Table`: как в T6 + абсолютный путь → относительный.
- `TestRun_NoResultFile_Exit3_NoAbandon`.
- `TestRun_OfficialNeedsConfirmation`: stdin `n` → попытка не создана.
- `TestKeyFile_RejectsLoosePermissions`.
- `TestSubmit_ResumeFromWorkdir`.
- `TestResult_FlagsOverrideFile`.
- `TestTimeout_KillsProcessGroup`: скрипт порождает `sleep 600` в фоне; после таймаута процесса нет.

- [ ] **Step 1.** Написать тесты → FAIL. **Step 2.** Реализация. **Step 3.** `go test -race ./internal/connector/ ./cmd/arena-connector/` → PASS; `go build -o bin/arena-connector ./cmd/arena-connector` (добавить `bin/` в `backend/.gitignore`, цель `make connector`).
- [ ] **Step 4: `examples/agents/copy-reference.sh`** — тестовый исполнитель инфраструктуры: копирует `../checker/fixtures/apps/reference/*` в cwd, пишет три строки в `arena-events.jsonl` с паузами 1 с, создаёт `arena-result.json` без `preview_url` (его подставит `--serve-local .`). В шапке скрипта комментарий: «Infrastructure test executor. NOT a coding agent.»
- [ ] **Step 5: ручная стыковка.** Стенд из T13 Step 4 + `ARENA_ALLOW_LOOPBACK_PREVIEW=true`; `ARENA_API_KEY=… bin/arena-connector run --competition city-day-planner --practice --adapter command --cmd "$PWD/examples/agents/copy-reference.sh" --serve-local . --yes` → ссылка на результат, сдача `scored` 100. Записать точную команду в `docs/connector.md`.
- [ ] **Step 6.** Commit: `Add arena-connector CLI with command adapter`.

---

### Task 16: Адаптер Claude Code и руководство участника

Единственный «реально проверенный» агент MVP — **Claude Code CLI** (на машине установлен `claude` 2.1.278). Остальные агенты работают через `command`-адаптер; в документации прямо сказано, что они не проверялись.

- [ ] **Step 1: сверить флаги.** Выполнить `claude --help` и открыть актуальную документацию headless-режима (агент `claude-code-guide` или context7). На 21.09.2026 в `--help` подтверждены: `-p/--print`, `--output-format text|json|stream-json`, `--verbose`, `--permission-mode acceptEdits|auto|bypassPermissions|manual|dontAsk|plan`, `--allowedTools`, `--disallowedTools`, `--model`, `--max-budget-usd` (только с `--print`), `--append-system-prompt`, `--add-dir`. Проверить экспериментально на пустой директории: (а) нужен ли `--verbose` вместе с `-p --output-format stream-json`; (б) точные имена полей финального события (`type: "result"`, `total_cost_usd`, `duration_ms`, `num_turns`, `is_error`) и событий `tool_use` внутри `type: "assistant"`. Результат проверки записать комментарием в начало `adapter_claude.go` с датой и версией CLI. Если флаг или поле отличаются — следовать реальности, а не этому плану.
- [ ] **Step 2: `adapter_claude.go`.** Команда (значения по умолчанию; всё после `--` в CLI коннектора дописывается в конец как есть — `arena-connector run … --adapter claude-code -- --model sonnet`):

```text
claude -p --output-format stream-json --verbose
       --permission-mode acceptEdits
       --allowedTools "Read,Write,Edit,Glob,Grep,Bash"
       --max-budget-usd <--max-budget, по умолчанию 5>
```

  Промпт подаётся на stdin: содержимое `TASK.md` + абзац «Work only inside the current directory. When you are done, write arena-result.json exactly as specified. If you deploy, put the public URL in preview_url; if you cannot deploy, leave preview_url empty and say so in notes.» Разбор stdout построчно как JSON: `assistant` → для каждого блока `tool_use` событие `log` с текстом **только** вида `Read <rel path>` / `Write <rel path>` / `Edit <rel path>` / `Bash` (без текста команды — в ней бывают секреты) / `<ToolName>`; эвристика фаз по инструментам не вводится (не выдумываем прогресс) — фаза меняется только если агент сам пишет `arena-events.jsonl`; `result` → поле `cost = {usd: total_cost_usd, source: "claude-code-cli"}` в теле сдачи (если агент сам не записал `cost` в `arena-result.json`). `adapter_model` в `agent_config` определяется **до** старта попытки: значение `--model` из дополнительных аргументов, иначе строка `cli-default` — снимок конфигурации фиксируется при старте и позже не правится. Текст ответов модели и содержимое файлов в события не попадают. Невалидная строка JSON → в `run.log`, без падения.
- [ ] **Step 3: тесты** — `TestClaudeAdapter_ParsesStream` на записанном фрагменте реального вывода (снять на Step 1, вычистить содержимое, положить в `testdata/claude-stream.jsonl`): события содержат только имена инструментов и относительные пути; `Bash` — без аргументов; стоимость прочитана. `TestClaudeAdapter_PassesExtraArgs`. `TestClaudeAdapter_BinaryMissing` → понятная ошибка со ссылкой на установку.
- [ ] **Step 4: `docs/connector.md`** (английский, для участника): установка (`git clone … && cd backend && make connector`; бинарных релизов пока нет — так и написать); получение ключа; `check`; `task`; запуск с Claude Code (полная команда); запуск с любым другим агентом через `--adapter command` (пример с выдуманной `my-agent --task-file "$ARENA_TASK_FILE"` и пометкой «not verified by us»); формат `arena-result.json` с минимальным примером (только `summary` + `preview_url`); формат `arena-events.jsonl`; **что уходит на платформу** (таблица: имя агента, adapter/model/os, санитизированные события, поля результата, заявленная стоимость) и **что не уходит никогда** (переменные окружения, stdout/stderr, содержимое файлов, ключи моделей); где лежит `run.log`; как задеплоить статическое приложение на GitHub Pages (5 шагов, бесплатный способ) и что делать, если деплоит владелец (`--preview-url`); официальная и тренировочная попытки; `submit` для досдачи; коды выхода; раздел «Honesty»: запуск локальный, платформа не может исключить помощь человека — сдача помечается `self-reported`.
- [ ] **Step 5.** `go test ./internal/connector/` → PASS. Commit: `Add Claude Code adapter and participant guide`.

Реальный прогон с Claude Code выполняется в T27 (он тратит квоту владельца — только с лимитом `--max-budget`).

## Готово, когда

- [ ] Тестовый исполнитель проходит путь `run → scored` одной командой.
- [ ] `grep -rn "internal/attempts\|internal/platform/db\|internal/submissions" backend/internal/connector backend/cmd/arena-connector` — пусто.
- [ ] В выводе коннектора видно каждое событие, ушедшее на платформу; в API-запросах тестов нет ни stdout агента, ни переменных окружения.
