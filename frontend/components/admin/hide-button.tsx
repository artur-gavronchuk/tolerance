'use client'

import { useState } from 'react'
import { EyeOff } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { moderation, type ModKind } from '@/lib/api'
import { useCanAdmin } from '@/lib/use-can-admin'
import { useT } from '@/lib/i18n/client'
import { moderationMessages } from '@/lib/i18n/messages/admin-moderation'
import { ReasonForm } from './reason-form'

// A small admin-only control next to a public entry, submission or bot: asks for a reason, then hides it.
export function HideButton({ kind, id }: { kind: ModKind; id?: string }) {
  const t = useT(moderationMessages)
  const can = useCanAdmin()
  const [open, setOpen] = useState(false)
  const [done, setDone] = useState(false)
  if (!can || !id) return null
  if (done) return <span className="text-xs font-semibold text-muted-foreground">{t('hidden')}</span>
  if (!open) {
    return (
      <Button type="button" size="xs" variant="ghost" className="text-muted-foreground" title={t('hideTitle')} onClick={() => setOpen(true)}>
        <EyeOff />{t('hide')}
      </Button>
    )
  }
  return (
    <div className="w-full max-w-sm">
      <ReasonForm label={t('hide')} run={(reason) => moderation.hide(kind, id, reason)} onDone={() => setDone(true)} onCancel={() => setOpen(false)} />
    </div>
  )
}
