import { apiRaw } from '@/lib/api'

// Plain-text report of one match for the bot's owner, written to be pasted into a coding agent.
export async function fetchMatchReport(matchId: string): Promise<string> {
  const res = await apiRaw(`/me/tanks/matches/${matchId}/report`)
  return res.text()
}

// Several reports as one paste, newest first, separated so the agent can tell them apart.
export async function fetchReports(matchIds: string[]): Promise<string> {
  const reports = await Promise.all(matchIds.map(fetchMatchReport))
  if (reports.length === 1) return reports[0]
  return `Reports for my last ${reports.length} Tanks matches, newest first.\n\n` + reports.join('\n\n---\n\n')
}

// Rejects if p has not settled within ms: the async clipboard API can wait forever on a permission prompt or an
// unfocused document.
function within<T>(p: Promise<T>, ms: number): Promise<T> {
  return new Promise((resolve, reject) => {
    const t = setTimeout(() => reject(new Error('clipboard timeout')), ms)
    p.then((v) => { clearTimeout(t); resolve(v) }, (e) => { clearTimeout(t); reject(e) })
  })
}

// The old synchronous route, for browsers where navigator.clipboard is missing or refuses.
function execCopy(text: string): boolean {
  const ta = document.createElement('textarea')
  ta.value = text
  ta.setAttribute('readonly', '')
  ta.style.cssText = 'position:fixed;top:0;left:0;opacity:0'
  document.body.appendChild(ta)
  ta.select()
  try {
    return document.execCommand('copy')
  } catch {
    return false
  } finally {
    ta.remove()
  }
}

// Copies the text the promise resolves to. Passing the promise to ClipboardItem keeps the click's user
// activation alive across the fetch (Safari drops it otherwise); other browsers fall back to writeText and
// finally to execCommand once the text is in hand. Throws if the fetch failed or nothing could copy.
export async function copyAsync(text: Promise<string>): Promise<void> {
  text.catch(() => {}) // surfaced below; avoid an unhandled rejection if the clipboard path throws first
  if (typeof ClipboardItem !== 'undefined' && navigator.clipboard?.write) {
    try {
      await within(
        navigator.clipboard.write([new ClipboardItem({ 'text/plain': text.then((t) => new Blob([t], { type: 'text/plain' })) })]),
        8000,
      )
      return
    } catch {
      // try the next route
    }
  }
  const t = await text
  try {
    await within(navigator.clipboard.writeText(t), 3000)
    return
  } catch {
    // try the next route
  }
  if (!execCopy(t)) throw new Error('Could not copy to the clipboard.')
}
