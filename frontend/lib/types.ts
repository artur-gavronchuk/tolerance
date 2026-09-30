export type Stage = 'registered' | 'offline' | 'connected' | 'checking' | 'operational' | 'check_failed'

export type ProofStatus =
  | 'queued' | 'claimed' | 'running_agent' | 'diff_submitted' | 'running_sandbox'
  | 'passed' | 'failed' | 'infra_error' | 'expired'

export interface AuthProviders { providers: ('github' | 'google')[]; dev_login: boolean }
export interface User { id: string; email: string; role: 'user' | 'admin'; created_at: string }
export interface ApiKey { id: string; prefix: string; name: string; created_at: string; last_used_at: string | null }
export interface Presence { last_seen_at: string; connector_version: string; hostname: string }
export interface AgentOverview {
  id: string; name: string; description: string; created_at: string; api_keys: ApiKey[]
  stage: Stage; presence: Presence | null; last_proof: Proof | null
  // Whether this agent appears in the public arena tables and has a public
  // profile. Ratings are computed either way.
  public: boolean
}
export interface Me { user: User; agent: AgentOverview | null }
export interface ProofTask {
  slug: string; title: string; language: string; agent_timeout_s: number; sandbox_timeout_s: number
  visible_tests: number; hidden_tests: number; task_md: string; repo_sha256: string
}
export interface TestResult { name: string; passed: boolean }
export interface SandboxResult { tests: TestResult[]; exit_code: number; output: string; timed_out: boolean }
export interface Proof {
  id: string; agent_id: string; task_slug: string; status: ProofStatus
  created_at: string; claimed_at: string | null; diff_submitted_at: string | null; finished_at: string | null
  diff: string; agent_log_tail: string; agent_duration_ms: number | null; agent_exit_code: number | null
  sandbox_result: SandboxResult | null; failure_reason: string
  kind: 'proof' | 'game_bot' | 'qualification' | 'challenge'
}

// Tanks: the public ladder, matches and bot profiles. See
// backend/contracts/openapi/openapi.yaml (`/tanks/*`, `/me/tanks*`).
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
  checks: Check[]; check_log: string; check_match_id: string | null; proof_id: string | null; created_at: string
}

export interface MyBot {
  id: string; name: string; rating: number; mu: number; sigma: number
  matches: number; wins: number; active_version: number | null
}

export interface MyTanks { bot: MyBot | null; versions: VersionView[]; agent_runs: Proof[]; matches: MatchView[] }

export interface LiveView { match_id: string | null; starts_at: string | null; duration_ms: number; now: string }

export interface MatchLog { match_id: string; slot: number; stderr: string }

// The arena: per-skill tables of agents, and challenges. See
// backend/contracts/openapi/openapi.yaml (`/leaderboard`, `/challenges*`).
export type Tier = 'none' | 'verified' | 'strong' | 'elite'

// Named SkillLeaderboardRow because LeaderboardEntry is the tanks ladder's.
export interface SkillLeaderboardRow {
  rank: number; agent_name: string; version_number: number; model: string; harness: string
  rating: number; uncertainty: number; access: number; tier: Tier; runs: number
  on_current_version: boolean; scored_at: string
}

export interface SkillSummary {
  slug: string; title: string; language: string; description: string
  pool_size: number; frozen: boolean
  rating: SkillRatingView | null; runs_today: number; can_start: boolean; blocked_reason: string
}

export interface SkillRatingView {
  skill_slug: string; rating: number; uncertainty: number; access: number; tier: Tier
  verified: boolean; runs: number; version_id: string; version_number: number
  on_current_version: boolean; prior_rating: number | null
}

export type ChallengeStatus = 'draft' | 'open' | 'closed' | 'published'

export interface ChallengeSummary {
  slug: string; title: string; summary: string; status: ChallengeStatus
  skill_slug: string; min_tier: Tier; opens_at: string; closes_at: string
  prizes: string; entrants: number
}

export interface ChallengeStanding {
  agent_name: string; score: number; diff_lines: number; submitted_at: string; rank: number; diff: string
}

export interface ChallengeView extends ChallengeSummary {
  task_md: string; hidden_tests: string[]; standings: ChallengeStanding[]
}

export interface ChallengeLists { open: ChallengeSummary[]; upcoming: ChallengeSummary[]; past: ChallengeSummary[] }

export interface MyChallengeEntry {
  challenge_slug: string; title: string; agent_id: string; version_id: string; proof_id: string
  consent_publish: boolean; score: number | null; diff_lines: number | null
  submitted_at: string | null; rank: number | null; created_at: string
}
