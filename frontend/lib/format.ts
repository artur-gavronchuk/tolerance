export function ago(iso: string, now = Date.now()) {
  const s = Math.max(0, Math.round((now - new Date(iso).getTime()) / 1000))
  if (s < 60) return `${s}s ago`
  if (s < 3600) return `${Math.round(s / 60)}m ago`
  if (s < 86400) return `${Math.round(s / 3600)}h ago`
  return `${Math.round(s / 86400)}d ago`
}

export function duration(ms: number | null) {
  if (ms == null) return '—'
  const s = Math.round(ms / 1000)
  return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m ${s % 60}s`
}

export const STATUS_LABEL: Record<string, string> = {
  queued: 'Waiting for the connector',
  claimed: 'Connector picked it up',
  running_agent: 'Your agent is working',
  diff_submitted: 'Diff received',
  running_sandbox: 'Running hidden tests',
  passed: 'Passed',
  failed: 'Failed',
  infra_error: 'Platform error',
  expired: 'Expired',
}

export const REASON_LABEL: Record<string, string> = {
  diff_not_applicable: 'The diff did not apply to a clean copy of the repository.',
  empty_diff: 'The agent changed nothing.',
  build_failed: 'The code did not compile in the sandbox.',
  tests_failed: 'One or more hidden tests failed.',
  timeout: 'The tests ran out of time in the sandbox.',
  not_claimed: 'No connector picked the task up within 5 minutes. Is `arena connect` running?',
  agent_timeout: 'The agent did not return a result within the time limit.',
  hidden_test_missing_or_failed: 'Not every hidden test ran and passed, so the fix could not be confirmed.',
  test_file_modified: 'The diff changed a test file. Fix the code, not the tests.',
  stuck: 'The sandbox run never finished on our side. Retry it.',
}
