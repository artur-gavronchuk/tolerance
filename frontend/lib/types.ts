export interface AuthProviders { providers: ('github' | 'google')[]; dev_login: boolean }

// The task of the day (`/daily*`, `/days`, `/submissions*`, `/leaderboard`, `/me`).
export interface TaskSummary {
  slug: string; title: string; language: 'go' | 'python'; difficulty: number
  task_md: string; repo_url: string
  kind: 'bugfix' | 'optimize'
  direction: 'max' | 'min' | null // optimize tasks: which way the score is better
}

export type SubmissionStatus = 'queued' | 'running' | 'passed' | 'failed' | 'infra_error'

export interface Submission {
  id: string; task_slug: string
  day: string | null // null = practice (the task is not today's)
  status: SubmissionStatus
  passed_tests: number; total_tests: number
  score: number | null // optimize tasks only
  failure_reason: string | null
  // optimize tasks: one entry per case, with its score and, when invalid, the reason
  tests: { name: string; passed: boolean; score?: number; reason?: string }[]
  log_tail: string; made_with: string
  created_at: string; finished_at: string | null
}

export interface Daily {
  day: string; closes_at: string; is_open: boolean
  task: TaskSummary
  attempts_per_day: number
  my: null | { attempts_used: number; best: Submission | null; submissions: Submission[] }
}

export interface DailyRow {
  id?: string; place: number; handle: string; made_with: string
  passed_tests: number; total_tests: number; score: number | null; submitted_at: string
}

// Published once a day closes (`/daily/{day}/reveal`).
export interface DayReveal {
  hidden_tests: { path: string; content: string }[]
  solutions: { id?: string; place: number; handle: string; made_with: string; submitted_at: string; diff: string }[]
}

export interface DayStats {
  participants: number; solvers: number; submissions: number
  by_tool: { made_with: string; participants: number; solvers: number }[]
}

export interface DayListItem {
  day: string
  task: { slug: string; title: string; language: 'go' | 'python'; difficulty: number }
  solvers: number
}

// Agent stacks (`/stacks`, `/daily/{day}/stacks`): free-text "made with" normalized to (tool, model).
// solve_rate, avg_tests_share and avg_attempts_to_pass only count bugfix days, null when there are none.
export interface StackRow {
  tool: string; model: string; label: string
  users: number; days_attempted: number; bugfix_days: number; optimize_days: number; days_solved: number
  avg_points: number // 0-100 per day, same scale as the overall leaderboard
  solve_rate: number | null; avg_tests_share: number | null; avg_attempts_to_pass: number | null
}

export interface OverallRow { place: number; handle: string; points: number; solved_days: number; current_streak: number }

export interface User { id: string; email: string; handle: string; role: 'user' | 'admin'; created_at: string }
export interface Me { user: User; streak: { current: number; best: number }; can_admin?: boolean }

// Tanks: the public ladder, matches and bot profiles (`/tanks/*`, `/me/tanks*`).
export type BotSource = 'agent' | 'upload' | 'house'

export interface Check { name: string; passed: boolean; detail: string }

export interface LeaderboardEntry {
  rank: number; bot_id: string; name: string; rating: number; mu: number; sigma: number
  matches: number; wins: number; house: boolean; source: BotSource; version: number
  // rating/mu/sigma/matches/wins are the current season's
  lifetime_rating: number; owner: string
  provisional: boolean // fewer than 10 season matches
}

export interface MatchPlayerView {
  slot: number; bot_id: string; name: string; house: boolean; source: string; version: number
  place: number | null; kills: number; damage: number; death_tick: number | null; status: string
  rating_before: number | null; rating_after: number | null
}

export interface MatchView {
  id: string; kind: 'ladder' | 'check' | 'tournament'; status: string; map: string; seed: number; ticks: number
  featured: boolean; has_replay: boolean; created_at: string; started_at: string | null; finished_at: string | null
  players: MatchPlayerView[]
}

export interface VersionPublic { number: number; source: string; status: string; created_at: string }

export interface BotProfile extends LeaderboardEntry {
  created_at: string; versions: VersionPublic[]
  season: SeasonView; seasons: BotSeasonResult[]; tournaments: BotTournament[]
}

// Seasons (a calendar month, UTC) and tournaments (weekly single elimination, best-of-3 series).
export interface SeasonWinner { bot_id: string; name: string; owner: string; rating: number }

export interface SeasonView {
  id: string; name: string; starts_at: string; ends_at: string; status: 'active' | 'archived'
  winner: SeasonWinner | null
}

export interface SeasonDetail { season: SeasonView; standings: LeaderboardEntry[]; total: number; now: string }

export interface BotSeasonResult { season_id: string; name: string; rank: number; rating: number; matches: number; wins: number }

export interface TournamentBot { bot_id: string; name: string; house: boolean; seed: number; owner: string }

export interface TournamentGame {
  game: number; match_id: string; status: string; map: string; winner_bot_id: string | null; has_replay: boolean
}

export interface TournamentPairing {
  id: string; round: number; position: number
  a: TournamentBot | null; b: TournamentBot | null
  wins_a: number; wins_b: number
  status: 'pending' | 'running' | 'finished'
  winner_bot_id: string | null; bye: boolean; games: TournamentGame[]
}

export interface TournamentView {
  id: string; name: string; season_id: string | null
  status: 'scheduled' | 'running' | 'finished' | 'cancelled'
  starts_at: string; started_at: string | null; finished_at: string | null
  size: number; rounds: number; best_of: number; entry_count: number
  champion: TournamentBot | null
  open: boolean // started on demand, not part of the weekly schedule
  entries: TournamentBot[] | null; pairings: TournamentPairing[] | null // detail view only
  now: string
}

export interface BotTournament {
  tournament_id: string; name: string; status: string; starts_at: string
  seed: number; rounds: number; result: string; champion: boolean; open: boolean
}

export interface Showcase {
  now: string; season: SeasonView; ladder: LeaderboardEntry[]
  tournament: TournamentView | null; next_tournament: TournamentView | null
  open_tournaments: TournamentView[]
  champions: TournamentView[]; notable: MatchView[]; past_seasons: SeasonView[]
}

export interface VersionView {
  id: string; number: number; source: string; status: 'pending' | 'active' | 'rejected'; language: string
  checks: Check[]; check_log: string; check_match_id: string | null; created_at: string
}

export interface MyBot {
  id: string; name: string; rating: number; mu: number; sigma: number
  matches: number; wins: number; active_version: number | null
}

export interface MyTanks { bot: MyBot | null; versions: VersionView[]; matches: MatchView[] }

export interface LiveView { match_id: string | null; starts_at: string | null; duration_ms: number; now: string }

export interface MatchLog { match_id: string; slot: number; stderr: string }

// Product tasks (weekly challenges scored by scenarios, then voted on).
export interface ProductScenarioResult { name: string; passed: boolean }

export interface ProductTask {
  slug: string; title: string; summary: string; kind: 'cli' | 'site'; phase: 'open' | 'voting' | 'final'
  opens_at: string; deadline: string; voting_ends_at: string; scenario_count: number; attempts: number; entry_count: number
  task_md?: string
  has_bench: boolean // cli: the tool is also timed on a large generated input
  winner?: { entry_id: string; handle: string; votes: number; passed: number; bench_ms: number | null; total: number }
}

// `/products`: every task that has opened, newest first, plus what is known about the ones still to come.
export interface ProductList { items: ProductTask[]; upcoming: { count: number; next_kind?: 'cli' | 'site'; next_opens_at: string } }

// Site entries: informational signals from the scoring run; never part of the ranking.
export interface A11yCounts { critical: number; serious: number; moderate: number; minor: number }
export interface SiteQuality {
  a11y?: { desktop: A11yCounts; mobile: A11yCounts; top_rules: string[] }
  perf?: { bytes: number; requests: number; dcl_ms: number; load_ms: number; errors: number }
  mobile?: { overflow: boolean; small_targets: number }
}

export interface ProductEntry {
  id: string; task_slug: string; handle?: string; status: 'queued' | 'running' | 'done' | 'infra_error'
  passed: number; total: number; failure_reason: string | null; results: ProductScenarioResult[]
  bench_ms: number | null // cli: trimmed median benchmark time in ms (faster ranks higher); null when there is no time
  bench_spread_ms: number | null // cli: ± half the range of the kept samples
  log_tail?: string; made_with: string; votes: number; voted: boolean; mine: boolean
  score?: number; comparisons: number // site tasks: Bradley-Terry score of the blind comparisons, and how many there were
  quality?: SiteQuality | null // site: null while hidden (blind voting, others' entries) or when the scan produced nothing
  created_at: string; finished_at: string | null
}

// Blind comparison of sites (`/products/{slug}/compare`): ids only until the person has judged.
export interface ComparePair { a: { id: string }; b: { id: string } }
export interface CompareNext { pair: ComparePair | null; judged: number; target: number }
export interface CompareRevealed { id: string; handle: string; made_with: string }
export interface CompareJudged { a: CompareRevealed; b: CompareRevealed; winner: 'a' | 'b' | 'tie'; judged: number; target: number }

// fastest_ms is the best benchmark time among all entries, known once the deadline has passed.
export interface ProductDetail extends ProductTask { attempts_used: number; mine: ProductEntry[]; fastest_ms: number | null }

export interface ProductSourceFile { path: string; size: number; content?: string; truncated?: boolean; binary?: boolean }

export interface ProductResults { task: ProductTask; entries: ProductEntry[]; total: number }

// A person's public page (`/users/{handle}`).
export interface ProfileDay {
  day: string
  task: { slug: string; title: string; language: 'go' | 'python'; difficulty: number }
  status: 'passed' | 'failed'
  passed_tests: number; total_tests: number; score: number | null
  made_with: string; attempts: number
}

export interface Profile {
  handle: string; joined_at: string
  streak: { current: number; best: number }
  solved_days: number; played_days: number; place: number | null
  tools: string[]; days: ProfileDay[]
}

// A person's activity across products, tanks and their main stack (`/users/{handle}/activity`).
export interface ActivityProduct {
  task_slug: string; task_title: string; kind: 'cli' | 'site'; phase: 'open' | 'voting' | 'final'; deadline: string
  entry_id: string; passed: number; bench_ms: number | null; total: number; votes: number; place: number | null; entrants: number; created_at: string
}

export interface ActivityBotTournament { id: string; name: string; starts_at: string; result: string; champion: boolean; open: boolean }

export interface ActivityBot {
  id: string; name: string; rating: number; rank: number | null; lifetime_rating: number
  matches: number; wins: number; total_matches: number; total_wins: number
  titles: number; best_finish: string; tournaments: ActivityBotTournament[]
}

export interface ActivityStack { tool: string; model: string; label: string; count: number }

export interface Activity {
  handle: string; stack: ActivityStack | null; products: ActivityProduct[]; bots: ActivityBot[]
}

// /admin: the owner's pulse of the platform (GET /admin/pulse, GET /admin/recent).
export interface AdminPoint { day: string; value: number }
export interface AdminPulse {
  generated_at: string
  users: { total: number; new_today: number; new_7d: number; active_today: number; signup_series: AdminPoint[] }
  daily: {
    task_slug: string; task_kind: string; task_title: string
    today: Record<string, number>
    unique_solvers: number; infra_rate_7d: number; submissions_7d: number; median_seconds: number | null
    submission_series: AdminPoint[]; user_series: AdminPoint[]
  }
  products: {
    task_slug: string; task_title: string; task_kind: string
    opens_at: string | null; deadline: string | null
    entries: number; by_status: Record<string, number>; votes: number
    next_kind: string; upcoming: number
  }
  tanks: {
    bots_total: number; bots_active: number; uploads_today: number; rejected_today: number
    matches_last_hour: Record<string, number>
    tournament: { id: string; name: string; status: string } | null
    ladder: { rank: number; bot_id: string; name: string; owner: string; rating: number; matches: number }[]
  }
  health: {
    jobs: { kind: string; state: string; count: number }[]
    oldest_queued_seconds: number | null
    retried_jobs: number; failed_jobs: number
    stuck: { kind: string; id: string; status: string; age_minutes: number; href: string }[]
    infra_errors: { at: string; kind: string; id: string; reason: string }[]
  }
}
export interface FunnelDay { day: string; visit: number; signin: number; download: number; upload: number; passed: number; returned: number | null }
export interface AdminFunnel {
  days: FunnelDay[]
  modes: { mode: 'daily' | 'tanks'; people: number; events: number; series: AdminPoint[] }[]
  pages: { path: string; views: number; visitors: number }[]
  sources: { source: string; visits: number }[]
}
export interface AdminEvent { at: string; type: 'signup' | 'submission' | 'product_entry' | 'bot_version' | 'tournament'; title: string; detail: string; status: string; href: string }

// Retention: `/me/recap` and `/me/notifications`.
export interface RecapResult { handle?: string; passed_tests: number; total_tests: number; score: number | null }
export interface Recap {
  yesterday: {
    day: string
    task: { slug: string; title: string; kind: 'bugfix' | 'optimize'; direction: 'max' | 'min' | null }
    mine: RecapResult | null; place: number | null; participants: number; winner: RecapResult | null
  } | null
  streak: { current: number; best: number }
  solved_days: string[]
  today: string
}

export interface AppNotification {
  id: string
  type: 'daily_verdict' | 'daily_final' | 'product_voting' | 'product_final' | 'tournament_soon' | 'tournament_entered'
    | 'tournament_won' | 'tournament_lost' | 'rank_drop'
  params: Record<string, string | number | null>
  created_at: string
  read: boolean
}
export interface NotificationList { items: AppNotification[]; unread: number }

// Moderation (`/admin/moderation/*`).
export interface ModUser { id: string; handle: string; email: string; role: string; created_at: string; banned_at: string | null; submissions: number; entries: number }
export interface ModItem { kind: 'entry' | 'submission' | 'bot'; id: string; label: string; status: string; at: string; hidden_at: string | null }
export interface ModLogItem { at: string; action: 'ban' | 'unban' | 'hide' | 'unhide'; kind: 'user' | 'entry' | 'submission' | 'bot'; id: string; label: string; actor: string; reason: string; active: boolean }

// Fair play (`/admin/fairplay`, `POST /reports`).
export type FairSignal = 'fast_solve' | 'burst' | 'shared_device' | 'shared_ip' | 'near_duplicate'
export interface FairFlag { id: string; signal: FairSignal; score: number; detail: Record<string, unknown> }
export interface FairItem {
  subject_kind: 'submission'; subject_id: string; user_id: string; handle: string; banned: boolean; hidden: boolean
  task_slug: string; day: string | null; solve_seconds: number | null; at: string; score: number; flags: FairFlag[]
}
export interface FairReport {
  id: string; reporter: string; target_kind: 'user'; target_id: string; user_id: string; handle: string; label: string
  banned: boolean; hidden: boolean; reason: string; details: string; at: string; others: number
}
export interface FairCluster { kind: 'ip' | 'device'; hash: string; users: { id: string; handle: string; banned: boolean }[] }
export interface FairOverview { flags: FairItem[]; reports: FairReport[]; clusters: FairCluster[] }
