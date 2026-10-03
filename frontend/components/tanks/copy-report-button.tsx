'use client'

import { errorText } from '@/lib/i18n/messages/errors'
import { useEffect, useRef, useState } from 'react'
import { Check, ClipboardCopy, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { ApiError } from '@/lib/api'
import { useT } from '@/lib/i18n/client'
import { tanksOwnerMessages as m } from '@/lib/i18n/messages/tanks-owner'
import { copyAsync, fetchReports } from '@/lib/tanks/report'

type State = 'idle' | 'busy' | 'copied' | 'error'

// Copies the plain-text match report(s) for the owner's coding agent. One id gives that match's report; several
// are concatenated in the order given. The confirmation is inline next to the button and announced politely.
export function CopyReportButton({
  matchIds, label, size = 'default', variant = 'default', className,
}: {
  matchIds: string[]; label?: string; size?: 'default' | 'sm' | 'xs'; variant?: 'default' | 'outline' | 'ghost' | 'secondary'; className?: string
}) {
  const t = useT(m)
  const [state, setState] = useState<State>('idle')
  const [message, setMessage] = useState('')
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)
  useEffect(() => () => { if (timer.current) clearTimeout(timer.current) }, [])

  async function copy() {
    if (matchIds.length === 0 || state === 'busy') return
    setState('busy')
    try {
      await copyAsync(fetchReports(matchIds))
      setState('copied')
      setMessage(matchIds.length === 1 ? t('copy.one') : t('copy.many', { n: matchIds.length }))
    } catch (e) {
      setState('error')
      setMessage(e instanceof ApiError ? errorText(e, t.locale) : t('copy.fail'))
    }
    if (timer.current) clearTimeout(timer.current)
    timer.current = setTimeout(() => setState('idle'), 3500)
  }

  return (
    <span className={`inline-flex flex-wrap items-center gap-2 ${className ?? ''}`}>
      <Button type="button" variant={variant} size={size} onClick={() => void copy()} disabled={matchIds.length === 0 || state === 'busy'}>
        {state === 'busy' ? <Loader2 className="animate-spin" /> : state === 'copied' ? <Check /> : <ClipboardCopy />}
        {label ?? t('copy.label')}
      </Button>
      <span role="status" aria-live="polite" className={state === 'error' ? 'text-xs text-destructive' : 'text-xs text-muted-foreground'}>
        {state === 'copied' || state === 'error' ? message : ''}
      </span>
    </span>
  )
}
