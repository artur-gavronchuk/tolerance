# Танки: игровая арена агентов. План реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Публичный постоянный турнир ботов-танков: бота пишет агент владельца через коннектор или человек руками, платформа проверяет бота пробным матчем, гоняет ладдер с рейтингом, показывает матчи в 2D/3D и эфир; плюс локальная игра в коннекторе, стартовые наборы, мини-агент и статья для Хабра.

**Architecture:** Новый домен `backend/internal/games` поверх среза 1. Движок танков — чистый Go без зависимостей (`internal/games/tanks`), общий для API и коннектора. Матч проводит `internal/games/match` через интерфейс `Bot` (домашний бот в горутине, локальный процесс, Docker-контейнер). Сервис `internal/games` хранит ботов, версии, матчи и повторы, крутит ладдер и эфир в горутине `cmd/api`, берёт работу из очереди `jobs`. Путь агента — это `proof` с `kind = game_bot`: коннектор не меняется, воркер proofs отдаёт дерево после diff в `games` через интерфейс `proofs.GameBotJudge`. Фронт: публичный раздел `/tanks` с просмотрщиком (canvas 2D и three.js 3D от одного модуля воспроизведения) и страница кабинета `/app/tanks`.

**Tech Stack:** Go 1.26+ (stdlib, pgx/v5, goose), PostgreSQL 16, Docker CLI, Python 3.12 и Node 22 в образе бота, Next.js 16 / React 19 / Tailwind 4, three.js, Anthropic Python SDK (мини-агент).

**Spec:** `docs/superpowers/specs/2026-09-25-tanks-arena-design.md`

## Global Constraints

- Go-модуль `tolerance`; переменные окружения с префиксом `ARENA_`; роли `arena_app` / `arena_migrate`.
- Временные метки в JSON в UTC (`.UTC()` при сканировании).
- Ошибки только через `httpx.Problem` / `httpx.WriteError`; тело `{code, message, request_id, fields?}`.
- Каждый новый маршрут добавляется в `cmd/api/handler.go` и в `contracts/openapi/openapi.yaml`; e2e валидирует ответы, включая ошибки.
- Интеграционные тесты через `dbtest.New`, утверждения через `AppPool`. Под `ARENA_TEST_REQUIRE_DOCKER=1` пропуск — провал.
- Всё, что бот пишет в stderr, и всё, что агент пишет в лог, проходит `sanitize.CleanLog` на сервере.
- Фронт: без моков, все данные из API; типы в `lib/types.ts`, запросы в `lib/api.ts`; 375 px без горизонтальной прокрутки. UI на английском.
- Коммиты: английский, повелительное наклонение, с заглавной, без префиксов `feat:`; завершаются строкой `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Документы в `docs/` — на русском; код и комментарии — на английском.
- Числа движка `tanks/1` (из спеки, дословно): поле 60×40; 10 тиков/с; 1200 тиков; 4 подшага; радиус танка 1.0; скорость 5 вперёд / 3 назад; поворот корпуса 2.5 рад/с; башни 4 рад/с; перезарядка 10 тиков; ствол 1.3; снаряд 24 ед/с, живёт 30 тиков; урон 25; HP 100; аптечка +35, возрождение через 150 тиков; зона с тика 800 по 1100 от радиуса 37 до 6, −1 HP/тик вне зоны.
- Протокол: `ready` за 5 с; ответ на тик до 200 мс; бесплатно 20 мс на тик; запас 20 с на матч; строка до 64 KiB; >1000 посторонних строк — `invalid`; stderr до 16 KiB на матч.
- Пакет бота: архив ≤ 1 MiB сжатым, ≤ 4 MiB распакованным, ≤ 200 файлов; языки `python` (`python3 -u <entry>`) и `javascript` (`node <entry>`).
- Лимиты: 20 загрузок версий в сутки на бота; запуск агента подчиняется лимитам proofs (онлайн, одна открытая, 10 в сутки); таймаут агента для `tanks-bot` 1200 с.
- Рейтинг: μ₀ = 25, σ₀ = 25/3, β = σ₀/2, κ = 0.0001; показ `round(1000 + 40·(μ − 3σ))`; новая версия: σ = max(σ, 5.0).
- Ладдер: `ARENA_MATCH_INTERVAL` по умолчанию 20 с, `ARENA_MATCH_CONCURRENCY` по умолчанию 1, домашние матчи без пользовательских ботов не чаще раза в 2 минуты; повторы хранятся 3 суток, кроме `featured` и пробных матчей.

## Review Focus

1. **Бот печатает отладку в stdout** (`print("thinking...")` перед JSON-ответом): посторонние строки игнорируются, ход засчитывается, если правильный ответ пришёл вовремя; бот не отключается, пока таких строк ≤ 1000; в `checks` появляется подсказка про stderr. Тест в задаче 3.
2. **Архив, собранный на macOS**: всё завёрнуто в одну папку (`mybot/bot.json`), рядом `__MACOSX/` и `._bot.py`. Одиночная корневая папка снимается, мусор macOS игнорируется, архив принимается. Тест в задаче 5.
3. **Бот не завершается после `end`** (`while True: pass`) или игнорирует закрытие stdin: `Close` убивает процесс вместе с потомками, матч заканчивается вовремя. Тест в задаче 4 (процесс) и 9 (Docker).
4. **API перезапустился посреди матча**: матч в `running` без живого воркера. Повторный захват job переигрывает матч с нуля; запись результата защищена статусом, рейтинг не применяется дважды; матч, который висит дольше 10 минут, становится `infra_error` без изменения рейтинга. Тест в задаче 10.
5. **Имя бота совпадает с домашним в другом регистре** (`Hunter`) или с чужим ботом: 409 `name_taken`, а не 500 от уникального индекса. Тест в задаче 8.

## Структура файлов

```
backend/
  migrations/00003_games.sql                  НОВАЯ: game_bots, bot_versions, matches, match_players, match_replays,
                                              tanks_broadcasts; proofs.kind/repo_tar/repo_sha256; proof_tasks.kind; jobs.kind
  internal/games/tanks/
    rules.go            Rules, DefaultRules, геометрия, углы
    maps.go             Map, Rect, Point, Maps(), MapByName, PickMap
    game.go             Game, New, Step, Over, события
    place.go            Placements
    protocol.go         StartMsg, TickMsg, EndMsg, CommandMsg, StartFor, TickFor
    replay.go           Replay, Frame, EncodeReplay, DecodeReplay, (*Game).Frame
    starter.go          //go:embed GAME.md AGENT_TASK.md starter; Starter(lang), GameMD, AgentTaskMD
    GAME.md, AGENT_TASK.md
    starter/python/{bot.json,bot.py,tanks.py}
    starter/js/{bot.json,bot.js,tanks.js}
  internal/games/tanks/house/   idle, hunter, sniper
  internal/games/match/
    bot.go              Bot, Launcher, Spec, Command()
    house.go            WithHouse (домашние боты в горутине)
    process.go          ProcessLauncher
    docker.go           DockerLauncher
    run.go              Config, Player, Result, PlayerOutcome, Run
    runtime/Dockerfile  образ arena-bot-runtime:1
  internal/games/botpkg/        Manifest, PackDir, Validate, Unpack
  internal/games/rating/        Rating, Update, Display, Refresh
  internal/games/
    model.go            представления API, константы
    bots.go             бот и версии: SaveBot, UploadVersion, MyTanks
    qualify.go          Qualify
    matches.go          RunMatch, чтение матчей, повторов, лидерборда, профиля
    ladder.go           ScheduleTick, SweepStuck, PruneReplays
    broadcast.go        RefreshBroadcast, Live
    agentrun.go         StartAgentRun, JudgeProof, RESULTS.md
    sync.go             Sync: домашние боты и proof_tasks 'tanks-bot'
    worker.go           Worker: jobs run_match/check_bot + тики планировщика
    http_public.go, http_me.go, http_connector.go
  internal/proofs/      kind, repo override, CreateWithRepo, TarFiles, GameBotJudge в воркере
  cmd/api/              config (ARENA_MATCH_*, ARENA_BOT_IMAGE), handler (маршруты), main (сервис и воркер games), main_test (e2e)
  cmd/migrate/main.go   + games.Sync
  cmd/arena/            tanks.go: new | play | submit
frontend/
  lib/types.ts, lib/api.ts                   + типы и запросы игры
  lib/tanks/replay.ts                        типы повтора, fetchReplay, parseReplayFile
  lib/tanks/playback.ts                      часы воспроизведения, интерполяция, цвета слотов
  components/tanks/…                         public-shell, viewer (canvas2d, scene3d, player), leaderboard, match-list, live
  app/tanks/…                                /tanks, /tanks/leaderboard, /tanks/matches/[id], /tanks/bots/[id], /tanks/docs, /tanks/replay
  app/app/tanks/page.tsx                     кабинет
examples/mini-agent/                          agent.py, requirements.txt, README.md
docs/articles/2026-09-habr-build-your-agent.md
Makefile, docker-compose.yml, .github/workflows/ci.yml, README.md, backend/README.md, CLAUDE.md, docs/how-it-works.md
```

---

### Task 1: Движок танков

**Files:**
- Create: `backend/internal/games/tanks/{rules.go,maps.go,game.go,place.go,replay.go}`
- Test: `backend/internal/games/tanks/{game_test.go,place_test.go,replay_test.go,maps_test.go}`

**Interfaces:**
- Consumes: ничего.
- Produces:

```go
package tanks

const EngineVersion = "tanks/1"

type Rules struct {
	Width              float64 `json:"width"`
	Height             float64 `json:"height"`
	TickRate           int     `json:"tick_rate"`
	Ticks              int     `json:"ticks"`
	Substeps           int     `json:"substeps"`
	TankRadius         float64 `json:"tank_radius"`
	TankSpeed          float64 `json:"tank_speed"`
	TankReverseSpeed   float64 `json:"tank_reverse_speed"`
	HullTurnRate       float64 `json:"hull_turn_rate"`
	TurretTurnRate     float64 `json:"turret_turn_rate"`
	ReloadTicks        int     `json:"reload_ticks"`
	MuzzleOffset       float64 `json:"muzzle_offset"`
	ShellSpeed         float64 `json:"shell_speed"`
	ShellLifetimeTicks int     `json:"shell_lifetime_ticks"`
	ShellDamage        int     `json:"shell_damage"`
	MaxHP              int     `json:"max_hp"`
	HealAmount         int     `json:"heal_amount"`
	HealRespawnTicks   int     `json:"heal_respawn_ticks"`
	PickupRadius       float64 `json:"pickup_radius"`
	ZoneStartTick      int     `json:"zone_start_tick"`
	ZoneEndTick        int     `json:"zone_end_tick"`
	ZoneStartRadius    float64 `json:"zone_start_radius"`
	ZoneEndRadius      float64 `json:"zone_end_radius"`
	ZoneDamagePerTick  int     `json:"zone_damage_per_tick"`
}

// DefaultRules: 60, 40, 10, 1200, 4, 1.0, 5, 3, 2.5, 4, 10, 1.3, 24, 30, 25, 100, 35, 150, 1.5, 800, 1100, 37, 6, 1.
func DefaultRules() Rules

type Point struct{ X, Y float64 }                        // json "x","y"
type Rect struct{ X, Y, W, H float64 }                   // json "x","y","w","h"; X,Y — левый нижний угол
type Zone struct{ X, Y, R float64 }                      // json "x","y","r"
type Map struct {
	Name    string
	Walls   []Rect
	Spawns  [4]Point   // симметричны относительно центра; 0↔2 и 1↔3 напротив
	Bonuses [4]Point
}
func Maps() []Map                    // "arena", "crossroads", "bunkers" — в этом порядке
func MapByName(name string) (Map, bool)
func PickMap(seed int64) Map         // Maps()[seed mod 3], неотрицательный остаток

type Command struct{ Move, Turn, Turret float64; Fire bool } // значения обрезаются в [-1, 1]

type Tank struct {
	ID                        int
	X, Y, Hull, Turret        float64
	HP, Reload                int
	Alive                     bool
	Kills, Damage             int
	DeathTick                 int // -1, пока жив
}
type Shell struct{ ID, Owner int; X, Y, VX, VY float64; Age int }
type Bonus struct{ X, Y float64; Active bool; RespawnAt int }

// Event: "shot" (A стрелял), "hit" (A попал в B на D), "kill" (A убил B; A = -1 — зона), "heal" (A взял аптечку, D — сколько).
type Event struct {
	T int    `json:"t"`
	E string `json:"e"`
	A int    `json:"a"`
	B *int   `json:"b,omitempty"`
	D *int   `json:"d,omitempty"`
}

type Game struct {
	Rules   Rules
	Map     Map
	Seed    int64
	Tick    int
	Tanks   []Tank
	Shells  []Shell
	Bonuses []Bonus
	Zone    Zone
	// unexported: nextShellID, rng *rand.Rand (math/rand/v2, PCG(seed, seed^0x9e3779b97f4a7c15))
}

// New: players 2..4. Слоты получают точки появления: для 2 игроков — спавны 0 и 2, для 3 — 0,1,2, для 4 — все;
// порядок точек перемешивается rng по сиду. Корпус и башня смотрят на центр поля. Все аптечки активны.
func New(rules Rules, m Map, players int, seed int64) *Game
// Step продвигает игру на один тик и возвращает события этого тика. len(cmds) == len(g.Tanks); команды мёртвых игнорируются.
func (g *Game) Step(cmds []Command) []Event
func (g *Game) Over() bool       // Tick >= Rules.Ticks или живых <= 1
func (g *Game) Alive() int

type Placement struct{ Slot, Place int }
func (g *Game) Placements() []Placement // по слотам (индекс = слот)
```

Порядок внутри `Step` (фиксирован, от него зависит детерминированность):
1. У каждого живого танка в порядке ID: `Reload--` если > 0; если `Fire` и `Reload == 0` — снаряд из точки `(X + cos(Turret)·MuzzleOffset, Y + sin(Turret)·MuzzleOffset)` со скоростью `ShellSpeed` вдоль `Turret`, `Reload = ReloadTicks`, событие `shot`.
2. `Substeps` раз с `dt = 1 / (TickRate · Substeps)`: у каждого живого танка `Hull += Turn·HullTurnRate·dt`, `Turret += Turret·TurretTurnRate·dt`, перемещение на `v·dt` вдоль `Hull`, где `v = Move·TankSpeed` при `Move ≥ 0`, иначе `Move·TankReverseSpeed`; затем выталкивание из стен и краёв (круг против прямоугольника: ближайшая точка прямоугольника, сдвиг на `R − dist` по нормали; если центр внутри прямоугольника — по кратчайшей оси); затем пары танков `i < j`: при пересечении раздвигаются поровну по линии центров (совпавшие центры — по оси x). Потом снаряды: сдвиг на `V·dt`; вылет за поле или попадание в стену — снаряд удаляется; первый по ID живой танк, кроме владельца, на расстоянии `≤ TankRadius` — урон `min(ShellDamage, HP)`, `hit`, при `HP ≤ 0` — `Alive = false`, `DeathTick = Tick`, `Kills++` у владельца, `kill`; снаряд удаляется.
3. `Age++` у снарядов; `Age ≥ ShellLifetimeTicks` — удалить.
4. Аптечки: активная аптечка, до которой живой танк ближе `PickupRadius`, у которого `HP < MaxHP` (первый по ID), — `HP = min(MaxHP, HP + HealAmount)`, `heal` с фактическим приростом, `Active = false`, `RespawnAt = Tick + HealRespawnTicks`. Неактивная с `Tick ≥ RespawnAt` снова активна.
5. Зона: `R` линейно от `ZoneStartRadius` к `ZoneEndRadius` на `[ZoneStartTick, ZoneEndTick]`; живые танки с расстоянием до центра больше `R` теряют `ZoneDamagePerTick`; смерть от зоны — `kill` с `A = -1`, без `Kills`.
6. `Tick++`.

Углы нормализуются в `(-π, π]` после каждого изменения. Снаряд, у которого ID владельца мёртв, продолжает лететь.

Места (`Placements`): живые выше мёртвых; живые — по HP убыв., затем урон убыв.; мёртвые — `DeathTick` убыв., затем урон убыв.; полное равенство ключей — общее место (соревновательная нумерация: 1, 1, 3).

Повтор:

```go
type ReplayPlayer struct {
	Slot    int    `json:"slot"`
	Name    string `json:"name"`
	BotID   string `json:"bot_id,omitempty"`
	Version int    `json:"version,omitempty"`
	House   bool   `json:"house"`
	Source  string `json:"source,omitempty"`
}
type Frame struct {
	T int         `json:"t"`
	K [][]float64 `json:"k"` // по слотам: x, y, hull, turret, hp, reload, alive(0|1)
	S [][]float64 `json:"s"` // id, owner, x, y
	B [][]float64 `json:"b"` // активные аптечки: x, y
	Z float64     `json:"z"` // радиус зоны
}
type PlayerResult struct {
	Slot      int    `json:"slot"`
	Place     int    `json:"place"`
	Kills     int    `json:"kills"`
	Damage    int    `json:"damage"`
	DeathTick *int   `json:"death_tick"`
	Status    string `json:"status"` // ok | crashed | timeout | invalid
}
type Replay struct {
	Version  int            `json:"version"` // 1
	Engine   string         `json:"engine"`  // EngineVersion
	Seed     int64          `json:"seed"`
	Map      string         `json:"map"`
	TickRate int            `json:"tick_rate"`
	Rules    Rules          `json:"rules"`
	Walls    []Rect         `json:"walls"`
	Players  []ReplayPlayer `json:"players"`
	Frames   []Frame        `json:"frames"`
	Events   []Event        `json:"events"`
	Result   []PlayerResult `json:"result"`
}
func (g *Game) Frame() Frame // координаты округлены до 0.01, углы до 0.001
func EncodeReplay(r Replay) ([]byte, error) // gzip(JSON)
func DecodeReplay(gz []byte) (Replay, error)
```

Сиды — неотрицательные `int64` меньше `1<<53` (JSON-числа в браузере).

- [ ] **Step 1: Тесты движения и столкновений.** В `game_test.go` помощник `newTestGame(t, walls []Rect, pos []Point)` строит `Game` через `New(DefaultRules(), Map{Name: "t", Walls: walls, Spawns: ..., Bonuses: ...}, len(pos), 1)` и ставит танкам позиции и углы вручную, аптечки выключает. Тесты:
  - `TestMoveForwardOneTick`: танк в (10,10), `Hull = 0`, `Move = 1` → после `Step` `X ≈ 10.5` (5 ед/с × 0.1 с), `Y == 10`.
  - `TestReverseIsSlower`: `Move = -1` → `X ≈ 9.7`.
  - `TestTurnRates`: `Turn = 1` → `Hull ≈ 0.25`; `Turret = -1` → `Turret ≈ -0.4`; корпус не вращает башню.
  - `TestCommandsClamped`: `Move = 7` ведёт себя как `1`.
  - `TestWallStopsTank`: стена `Rect{12, 0, 2, 40}`, танк в (10.8, 20) едет вправо 5 тиков → `X ≤ 12 − 1.0 + 1e-9`.
  - `TestFieldEdgeStopsTank`: танк у (1.2, 20) едет влево → `X ≥ 1.0`.
  - `TestTanksPushApart`: два танка в (10,10) и (11,10) → после `Step` с нулевыми командами расстояние ≥ 2.0 − 1e-9.
- [ ] **Step 2: Тесты стрельбы, урона, аптечек, зоны.**
  - `TestFireSpawnsShellAndReloads`: `Fire = true` → 1 снаряд, `Reload == 10`, событие `shot`; ещё `Step` с `Fire` → новых снарядов нет, `Reload == 9`.
  - `TestShellHitsAndKills`: танк 0 в (10,20) башней на танк 1 в (20,20); 4 попадания убивают: после достаточного числа тиков с `Fire` у танка 1 `Alive == false`, `DeathTick` выставлен, у танка 0 `Kills == 1`, `Damage == 100`, события `hit` ×4 и `kill` с `A = 0, B = 1`.
  - `TestShellDoesNotHitOwner`: танк стреляет, стоя на месте, — своё HP не меняется.
  - `TestShellStoppedByWall`: стена между танками — HP цели не меняется, снаряд исчезает.
  - `TestShellExpires`: снаряд в пустом поле исчезает через 30 тиков.
  - `TestHealPickup`: HP 50, аптечка рядом → HP 85, событие `heal` с `D = 35`; танк с HP 100 аптечку не берёт; через 150 тиков после взятия аптечка снова активна.
  - `TestZoneShrinksAndDamages`: на тике 800 `Zone.R == 37`, на 950 — `21.5`, на 1100 и позже — `6`; танк в углу (1,1) на тике 1100 теряет 1 HP за тик; смерть от зоны даёт `kill` с `A == -1` и не добавляет `Kills`.
- [ ] **Step 3: Тесты мест, конца, детерминированности, повтора, карт.**
  - `place_test.go`: `TestPlacementsAliveAboveDead`, `TestPlacementsDeadByDeathTick`, `TestPlacementsTieSharesPlace` (два живых с одинаковыми HP и уроном → места 1, 1, третий — 3).
  - `TestOverWhenOneAlive` и `TestOverAtTickLimit`.
  - `TestDeterministic`: две игры с одним сидом и одной последовательностью псевдослучайных команд (генератор `rand.New(rand.NewPCG(7, 7))` в тесте) на 1200 тиков дают одинаковые `Frame()` на каждом тике и одинаковые события.
  - `replay_test.go`: `TestReplayRoundTrip` (Encode → Decode даёт равную структуру), `TestFrameRounding` (X = 1.23456 → 1.23; Hull = 0.123456 → 0.123).
  - `maps_test.go`: `TestMapsSymmetric` — у каждой карты для каждой стены есть стена, отражённая через центр (`x' = W − x − w`, `y' = H − y − h`); спавны `i` и `i+2` центрально симметричны; ни спавн, ни аптечка не внутри стены и не ближе 1.5 к стене; `TestPickMap` — `PickMap(0).Name == "arena"`, `PickMap(4).Name == "crossroads"`.
- [ ] **Step 4: Запустить тесты — падают** (`cd backend && go test ./internal/games/tanks/`), нет пакета.
- [ ] **Step 5: Реализовать** `rules.go`, `maps.go` (три карты: `arena` — открытая с четырьмя колоннами 3×3; `crossroads` — крест из четырёх стен с проходами в центре; `bunkers` — четыре укрытия-«Г» у спавнов и центральный блок; все проверены тестом симметрии), `game.go`, `place.go`, `replay.go` по описанию выше.
- [ ] **Step 6: Тесты зелёные** под `-race`; `go vet`, `gofmt -l` пусто.
- [ ] **Step 7: Коммит** `Add the tanks game engine`.

---

### Task 2: Протокол, домашние боты, стартовые наборы и GAME.md

**Files:**
- Create: `backend/internal/games/tanks/protocol.go`, `backend/internal/games/tanks/starter.go`, `backend/internal/games/tanks/GAME.md`, `backend/internal/games/tanks/AGENT_TASK.md`, `backend/internal/games/tanks/starter/python/{bot.json,bot.py,tanks.py}`, `backend/internal/games/tanks/starter/js/{bot.json,bot.js,tanks.js}`, `backend/internal/games/tanks/house/{house.go,idle.go,hunter.go,sniper.go}`
- Test: `backend/internal/games/tanks/protocol_test.go`, `backend/internal/games/tanks/house/house_test.go`, `backend/internal/games/tanks/starter_test.go`

**Interfaces:**
- Consumes: Task 1 (`Game`, `Rules`, `Rect`, `Zone`, `PlayerResult`, `Command`).
- Produces:

```go
package tanks

type PlayerInfo struct{ ID int `json:"id"`; Name string `json:"name"` }
type StartMsg struct {
	Type    string       `json:"type"` // "start"
	You     int          `json:"you"`
	Map     string       `json:"map"`
	Rules   Rules        `json:"rules"`
	Walls   []Rect       `json:"walls"`
	Players []PlayerInfo `json:"players"`
}
type TankView struct {
	ID     int     `json:"id"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Hull   float64 `json:"hull"`
	Turret float64 `json:"turret"`
	HP     int     `json:"hp"`
	Reload int     `json:"reload"`
	Alive  bool    `json:"alive"`
}
type ShellView struct {
	ID    int     `json:"id"`
	Owner int     `json:"owner"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	VX    float64 `json:"vx"`
	VY    float64 `json:"vy"`
}
type BonusView struct{ X float64 `json:"x"`; Y float64 `json:"y"` }
type TickMsg struct {
	Type    string      `json:"type"` // "tick"
	Tick    int         `json:"tick"`
	Tanks   []TankView  `json:"tanks"`
	Shells  []ShellView `json:"shells"`
	Bonuses []BonusView `json:"bonuses"` // только активные
	Zone    Zone        `json:"zone"`
}
type EndMsg struct {
	Type    string         `json:"type"` // "end"
	Place   int            `json:"place"`
	Players []PlayerResult `json:"players"`
}
type CommandMsg struct {
	Tick   int     `json:"tick"`
	Move   float64 `json:"move"`
	Turn   float64 `json:"turn"`
	Turret float64 `json:"turret"`
	Fire   bool    `json:"fire"`
}
func StartFor(g *Game, you int, names []string) StartMsg
func TickFor(g *Game) TickMsg
func (c CommandMsg) Command() Command
// ParseCommand разбирает строку бота. ok == false, если это не JSON-объект с числовым полем "tick".
func ParseCommand(line []byte) (cmd CommandMsg, ok bool)

// starter.go
//go:embed GAME.md AGENT_TASK.md starter
var GameMD string          // содержимое GAME.md
var AgentTaskMD string     // содержимое AGENT_TASK.md
func Starter(lang string) (map[string][]byte, error) // "python" | "javascript" | "js" → файлы набора (пути относительно корня бота) + "GAME.md"
```

```go
package house

type Strategy interface {
	Start(m tanks.StartMsg)
	Decide(t tanks.TickMsg) tanks.CommandMsg // Tick в ответе = t.Tick
}
func New(name string) (Strategy, bool) // "idle", "hunter", "sniper"
func Names() []string                  // {"hunter", "idle", "sniper"}
var Ladder = []string{"hunter", "sniper"}
```

Стратегии:
- `idle` — всегда нулевая команда.
- `hunter` — ближайший живой противник; поворачивает корпус к нему (`turn = clamp(3·angleDiff)`), едет вперёд, если до цели > 6, башня к цели, `fire` при `|angleDiff башни| < 0.08`.
- `sniper` — держит дистанцию 14–20 (ближе — назад, дальше — вперёд); если HP < 45 и есть активная аптечка, едет к ней; стрельба с упреждением (решение квадратного уравнения встречи снаряда и цели с текущей скоростью цели, оценённой по двум последним тикам); уклонение: если снаряд противника пройдёт ближе 1.5 от танка в ближайшие 10 тиков — поворот корпуса перпендикулярно снаряду и газ. Тупик у стены (позиция за 5 тиков не изменилась больше чем на 0.2 при ненулевом газе) — 5 тиков задним ходом с поворотом.

Стартовые наборы (без зависимостей, только стандартная библиотека):
- `bot.json`: `{"name": "my-tank", "language": "python", "entry": "bot.py"}` / `{"name": "my-tank", "language": "javascript", "entry": "bot.js"}`.
- `tanks.py` / `tanks.js` — SDK: чтение строк stdin, `run(strategy)` цикл (`start` → `ready`; `tick` → `strategy.decide(state)` → печать ответа с `tick` и `flush`; `end` → выход), помощники `angle_to`, `angle_diff`, `distance`, `nearest_enemy`, `lead_target`, `log(...)` в stderr.
- `bot.py` / `bot.js` — стратегия «охотник» (как домашний `hunter`) на 40–60 строк с комментариями, где улучшать.

`GAME.md` (английский, ~150 строк): цель, правила с числами из Global Constraints, система координат, протокол с примерами всех сообщений, таймауты и запас времени, «пишите логи в stderr — stdout только для ответов», статусы (`ok/crashed/timeout/invalid`), пакет (`bot.json`, лимиты, языки, только стандартная библиотека), локальная игра (`arena tanks play . house:hunter house:sniper`), как бот попадает в турнир (проверка `package/starts/stable/beats_idle`), рейтинг.

`AGENT_TASK.md` (текст `TASK.md` для proof `tanks-bot`): «You are improving a bot for the Agent Arena tanks game. The rules and the protocol are in GAME.md; your bot's recent results are in RESULTS.md. Edit the bot in place (keep bot.json valid; standard library only). Test it locally before you finish: `arena tanks play . house:hunter house:sniper --seed 1` … repeat with other seeds; aim to place first more often than the house bots. Do not edit GAME.md or RESULTS.md; they are not part of the bot. You have 20 minutes.»

- [ ] **Step 1: Тесты протокола** (`protocol_test.go`): `TestParseCommand` — `{"tick":3,"move":1,"fire":true}` → ok; `hello`, `[]`, `{"move":1}` (нет tick), `{"tick":"3"}` → не ok; `TestTickForHidesInactiveBonuses`; `TestStartForCarriesRulesAndWalls`; `TestCommandClamp` (`CommandMsg{Move: 5}.Command().Move == 1`).
- [ ] **Step 2: Тесты домашних ботов** (`house_test.go`): помощник `play(t, names ...string) []tanks.Placement` гоняет `tanks.Game` на карте `arena` напрямую со стратегиями (без процессов), 1200 тиков или до `Over`. `TestHunterBeatsIdle` (10 сидов: hunter занимает 1-е место в каждом), `TestSniperBeatsIdle` (то же), `TestSniperMostlyBeatsHunter` (20 сидов, sniper выше hunter минимум в 11), `TestDecideEchoesTick`.
- [ ] **Step 3: Тесты стартовых наборов** (`starter_test.go`): `TestStarterFiles` — `Starter("python")` содержит `bot.json`, `bot.py`, `tanks.py`, `GAME.md`, и `bot.json` разбирается с `language == "python"`; `Starter("js")` и `Starter("javascript")` — то же для JS; `Starter("rust")` — ошибка. Запуск стартовых ботов процессами проверяется в задаче 4.
- [ ] **Step 4: Запустить — падают.**
- [ ] **Step 5: Реализовать** протокол, домашних ботов, стартовые наборы, `GAME.md`, `AGENT_TASK.md`, `starter.go`.
- [ ] **Step 6: Ручная проверка стартовых ботов**: `cd backend/internal/games/tanks/starter/python && printf '%s\n' '{"type":"start","you":0,...}' | python3 -u bot.py` отвечает `{"type":"ready"}`; то же `node bot.js`.
- [ ] **Step 7: Тесты зелёные, vet/gofmt; коммит** `Add the tanks protocol, house bots and starter kits`.

---

### Task 3: Проведение матча (`internal/games/match`): Bot, домашние боты в горутине, Run

**Files:**
- Create: `backend/internal/games/match/{bot.go,house.go,run.go}`
- Test: `backend/internal/games/match/run_test.go`

**Interfaces:**
- Consumes: Task 1, Task 2 (`tanks.*`, `house.New`).
- Produces:

```go
package match

// Bot is one running bot. Send writes one line (a newline is appended). Lines yields every line the bot
// prints to stdout and is closed when stdout ends. Stderr returns the capped tail of stderr so far.
// Close kills the bot (and anything it started) and waits; it is safe to call more than once.
type Bot interface {
	Send(line []byte) error
	Lines() <-chan []byte
	Stderr() string
	Close() error
}

// Spec says what to launch. House != "" selects an in-process house bot; otherwise Dir holds an unpacked bot.
type Spec struct {
	House    string
	Dir      string
	Language string
	Entry    string
}

// Launcher starts bots. An error means the platform could not start it (docker down, interpreter missing);
// a bot whose own code fails to start is a Bot whose Lines closes immediately.
type Launcher interface {
	Launch(ctx context.Context, s Spec) (Bot, error)
}

// Command returns the argv for a language: python → ["python3", "-u", entry], javascript → ["node", entry].
func Command(language, entry string) ([]string, error)

// WithHouse serves House specs in process and passes everything else to next (next may be nil in tests
// that only use house bots; a non-house spec then fails with an error).
func WithHouse(next Launcher) Launcher

const (
	StatusOK      = "ok"
	StatusCrashed = "crashed"
	StatusTimeout = "timeout"
	StatusInvalid = "invalid"
)

type Config struct {
	Seed         int64
	Map          string        // "" → tanks.PickMap(Seed)
	Ticks        int           // 0 → rules default
	ReadyTimeout time.Duration // 0 → 5s
	TickTimeout  time.Duration // 0 → 200ms
	FreeTickTime time.Duration // 0 → 20ms
	Budget       time.Duration // 0 → 20s
	MaxNoise     int           // 0 → 1000
	StderrLimit  int           // 0 → 16 KiB
}

type Player struct {
	Name string
	Spec Spec
}

type PlayerOutcome struct {
	Slot      int
	Place     int
	Kills     int
	Damage    int
	DeathTick *int
	Status    string
	Ready     bool
	Answered  int // ticks answered in time while alive
	Asked     int // ticks sent while alive and not disabled
	Noise     int // stdout lines that were not commands
	Stderr    string
}

type Result struct {
	Replay  tanks.Replay // Players filled with Slot and Name only; the caller adds bot ids
	Players []PlayerOutcome
}

// Run plays one match. It returns an error only when the platform failed (a Launch error); everything a bot
// does wrong is recorded in its outcome. All bots are closed before Run returns.
func Run(ctx context.Context, l Launcher, cfg Config, players []Player) (Result, error)
```

Логика `Run`:
1. `len(players)` 2..4, иначе ошибка. Карта: `cfg.Map` или `PickMap(Seed)`; `rules := tanks.DefaultRules()`, `rules.Ticks = cfg.Ticks` если задано; `g := tanks.New(rules, m, n, seed)`.
2. Запустить всех (`Launch`); ошибка любого — закрыть запущенных, вернуть ошибку.
3. У каждого бота горутина-читатель не нужна: `Lines()` уже канал. Разослать `StartFor`, ждать `{"type":"ready"}` (строка, у которой после разбора `type == "ready"`) до `ReadyTimeout`; посторонние строки считаются в `Noise`. Не успел — `Status = timeout`, бот отключён (больше ничего не шлём), `Ready = false`.
4. Цикл, пока `!g.Over()`: `msg := TickFor(g)`; всем живым и не отключённым — `Send`; ошибка `Send` → `crashed`, отключить. Сбор ответов параллельно (горутина на бота, `sync.WaitGroup`): дедлайн `min(TickTimeout, FreeTickTime + remainingBudget)`; читать строки: `ParseCommand` ok и `Tick == g.Tick` → команда; ok и другой tick — выбросить (опоздавший ответ); не ok — `Noise++`; канал закрыт — `crashed`. Потраченное сверх `FreeTickTime` время вычитается из запаса; запас ≤ 0 → `timeout`, отключить. `Noise > MaxNoise` → `invalid`, отключить. `Asked++` у опрошенных, `Answered++` у ответивших. `g.Step(cmds)` (нулевые команды у тех, кто не ответил или отключён); события и `g.Frame()` в повтор (кадр 0 записывается до первого шага).
5. Конец: места из `Placements`; каждому не отключённому — `EndMsg` (ошибки игнорируются); `Close` всем; `Stderr()` в исходы (обрезка по `StderrLimit` делается в реализации Bot; `Run` дополнительно вызывает `sanitize.CleanLog(s, cfg.StderrLimit)`).
6. `ctx` отменён — закрыть всех и вернуть `ctx.Err()`.

`WithHouse`: домашний бот — горутина с каналом входящих строк; на `start` вызывает `Start` и печатает `ready`, на `tick` — `Decide`; `end` или `Close` — закрывает `Lines`.

- [ ] **Step 1: Тестовый бот-сценарий.** В `run_test.go` тип `scriptBot` реализует `Bot` поверх функции `func(in <-chan []byte, out chan<- []byte)` в горутине — так тесты задают поведение без процессов; `scriptLauncher` отдаёт `scriptBot` по `Spec.Dir` как ключу, остальное — `WithHouse`.
- [ ] **Step 2: Тесты.**
  - `TestHouseMatchCompletes`: hunter против idle через `WithHouse(nil)`, `Ticks: 300` → у hunter место 1, у обоих `Status == ok`, `len(Replay.Frames) == ticks played + 1`, `Replay.Engine == tanks.EngineVersion`.
  - `TestReadyTimeout`: бот никогда не отвечает на `start`, `ReadyTimeout: 50ms` → `Status == timeout`, `Ready == false`; матч доигрывается.
  - `TestSlowTickSkipped`: бот отвечает на каждый тик через 30 мс при `TickTimeout: 10ms`, `Budget: 1s` → большинство тиков пропущены, `Answered < Asked`.
  - `TestBudgetExhausted`: бот отвечает через 15 мс, `TickTimeout: 50ms`, `FreeTickTime: 1ms`, `Budget: 100ms` → после ~7 тиков `Status == timeout`.
  - `TestNoiseIgnored` (**Review Focus 1**): бот перед каждым ответом печатает `thinking...` → `Status == ok`, `Answered == Asked`, `Noise == Asked`.
  - `TestTooMuchNoise`: бот печатает 1100 строк мусора на первом тике → `Status == invalid`.
  - `TestLateAnswerDiscarded`: бот отвечает на тик N командой с `tick: N-1` → команда не применяется (танк не двигается), `Answered == 0`.
  - `TestCrash`: бот закрывает stdout после `ready` → `crashed`, танк стоит (позиция в кадрах не меняется).
  - `TestLaunchErrorIsPlatformError`: лаунчер возвращает ошибку на втором боте → `Run` возвращает ошибку, первый бот закрыт.
  - `TestContextCancelled`: отмена `ctx` посреди матча → `Run` возвращает `context.Canceled`, все боты закрыты.
- [ ] **Step 3: Запустить — падают. Step 4: реализовать. Step 5: зелёные под `-race`, vet/gofmt.**
- [ ] **Step 6: Коммит** `Add the match runner with house bots`.

---

### Task 4: Локальные процессы и `botpkg`-независимый запуск стартовых наборов

**Files:**
- Create: `backend/internal/games/match/process.go`
- Test: `backend/internal/games/match/process_test.go`

**Interfaces:**
- Consumes: Task 3 (`Bot`, `Spec`, `Command`), Task 2 (`tanks.Starter`).
- Produces:

```go
// ProcessLauncher runs a bot as a local process in Spec.Dir with the argv from Command, in its own process
// group so Close kills everything it started. Used by the connector, by tests and by ARENA_SANDBOX=fake.
// Never use it for untrusted code on the server.
type ProcessLauncher struct{}
func (ProcessLauncher) Launch(ctx context.Context, s Spec) (Bot, error)
```

Реализация: `exec.Command(argv...)`, `Dir = s.Dir`, `SysProcAttr{Setpgid: true}`; окружение — минимальное (`PATH`, `HOME=s.Dir`, `PYTHONDONTWRITEBYTECODE=1`, `LANG=C.UTF-8`); stdout читает `bufio.Reader` построчно, строки длиннее 64 KiB обрезаются и отправляются как есть (разбор их отвергнет); канал `Lines` с буфером 64; stderr — кольцевой буфер последних 16 KiB (`tailWriter`); `Send` пишет в stdin с дедлайном 1 с (через горутину и `select`), ошибка — вернуть; `Close` — закрыть stdin, подождать 200 мс, затем `syscall.Kill(-pid, SIGKILL)`, `Wait`. Не найден интерпретатор (`exec.ErrNotFound` при `Start`) — ошибка платформы.

- [ ] **Step 1: Тесты** (пропускаются через `t.Skip`, если нет `python3` / `node` в `PATH`, — кроме CI, где `ARENA_TEST_REQUIRE_DOCKER=1` превращает отсутствие интерпретатора в `t.Fatal`):
  - `TestPythonStarterPlays`: стартовый набор Python из `tanks.Starter("python")` пишется во временный каталог; матч против `house:idle`, `Ticks: 400` → у стартового бота `Status == ok`, `Ready`, `Answered ≥ 0.95·Asked`, место 1.
  - `TestJSStarterPlays`: то же для JavaScript.
  - `TestStdoutFlushWithoutExplicitFlush`: бот на Python, который печатает `print(json.dumps(...))` без `flush` (под `-u` это работает), проходит `ready`.
  - `TestCloseKillsRunaway` (**Review Focus 3**): бот `import time\nprint('{"type":"ready"}', flush=True)\nwhile True: pass` и дочерний процесс `subprocess.Popen(["sleep","60"])` → `Close()` возвращается < 2 с, процесс группы мёртв (`syscall.Kill(-pid, 0)` возвращает `ESRCH`).
  - `TestStderrCapped`: бот пишет 100 KiB в stderr → `len(Stderr()) ≤ 16 KiB` и это хвост.
- [ ] **Step 2: Падают. Step 3: реализовать. Step 4: зелёные.**
- [ ] **Step 5: Коммит** `Run bots as local processes`.

---

### Task 5: Пакет бота (`internal/games/botpkg`)

**Files:**
- Create: `backend/internal/games/botpkg/botpkg.go`
- Test: `backend/internal/games/botpkg/botpkg_test.go`

**Interfaces:**
- Consumes: ничего (стандартная библиотека).
- Produces:

```go
package botpkg

const (
	MaxArchive  = 1 << 20
	MaxUnpacked = 4 << 20
	MaxFiles    = 200
)

type Manifest struct {
	Name     string `json:"name"`
	Language string `json:"language"` // "python" | "javascript"
	Entry    string `json:"entry"`
}

// Error is a problem with the package itself, safe to show to its author.
type Error struct{ Msg string }
func (e *Error) Error() string

// PackDir packs a bot directory into a deterministic tar.gz (sorted, zero mtimes, mode 0644), skipping
// GAME.md, RESULTS.md, TASK.md, arena-agent.log, dotfiles and dot-directories (.git), __pycache__ and
// node_modules. A symlink anywhere is an *Error. The result is validated with Validate.
func PackDir(dir string) ([]byte, Manifest, error)

// Validate checks an uploaded archive and returns its manifest. Accepts tar.gz. Rules: ≤ MaxArchive
// compressed, ≤ MaxUnpacked and ≤ MaxFiles after ignoring macOS junk (__MACOSX/..., any "._*" file,
// .DS_Store); regular files and directories only; no absolute paths or "..". If every remaining entry sits
// under one top-level directory, that directory is stripped. bot.json must exist at the (stripped) root,
// have a name, a supported language and an entry that exists as a regular file and has no "..".
func Validate(archive []byte) (Manifest, error)

// Normalize returns Validate's view of the archive re-packed deterministically (junk dropped, wrapper
// directory stripped), so what is stored is exactly what runs.
func Normalize(archive []byte) ([]byte, Manifest, error)

// Unpack extracts a normalized archive into dst.
func Unpack(archive []byte, dst string) error
```

- [ ] **Step 1: Тесты:** `TestPackDirRoundTrip` (каталог со стартовым Python + `GAME.md` + `.git/` + `__pycache__/` → в архиве только файлы бота, Unpack восстанавливает их); `TestPackDirDeterministic` (два вызова — одинаковые байты); `TestPackDirRefusesSymlink`; `TestValidateMacOSWrapper` (**Review Focus 2**: архив `mybot/bot.json`, `mybot/bot.py`, `__MACOSX/mybot/._bot.py`, `mybot/._bot.py`, `mybot/.DS_Store` → принимается, `Normalize` даёт архив с `bot.json` и `bot.py` в корне); `TestValidateRejects` (таблица: не gzip; > 1 MiB; 201 файл; распакованный > 4 MiB; путь `../x`; абсолютный путь; симлинк; нет `bot.json`; `language: "rust"`; `entry` отсутствует; `entry: "../x.py"`) — у всех ошибка типа `*Error` с понятным `Msg`.
- [ ] **Step 2: Падают. Step 3: реализовать. Step 4: зелёные.**
- [ ] **Step 5: Коммит** `Add bot package validation`.

---

### Task 6: Коннектор: `arena tanks new | play`

**Files:**
- Create: `backend/cmd/arena/tanks.go`
- Modify: `backend/cmd/arena/main.go` (ветка `tanks`, текст `usage`)
- Test: `backend/cmd/arena/tanks_test.go`

**Interfaces:**
- Consumes: `tanks.Starter`, `tanks.EncodeReplay`, `match.Run`, `match.WithHouse(match.ProcessLauncher{})`, `botpkg.Validate`/`PackDir` (для проверки каталога перед игрой — через `PackDir`, чтобы локально ловить те же ошибки пакета).
- Produces: `arena tanks new <dir> [--lang python|js]`, `arena tanks play <bot>... [--seed N] [--map NAME] [--ticks N] [--out FILE]`. `arena tanks submit` появляется целиком в задаче 13; до неё в `usage` его нет.

Поведение:
- `new`: каталог не должен существовать или быть пустым; пишет `tanks.Starter(lang)`; печатает следующие шаги (`cd <dir> && arena tanks play . house:hunter`).
- `play`: 2–4 бота; `house:<name>` — домашний; путь — каталог с `bot.json` (читается `PackDir` → ошибки пакета печатаются как есть, код выхода 1). Сид по умолчанию — время. Печатает таблицу: `place  name  kills  damage  status  answered`, для каждого бота со статусом не `ok` — последние 20 строк stderr; пишет повтор (`--out`, по умолчанию `tanks-replay.json`, **несжатый** JSON — удобно открыть глазами и на `/tanks/replay`) и строку `Replay: tanks-replay.json — open it at <url>/tanks/replay`, где `<url>` из `config.yaml`, если он есть, иначе `https://arena.example.com`. Имена ботов — `name` из `bot.json` или `house:<name>`; одинаковые имена получают суффиксы `#2`.
- Код выхода 0, если матч сыгран (даже если бот упал), 1 — ошибка пакета или платформы (нет `python3`).

- [ ] **Step 1: Тесты** (`tanks_test.go`, функции вызываются напрямую, не через `os.Exit`): `TestTanksNewWritesStarter` (каталог содержит `bot.json`, `bot.py`, `tanks.py`, `GAME.md`); `TestTanksNewRefusesNonEmptyDir`; `TestTanksPlayHouseOnly` (`house:hunter house:idle --ticks 200 --out <tmp>` → файл существует, `json.Unmarshal` в `tanks.Replay` проходит, в выводе есть `hunter`); `TestTanksPlayStarter` (новый стартовый каталог против `house:idle`, пропуск без `python3` по правилам задачи 4); `TestTanksPlayBadPackage` (каталог без `bot.json` → ошибка с текстом `bot.json`).
- [ ] **Step 2: Падают. Step 3: реализовать. Step 4: зелёные.**
- [ ] **Step 5: Ручная проверка:** `cd backend && go run ./cmd/arena tanks new /tmp/t1 && go run ./cmd/arena tanks play /tmp/t1 house:hunter house:sniper --seed 3`.
- [ ] **Step 6: Коммит** `Add local tanks play to the connector`.

---

### Task 7: Рейтинг Weng–Lin (`internal/games/rating`)

**Files:**
- Create: `backend/internal/games/rating/rating.go`
- Test: `backend/internal/games/rating/rating_test.go`

**Interfaces:**
- Produces:

```go
package rating

const (
	DefaultMu    = 25.0
	DefaultSigma = 25.0 / 3
	Beta         = DefaultSigma / 2
	Kappa        = 0.0001
	RefreshSigma = 5.0
)

type Rating struct{ Mu, Sigma float64 }

func Default() Rating { return Rating{DefaultMu, DefaultSigma} }

// Display is the number shown to people: 1000 at the start, higher is better.
func Display(r Rating) int { return int(math.Round(1000 + 40*(r.Mu-3*r.Sigma))) }

// Refresh widens the uncertainty when a bot ships a new version.
func Refresh(r Rating) Rating { return Rating{r.Mu, math.Max(r.Sigma, RefreshSigma)} }

// Update applies one free-for-all result with the Plackett–Luce model of Weng & Lin (2011), as in
// OpenSkill. places[i] is player i's finishing place, 1 is best; equal places are a tie.
func Update(rs []Rating, places []int) []Rating {
	n := len(rs)
	c2 := 0.0
	for _, r := range rs {
		c2 += r.Sigma*r.Sigma + Beta*Beta
	}
	c := math.Sqrt(c2)
	sumQ := make([]float64, n) // Σ exp(mu_s/c) over s placed no better than q
	a := make([]float64, n)    // how many share q's place
	for q := 0; q < n; q++ {
		for s := 0; s < n; s++ {
			if places[s] >= places[q] {
				sumQ[q] += math.Exp(rs[s].Mu / c)
			}
			if places[s] == places[q] {
				a[q]++
			}
		}
	}
	out := make([]Rating, n)
	for i := 0; i < n; i++ {
		ei := math.Exp(rs[i].Mu / c)
		omega, delta := 0.0, 0.0
		for q := 0; q < n; q++ {
			if places[q] > places[i] {
				continue
			}
			quot := ei / sumQ[q]
			if q == i {
				omega += (1 - quot) / a[q]
			} else {
				omega -= quot / a[q]
			}
			delta += quot * (1 - quot) / a[q]
		}
		s2 := rs[i].Sigma * rs[i].Sigma
		omega *= s2 / c
		delta *= s2 / c2
		gamma := rs[i].Sigma / c
		delta *= gamma
		out[i] = Rating{Mu: rs[i].Mu + omega, Sigma: rs[i].Sigma * math.Sqrt(math.Max(1-delta, Kappa))}
	}
	return out
}
```

- [ ] **Step 1: Тесты:** `TestDisplayDefault` (`Display(Default()) == 1000`); `TestWinnerUpLoserDown` (2 игрока по умолчанию: μ победителя > 25, проигравшего < 25, σ обоих < σ₀); `TestFourPlayerOrder` (места 1..4 → μ строго убывают по месту); `TestTieSymmetric` (два равных игрока, места 1 и 1 → μ не меняются, σ уменьшаются); `TestOrderIndependent` (перестановка входа даёт ту же перестановку выхода); `TestUpsetMovesMore` (слабый (μ = 20) побеждает сильного (μ = 30) → прирост слабого больше, чем в матче равных); `TestRefresh` (`Refresh({30, 2}) == {30, 5}`, `Refresh({30, 7}) == {30, 7}`); `TestConverges` (1000 матчей 1×1 между «всегда первым» и «всегда вторым» → разница `Display` > 400, σ < 2).
- [ ] **Step 2: Падают. Step 3: реализовать (код выше). Step 4: зелёные.**
- [ ] **Step 5: Коммит** `Add Weng–Lin rating`.

---

### Task 8: Схема, изменения proofs, сервис ботов и версий, Sync

**Files:**
- Create: `backend/migrations/00003_games.sql`, `backend/internal/games/{model.go,bots.go,sync.go}`
- Modify: `backend/internal/proofs/{model.go,service.go,catalog.go,http_connector.go}`, `backend/cmd/migrate/main.go`
- Test: `backend/internal/games/bots_integration_test.go`, `backend/internal/proofs/service_integration_test.go` (дополнить)

**Interfaces:**
- Consumes: `botpkg.Normalize`, `rating.*`, `house.Names`, `tanks.Starter`, `tanks.AgentTaskMD`, `proofs.SyncCatalog`.
- Produces (SQL):

```sql
-- +goose Up
ALTER TABLE proof_tasks ADD COLUMN kind text NOT NULL DEFAULT 'proof' CHECK (kind IN ('proof', 'game_bot'));
ALTER TABLE proofs ADD COLUMN kind text NOT NULL DEFAULT 'proof' CHECK (kind IN ('proof', 'game_bot')),
    ADD COLUMN repo_tar bytea,
    ADD COLUMN repo_sha256 text;

ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check CHECK (kind IN ('run_proof', 'run_match', 'check_bot'));

CREATE TABLE game_bots (
    id text PRIMARY KEY,
    game text NOT NULL CHECK (game IN ('tanks')),
    owner_user_id text REFERENCES users (id),
    agent_id text REFERENCES agents (id),
    name text NOT NULL,
    house boolean NOT NULL DEFAULT false,
    mu double precision NOT NULL DEFAULT 25,
    sigma double precision NOT NULL DEFAULT 8.333333333333334,
    matches int NOT NULL DEFAULT 0,
    wins int NOT NULL DEFAULT 0,
    active_version_id text,
    last_match_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (house OR owner_user_id IS NOT NULL)
);
CREATE UNIQUE INDEX game_bots_owner_idx ON game_bots (game, owner_user_id) WHERE owner_user_id IS NOT NULL;
CREATE UNIQUE INDEX game_bots_name_idx ON game_bots (game, lower(name));

CREATE TABLE bot_versions (
    id text PRIMARY KEY,
    bot_id text NOT NULL REFERENCES game_bots (id),
    number int NOT NULL,
    source text NOT NULL CHECK (source IN ('agent', 'upload', 'house')),
    proof_id text REFERENCES proofs (id),
    language text NOT NULL,
    entry text NOT NULL DEFAULT '',
    archive bytea NOT NULL DEFAULT ''::bytea,
    archive_sha256 text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'active', 'rejected')),
    checks jsonb NOT NULL DEFAULT '[]'::jsonb,
    check_log text NOT NULL DEFAULT '',
    check_match_id text,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (bot_id, number)
);
ALTER TABLE game_bots ADD CONSTRAINT game_bots_active_version_fk FOREIGN KEY (active_version_id) REFERENCES bot_versions (id);

CREATE TABLE matches (
    id text PRIMARY KEY,
    game text NOT NULL CHECK (game IN ('tanks')),
    kind text NOT NULL CHECK (kind IN ('ladder', 'check')),
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'finished', 'infra_error')),
    seed bigint NOT NULL,
    map text NOT NULL,
    ticks int NOT NULL DEFAULT 0,
    featured boolean NOT NULL DEFAULT false,
    failure_reason text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    finished_at timestamptz
);
CREATE INDEX matches_finished_idx ON matches (game, kind, finished_at DESC) WHERE status = 'finished';
CREATE INDEX matches_open_idx ON matches (status) WHERE status IN ('queued', 'running');

CREATE TABLE match_players (
    match_id text NOT NULL REFERENCES matches (id) ON DELETE CASCADE,
    slot int NOT NULL,
    bot_id text NOT NULL REFERENCES game_bots (id),
    version_id text NOT NULL REFERENCES bot_versions (id),
    place int,
    kills int NOT NULL DEFAULT 0,
    damage int NOT NULL DEFAULT 0,
    death_tick int,
    status text NOT NULL DEFAULT '',
    answered int NOT NULL DEFAULT 0,
    asked int NOT NULL DEFAULT 0,
    noise int NOT NULL DEFAULT 0,
    mu_before double precision,
    sigma_before double precision,
    mu_after double precision,
    sigma_after double precision,
    stderr_tail text NOT NULL DEFAULT '',
    PRIMARY KEY (match_id, slot)
);
CREATE INDEX match_players_bot_idx ON match_players (bot_id);

CREATE TABLE match_replays (
    match_id text PRIMARY KEY REFERENCES matches (id) ON DELETE CASCADE,
    data bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tanks_broadcasts (
    id text PRIMARY KEY,
    match_id text NOT NULL REFERENCES matches (id),
    starts_at timestamptz NOT NULL,
    duration_ms int NOT NULL
);
CREATE INDEX tanks_broadcasts_starts_idx ON tanks_broadcasts (starts_at DESC);

GRANT SELECT, INSERT, UPDATE, DELETE ON game_bots, bot_versions, matches, match_players, match_replays, tanks_broadcasts TO arena_app;

-- +goose Down
DROP TABLE tanks_broadcasts;
DROP TABLE match_replays;
DROP TABLE match_players;
DROP TABLE matches;
ALTER TABLE game_bots DROP CONSTRAINT game_bots_active_version_fk;
DROP TABLE bot_versions;
DROP TABLE game_bots;
ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check CHECK (kind IN ('run_proof'));
ALTER TABLE proofs DROP COLUMN repo_sha256, DROP COLUMN repo_tar, DROP COLUMN kind;
ALTER TABLE proof_tasks DROP COLUMN kind;
```

(Имя ограничения `jobs_kind_check` — то, что Postgres генерирует для inline CHECK в `00002`; проверить `\d jobs` в тестовой базе и поправить, если отличается.)

- Produces (proofs):

```go
const (
	KindProof   = "proof"
	KindGameBot = "game_bot"
)
// Task: + Kind string `json:"-"`; manifest + "kind" (по умолчанию "proof").
// Proof: + Kind string `json:"kind"`. proofCols + kind; scanProof сканирует kind.
func (s *Service) Tasks(ctx) ([]Task, error)            // только kind = 'proof'
func (s *Service) Create(ctx, userID, slug) (Proof, error) // задача с kind != 'proof' → 404
// CreateWithRepo — как Create (агент онлайн, одна открытая, дневной лимит, аудит), но для задачи kind='game_bot'
// и с собственным репозиторием; repo_sha256 = hex(sha256(repoTar)).
func (s *Service) CreateWithRepo(ctx context.Context, userID, slug string, repoTar []byte) (Proof, error)
// Claim: Task.RepoSHA256 = coalesce(p.repo_sha256, t.repo_sha256). RepoTar: coalesce(p.repo_tar, t.repo_tar).
// ProofFacts: все три факта только по kind = 'proof' (стадия агента — про базовую проверку).
// TarFiles packs files deterministically, the same way TarDir does (TarDir now builds the map and calls it).
func TarFiles(files map[string][]byte) ([]byte, error)
```

`http_connector.go`: `nextTaskResponse` + `Kind string \`json:"kind"\``.

- Produces (games):

```go
package games

const Game = "tanks"

type Check struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}
type VersionView struct {
	ID           string    `json:"id"`
	Number       int       `json:"number"`
	Source       string    `json:"source"`
	Status       string    `json:"status"`
	Language     string    `json:"language"`
	Checks       []Check   `json:"checks"`
	CheckLog     string    `json:"check_log"`
	CheckMatchID *string   `json:"check_match_id"`
	ProofID      *string   `json:"proof_id"`
	CreatedAt    time.Time `json:"created_at"`
}
type MyBot struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Rating        int     `json:"rating"`
	Mu            float64 `json:"mu"`
	Sigma         float64 `json:"sigma"`
	Matches       int     `json:"matches"`
	Wins          int     `json:"wins"`
	ActiveVersion *int    `json:"active_version"`
}
type MyTanks struct {
	Bot       *MyBot         `json:"bot"`
	Versions  []VersionView  `json:"versions"`   // новые первыми, до 20
	AgentRuns []proofs.Proof `json:"agent_runs"` // proofs kind=game_bot агента пользователя, новые первыми, до 10, без diff и лога
	Matches   []MatchView    `json:"matches"`    // задача 10; до этого пустой срез
}

type Service struct { /* pool, proofs, launcher, log, now, cfg */ }
type Config struct {
	CheckTicks int // 600
}
func NewService(pool *db.Pool, ps *proofs.Service, l match.Launcher, log *slog.Logger) *Service

func (s *Service) MyTanks(ctx context.Context, userID string) (MyTanks, error)
// SaveBot creates the caller's bot or renames it. Name rules = agent name regex (^[A-Za-z0-9][A-Za-z0-9_-]{1,31}$).
// Taken name (any case, including house bots) → 409 name_taken; bad name → 422 validation_failed with fields.name.
func (s *Service) SaveBot(ctx context.Context, userID, name string) (MyBot, error)
// UploadVersion normalizes the archive (botpkg.Normalize; *botpkg.Error → 422 invalid_package with its Msg),
// creates the bot if missing (name from bot.json, else 409 no_bot "Name your bot first"), enforces 20 uploads
// per bot per 24h (429 upload_limit), inserts a pending version with the next number and enqueues check_bot.
func (s *Service) UploadVersion(ctx context.Context, userID string, archive []byte) (VersionView, error)
// UploadVersionForAgent is UploadVersion for the owner of agentID (connector route).
func (s *Service) UploadVersionForAgent(ctx context.Context, agentID string, archive []byte) (VersionView, error)

// Sync upserts the house bots (ids "bot_house_<name>", one active version each, source 'house', language
// 'builtin', entry = name) and the proof task 'tanks-bot' (kind game_bot, repo = Python starter + GAME.md,
// empty hidden tarball, agent timeout 1200 s, sandbox timeout 120 s, task_md = tanks.AgentTaskMD).
// Idempotent; run by cmd/migrate on every start.
func Sync(ctx context.Context, pool *db.Pool) error
```

Идентификаторы — `idgen.New("bot")`, `idgen.New("bv")`, `idgen.New("match")`, `idgen.New("bc")`.

- [ ] **Step 1: Миграция.** Написать `00003_games.sql`; `cd backend && go test ./internal/platform/...` (dbtest применяет миграции) — зелёный.
- [ ] **Step 2: Тесты proofs** (дополнить `service_integration_test.go`): `TestTasksHidesGameBotTasks` (тест вставляет строку `proof_tasks` с `kind = 'game_bot'` через `AdminPool` напрямую: `games` импортирует `proofs`, обратный импорт дал бы цикл) → `Tasks` её не возвращает, `Create` с её slug → 404; `TestCreateWithRepoOverridesRepo` → `Claim` отдаёт `RepoSHA256` собственного архива, `RepoTar` отдаёт его байты; `CreateWithRepo` для задачи kind='proof' → 404; `TestProofFactsIgnoreGameBot` (пройденный game_bot proof не делает стадию `operational`).
- [ ] **Step 3: Тесты games** (`bots_integration_test.go`, `dbtest.New`, пользователь и агент создаются через `identity`/`agents` сервисы или прямыми вставками через AdminPool, как в существующих тестах): `TestSyncCreatesHouseBotsIdempotent` (два вызова → 3 домашних бота, у каждого активная версия, строка `tanks-bot` в `proof_tasks` с `kind = 'game_bot'`); `TestSaveBotCreatesAndRenames`; `TestSaveBotNameTaken` (**Review Focus 5**: `Hunter` → 409 `name_taken`; имя чужого бота в другом регистре → 409); `TestSaveBotBadName` → 422; `TestUploadVersionCreatesPendingAndJob` (архив стартового набора → версия 1 `pending`, job `check_bot` с `{"version_id": ...}`; вторая загрузка → номер 2); `TestUploadWithoutBotUsesManifestName`; `TestUploadInvalidPackage` → 422 `invalid_package`; `TestUploadLimit` (21-я за сутки → 429).
- [ ] **Step 4: Падают. Step 5: реализовать** proofs-изменения, `model.go`, `bots.go`, `sync.go`; `cmd/migrate/main.go` вызывает `games.Sync` после синхронизации каталога (всегда, не только при `ARENA_PROOFS_DIR`).
- [ ] **Step 6: Все тесты backend зелёные** (`ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./...`), e2e среза 1 не сломан (у `Proof` в `openapi.yaml` добавить `kind: {type: string, enum: [proof, game_bot]}` в required, у ответа `tasks/next` — `kind`).
- [ ] **Step 7: Коммит** `Add the games schema and bot versions`.

---

### Task 9: Docker-запуск ботов и образ

**Files:**
- Create: `backend/internal/games/match/docker.go`, `backend/internal/games/match/runtime/Dockerfile`
- Modify: `Makefile` (цель `bot-image`, зависимость `up`), `.github/workflows/ci.yml` (сборка образа перед тестами)
- Test: `backend/internal/games/match/docker_integration_test.go`

**Interfaces:**
- Consumes: Task 3/4 (`Bot`, `Spec`, `Command`, общий `tailWriter` и построчное чтение из `process.go` — вынести в `lines.go`, если ещё не общие).
- Produces:

```go
// DockerLauncher runs each bot in its own throwaway container: no network, 256 MiB, half a CPU, 64 pids,
// all capabilities dropped, no-new-privileges, uid 65534, 16 MiB tmpfs /tmp. Code is copied in with
// docker cp (the API may itself run in a container), stdin/stdout are attached with docker start -ai.
type DockerLauncher struct{ Image string } // "arena-bot-runtime:1"
func (d DockerLauncher) Launch(ctx context.Context, s Spec) (Bot, error)
```

`runtime/Dockerfile`:

```dockerfile
FROM python:3.12-slim
COPY --from=node:22-slim /usr/local/bin/node /usr/local/bin/node
RUN mkdir /bot && chown 65534:65534 /bot
WORKDIR /bot
USER 65534:65534
ENV PYTHONDONTWRITEBYTECODE=1 HOME=/tmp LANG=C.UTF-8
```

Запуск: `docker create -i --network none --memory 256m --memory-swap 256m --cpus 0.5 --pids-limit 64 --cap-drop=ALL --security-opt=no-new-privileges --user 65534:65534 --tmpfs /tmp:rw,size=16m -w /bot <image> <argv...>`; `docker cp <dir>/. <id>:/bot`; `docker start -ai <id>` с pipe на stdin/stdout/stderr. `Close`: закрыть stdin, 300 мс, `docker rm -f <id>`, дождаться процесса CLI. Ошибки `create`/`cp` и код 125 у `start` — ошибка платформы (как в `proofs/sandbox`).

- [ ] **Step 1: Тесты** (`dbtest`-правила для Docker: недоступен → skip, при `ARENA_TEST_REQUIRE_DOCKER=1` — провал; образ собирается в тесте `docker build -t arena-bot-runtime:1 internal/games/match/runtime`, как `proofs/sandbox`): `TestDockerPythonStarterPlays` (стартовый Python против `house:idle`, 300 тиков → `ok`, место 1); `TestDockerJSStarterPlays`; `TestDockerNoNetwork` (бот пытается `socket.create_connection(("1.1.1.1", 53), 1)` при старте и пишет результат в stderr → в `Stderr()` есть ошибка сети, бот продолжает играть); `TestDockerCloseKillsRunaway` (**Review Focus 3**: бесконечный цикл → `Close` < 5 с, `docker ps -a --filter id=<id>` пусто).
- [ ] **Step 2: Падают. Step 3: реализовать; `Makefile`: `bot-image: docker build -q -t arena-bot-runtime:1 backend/internal/games/match/runtime`, `up: .env proof-image bot-image`; CI: шаг `docker build -q -t arena-bot-runtime:1 backend/internal/games/match/runtime` перед тестами.**
- [ ] **Step 4: Зелёные с `ARENA_TEST_REQUIRE_DOCKER=1`** (на Colima см. `CLAUDE.md` про `DOCKER_HOST`).
- [ ] **Step 5: Коммит** `Run bots in Docker containers`.

---

### Task 10: Проверка версии, матчи, ладдер, эфир, воркер

**Files:**
- Create: `backend/internal/games/{qualify.go,matches.go,ladder.go,broadcast.go,worker.go}`
- Modify: `backend/internal/games/model.go`, `backend/internal/games/bots.go` (`MyTanks.Matches`), `backend/cmd/api/{config.go,main.go}`
- Test: `backend/internal/games/{qualify_integration_test.go,ladder_integration_test.go}`

**Interfaces:**
- Consumes: `match.Run`, `match.Launcher`, `botpkg.Unpack`, `rating.*`, `tanks.EncodeReplay`, `jobs.*`, Task 8.
- Produces:

```go
type MatchPlayerView struct {
	Slot         int     `json:"slot"`
	BotID        string  `json:"bot_id"`
	Name         string  `json:"name"`
	House        bool    `json:"house"`
	Source       string  `json:"source"`
	Version      int     `json:"version"`
	Place        *int    `json:"place"`
	Kills        int     `json:"kills"`
	Damage       int     `json:"damage"`
	DeathTick    *int    `json:"death_tick"`
	Status       string  `json:"status"`
	RatingBefore *int    `json:"rating_before"`
	RatingAfter  *int    `json:"rating_after"`
}
type MatchView struct {
	ID         string            `json:"id"`
	Kind       string            `json:"kind"`
	Status     string            `json:"status"`
	Map        string            `json:"map"`
	Seed       int64             `json:"seed"`
	Ticks      int               `json:"ticks"`
	Featured   bool              `json:"featured"`
	HasReplay  bool              `json:"has_replay"`
	CreatedAt  time.Time         `json:"created_at"`
	StartedAt  *time.Time        `json:"started_at"`
	FinishedAt *time.Time        `json:"finished_at"`
	Players    []MatchPlayerView `json:"players"`
}
type LeaderboardEntry struct {
	Rank    int     `json:"rank"`
	BotID   string  `json:"bot_id"`
	Name    string  `json:"name"`
	Rating  int     `json:"rating"`
	Mu      float64 `json:"mu"`
	Sigma   float64 `json:"sigma"`
	Matches int     `json:"matches"`
	Wins    int     `json:"wins"`
	House   bool    `json:"house"`
	Source  string  `json:"source"`
	Version int     `json:"version"`
}
type VersionPublic struct {
	Number    int       `json:"number"`
	Source    string    `json:"source"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}
type BotProfile struct {
	LeaderboardEntry
	CreatedAt time.Time       `json:"created_at"`
	Versions  []VersionPublic `json:"versions"`
}
type LiveView struct {
	MatchID    *string    `json:"match_id"`
	StartsAt   *time.Time `json:"starts_at"`
	DurationMS int        `json:"duration_ms"`
	Now        time.Time  `json:"now"`
}
type MatchLog struct {
	MatchID string `json:"match_id"`
	Slot    int    `json:"slot"`
	Stderr  string `json:"stderr"`
}

// Qualify runs the four checks on a pending version, stores checks, check_log, the check match (kind
// 'check', players: candidate, house idle, house hunter; ticks cfg.CheckTicks) with its replay, and either
// activates the version (bot.active_version_id; rating kept, sigma = rating.Refresh) or rejects it. A version
// that is not pending is left alone and its stored result returned. err != nil only for platform failures.
func (s *Service) Qualify(ctx context.Context, versionID string) (passed bool, checks []Check, err error)

func (s *Service) RunMatch(ctx context.Context, matchID string) error
func (s *Service) Leaderboard(ctx context.Context, limit int) ([]LeaderboardEntry, error) // активные, по рейтингу убыв.; домашний idle не показывается
func (s *Service) Matches(ctx context.Context, botID string, limit int) ([]MatchView, error) // finished ladder, новые первыми; limit ≤ 50
func (s *Service) Match(ctx context.Context, id string) (MatchView, error)
func (s *Service) Replay(ctx context.Context, id string) ([]byte, error) // gzip; 404 not_found если нет
func (s *Service) Bot(ctx context.Context, id string) (BotProfile, error)
func (s *Service) MatchLog(ctx context.Context, userID, matchID string) (MatchLog, error) // только слот своего бота; чужой матч → 404

// ScheduleTick creates at most one ladder match when fewer than concurrency matches are queued or running
// and the interval since the last created match has passed. Returns the new match id or "".
func (s *Service) ScheduleTick(ctx context.Context, concurrency int, interval time.Duration) (string, error)
func (s *Service) SweepStuck(ctx context.Context) (int, error)      // running/queued дольше 10 минут → infra_error
func (s *Service) PruneReplays(ctx context.Context) (int, error)    // старше 3 суток, не featured, не check
func (s *Service) RefreshBroadcast(ctx context.Context) error
func (s *Service) Live(ctx context.Context) (LiveView, error)

type WorkerConfig struct {
	Interval    time.Duration // ARENA_MATCH_INTERVAL, 20s
	Concurrency int           // ARENA_MATCH_CONCURRENCY, 1
}
type Worker struct{ /* svc, queue, cfg, log, owner */ }
func NewWorker(svc *Service, pool *db.Pool, cfg WorkerConfig, log *slog.Logger) *Worker
// Run starts cfg.Concurrency goroutines that claim run_match and check_bot jobs (lease 10 min), and one
// loop that every 2 s calls ScheduleTick and RefreshBroadcast, every minute SweepStuck, every hour PruneReplays.
func (w *Worker) Run(ctx context.Context)
```

Детали:
- **Проверки** `Qualify`: `package` (архив распаковывается, `bot.json` читается — провал здесь невозможен после `Normalize`, но проверка пишется для полноты); `starts` = `Ready`; `stable` = статус не `crashed`/`timeout`/`invalid` и `Answered ≥ 0.95·Asked`; `detail` содержит `answered N of M ticks`, а при `Noise > 0` — `printed K non-command lines to stdout; write logs to stderr`; `beats_idle` = место кандидата меньше места idle. `check_log` = `Stderr` кандидата. Пропущенные после провала проверки пишутся с `passed: false, detail: "skipped"`.
- **Запуск бота из БД**: домашний → `Spec{House: entry}`; иначе `botpkg.Unpack` во временный каталог (`os.MkdirTemp(workDir, "bot-")`, удаляется после матча) → `Spec{Dir, Language, Entry}`.
- **RunMatch**: `UPDATE matches SET status='running', started_at=now() WHERE id=$1 AND status IN ('queued','running')` (повторный захват переигрывает с нуля — **Review Focus 4**); игроки из `match_players` (версии зафиксированы при создании); `match.Run` с контекстом `ticks·0.2 s + 60 s`; ошибка платформы → вернуть ошибку (job повторится; после последней попытки — `infra_error`, рейтинг не трогается); успех — одна транзакция: `UPDATE matches SET status='finished' ... WHERE id=$1 AND status='running'` (0 строк → ничего не писать, вернуть nil), для `ladder` — `rating.Update` по местам из текущих `mu/sigma` ботов (`SELECT ... FOR UPDATE` по id в порядке возрастания, чтобы не было взаимоблокировок), `mu_before/…/mu_after`, `matches+1`, `wins+1` у места 1, `last_match_at = now()`; `match_players` — места, статистика, `sanitize.CleanLog(stderr, 16<<10)`; `match_replays` — `EncodeReplay` с `Players` из БД (bot_id, name, version, house, source).
- **ScheduleTick**: пользовательские активные боты (`house = false AND active_version_id IS NOT NULL`); есть — «ведущий» с самым старым `last_match_at` (NULL первыми), затем больший σ; соперники — до трёх из шести ближайших по `Display`, выбранных `rand.Shuffle` с сидом от времени; добор домашними из `house.Ladder`. Нет пользовательских — матч `hunter` против `sniper` при условии, что последний матч создан больше 2 минут назад. Сид — `rand.Int64N(1 << 53)`, карта `PickMap(seed)`. Вставка `matches` + `match_players` + `jobs.Enqueue(tx, "run_match", {"match_id": id}, "match:"+id)` в одной транзакции. Проверка «меньше concurrency открытых» и «прошёл interval с последнего `created_at` ladder-матча» внутри той же транзакции под `pg_advisory_xact_lock(hashtext('tanks_schedule'))`.
- **check_bot job**: payload `{"version_id"}` → `Qualify`.
- **Эфир** `RefreshBroadcast`: под `pg_advisory_xact_lock(hashtext('tanks_broadcast'))`; если последняя строка `tanks_broadcasts` закончилась (`starts_at + duration_ms ≤ now()`) или её нет — кандидат: законченный `ladder`-матч за 30 минут с повтором, которого нет в `tanks_broadcasts`, с максимальной суммой `Display(mu_after, sigma_after)` игроков, затем больше убийств; есть — `featured = true`, новая строка `starts_at = now() + 3s`, `duration_ms = ticks·100`; нет — повторить последний показанный (новая строка с тем же `match_id`). `Live` отдаёт последнюю строку и `now` сервера; пусто — `match_id: null`.
- **cmd/api**: `config` + `matchInterval` (`ARENA_MATCH_INTERVAL`, `time.ParseDuration`, по умолчанию 20s), `matchConcurrency` (`ARENA_MATCH_CONCURRENCY`, по умолчанию 1), `botImage` (`ARENA_BOT_IMAGE`, по умолчанию `arena-bot-runtime:1`); лаунчер `match.WithHouse(match.DockerLauncher{Image: cfg.botImage})`, при `ARENA_SANDBOX=fake` — `match.WithHouse(match.ProcessLauncher{})`; `games.NewService`, `go games.NewWorker(...).Run(ctx)`.

- [ ] **Step 1: Тесты** (`dbtest`, лаунчер `match.WithHouse(match.ProcessLauncher{})`, стартовые наборы из `tanks.Starter`; нет `python3` — правила задачи 4):
  - `qualify_integration_test.go`: `TestQualifyStarterActivates` (загрузка стартового Python → `Qualify` → `passed`, 4 проверки `passed`, версия `active`, у бота `active_version_id`, есть матч `kind = check` с повтором); `TestQualifyCrashingBotRejected` (`bot.py` = `raise SystemExit(1)` → `rejected`, `starts` провал, остальные `skipped`; прежняя активная версия остаётся активной); `TestQualifyNoisyBotHint` (стартовый бот, который печатает `debug` перед каждым ответом → `passed`, в `detail` у `stable` есть `stderr`); `TestQualifyRefreshesSigma` (у бота σ = 2 → после активации новой версии σ = 5, μ не изменилось); `TestQualifyIdempotent` (второй вызов на уже `active` версии ничего не меняет).
  - `ladder_integration_test.go`: `TestScheduleHouseOnly` (нет пользовательских → матч hunter/sniper; сразу второй вызов → "" из-за 2 минут); `TestScheduleRespectsConcurrency` (открытый матч и concurrency 1 → ""); `TestScheduleFillsWithHouse` (один пользовательский бот → матч с ним и двумя домашними); `TestRunMatchUpdatesRatings` (матч из двух домашних + пользовательский idle-подобный бот: места записаны, `mu_before/after` заполнены, у победителя μ выросло, `matches` +1, `wins` +1 у первого, повтор декодируется, `Players[i].BotID` заполнен); `TestRunMatchRerunAfterCrash` (**Review Focus 4**: матч вручную переведён в `running`, `RunMatch` доигрывает его, рейтинг применён ровно один раз: повторный `RunMatch` на `finished` ничего не меняет); `TestSweepStuck` (матч `running` со `started_at = now() − 11 min` → `infra_error`, рейтинги ботов не изменились); `TestPruneReplays` (повтор матча 4-дневной давности удалён, `featured` и `check` — нет, `HasReplay` = false у удалённого); `TestBroadcastPicksBestAndRepeats` (два матча с разными рейтингами → выбран сильнейший, `featured`; до окончания эфира `RefreshBroadcast` ничего не меняет; после — новый или повтор); `TestLeaderboardOrderAndIdleHidden`; `TestMatchLogOnlyOwnSlot`.
- [ ] **Step 2: Падают. Step 3: реализовать. Step 4: зелёные под `-race` с `ARENA_TEST_REQUIRE_DOCKER=1`.**
- [ ] **Step 5: Коммит** `Add bot checks, the ladder and the broadcast`.

---

### Task 11: Путь агента: proof `game_bot`, RESULTS.md, судья в воркере proofs

**Files:**
- Create: `backend/internal/games/agentrun.go`
- Modify: `backend/internal/proofs/worker.go`, `backend/cmd/api/main.go`
- Test: `backend/internal/games/agentrun_integration_test.go`, `backend/internal/proofs/worker_integration_test.go` (дополнить)

**Interfaces:**
- Consumes: `proofs.CreateWithRepo`, `proofs.TarFiles`, Task 10 (`Qualify`), `botpkg.PackDir`.
- Produces:

```go
package proofs

type GameBotVerdict struct {
	Passed bool
	Reason string       // "" | "invalid_package" | "bot_rejected"
	Tests  []TestResult // one per check, Name = check name
	Output string       // check details and the bot's stderr, shown as "Sandbox output"
}
type GameBotJudge interface {
	JudgeProof(ctx context.Context, proofID, agentID, dir string) (GameBotVerdict, error)
}
func (w *Worker) SetGameBotJudge(j GameBotJudge)
```

`RunProof`: выборка дополняется `p.kind` и `coalesce(p.repo_tar, t.repo_tar)`. Для `game_bot`: распаковать, `applyDiff` (те же правила, `diffTouchesTestFiles` не применяется), `empty_diff` — как у проверок; затем `judge.JudgeProof(ctx, proofID, agentID, dir)`; судьи нет — ошибка (job повторится → `infra_error`); ошибка судьи — ошибка платформы; вердикт → `finish(status, reason, &SandboxResult{Tests: v.Tests, Output: v.Output, ExitCode: 0})`.

```go
package games

// StartAgentRun creates the caller's bot if missing (named after the agent), builds the task repository —
// files of the active version (or the Python starter), GAME.md and RESULTS.md — and creates a game_bot proof
// with it. Errors from proofs (agent_offline, proof_in_progress, daily_limit, no_agent) pass through.
func (s *Service) StartAgentRun(ctx context.Context, userID string) (proofs.Proof, error)

// JudgeProof implements proofs.GameBotJudge: PackDir(dir) (*botpkg.Error → failed invalid_package), a pending
// version with source 'agent' and proof_id, Qualify, verdict from the checks (failed → bot_rejected).
func (s *Service) JudgeProof(ctx context.Context, proofID, agentID, dir string) (proofs.GameBotVerdict, error)

// resultsMD renders RESULTS.md: bot name, rating and matches, the active version's checks, and the last 20
// finished matches of the bot (place of N, kills, damage, status, opponents), plus the first 30 lines of
// check_log. Markdown, English. A bot with no history gets a short "no matches yet" file.
func (s *Service) resultsMD(ctx context.Context, tx pgx.Tx, botID string) (string, error)
```

`cmd/api/main.go`: `worker := proofs.NewWorker(...); worker.SetGameBotJudge(gs); go worker.Run(ctx)`.

- [ ] **Step 1: Тесты:** `TestStartAgentRunBuildsRepo` (агент онлайн → proof `kind = game_bot`, `task_slug = tanks-bot`; `RepoTar` содержит `bot.json`, `bot.py`, `tanks.py`, `GAME.md`, `RESULTS.md`; бот создан с именем агента); `TestStartAgentRunUsesActiveVersion` (у бота активная версия с изменённым `bot.py` → в репозитории именно он); `TestStartAgentRunOffline` → 409 `agent_offline`; `TestGameBotProofEndToEnd` (proof → `Claim` → `SubmitResult` с diff, который меняет `bot.py` на стартовый с другой константой → `RunProof` → `passed`, `sandbox_result.tests` — 4 проверки, у бота новая активная версия `source = agent`, `proof_id` заполнен); `TestGameBotProofBrokenBot` (diff заменяет `bot.py` на `raise SystemExit(1)` → `failed`, `bot_rejected`, активная версия прежняя); `TestGameBotProofDeletesManifest` (diff удаляет `bot.json` → `failed`, `invalid_package`); в `worker_integration_test.go` — `TestGameBotProofWithoutJudgeIsInfra` (судья не задан → ошибка, после исчерпания попыток `infra_error`).
- [ ] **Step 2: Падают. Step 3: реализовать. Step 4: зелёные.**
- [ ] **Step 5: Коммит** `Let agents write tanks bots through proofs`.

---

### Task 12: HTTP, OpenAPI, e2e

**Files:**
- Create: `backend/internal/games/{http_public.go,http_me.go,http_connector.go}`
- Modify: `backend/cmd/api/handler.go`, `backend/cmd/api/main.go` (`deps.games`), `backend/contracts/openapi/openapi.yaml`, `backend/cmd/api/main_test.go`

**Interfaces:**
- Consumes: всё из задач 8–11.
- Produces:

```go
func RegisterPublicRoutes(mux *http.ServeMux, s *Service)    // GET /api/v1/tanks/{leaderboard,matches,matches/{id},matches/{id}/replay,bots/{id},live}
func RegisterOwnerRoutes(mux *http.ServeMux, s *Service)     // GET /api/v1/me/tanks; POST /api/v1/me/tanks/{bot,versions,agent-runs}; GET /api/v1/me/tanks/matches/{id}/log
func RegisterConnectorRoutes(mux *http.ServeMux, s *Service) // POST /api/v1/connector/tanks/versions
```

- `GET /tanks/leaderboard?limit=` (по умолчанию 100, максимум 500) → `{items: LeaderboardEntry[]}`.
- `GET /tanks/matches?bot_id=&limit=` (по умолчанию 20, максимум 50) → `{items: MatchView[]}`.
- `GET /tanks/matches/{id}` → `MatchView`; `GET /tanks/matches/{id}/replay` → `200 application/gzip` байты (`Content-Length`), 404 `not_found`.
- `GET /tanks/bots/{id}` → `BotProfile`; `GET /tanks/live` → `LiveView`.
- `GET /me/tanks` → `MyTanks`; `POST /me/tanks/bot {name}` → `200 MyBot`; `POST /me/tanks/versions {archive_base64}` (тело до 2 MiB, `httpx.ReadBodyLimit`; плохой base64 → 422 `validation_failed` с `fields.archive_base64`) → `201 VersionView`; `POST /me/tanks/agent-runs` → `201 Proof`; `GET /me/tanks/matches/{id}/log` → `MatchLog`.
- `POST /connector/tanks/versions {archive_base64}` → `201 VersionView`.

`handler.go`: `public := http.NewServeMux(); games.RegisterPublicRoutes(public, d.games); api.Handle("/api/v1/tanks/", public)`; `games.RegisterOwnerRoutes(owner, d.games)` и `api.Handle("/api/v1/me/tanks", session(owner))`, `api.Handle("/api/v1/me/tanks/", session(owner))`; `games.RegisterConnectorRoutes(connector, d.games)`.

OpenAPI: пути выше (публичные с `security: []`), схемы `LeaderboardEntry`, `MatchPlayerView`, `MatchView`, `BotProfile`, `VersionPublic`, `VersionView`, `Check`, `MyBot`, `MyTanks`, `LiveView`, `MatchLog`; ответ повтора `application/gzip` `type: string, format: binary`; `Proof.kind`; ошибки через существующую `Problem`.

- [ ] **Step 1: e2e** в `main_test.go` (`newE2E` вызывает `games.Sync(ctx, d.AdminPool)`, `games.NewService` с `match.WithHouse(match.ProcessLauncher{})`, воркер games не запускается — тест вызывает методы напрямую): `TestTanksPublicEmpty` (лидерборд — 2 записи: домашние `hunter` и `sniper`, `idle` скрыт; `live.match_id == null`; матч `nope` → 404); `TestTanksUploadAndQualify` (регистрация → `POST /me/tanks/bot {name:"rookie"}` → загрузка base64 стартового архива → 201 `pending` → `svc.Qualify` → `GET /me/tanks` показывает `active`, 4 проверки; `GET /tanks/bots/{id}` — профиль; плохой base64 → 422; мусорный архив → 422 `invalid_package`); `TestTanksLadderPublic` (`ScheduleTick` + `RunMatch` → `/tanks/matches` содержит матч, `/replay` отдаёт gzip, который декодируется `tanks.DecodeReplay`; `/me/tanks/matches/{id}/log` — свой слот; чужой пользователь → 404); `TestTanksAgentRun` (агент онлайн через heartbeat → `POST /me/tanks/agent-runs` → 201 `kind: game_bot`; коннектор `tasks/next` отдаёт `kind: game_bot` и `repo_sha256` архива; скачанный `repo.tar.gz` совпадает по sha256); `TestTanksConnectorUpload` (ключ агента → `POST /connector/tanks/versions` → 201).
- [ ] **Step 2: Падают. Step 3: реализовать маршруты и контракт. Step 4: весь backend зелёный с `ARENA_TEST_REQUIRE_DOCKER=1`, `go vet`, `gofmt`.**
- [ ] **Step 5: Коммит** `Expose the tanks API`.

---

### Task 13: Коннектор: `arena tanks submit`

**Files:**
- Modify: `backend/cmd/arena/{tanks.go,client.go}`
- Test: `backend/cmd/arena/tanks_test.go` (дополнить)

**Interfaces:**
- Consumes: `botpkg.PackDir`, `POST /api/v1/connector/tanks/versions`.
- Produces: `func (c *client) SubmitBot(ctx context.Context, archive []byte) (versionResp, error)`, где `versionResp{ID string; Number int; Status string}`; команда печатает `Uploaded version N (pending). It plays a check match in a minute: <url>/app/tanks`.

- [ ] **Step 1: Тест** с `httptest.Server`: `TestTanksSubmit` — сервер проверяет `Authorization: Bearer ak_…`, декодирует `archive_base64`, `botpkg.Validate` проходит, отвечает 201; вывод команды содержит `version 1`; `TestTanksSubmitInvalid` — каталог без `bot.json` → ошибка до запроса.
- [ ] **Step 2: Падает. Step 3: реализовать. Step 4: зелёный. Step 5: коммит** `Add arena tanks submit`.

---

### Task 14: Фронт: типы, API, модуль воспроизведения, 2D-просмотрщик, публичные страницы

**Files:**
- Create: `frontend/lib/tanks/replay.ts`, `frontend/lib/tanks/playback.ts`, `frontend/components/tanks/{public-shell.tsx,leaderboard.tsx,match-list.tsx,bot-badge.tsx,viewer/canvas2d.tsx,viewer/replay-player.tsx,viewer/controls.tsx,viewer/scoreboard.tsx,viewer/event-feed.tsx,live.tsx}`, `frontend/app/tanks/{layout.tsx,page.tsx,leaderboard/page.tsx,matches/[id]/page.tsx,bots/[id]/page.tsx,docs/page.tsx,replay/page.tsx}`
- Modify: `frontend/lib/types.ts`, `frontend/lib/api.ts`, `frontend/app/page.tsx`, `frontend/app/login/page.tsx` и `frontend/app/signup/page.tsx` (ссылка «Watch the tanks arena»)

**Interfaces:**
- Consumes: API задачи 12, формат повтора задачи 1.
- Produces:

```ts
// lib/types.ts
export interface Check { name: string; passed: boolean; detail: string }
export interface LeaderboardEntry { rank: number; bot_id: string; name: string; rating: number; mu: number; sigma: number; matches: number; wins: number; house: boolean; source: 'agent' | 'upload' | 'house'; version: number }
export interface MatchPlayerView { slot: number; bot_id: string; name: string; house: boolean; source: string; version: number; place: number | null; kills: number; damage: number; death_tick: number | null; status: string; rating_before: number | null; rating_after: number | null }
export interface MatchView { id: string; kind: 'ladder' | 'check'; status: string; map: string; seed: number; ticks: number; featured: boolean; has_replay: boolean; created_at: string; started_at: string | null; finished_at: string | null; players: MatchPlayerView[] }
export interface VersionPublic { number: number; source: string; status: string; created_at: string }
export interface BotProfile extends LeaderboardEntry { created_at: string; versions: VersionPublic[] }
export interface VersionView { id: string; number: number; source: string; status: 'pending' | 'active' | 'rejected'; language: string; checks: Check[]; check_log: string; check_match_id: string | null; proof_id: string | null; created_at: string }
export interface MyBot { id: string; name: string; rating: number; mu: number; sigma: number; matches: number; wins: number; active_version: number | null }
export interface MyTanks { bot: MyBot | null; versions: VersionView[]; agent_runs: Proof[]; matches: MatchView[] }
export interface LiveView { match_id: string | null; starts_at: string | null; duration_ms: number; now: string }
export interface MatchLog { match_id: string; slot: number; stderr: string }
// Proof: + kind: 'proof' | 'game_bot'

// lib/tanks/replay.ts
export interface Replay { version: 1; engine: string; seed: number; map: string; tick_rate: number; rules: Rules; walls: Wall[]; players: ReplayPlayer[]; frames: Frame[]; events: ReplayEvent[]; result: PlayerResult[] }
export async function fetchReplay(matchId: string): Promise<Replay>   // fetch → DecompressionStream('gzip') → JSON
export async function parseReplayFile(file: File): Promise<Replay>     // .json или .json.gz
// lib/tanks/playback.ts
export const SLOT_COLORS: readonly string[]                            // 4 контрастных цвета, одинаковые в 2D и 3D
export interface TankState { x: number; y: number; hull: number; turret: number; hp: number; alive: boolean }
export interface Snapshot { t: number; tanks: TankState[]; shells: { id: number; owner: number; x: number; y: number }[]; bonuses: { x: number; y: number }[]; zone: number }
export function snapshotAt(r: Replay, tickFloat: number): Snapshot    // линейная интерполяция соседних кадров; углы — по кратчайшей дуге
export function eventsBetween(r: Replay, fromTick: number, toTick: number): ReplayEvent[]
export class Clock { constructor(r: Replay); play(): void; pause(): void; seek(tick: number): void; setSpeed(x: number): void; now(): number /* tickFloat */ ; onEnd?: () => void }
```

Просмотрщик (`replay-player.tsx`): пропсы `{ replay: Replay; startTick?: number; live?: boolean; autoPlay?: boolean }`; внутри `Clock`, цикл `requestAnimationFrame`, рендер через `canvas2d.tsx` (переключатель 2D/3D появляется в задаче 15); управление: play/pause, скорость 0.5/1/2/4, ползунок перемотки (в `live` скрыт), счётчик времени `m:ss`; справа (снизу на мобильном) табло: цвет, имя, HP-полоса, убийства; лента последних событий («Hunter hit rookie −25», «sniper destroyed Hunter»). Canvas масштабируется по ширине контейнера с `devicePixelRatio`; y переворачивается; эффекты: вспышка у ствола на `shot`, частицы на `kill`, мигание при `hit`, пунктир зоны и затемнение снаружи.

Страницы:
- `layout.tsx` — `PublicShell`: шапка «Agent Arena · Tanks», ссылки Live / Leaderboard / Docs, справа `Dashboard` (если `useMe` вернул пользователя) или `Sign in` / `Sign up`.
- `/tanks` — hero: `Live` (эфир: `GET /tanks/live`, смещение `now − starts_at` с поправкой на разницу часов `serverNow − Date.now()`, по окончании — новый запрос; пусто — «No matches yet» и ссылка на docs), справа/ниже: заголовок «AI agents write tank bots. Bots fight. You watch.», кнопки «Connect your agent» (`/signup` или `/app/tanks`) и «Write a bot by hand» (`/tanks/docs`); топ-10 лидерборда; последние 10 матчей.
- `/tanks/leaderboard` — таблица: ранг, имя (ссылка), отметка (`agent` / `upload` / `house`), рейтинг, матчи, победы, win rate; на мобильном — компактные строки.
- `/tanks/matches/[id]` — `MatchView` + `fetchReplay` → плеер; таблица итогов (место, имя, убийства, урон, статус, изменение рейтинга `+12` / `−8`); `Copy link`; `has_replay = false` — «Replay expired» и таблица.
- `/tanks/bots/[id]` — профиль, версии, последние матчи.
- `/tanks/docs` — Quick start (3 пути: агент через коннектор, `arena tanks new/play/submit`, загрузка архива в кабинете), правила и протокол (перенос `GAME.md` в JSX, блоки кода с копированием), ограничения, рейтинг.
- `/tanks/replay` — выбор/перетаскивание файла, `parseReplayFile`, плеер.
- `app/page.tsx`: не авторизован → `/tanks` (вместо `/login`).

- [ ] **Step 1: Реализовать** типы, API-функции (`api<LeaderboardEntry[]>` через `{items}`), `replay.ts`, `playback.ts`, компоненты, страницы.
- [ ] **Step 2: `cd frontend && pnpm typecheck && pnpm build`** — зелёные.
- [ ] **Step 3: Проверка в браузере:** `make up` (или `make run-api` + `make run-web` с `ARENA_SANDBOX=fake`), через 2–3 минуты на `/tanks` идёт эфир домашних ботов; открыть матч, промотать, сменить скорость; ширина 375 px — нет горизонтальной прокрутки; `/tanks/replay` открывает файл из `arena tanks play`.
- [ ] **Step 4: Коммит** `Add the public tanks pages and the 2D replay viewer`.

---

### Task 15: Фронт: 3D-просмотрщик

**Files:**
- Create: `frontend/components/tanks/viewer/scene3d.tsx`
- Modify: `frontend/components/tanks/viewer/replay-player.tsx` (переключатель 2D/3D, `next/dynamic` с `ssr: false` для 3D), `frontend/package.json` (`three`, `@types/three`)

**Interfaces:**
- Consumes: `snapshotAt`, `eventsBetween`, `SLOT_COLORS`, `Clock` из задачи 14.
- Produces: `export default function Scene3D(props: { replay: Replay; clock: Clock }): JSX.Element` — канвас three.js на всю ширину плеера, та же высота, что у 2D.

Сцена: поле 60×40 (плоскость с сеткой), стены — `BoxGeometry` высотой 1.5, танк — группа: корпус (`Box 2.0×1.4×0.6` цвета слота), башня (`Cylinder r 0.5 h 0.4`), ствол (`Cylinder r 0.1 l 1.3`), полоска HP-спрайтом над танком; снаряды — светящиеся сферы r 0.2; аптечки — вращающийся зелёный крест; зона — полупрозрачный цилиндр без крышек радиуса `zone`; мёртвый танк — тёмный и наклонённый; взрыв на `kill` — расширяющаяся сфера 0.5 с. Свет: `HemisphereLight` + `DirectionalLight` с тенями (отключить тени при ширине < 640). Камера: `OrbitControls` (`three/examples/jsm/controls/OrbitControls.js`), режимы «Overview» и «Follow» (следит за выбранным танком, выбор кликом по строке табло). Координаты движка (x, y) → three (x − 30, 0, −(y − 20)). Ресурсы освобождаются при размонтировании (`dispose` геометрий, материалов, рендерера).

- [ ] **Step 1: `pnpm add three && pnpm add -D @types/three`.**
- [ ] **Step 2: Реализовать** сцену и переключатель; 3D-чанк не входит в первую загрузку `/tanks` (проверить вывод `pnpm build`: размер First Load JS у `/tanks` не вырос больше чем на 10 kB).
- [ ] **Step 3: `pnpm typecheck && pnpm build`**; проверка в браузере: переключение 2D↔3D не сбрасывает время; на 375 px работает вращение пальцем.
- [ ] **Step 4: Коммит** `Add the 3D replay viewer`.

---

### Task 16: Фронт: кабинет `/app/tanks`

**Files:**
- Create: `frontend/app/app/tanks/page.tsx`, `frontend/components/tanks/{bot-name-form.tsx,upload-version.tsx,version-list.tsx,agent-run-card.tsx,my-matches.tsx}`
- Modify: `frontend/components/app-shell.tsx` (пункт `Tanks`), `frontend/components/proof/result.tsx` (заголовки для `game_bot`: «Your agent's bot is in the arena» / «The bot did not pass the check»), `frontend/lib/format.ts` (`REASON_LABEL`: `invalid_package`, `bot_rejected`)

**Interfaces:**
- Consumes: `GET /me/tanks`, `POST /me/tanks/{bot,versions,agent-runs}`, `GET /me/tanks/matches/{id}/log`, `useMe`.

Экран:
1. Нет бота — форма имени (по умолчанию имя агента) и пояснение.
2. Карточка бота: имя (переименовать), рейтинг, матчи, победы, активная версия, ссылка на публичный профиль.
3. «Let my agent write the bot» — кнопка; выключена, если нет агента (ссылка на создание) или стадия `offline`/`registered` (подсказка `arena connect`); после нажатия — ссылка на proof `/app/proofs/{id}`; список последних запусков агента со статусами.
4. «Upload by hand» — выбор `.tar.gz`/`.tgz` файла (FileReader → base64), подсказка `tar czf bot.tar.gz -C mybot .` и `arena tanks submit mybot`; ошибки API показываются текстом.
5. Версии: номер, отметка источника, статус (pending — «checking…» с опросом `GET /me/tanks` раз в 3 с, пока есть pending), проверки списком с `detail`, «Check log» раскрывающийся, ссылка на пробный матч.
6. Мои последние матчи: место, соперники, ссылка на матч, «My bot's log» (загружает `MatchLog`).

- [ ] **Step 1: Реализовать. Step 2: `pnpm typecheck && pnpm build`. Step 3: браузер:** регистрация → имя бота → загрузка архива стартового набора → через ~10 с версия `active`; с запущенным `arena connect` и `agent.command`, заданным как `cp -r /tmp/t1/. .` (детерминированная «агентская» правка), кнопка агента создаёт proof, который проходит.
- [ ] **Step 4: Коммит** `Add the tanks page to the dashboard`.

---

### Task 17: Мини-агент и статья

**Files:**
- Create: `examples/mini-agent/{agent.py,requirements.txt,README.md}`, `docs/articles/2026-09-habr-build-your-agent.md`

**Interfaces:**
- Consumes: команды коннектора (`arena tanks new|play|submit`, `arena connect`), `GAME.md`, `AGENT_TASK.md`.

Мини-агент (перед написанием загрузить skill `claude-api` за актуальными моделями и API): Python ≥ 3.10, `anthropic` SDK; модель по умолчанию `claude-sonnet-5`, переопределяется `AGENT_MODEL`; инструменты `list_files(path)`, `read_file(path)`, `write_file(path, content)`, `run(command, timeout_s ≤ 120)` — все в пределах рабочего каталога (пути вне него отклоняются), вывод команд обрезается до 8 KiB; системный промпт короткий; задание — аргумент или `TASK.md`; цикл до `stop_reason == "end_turn"` или 40 шагов; печатает каждый вызов инструмента. Около 150 строк без учёта комментариев. README: установка, `ANTHROPIC_API_KEY`, запуск на стартовом боте (`arena tanks new bot && cd bot && python ../agent.py "Make this bot beat house:sniper; test with arena tanks play . house:sniper house:hunter"`), подключение к арене (`agent.command: python /path/to/agent.py`).

Статья (русский, ~2500–3500 слов, тон Хабра, без маркетинговых штампов):
1. Что такое агент на практике: модель + инструменты + цикл; чем отличается от чата.
2. Минимальный агент: код по частям (инструменты, цикл, ограничения безопасности — рабочий каталог, таймауты, лимит шагов).
3. Обратная связь — главное: агент без проверки своих действий гадает. Тесты, линтер, игра.
4. Практика: агент пишет бота для танков. Правила в двух абзацах, протокол, стартовый бот, `arena tanks play`, что агент делает за 10 минут (реальный лог прогона, снятый при проверке задачи 17: вставить фактические выдержки, не выдуманные).
5. Ошибки, на которых спотыкаются агенты: stdout вместо stderr, буферизация, выход за время тика, упреждение.
6. «Попробуйте сами»: регистрация, агент, ключ, `arena connect`, кнопка «Let my agent write the bot», эфир и таблица; ручная загрузка тоже можно, но в таблице видно, кого написал агент.
7. Что дальше: квалификация по направлениям, рейтинг агентов и соревнования (челленджи, сезоны) — куда растёт платформа (коротко, по roadmap). Про заказы и заработок не писать: биржа отложена.

Адрес сайта — `https://arena.example.com` (плейсхолдер, как в `arena init`); в начале файла HTML-комментарий «заменить адрес перед публикацией».

- [ ] **Step 1: Реализовать агента; проверить руками** на стартовом боте с реальным ключом, если `ANTHROPIC_API_KEY` есть в окружении; если ключа нет — проверить синтаксис (`python -m py_compile`) и инструменты без модели (маленький тест `test_tools.py` на `list_files/read_file/write_file/run` и отказ путей вне каталога, `python -m pytest` или `python test_tools.py`), и в статье не приводить лог прогона, которого не было, а оставить пометку для автора вставить свой.
- [ ] **Step 2: Написать статью.**
- [ ] **Step 3: Коммит** `Add a minimal agent example and the Habr article draft`.

---

### Task 18: Эксплуатация и документация

**Files:**
- Modify: `docker-compose.yml` (env `ARENA_MATCH_INTERVAL`, `ARENA_MATCH_CONCURRENCY`, `ARENA_BOT_IMAGE`), `.env.example`, `README.md`, `backend/README.md`, `docs/how-it-works.md` (раздел «Танки»), `CLAUDE.md` (архитектура: `internal/games`, `proofs.kind`, образ бота, команды `make bot-image`), `docs/superpowers/specs/2026-09-23-platform-roadmap.md` (строка про боковую ветку «Танки» и ссылка на спеку)

- [ ] **Step 1: Обновить файлы.** В `CLAUDE.md`: пакеты `internal/games/*`, что `proofs.kind` уже существует (`proof | game_bot`) и срез 2 должен расширить CHECK, образ `arena-bot-runtime:1` (как `arena-proof-go:1`), публичные маршруты `/api/v1/tanks/*` без авторизации, `ARENA_SANDBOX=fake` запускает ботов процессами на хосте.
- [ ] **Step 2: Полная проверка как в CI:** `make check`, `cd backend && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./...`, `cd frontend && pnpm typecheck && pnpm build`; `make up` → через 3 минуты на `/tanks` эфир, в лидерборде домашние боты.
- [ ] **Step 3: Коммит** `Document the tanks arena`.

---

## Самопроверка

**Покрытие спеки.** §1 путь читателя → задачи 6, 11, 13, 14, 16, 17. §2 решения → Global Constraints. §3.1 правила → задача 1. §3.2 протокол → задачи 2–3 (в т.ч. посторонние строки). §3.3 пакет → задача 5. §3.4 стартовые наборы и домашние боты → задача 2. §4.1 данные → задача 8. §4.2 проверка версии → задача 10. §4.3 путь агента → задачи 8 (proofs), 11. §4.4 проведение матча и Docker → задачи 3, 4, 9. §4.5 ладдер → задача 10. §4.6 эфир → задачи 10, 14. §4.7 API → задача 12. §4.8 повтор → задача 1. §5 коннектор → задачи 6, 13. §6 сайт → задачи 14–16. §7 статья и мини-агент → задача 17. §8 тесты → по задачам. §9 порядок → совпадает; документация → задача 18.

**Плейсхолдеры.** Адрес `https://arena.example.com` — намеренный, как в `arena init`. Имя ограничения `jobs_kind_check` проверяется в задаче 8 шаг 1.

**Согласованность типов.** `Check` (games) ↔ `TestResult` (proofs) переводится в `JudgeProof`; `MatchView.Players[].Place` — указатель (NULL у незаконченных); `LeaderboardEntry` встраивается в `BotProfile`; `CommandMsg.Tick` сверяется с `Game.Tick` до шага; сиды `int64 < 2^53` в Go, SQL `bigint`, TS `number`.

**Review Focus → тесты:** 1 → задача 3 `TestNoiseIgnored`, задача 10 `TestQualifyNoisyBotHint`; 2 → задача 5 `TestValidateMacOSWrapper`; 3 → задача 4 `TestCloseKillsRunaway`, задача 9 `TestDockerCloseKillsRunaway`; 4 → задача 10 `TestRunMatchRerunAfterCrash`, `TestSweepStuck`; 5 → задача 8 `TestSaveBotNameTaken`.
