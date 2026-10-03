'use client'

import { Button } from '@/components/ui/button'
import { usePT } from './phase'

// Under a standings list the server cut to a page: the next page on demand.
export function ShowMore({ shown, total, onMore }: { shown: number; total: number; onMore: () => void }) {
  const t = usePT()
  if (shown >= total) return null
  return (
    <div className="flex justify-center">
      <Button variant="outline" onClick={onMore}>{t('res.showMore', { shown, total })}</Button>
    </div>
  )
}
