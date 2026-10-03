'use client'

import { useCallback, useEffect, useState } from 'react'
import { api, post } from './api'

export interface UploadLinkStatus { active: boolean; created_at: string | null; last_used_at: string | null }

const KEY = 'upload-link-token'

// The token is shown by the server once; this browser keeps a copy so the prompt can be shown again.
const readToken = () => { try { return localStorage.getItem(KEY) } catch { return null } }
const writeToken = (t: string | null) => { try { t ? localStorage.setItem(KEY, t) : localStorage.removeItem(KEY) } catch {} }

export function useUploadLink(enabled = true) {
  const [status, setStatus] = useState<UploadLinkStatus | null>(null)
  const [token, setToken] = useState<string | null>(null)
  const [error, setError] = useState<unknown>(null)
  const load = useCallback(async () => {
    try {
      const s = await api<UploadLinkStatus>('/me/upload-link')
      setStatus(s)
      if (!s.active) writeToken(null)
      setToken(s.active ? readToken() : null)
    } catch (e) { setError(e) }
  }, [])
  useEffect(() => { if (enabled) void load() }, [enabled, load])
  const rotate = useCallback(async () => {
    setError(null)
    try {
      const r = await post<UploadLinkStatus & { token: string }>('/me/upload-link')
      writeToken(r.token)
      setToken(r.token)
      setStatus({ active: r.active, created_at: r.created_at, last_used_at: r.last_used_at })
    } catch (e) { setError(e) }
  }, [])
  const revoke = useCallback(async () => {
    setError(null)
    try {
      await api('/me/upload-link', { method: 'DELETE' })
      writeToken(null)
      setToken(null)
      setStatus({ active: false, created_at: null, last_used_at: null })
    } catch (e) { setError(e) }
  }, [])
  return { status, token, error, rotate, revoke }
}

export const linkBase = (token: string) => `${window.location.origin}/api/v1/u/${token}`

export type AgentTarget = { kind: 'daily' } | { kind: 'product'; slug: string; site: boolean } | { kind: 'tanks' }

// Ready-to-paste prompt for the agent. English on purpose: it is read by the agent, not the person.
export function agentPrompt(target: AgentTarget, base: string): string {
  const head = `You can fetch and submit work yourself with curl, no login needed. My personal upload link is below; treat it as a secret (never print it in a commit, a file or a public place).\nL=${base}\n\n`
  const poll = (path: string, done: string) => `Poll until ${done}: curl -sS $L/${path}`
  if (target.kind === 'daily') return head + `1. Download today's task and unpack it: curl -sSfL -o repo.zip $L/daily/repo.zip && unzip -q repo.zip -d task && cd task
2. Read TASK.md and fix the issue (or optimize, as it says). Do not modify or delete tests and do not add test files; keep the change minimal.
3. Zip the repository: zip -r ../solution.zip . -x '.git/*'
4. Upload: curl -sS -F file=@../solution.zip -F "made_with=<your tool + model, e.g. Claude Code + Opus>" $L/daily
   The JSON has submission.id.
5. ${poll('submissions/<id>', 'status is passed, failed or infra_error')}
   (the JSON has status, passed_tests/total_tests, score, failure_reason, tests, log_tail). If it failed, read failure_reason, tests and log_tail, fix the code and upload again — attempts per day are limited, so be careful. infra_error is the platform's fault and does not use an attempt.`
  if (target.kind === 'product') return head + `This is the product task "${target.slug}".
1. Read the brief: curl -sS $L/products/${target.slug}   (JSON; the task text is in it)
2. Build it. ${target.site ? 'Deliver a zip of static files with index.html at the top level.' : 'Deliver a zip with the program, as the brief describes.'} Max 5 MB.
3. Upload: curl -sS -F file=@product.zip -F "made_with=<your tool + model>" $L/products/${target.slug}
   The JSON has entry.id.
4. ${poll(`products/${target.slug}/entries/<id>`, 'status is done or infra_error')}
   (passed/total scenarios, failure_reason, results, log_tail). If checks fail, fix and upload again — the number of uploads is limited.`
  return head + `This is my tank bot for the Tanks ladder. Start from the starter kit: curl -sSfL -o kit.zip ${window.location.origin}/api/v1/tanks/starter/python.zip (or js.zip) and read GAME.md.
1. Improve the bot, keep bot.json valid and the same entry file. Standard library only.
2. Zip the folder and upload the raw archive as the request body: curl -sS -X POST -H 'Content-Type: application/zip' --data-binary @bot.zip $L/tanks
   The JSON has version.id.
3. ${poll('tanks/versions/<id>', 'version.status is active or rejected')}
   It lists the checks, check_match_report_url and latest_matches with report_url.
4. Read the plain-text match reports: curl -sS <report_url>. They say what hit the tank, how many shots landed and where it got stuck. Work out what went wrong, fix the bot and upload a new version (step 2). Repeat until it wins.`
}
