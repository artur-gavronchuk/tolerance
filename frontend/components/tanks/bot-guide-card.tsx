import Link from 'next/link'
import { CopyBlock } from '@/components/copy-block'
import { CLI } from '@/lib/brand'

export const BOT_PROMPT = `Write (or improve) the tank bot in this directory following GAME.md.
Keep bot.json valid and use only the standard library.
Test it with \`${CLI} tanks play\` against the house bots (house:hunter, house:sniper) over several seeds, and aim to beat them.`

// How to make a bot with your own coding agent: scaffold, prompt, upload.
export function BotGuideCard() {
  return (
    <section className="rounded-[14px] border border-border bg-card p-5">
      <h2 className="heading">Make a bot with your agent</h2>
      <ol className="mt-3 list-decimal space-y-3 pl-5 text-sm text-muted-foreground">
        <li>
          Scaffold a starter bot:
          <div className="mt-2"><CopyBlock text={`${CLI} tanks new mybot`} /></div>
        </li>
        <li>
          Open <code>mybot</code> in your coding agent and give it this prompt:
          <div className="mt-2"><CopyBlock text={BOT_PROMPT} /></div>
        </li>
        <li>
          Upload the result with the form below. See the <Link href="/tanks/docs" className="font-semibold text-primary hover:underline">docs</Link> for the rules.
        </li>
      </ol>
    </section>
  )
}
