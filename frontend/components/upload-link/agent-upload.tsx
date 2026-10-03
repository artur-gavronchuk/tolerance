'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Bot } from 'lucide-react'
import { CopyBlock } from '@/components/copy-block'
import { Button } from '@/components/ui/button'
import { errorText } from '@/lib/format'
import { useT } from '@/lib/i18n/client'
import { uploadLinkMessages as m } from '@/lib/i18n/messages/upload-link'
import { agentPrompt, linkBase, useUploadLink, type AgentTarget } from '@/lib/upload-link'
import { useMe } from '@/lib/use-me'

// "Let my agent upload": reveals a ready-to-paste prompt with the person's personal curl link in it.
export function AgentUpload({ target }: { target: AgentTarget }) {
  const t = useT(m)
  const { me } = useMe()
  const signedIn = !!me
  const [open, setOpen] = useState(false)
  const { status, token, error, rotate } = useUploadLink(open && signedIn)
  const [busy, setBusy] = useState(false)
  async function create() {
    setBusy(true)
    await rotate()
    setBusy(false)
  }
  return (
    <div className="rounded-[12px] border border-dashed border-input p-3">
      <button type="button" aria-expanded={open} onClick={() => setOpen(!open)} className="flex w-full items-center gap-2 text-left text-sm font-bold">
        <Bot className="size-4 text-primary" />{t('toggle')}
        <span className="ml-auto text-xs font-normal text-muted-foreground">{open ? '−' : '+'}</span>
      </button>
      {!open && <p className="mt-1 pl-6 text-xs text-muted-foreground">{t('toggleHint')}</p>}
      {open && (
        <div className="mt-3 space-y-3">
          {!signedIn ? (
            <p className="text-sm text-muted-foreground">{t('signIn')}</p>
          ) : !status ? (
            <p className="text-sm text-muted-foreground">{t('loading')}</p>
          ) : token ? (
            <>
              <p className="text-sm text-muted-foreground">{t('promptLead')}</p>
              <CopyBlock text={agentPrompt(target, linkBase(token))} />
              <p className="text-xs text-muted-foreground">{t('secretNote')} <Link href={`/u/${me!.user.handle}`} className="font-semibold text-primary hover:underline">{t('manage')}</Link></p>
            </>
          ) : (
            <>
              <p className="text-sm text-muted-foreground">{status.active ? t('lostToken') : t('noLink')}</p>
              <Button type="button" size="sm" disabled={busy} onClick={() => void create()}>{busy ? t('creating') : status.active ? t('rotate') : t('create')}</Button>
            </>
          )}
          {error != null && <p role="alert" className="text-sm text-destructive">{t('failed', { error: errorText(error, t.locale) })}</p>}
        </div>
      )}
    </div>
  )
}
