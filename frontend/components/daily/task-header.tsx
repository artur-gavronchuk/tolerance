import { Badge } from '@/components/ui/badge'
import { difficultyLabel } from '@/lib/format'
import { useT } from '@/lib/i18n/client'
import { dailyMessages } from '@/lib/i18n/messages/daily'

export function TaskBadges({ language, difficulty, kind, direction }: {
  language: string; difficulty: number; kind?: string; direction?: 'max' | 'min' | null
}) {
  const t = useT(dailyMessages)
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Badge variant="secondary">{language === 'go' ? 'Go' : language === 'python' ? 'Python' : language}</Badge>
      <Badge variant={difficulty >= 3 ? 'destructive' : difficulty === 2 ? 'default' : 'outline'}>
        {difficultyLabel(difficulty, t.locale)}
      </Badge>
      {kind === 'optimize' && (
        <Badge variant="default">{direction === 'min' ? t('optimizeLower') : t('optimizeHigher')}</Badge>
      )}
    </div>
  )
}
