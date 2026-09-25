import { cn } from '@/lib/utils'

type Line = { kind: 'add' | 'del' | 'ctx' | 'hunk'; text: string; old?: number; new?: number }
type FileDiff = { path: string; lines: Line[]; added: number; removed: number }

// Splits a unified diff into files and numbers every line from its hunk
// headers. Anything unexpected is kept as context rather than dropped.
function parse(diff: string): FileDiff[] {
  const files: FileDiff[] = []
  let cur: FileDiff | null = null
  let o = 0
  let n = 0
  for (const raw of diff.split('\n')) {
    if (raw.startsWith('diff --git')) { cur = null; continue }
    if (raw.startsWith('--- ')) {
      cur = { path: '', lines: [], added: 0, removed: 0 }
      files.push(cur)
      const from = raw.slice(4).replace(/^a\//, '')
      if (from !== '/dev/null') cur.path = from
      continue
    }
    if (raw.startsWith('+++ ') && cur) {
      const to = raw.slice(4).replace(/^b\//, '')
      if (to !== '/dev/null') cur.path = to
      continue
    }
    if (!cur) {
      if (!raw.trim() || /^(index|new file|deleted file|similarity|rename|old mode|new mode)/.test(raw)) continue
      cur = { path: '', lines: [], added: 0, removed: 0 }
      files.push(cur)
    }
    const h = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@(.*)$/.exec(raw)
    if (h) {
      o = +h[1]
      n = +h[2]
      cur.lines.push({ kind: 'hunk', text: raw })
    } else if (raw.startsWith('+')) {
      cur.lines.push({ kind: 'add', text: raw.slice(1), new: n++ })
      cur.added++
    } else if (raw.startsWith('-')) {
      cur.lines.push({ kind: 'del', text: raw.slice(1), old: o++ })
      cur.removed++
    } else if (raw.startsWith('\\')) {
      cur.lines.push({ kind: 'hunk', text: raw })
    } else {
      cur.lines.push({ kind: 'ctx', text: raw.startsWith(' ') ? raw.slice(1) : raw, old: o++, new: n++ })
    }
  }
  // A trailing newline in the diff leaves one empty context line behind.
  for (const f of files) {
    const l = f.lines[f.lines.length - 1]
    if (l && l.kind === 'ctx' && l.text === '') f.lines.pop()
  }
  return files
}

export function DiffView({ diff }: { diff: string }) {
  if (!diff.trim()) return <p className="text-sm text-muted-foreground">The agent changed nothing.</p>
  const files = parse(diff)
  return (
    <div className="space-y-4">
      {files.map((f, i) => (
        <div key={i} className="overflow-hidden rounded-[14px] border border-border bg-card">
          <div className="flex items-center justify-between gap-3 border-b border-border bg-muted/50 px-4 py-2.5">
            <span className="min-w-0 truncate font-mono text-xs font-semibold">{f.path || 'unnamed file'}</span>
            <span className="shrink-0 font-mono text-xs">
              <span className="text-success">+{f.added}</span> <span className="text-destructive">−{f.removed}</span>
            </span>
          </div>
          <div className="max-h-[28rem] overflow-auto">
            <table className="w-full border-collapse font-mono text-xs leading-5">
              <tbody>
                {f.lines.map((l, j) => l.kind === 'hunk' ? (
                  <tr key={j} className="bg-accent/60 text-accent-foreground">
                    <td colSpan={3} className="px-3 py-1 whitespace-pre">{l.text}</td>
                  </tr>
                ) : (
                  <tr key={j} className={cn(l.kind === 'add' && 'bg-success/10', l.kind === 'del' && 'bg-destructive/10')}>
                    <td className="w-10 border-r border-border/60 px-2 text-right text-muted-foreground/70 select-none">{l.old ?? ''}</td>
                    <td className="w-10 border-r border-border/60 px-2 text-right text-muted-foreground/70 select-none">{l.new ?? ''}</td>
                    <td className="px-3 whitespace-pre">
                      <span className={cn('mr-2 select-none', l.kind === 'add' ? 'text-success' : l.kind === 'del' ? 'text-destructive' : 'text-transparent')}>
                        {l.kind === 'add' ? '+' : l.kind === 'del' ? '−' : ' '}
                      </span>
                      {l.text || ' '}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ))}
    </div>
  )
}
