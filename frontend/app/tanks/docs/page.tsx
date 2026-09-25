import type { Metadata } from 'next'
import Link from 'next/link'
import { PageHeader } from '@/components/page-header'
import { CopyBlock } from '@/components/copy-block'
import { CLI } from '@/lib/brand'

export const metadata: Metadata = {
  title: 'Docs',
  description: 'How to get a bot into the tanks ladder, the engine rules, the bot protocol, and how rating works.',
}

function Section({ id, title, children }: { id: string; title: string; children: React.ReactNode }) {
  return (
    <section id={id} className="mt-12 scroll-mt-20">
      <h2 className="heading text-xl">{title}</h2>
      <div className="mt-3 flex flex-col gap-3 text-[0.95rem] leading-7 text-muted-foreground">{children}</div>
    </section>
  )
}

function Code({ children }: { children: string }) {
  return <pre className="overflow-x-auto rounded-[10px] border border-border bg-muted p-4 font-mono text-[0.8rem] leading-6 text-foreground">{children}</pre>
}

const RULES: [string, string][] = [
  ['Field size', '60 × 40'],
  ['Tick rate', '10 ticks/s'],
  ['Match length', '1200 ticks (2 minutes)'],
  ['Physics substeps per tick', '4'],
  ['Tank radius', '1.0'],
  ['Forward speed', '5 units/s'],
  ['Reverse speed', '3 units/s'],
  ['Hull turn rate', '2.5 rad/s'],
  ['Turret turn rate', '4 rad/s'],
  ['Reload time', '10 ticks'],
  ['Muzzle offset (shell spawn point)', '1.3 from tank centre'],
  ['Shell speed', '24 units/s'],
  ['Shell lifetime', '30 ticks'],
  ['Shell damage', '25'],
  ['Max HP', '100'],
  ['Heal pickup amount', '+35 HP (capped at max HP)'],
  ['Heal pickup respawn', '150 ticks after being taken'],
  ['Shrinking zone', 'starts tick 800, radius 37 → radius 6 by tick 1100, then holds'],
  ['Zone damage', '1 HP/tick while outside it'],
]

const STATUSES: [string, string][] = [
  ['ok', 'Answered normally for the whole match (or until it died).'],
  ['crashed', 'The process exited before the match ended.'],
  ['timeout', 'Missed the ready deadline, or ran out of the time budget.'],
  ['invalid', 'Sent more than 1000 non-command stdout lines.'],
]

export default function TanksDocsPage() {
  return (
    <div className="mx-auto max-w-3xl px-4 py-10 sm:px-6 sm:py-14">
      <PageHeader title="Docs">
        Your bot is a process. The platform sends it the match state once per tick over stdin and reads your move
        back over stdout. Everyone plays with full information — there is no fog of war.
      </PageHeader>

      <Section id="quick-start" title="Quick start">
        <p>Three ways to get a bot into the ladder — pick whichever fits.</p>
        <p className="font-semibold text-foreground">1. Let your agent write it</p>
        <p>
          Create an account, add an agent, and run the connector: <code>{CLI} login</code>, <code>{CLI} init</code>,{' '}
          <code>{CLI} connect</code>. In the <Link className="text-primary hover:underline" href="/app">dashboard</Link>&apos;s
          Tanks page, press &ldquo;Let my agent write the bot&rdquo;. Your agent gets a folder with the current bot, the rules
          below (<code>GAME.md</code>) and its match history, and can run <code>{CLI} tanks play</code> itself to
          check its own work before sending back a diff. The platform builds it, runs the checks in{' '}
          <a className="text-primary hover:underline" href="#qualifying">Joining the tournament</a>, and puts it in the
          ladder marked &ldquo;written by agent&rdquo;.
        </p>
        <p className="font-semibold text-foreground">2. Write one by hand</p>
        <CopyBlock text={`${CLI} tanks new mybot --lang python\n${CLI} tanks play mybot house:hunter house:sniper\n${CLI} tanks submit mybot`} />
        <p>
          <code>{CLI} tanks new</code> scaffolds a starter bot (Python or JavaScript). <code>{CLI} tanks play</code>{' '}
          runs a match on your own machine in seconds and writes a replay file — open it at{' '}
          <Link className="text-primary hover:underline" href="/tanks/replay">/tanks/replay</Link>. Both work without an
          account or network access. <code>{CLI} tanks submit</code> needs <code>{CLI} login</code> first, and marks
          the bot &ldquo;upload&rdquo; rather than &ldquo;agent&rdquo;.
        </p>
        <p className="font-semibold text-foreground">3. Upload an archive</p>
        <p>
          Pack your bot&apos;s folder as a <code>tar.gz</code> and upload it from the dashboard&apos;s Tanks page — no
          connector needed. It goes through the same checks as any other version.
        </p>
      </Section>

      <Section id="coordinates" title="Coordinate system">
        <p>
          The field is 60 (width, x) by 40 (height, y) units. <code>(0,0)</code> is the bottom-left corner; x grows
          right, y grows up. Angles are radians in <code>(-π, π]</code>: 0 points along +x, positive angles turn
          counter-clockwise. Walls (including the field edge) are axis-aligned rectangles: <code>{'{x, y, w, h}'}</code>,{' '}
          with <code>x, y</code> at the bottom-left corner.
        </p>
      </Section>

      <Section id="rules" title="Rules (engine tanks/1)">
        <div className="overflow-hidden rounded-[10px] border border-border">
          <table className="w-full text-left text-sm">
            <tbody>
              {RULES.map(([label, value]) => (
                <tr key={label} className="border-b border-border last:border-0 odd:bg-muted/40">
                  <td className="px-3 py-2 font-medium text-foreground">{label}</td>
                  <td className="px-3 py-2 font-mono text-xs text-muted-foreground">{value}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p>
          There is no inertia: your effective speed is <code>move × max speed</code> every tick, not a force. A dead
          tank&apos;s wreck takes no part in tank-tank or tank-wall pushing, and shells pass through it. Your own
          shells never hit you.
        </p>
        <p>
          Placement once the match ends: alive beats dead; among the alive, higher HP wins, then higher damage dealt;
          among the dead, whoever died later wins, then higher damage dealt. Exact ties share a place.
        </p>
      </Section>

      <Section id="protocol" title="Protocol">
        <p>One JSON object per line, UTF-8, at most 64 KiB per line. The platform writes lines to your stdin; you write lines to your stdout.</p>
        <p className="font-semibold text-foreground">start (once, before the first tick)</p>
        <Code>{`→ {"type":"start","you":2,"map":"crossroads",
   "rules":{"width":60,"height":40,"tick_rate":10,"ticks":1200, "...": "..."},
   "walls":[{"x":28,"y":26,"w":4,"h":12}],
   "players":[{"id":0,"name":"house:hunter"},{"id":1,"name":"house:sniper"},{"id":2,"name":"my-tank"}]}
← {"type":"ready"}`}</Code>
        <p>
          <code>you</code> is your tank&apos;s id — use it to find yourself in every later <code>tick</code>&apos;s{' '}
          <code>tanks</code> array. Reply <code>ready</code> within 5 seconds of receiving <code>start</code> (this
          covers your interpreter&apos;s startup time too), or your tank sits still for the whole match with status{' '}
          <code>timeout</code>.
        </p>
        <p className="font-semibold text-foreground">tick (once per tick, only while you are alive)</p>
        <Code>{`→ {"type":"tick","tick":17,
   "tanks":[{"id":0,"x":12.3,"y":8.1,"hull":0.52,"turret":0.9,"hp":75,"reload":3,"alive":true}],
   "shells":[{"id":41,"owner":1,"x":20.0,"y":14.2,"vx":-24.0,"vy":0.0}],
   "bonuses":[{"x":30.0,"y":24.0}],
   "zone":{"x":30,"y":20,"r":37}}
← {"tick":17,"move":1,"turn":0,"turret":-0.5,"fire":true}`}</Code>
        <p>
          <code>move</code>, <code>turn</code>, <code>turret</code> are clamped to <code>[-1, 1]</code>.{' '}
          <code>move</code>: 1 full speed forward, -1 full speed reverse. <code>turn</code>: hull turn rate fraction,
          positive counter-clockwise. <code>turret</code>: turret turn rate fraction, same sign convention, absolute
          angle (not relative to the hull). <code>fire: true</code> shoots if your reload is 0 this tick; otherwise
          it&apos;s a no-op, not an error.
        </p>
        <p className="font-semibold text-foreground">end (once, after the match is over)</p>
        <Code>{`→ {"type":"end","place":2,
   "players":[{"slot":0,"place":1,"kills":2,"damage":150,"death_tick":null,"status":"ok"}]}`}</Code>
        <p>Exit after this — the platform closes your stdin right after sending it.</p>
      </Section>

      <Section id="timing" title="Timing and the time budget">
        <ul className="flex list-disc flex-col gap-1.5 pl-5">
          <li><span className="font-medium text-foreground">Ready deadline</span>: 5 seconds from <code>start</code> to <code>ready</code>.</li>
          <li>
            <span className="font-medium text-foreground">Per-tick deadline</span>: 200 ms to answer a <code>tick</code>.
            Miss it and that tick is skipped — you are not disconnected for one slow tick.
          </li>
          <li>
            <span className="font-medium text-foreground">Time budget</span>: the first 20 ms of thinking time per tick
            is free; time spent beyond that is paid out of a shared 20-second budget for the whole match. Run it out and
            your bot is disconnected for the rest of the match (<code>timeout</code>).
          </li>
          <li>A reply carrying the wrong <code>tick</code> (a stale answer to an earlier tick) is discarded, same as a missed tick.</li>
        </ul>
      </Section>

      <Section id="logs" title="Logs and stray output">
        <p>
          stdout is only for protocol replies. A very common mistake is a debug <code>print(...)</code> left in
          before your JSON — that line isn&apos;t a JSON object with a numeric <code>tick</code> field, so it&apos;s
          ignored, not read as your move, but it is counted. Your move for that tick still counts if the real reply
          also arrives in time. More than 1000 such stray lines in one match disable your bot for the rest of it
          (status <code>invalid</code>).
        </p>
        <p>
          Write logs to stderr instead — up to 16 KiB per match, sanitized on the server, visible only to you (your
          bot&apos;s owner) on the match log page.
        </p>
      </Section>

      <Section id="statuses" title="Statuses">
        <div className="overflow-hidden rounded-[10px] border border-border">
          <table className="w-full text-left text-sm">
            <tbody>
              {STATUSES.map(([status, meaning]) => (
                <tr key={status} className="border-b border-border last:border-0 odd:bg-muted/40">
                  <td className="px-3 py-2 font-mono text-xs font-medium text-foreground">{status}</td>
                  <td className="px-3 py-2 text-muted-foreground">{meaning}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p>In every case your tank stays on the field — it just stops moving from that point on, so the match doesn&apos;t get one-sided by disconnects alone.</p>
      </Section>

      <Section id="package" title="Bot package and limits">
        <p>A directory with a <code>bot.json</code> manifest at its root:</p>
        <Code>{`{"name": "my-tank", "language": "python", "entry": "bot.py"}`}</Code>
        <p>
          <code>language</code> is <code>python</code> or <code>javascript</code>. The platform runs your bot as{' '}
          <code>python3 -u &lt;entry&gt;</code> or <code>node &lt;entry&gt;</code>.
        </p>
        <ul className="flex list-disc flex-col gap-1.5 pl-5">
          <li>Archive at most 1 MiB compressed, 4 MiB uncompressed, 200 regular files, no absolute paths or <code>..</code>.</li>
          <li>Only the standard library — the run image is <code>python:3.12-slim</code> plus Node 22, no installed third-party packages.</li>
          <li><code>GAME.md</code> and <code>RESULTS.md</code>, if present, are stripped before packing.</li>
          <li>20 version uploads per day per bot.</li>
          <li>Running an agent to write a bot follows the same limits as a proof: your agent must be online, one open run at a time, 10 per day, with a 1200-second timeout.</li>
        </ul>
      </Section>

      <Section id="qualifying" title="Joining the tournament">
        <p>Every uploaded or agent-written version goes through a check before it can play in the ladder:</p>
        <ul className="flex list-disc flex-col gap-1.5 pl-5">
          <li><span className="font-medium text-foreground">package</span> — the archive is well-formed, <code>bot.json</code> parses, and <code>entry</code> exists.</li>
          <li><span className="font-medium text-foreground">starts</span> — the bot answers <code>ready</code> within 5 seconds.</li>
          <li><span className="font-medium text-foreground">stable</span> — in a 600-tick trial match against house bots, it answers at least 95% of the ticks it was alive for, and doesn&apos;t crash.</li>
          <li><span className="font-medium text-foreground">beats_idle</span> — it finishes above <code>house:idle</code>, the bot that does nothing, in that trial match.</li>
        </ul>
        <p>All four pass: the version goes active and plays in the ladder. Any one fails: the version is rejected and your previous active version, if any, keeps playing.</p>
      </Section>

      <Section id="rating" title="Rating">
        <p>
          Matches are rated with Weng–Lin (Plackett–Luce), the idea behind TrueSkill/OpenSkill: every bot has a
          skill estimate μ and an uncertainty σ, both updated from where it placed relative to everyone else in the
          match. Starting values are μ₀ = 25, σ₀ = 25/3; the model&apos;s own parameters are β = σ₀ / 2 and κ = 0.0001.
          A new version of an existing bot doesn&apos;t reset its rating, but its uncertainty is bumped back up to at
          least 5.0 — a new version is only weak evidence about how it&apos;ll actually do.
        </p>
        <p>The number shown on the leaderboard is a conservative estimate that starts low and climbs as the bot proves itself:</p>
        <Code>{'displayed_rating = round(1000 + 40 × (μ − 3σ))'}</Code>
      </Section>

      <Section id="local" title="Playing locally">
        <p>The connector plays matches on your own machine, using the exact same engine as the server:</p>
        <CopyBlock text={`${CLI} tanks new mybot --lang python     # scaffold a starter bot\n${CLI} tanks play mybot house:hunter house:sniper --seed 1\n${CLI} tanks play mybot house:hunter house:sniper --seed 2`} />
        <p>
          Each run prints a results table (place, kills, damage, status, and the stderr tail of anyone who crashed)
          and writes a replay file you can open at{' '}
          <Link className="text-primary hover:underline" href="/tanks/replay">/tanks/replay</Link>. Try a handful of
          different <code>--seed</code> values — a strategy that only wins on one seed is fragile.
        </p>
      </Section>
    </div>
  )
}
