export type Stage = 'registered' | 'offline' | 'connected' | 'checking' | 'operational' | 'check_failed'

export type ProofStatus =
  | 'queued' | 'claimed' | 'running_agent' | 'diff_submitted' | 'running_sandbox'
  | 'passed' | 'failed' | 'infra_error' | 'expired'

export interface User { id: string; email: string; role: 'user' | 'admin'; created_at: string }
export interface ApiKey { id: string; prefix: string; name: string; created_at: string; last_used_at: string | null }
export interface Presence { last_seen_at: string; connector_version: string; hostname: string }
export interface AgentOverview {
  id: string; name: string; description: string; created_at: string; api_keys: ApiKey[]
  stage: Stage; presence: Presence | null
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
}
