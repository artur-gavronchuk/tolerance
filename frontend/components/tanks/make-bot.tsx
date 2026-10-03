'use client'

import Link from 'next/link'
import { Download } from 'lucide-react'
import { buttonVariants } from '@/components/ui/button'
import { CopyBlock } from '@/components/copy-block'
import { CLI } from '@/lib/brand'
import { UploadVersion } from './upload-version'
import { AgentUpload } from '@/components/upload-link/agent-upload'
import type { VersionView } from '@/lib/types'
import { cn } from '@/lib/utils'
import { useT } from '@/lib/i18n/client'
import { tanksOwnerMessages as m, ownerError } from '@/lib/i18n/messages/tanks-owner'

// Keep in step with tanks.AgentPrompt in backend/internal/games/tanks/starter.go (the starter kit's README).
export const BOT_PROMPT = `Read GAME.md in this folder, then improve the tank bot (bot.py or bot.js, whichever is here) so it beats as many house bots as you can: hunter and sniper first, then duelist, warden, ace. Keep bot.json valid and keep the same entry file. Use only the standard library of the language. You do not need to install or run anything: the platform plays the bot for you once the folder is zipped and uploaded. After each upload I will paste you a match report from the site (what hit you, how many shots landed, where the tank got stuck, the bot's stderr): read it, work out what went wrong and fix the bot.`

function Step({ n, title, children }: { n: number; title: string; children: React.ReactNode }) {
  return (
    <li className="flex gap-3">
      <span className="mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full bg-primary/10 text-xs font-bold text-primary">{n}</span>
      <div className="min-w-0 flex-1 space-y-2.5">
        <h3 className="text-sm font-bold">{title}</h3>
        {children}
      </div>
    </li>
  )
}

// The whole bot loop with nothing to install: download a starter kit, hand the
// folder to your own coding agent, zip it and upload it. The platform is the
// test harness - the trial match, its replay and the ladder matches show up
// right below on the same page.
export function MakeBot({ onUploaded }: { onUploaded: (v: VersionView) => void }) {
  const t = useT(m)
  return (
    <section className="rounded-[14px] border border-border bg-card p-5">
      <h2 className="heading text-xl">{t('make.title')}</h2>
      <p className="mt-1 text-sm text-muted-foreground">
        {t('make.intro')}
      </p>
      <ol className="mt-5 space-y-6">
        <Step n={1} title={t('make.step1')}>
          <div className="flex flex-wrap gap-2">
            <a href="/api/v1/tanks/starter/python.zip" download className={cn(buttonVariants({ variant: 'outline' }))}>
              <Download />{t('make.python')}
            </a>
            <a href="/api/v1/tanks/starter/js.zip" download className={cn(buttonVariants({ variant: 'outline' }))}>
              <Download />{t('make.js')}
            </a>
          </div>
          <p className="text-xs text-muted-foreground">
            {t('make.kitHelp')}
          </p>
        </Step>
        <Step n={2} title={t('make.step2')}>
          <p className="text-sm text-muted-foreground">
            {t('make.step2Help')}
          </p>
          <CopyBlock text={BOT_PROMPT} />
        </Step>
        <Step n={3} title={t('make.step3')}>
          <AgentUpload target={{ kind: 'tanks' }} />
          <UploadVersion onUploaded={onUploaded} />
          <p className="text-xs text-muted-foreground">
            {t('make.step3Help1')}
            <Link href="/tanks/docs" className="font-semibold text-primary hover:underline">{t('make.docs')}</Link>
            {t('make.step3Help2')}
          </p>
        </Step>
      </ol>
      <p className="mt-6 border-t border-border pt-4 text-xs text-muted-foreground">
        {t('make.advanced1')}<code>{CLI}</code>{t('make.advanced2')}<code>{CLI} tanks play</code>{t('make.advanced3')}
        <Link href="/tanks/docs#local" className="font-semibold text-primary hover:underline">{t('make.docs')}</Link>{t('make.advanced4')}
      </p>
    </section>
  )
}
