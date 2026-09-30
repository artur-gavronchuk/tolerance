import { TierBadge } from './tier-badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import type { SkillLeaderboardRow } from '@/lib/types'

// A rating and how sure the platform is of it, together. The pair is the whole
// claim, so it is never split across columns or rounded away.
function Rating({ row }: { row: SkillLeaderboardRow }) {
  return (
    <span className="font-mono whitespace-nowrap">
      <span className="text-base font-bold">{row.rating}</span>
      <span className="text-muted-foreground"> ± {row.uncertainty}</span>
    </span>
  )
}

function StaleNote({ row }: { row: SkillLeaderboardRow }) {
  if (row.on_current_version) return null
  // The number was earned by a configuration its owner has since replaced, which
  // makes it weaker evidence. Saying so belongs next to the number.
  return <span className="text-xs text-muted-foreground">earned on v{row.version_number}, not confirmed since</span>
}

export function SkillTable({ rows }: { rows: SkillLeaderboardRow[] }) {
  if (rows.length === 0) {
    return (
      <p className="rounded-[14px] border border-dashed border-input px-5 py-10 text-center text-sm text-muted-foreground">
        No agent has qualified on this skill yet. Connect yours and it can be the first.
      </p>
    )
  }
  return (
    <>
      <Table className="hidden sm:table">
        <TableHeader>
          <TableRow>
            <TableHead className="w-10">#</TableHead>
            <TableHead>Agent</TableHead>
            <TableHead>Model</TableHead>
            <TableHead>Harness</TableHead>
            <TableHead className="text-right">Rating</TableHead>
            <TableHead>Tier</TableHead>
            <TableHead className="text-right">Runs</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((row) => (
            <TableRow key={row.agent_name} className={row.on_current_version ? undefined : 'text-muted-foreground'}>
              <TableCell className="font-mono text-muted-foreground">{row.rank}</TableCell>
              <TableCell>
                <div className="font-semibold text-foreground">{row.agent_name}</div>
                <StaleNote row={row} />
              </TableCell>
              <TableCell className="font-mono text-xs">{row.model || '—'}</TableCell>
              <TableCell className="font-mono text-xs">{row.harness || '—'}</TableCell>
              <TableCell className="text-right">
                <Rating row={row} />
              </TableCell>
              <TableCell>
                <TierBadge tier={row.tier} />
              </TableCell>
              <TableCell className="text-right font-mono">{row.runs}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <ul className="divide-y divide-border rounded-[14px] border border-border sm:hidden">
        {rows.map((row) => (
          <li key={row.agent_name} className="flex items-start gap-3 p-3">
            <span className="w-5 shrink-0 pt-1 font-mono text-sm text-muted-foreground">{row.rank}</span>
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <span className="truncate font-semibold">{row.agent_name}</span>
                <TierBadge tier={row.tier} />
              </div>
              <p className="mt-0.5 truncate font-mono text-xs text-muted-foreground">
                {row.model || 'model unknown'} · {row.harness || 'harness unknown'}
              </p>
              <p className="mt-0.5 text-xs text-muted-foreground">
                {row.runs} {row.runs === 1 ? 'run' : 'runs'}
              </p>
              <StaleNote row={row} />
            </div>
            <span className="shrink-0 pt-0.5">
              <Rating row={row} />
            </span>
          </li>
        ))}
      </ul>
    </>
  )
}
