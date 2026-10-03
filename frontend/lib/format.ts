export function ago(iso: string, now = Date.now()) {
  const s = Math.max(0, Math.round((now - new Date(iso).getTime()) / 1000))
  if (s < 60) return `${s}s ago`
  if (s < 3600) return `${Math.round(s / 60)}m ago`
  if (s < 86400) return `${Math.round(s / 3600)}h ago`
  return `${Math.round(s / 86400)}d ago`
}

// Why a submission failed, in words. Keys are the backend's failure_reason values.
export const REASON_LABEL: Record<string, string> = {
  diff_does_not_apply: 'Your patch does not apply to a clean copy of the repository.',
  touches_test_files: 'The change touches test files. Fix the code, not the tests.',
  hidden_tests_failed: 'Some hidden tests failed.',
  tests_did_not_run: 'The tests did not run. Does the code still build?',
  timeout: 'The tests ran out of time in the sandbox.',
}

export function reasonLabel(reason: string | null) {
  if (!reason) return null
  return REASON_LABEL[reason] ?? reason.replace(/_/g, ' ')
}

export const STATUS_LABEL: Record<string, string> = {
  queued: 'Queued',
  running: 'Running hidden tests',
  passed: 'Passed',
  failed: 'Failed',
  infra_error: 'Platform error, not counted against you',
}

export const DIFFICULTY_LABEL = ['', 'Easy', 'Medium', 'Hard']

// "05:12:09" until `iso`, or "closed" once it has passed.
export function countdown(iso: string, now = Date.now()) {
  const s = Math.floor((new Date(iso).getTime() - now) / 1000)
  if (s <= 0) return 'closed'
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(Math.floor(s / 3600))}:${p(Math.floor((s % 3600) / 60))}:${p(s % 60)}`
}
