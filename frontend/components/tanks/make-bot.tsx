import Link from 'next/link'
import { Download } from 'lucide-react'
import { buttonVariants } from '@/components/ui/button'
import { CopyBlock } from '@/components/copy-block'
import { CLI } from '@/lib/brand'
import { UploadVersion } from './upload-version'
import type { VersionView } from '@/lib/types'
import { cn } from '@/lib/utils'

// Keep in step with tanks.AgentPrompt in backend/internal/games/tanks/starter.go (the starter kit's README).
export const BOT_PROMPT = `Read GAME.md in this folder, then improve the tank bot (bot.py or bot.js, whichever is here) so it beats the house bots hunter and sniper. Keep bot.json valid and keep the same entry file. Use only the standard library of the language. You do not need to install or run anything: the platform plays the bot for you once the folder is zipped and uploaded.`

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
  return (
    <section className="rounded-[14px] border border-border bg-card p-5">
      <h2 className="heading text-xl">Make a bot with your agent</h2>
      <p className="mt-1 text-sm text-muted-foreground">
        Nothing to install. The platform plays your bot for you, so every upload is tested against the house bots.
      </p>
      <ol className="mt-5 space-y-6">
        <Step n={1} title="Download a starter kit">
          <div className="flex flex-wrap gap-2">
            <a href="/api/v1/tanks/starter/python.zip" download className={cn(buttonVariants({ variant: 'outline' }))}>
              <Download />Python
            </a>
            <a href="/api/v1/tanks/starter/js.zip" download className={cn(buttonVariants({ variant: 'outline' }))}>
              <Download />JavaScript
            </a>
          </div>
          <p className="text-xs text-muted-foreground">
            A working bot, the rules (GAME.md) and a short README. Unzip it anywhere.
          </p>
        </Step>
        <Step n={2} title="Give the folder to your coding agent">
          <p className="text-sm text-muted-foreground">
            Open it in Claude Code, Cursor, Codex or any agent and send this prompt:
          </p>
          <CopyBlock text={BOT_PROMPT} />
        </Step>
        <Step n={3} title="Zip the folder and upload it">
          <UploadVersion onUploaded={onUploaded} />
          <p className="text-xs text-muted-foreground">
            Each version gets a trial match, then plays the ladder. Read the check results and replays below, paste
            what went wrong back to your agent and upload the next version. The{' '}
            <Link href="/tanks/docs" className="font-semibold text-primary hover:underline">docs</Link> have the rules.
          </p>
        </Step>
      </ol>
      <p className="mt-6 border-t border-border pt-4 text-xs text-muted-foreground">
        Advanced: the optional <code>{CLI}</code> command-line tool runs matches on your own machine
        (<code>{CLI} tanks play</code>), see the <Link href="/tanks/docs#local" className="font-semibold text-primary hover:underline">docs</Link>.
      </p>
    </section>
  )
}
