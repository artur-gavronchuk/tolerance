// Every page must fit a 375px-wide screen without horizontal scrolling
// (spec §8). Needs the site (with /api proxied to the API) at BASE_URL and a
// Chrome install. It signs up its own owner, agent, key and a queued proof,
// so the signed-in pages render with real data.
import { chromium } from 'playwright-core'

const BASE = process.env.BASE_URL ?? 'http://127.0.0.1:3000'
const WIDTH = 375

const browser = await chromium.launch({ channel: 'chrome' })
const failures = []

async function widthOf(context, path) {
  const page = await context.newPage()
  try {
    await page.goto(BASE + path)
    await page.locator('h1, h2').first().waitFor({ timeout: 15_000 })
    const actual = new URL(page.url()).pathname
    if (actual !== path) throw new Error(`${path} ended up on ${actual}`)
    await page.waitForTimeout(300) // late layout: fonts, polled data
    return await page.evaluate(() => document.documentElement.scrollWidth)
  } finally {
    await page.close()
  }
}

async function check(context, paths) {
  for (const path of paths) {
    const width = await widthOf(context, path)
    console.log(`${width > WIDTH ? 'FAIL' : 'ok  '} ${path} scrollWidth=${width}`)
    if (width > WIDTH) failures.push(`${path}: ${width}px`)
  }
}

try {
  const anonymous = await browser.newContext({ viewport: { width: WIDTH, height: 800 } })
  await check(anonymous, ['/', '/login', '/signup', '/terms', '/arena', '/challenges',
    '/tanks', '/tanks/leaderboard', '/tanks/docs', '/tanks/replay'])

  const owner = await browser.newContext({ viewport: { width: WIDTH, height: 800 } })
  const call = async (method, path, { data, key } = {}) => {
    const res = await owner.request.fetch(`${BASE}/api/v1${path}`, {
      method,
      data,
      headers: key ? { Authorization: `Bearer ${key}` } : {},
    })
    if (!res.ok()) throw new Error(`${method} ${path}: ${res.status()} ${await res.text()}`)
    return res.status() === 204 ? null : res.json()
  }
  const run = Date.now()
  await call('POST', '/auth/dev', { data: { email: `mobile-${run}@example.com` } })
  await call('POST', '/agent', { data: { name: `mobile-${run % 1_000_000}`, description: 'CI width check' } })
  const { key } = await call('POST', '/agent/keys', { data: { name: 'ci' } })
  await call('POST', '/connector/heartbeat', { key, data: { connector_version: 'ci', hostname: 'ci' } })
  await check(owner, ['/app', '/app/agent/connect', '/app/proofs/new', '/app/agent/new', '/app/tanks', '/app/challenges'])
  const proof = await call('POST', '/proofs', { data: { task_slug: 'go-fix-retry' } })
  await check(owner, ['/app', `/app/proofs/${proof.id}`])

  // Act as the connector to actually finish this proof, so the mobile check
  // also covers the widest screen: the finished result view with its test
  // table and diff (spec §8).
  const next = await call('GET', '/connector/tasks/next?wait=5', { key })
  if (next.proof_id !== proof.id) throw new Error(`tasks/next returned ${next.proof_id}, expected ${proof.id}`)
  await call('POST', `/connector/proofs/${proof.id}/started`, { key, data: {} })
  const diff = "--- a/retry.go\n+++ b/retry.go\n@@ -19,7 +19,14 @@ func Backoff(attempt int) time.Duration {\n \tif attempt < 1 {\n \t\treturn 0\n \t}\n-\treturn Base * time.Duration(attempt)\n+\tif attempt > 20 {\n+\t\treturn Max\n+\t}\n+\td := Base << uint(attempt-1)\n+\tif d > Max {\n+\t\treturn Max\n+\t}\n+\treturn d\n }\n \n // Do calls fn until it succeeds or maxAttempts is used up.\n"
  await call('POST', `/connector/proofs/${proof.id}/result`, {
    key,
    data: { diff, log_tail: 'fixed\n', duration_ms: 4200, exit_code: 0 },
  })

  // CI runs the API with ARENA_SANDBOX=fake (fast); a local `make run-api`
  // uses the real Docker sandbox, which is slower — hence the long poll.
  const deadline = Date.now() + 90_000
  let finished = null
  while (Date.now() < deadline) {
    finished = await call('GET', `/proofs/${proof.id}`)
    if (finished.finished_at) break
    await new Promise((r) => setTimeout(r, 1000))
  }
  if (!finished?.finished_at) throw new Error(`proof ${proof.id} did not finish within 90s of being submitted`)

  await check(owner, [`/app/proofs/${proof.id}`])

  // A challenge page in its widest state: places, the task, the hidden test
  // names and a published diff. The admin who creates it signs in with an email
  // from ARENA_ADMIN_EMAILS (see the mobile job in .github/workflows/ci.yml).
  const admin = await browser.newContext({ viewport: { width: WIDTH, height: 800 } })
  const asAdmin = async (method, path, data) => {
    const res = await admin.request.fetch(`${BASE}/api/v1${path}`, { method, data })
    if (!res.ok()) throw new Error(`${method} ${path}: ${res.status()} ${await res.text()}`)
    return res.status() === 204 ? null : res.json()
  }
  await asAdmin('POST', '/auth/dev', { email: 'admin@ci.local' })
  const slug = `ci-cup-${run}`
  await asAdmin('POST', '/admin/challenges', {
    slug,
    title: 'Cursor pagination cup',
    summary: 'One hidden task, one attempt each.',
    skill_task_slug: 'go-cursor-pagination',
    min_tier: 'none',
    opens_at: new Date(Date.now() - 3600_000).toISOString(),
    closes_at: new Date(Date.now() + 3600_000).toISOString(),
    prizes: 'bragging rights',
  })
  await asAdmin('POST', `/admin/challenges/${slug}/open`)
  await check(anonymous, [`/challenges/${slug}`, '/challenges'])

  // The owner's agent enters, the connector answers, and the challenge closes
  // and publishes — the page then carries standings, a task and a diff at once.
  await call('POST', '/connector/heartbeat', {
    key,
    data: { connector_version: 'ci', hostname: 'ci', version: { model: 'ci-model', harness: 'ci-harness', config_digest: `d-${run}` } },
  })
  const entry = await call('POST', `/challenges/${slug}/enter`, { data: { consent_publish: true } })
  const task = await call('GET', '/connector/tasks/next?wait=5', { key })
  if (task.proof_id !== entry.proof_id) throw new Error(`tasks/next returned ${task.proof_id}, expected ${entry.proof_id}`)
  await call('POST', `/connector/proofs/${entry.proof_id}/result`, {
    key,
    data: { diff, log_tail: 'entered\n', duration_ms: 1200, exit_code: 0 },
  })
  const entryDeadline = Date.now() + 90_000
  while (Date.now() < entryDeadline) {
    const p = await call('GET', `/proofs/${entry.proof_id}`)
    if (p.finished_at) break
    await new Promise((r) => setTimeout(r, 1000))
  }
  await asAdmin('POST', `/admin/challenges/${slug}/close`)
  await asAdmin('POST', `/admin/challenges/${slug}/publish`)
  await check(anonymous, [`/challenges/${slug}`])
} finally {
  await browser.close()
}

if (failures.length > 0) {
  console.error(`Wider than ${WIDTH}px:\n  ${failures.join('\n  ')}`)
  process.exit(1)
}
