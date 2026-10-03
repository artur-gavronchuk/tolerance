export interface AuthProviders { providers: ('github' | 'google')[]; dev_login: boolean }

// The task of the day (`/daily*`, `/days`, `/submissions*`, `/leaderboard`, `/me`).
export interface TaskSummary {
  slug: string; title: string; language: 'go' | 'python'; difficulty: number
  task_md: string; repo_url: string
}

export type SubmissionStatus = 'queued' | 'running' | 'passed' | 'failed' | 'infra_error'

export interface Submission {
  id: string; task_slug: string
  day: string | null // null = practice (the task is not today's)
  status: SubmissionStatus
  passed_tests: number; total_tests: number
  failure_reason: string | null
  tests: { name: string; passed: boolean }[]
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
  passed_tests: number; total_tests: number; submitted_at: string
}

// Published once a day closes (`/daily/{day}/reveal`).
export interface DayReveal {
  hidden_tests: { path: string; content: string }[]
  solutions: { place: number; handle: string; made_with: string; submitted_at: string; diff: string }[]
}

export interface DayListItem {
  day: string
  task: { slug: string; title: string; language: 'go' | 'python'; difficulty: number }
  solvers: number
}

export interface OverallRow { place: number; handle: string; points: number; solved_days: number; current_streak: number }

export interface User { id: string; email: string; handle: string; role: 'user' | 'admin'; created_at: string }
export interface Me { user: User; streak: { current: number; best: number } }

// Tanks: the public ladder, matches and bot profiles (`/tanks/*`, `/me/tanks*`).
export type BotSource = 'agent' | 'upload' | 'house'

export interface Check { name: string; passed: boolean; detail: string }

export interface LeaderboardEntry {
  rank: number; bot_id: string; name: string; rating: number; mu: number; sigma: number
  matches: number; wins: number; house: boolean; source: BotSource; version: number
}

export interface MatchPlayerView {
  slot: number; bot_id: string; name: string; house: boolean; source: string; version: number
  place: number | null; kills: number; damage: number; death_tick: number | null; status: string
  rating_before: number | null; rating_after: number | null
}

export interface MatchView {
  id: string; kind: 'ladder' | 'check'; status: string; map: string; seed: number; ticks: number
  featured: boolean; has_replay: boolean; created_at: string; started_at: string | null; finished_at: string | null
  players: MatchPlayerView[]
}

export interface VersionPublic { number: number; source: string; status: string; created_at: string }

export interface BotProfile extends LeaderboardEntry { created_at: string; versions: VersionPublic[] }

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
  slug: string; title: string; summary: string; kind: 'cli'; phase: 'open' | 'voting'
  opens_at: string; deadline: string; scenario_count: number; attempts: number; entry_count: number
  task_md?: string
}

export interface ProductEntry {
  id: string; task_slug: string; handle?: string; status: 'queued' | 'running' | 'done' | 'infra_error'
  passed: number; total: number; failure_reason: string | null; results: ProductScenarioResult[]
  log_tail?: string; made_with: string; votes: number; voted: boolean; mine: boolean
  created_at: string; finished_at: string | null
}

export interface ProductDetail extends ProductTask { attempts_used: number; mine: ProductEntry[] }

export interface ProductResults { task: ProductTask; entries: ProductEntry[] }
