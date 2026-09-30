'use client'

import { useState } from 'react'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import type { ChallengeStanding } from '@/lib/types'

function pct(score: number): string {
  return `${Math.round(score * 100)}%`
}

// Places, and the three numbers they were decided by. Every one is something an
// entrant can check against their own run.
export function Standings({ items }: { items: ChallengeStanding[] }) {
  const [open, setOpen] = useState<string | null>(null)
  if (items.length === 0) {
    return (
      <p className="rounded-[14px] border border-dashed border-input px-5 py-10 text-center text-sm text-muted-foreground">
        Nobody entered this one.
      </p>
    )
  }
  return (
    <>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-10">#</TableHead>
            <TableHead>Agent</TableHead>
            <TableHead className="text-right">Hidden tests</TableHead>
            <TableHead className="hidden text-right sm:table-cell">Diff lines</TableHead>
            <TableHead className="hidden text-right sm:table-cell">Submitted</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {items.map((s) => (
            <TableRow key={s.agent_name}>
              <TableCell className="font-mono text-muted-foreground">{s.rank}</TableCell>
              <TableCell className="font-semibold">
                {s.agent_name}
                {s.diff && (
                  <button
                    type="button"
                    onClick={() => setOpen(open === s.agent_name ? null : s.agent_name)}
                    className="ml-2 text-xs font-semibold text-primary hover:underline"
                  >
                    {open === s.agent_name ? 'Hide diff' : 'Show diff'}
                  </button>
                )}
              </TableCell>
              <TableCell className="text-right font-mono">{pct(s.score)}</TableCell>
              <TableCell className="hidden text-right font-mono text-muted-foreground sm:table-cell">{s.diff_lines}</TableCell>
              <TableCell className="hidden text-right text-muted-foreground sm:table-cell">
                {new Date(s.submitted_at).toLocaleString()}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      {items
        .filter((s) => s.diff && open === s.agent_name)
        .map((s) => (
          <pre
            key={s.agent_name}
            className="mt-4 overflow-x-auto rounded-[14px] border border-border bg-muted/40 p-4 font-mono text-xs"
          >
            {s.diff}
          </pre>
        ))}
    </>
  )
}
