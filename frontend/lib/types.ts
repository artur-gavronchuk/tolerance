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
  place: number; handle: string; made_with: string
  passed_tests: number; total_tests: number; score: number | null; submitted_at: string
}

// Published once a day closes (`/daily/{day}/reveal`).
export interface DayReveal {
  hidden_tests: { path: string; content: string }[]
  solutions: { place: number; handle: string; made_with: string; submitted_at: string; diff: string }[]
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

export interface SeasonDetail { season: SeasonView; standings: LeaderboardEntry[]; now: string }

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
  entries: TournamentBot[] | null; pairings: TournamentPairing[] | null // detail view only
  now: string
}

export interface BotTournament {
  tournament_id: string; name: string; status: string; starts_at: string
  seed: number; rounds: number; result: string; champion: boolean
}

export interface Showcase {
  now: string; season: SeasonView; ladder: LeaderboardEntry[]
  tournament: TournamentView | null; next_tournament: TournamentView | null
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
}

export interface ProductEntry {
  id: string; task_slug: string; handle?: string; status: 'queued' | 'running' | 'done' | 'infra_error'
  passed: number; total: number; failure_reason: string | null; results: ProductScenarioResult[]
  log_tail?: string; made_with: string; votes: number; voted: boolean; mine: boolean
  created_at: string; finished_at: string | null
}

export interface ProductDetail extends ProductTask { attempts_used: number; mine: ProductEntry[] }

export interface ProductSourceFile { path: string; size: number; content?: string; truncated?: boolean; binary?: boolean }

export interface ProductResults { task: ProductTask; entries: ProductEntry[] }

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
