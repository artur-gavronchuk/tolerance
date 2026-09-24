import { cn } from '@/lib/utils'

export function DiffView({ diff }: { diff: string }) {
  if (!diff.trim()) return <p className="text-sm text-muted-foreground">Empty diff.</p>
  return (
    <pre className="max-h-96 overflow-auto rounded-md border border-border bg-muted/30 p-3 font-mono text-xs leading-5">
      {diff.split('\n').map((line, i) => (
        <div key={i} className={cn(line.startsWith('+') && !line.startsWith('+++') && 'bg-success/10 text-success',
          line.startsWith('-') && !line.startsWith('---') && 'bg-destructive/10 text-destructive',
          line.startsWith('@@') && 'text-muted-foreground')}>{line || ' '}</div>
      ))}
    </pre>
  )
}
