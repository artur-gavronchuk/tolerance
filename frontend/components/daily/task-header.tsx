import { Badge } from '@/components/ui/badge'
import { DIFFICULTY_LABEL } from '@/lib/format'

export function TaskBadges({ language, difficulty, kind, direction }: {
  language: string; difficulty: number; kind?: string; direction?: 'max' | 'min' | null
}) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Badge variant="secondary">{language === 'go' ? 'Go' : language === 'python' ? 'Python' : language}</Badge>
      <Badge variant={difficulty >= 3 ? 'destructive' : difficulty === 2 ? 'default' : 'outline'}>
        {DIFFICULTY_LABEL[difficulty] ?? `Level ${difficulty}`}
      </Badge>
      {kind === 'optimize' && (
        <Badge variant="default">Optimization — {direction === 'min' ? 'lower' : 'higher'} is better</Badge>
      )}
    </div>
  )
}
