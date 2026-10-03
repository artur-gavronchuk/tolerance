import type { Metadata } from 'next'
import Link from 'next/link'
import { PageHeader } from '@/components/page-header'
import { CopyBlock } from '@/components/copy-block'
import { CLI } from '@/lib/brand'
import { InstallCli } from '@/components/tanks/install-cli'
import { getT } from '@/lib/i18n/server'
import { tanksDocsMessages as m } from '@/lib/i18n/messages/tanks-docs'

export async function generateMetadata(): Promise<Metadata> {
  const t = await getT(m)
  return { title: t('metaTitle'), description: t('metaDescription') }
}

// Renders <c>code</c>, <b>emphasis</b> and <l>link</l> markup from the message tables.
function Rich({ s, href, vars }: { s: string; href?: string; vars?: Record<string, string> }) {
  const text = s.replace(/\{(\w+)\}/g, (all, k: string) => vars?.[k] ?? all)
  const parts = text.split(/(<c>.*?<\/c>|<b>.*?<\/b>|<l>.*?<\/l>)/g)
  return (
    <>
      {parts.map((p, i) => {
        const inner = p.slice(3, -4).replace(/&lt;/g, '<').replace(/&gt;/g, '>')
        if (p.startsWith('<c>')) return <code key={i}>{inner}</code>
        if (p.startsWith('<b>')) return <span key={i} className="font-medium text-foreground">{inner}</span>
        if (p.startsWith('<l>')) return <Link key={i} className="text-primary hover:underline" href={href ?? '#'}>{inner}</Link>
        return p
      })}
    </>
  )
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

const RULE_COUNT = 19
const TOC_IDS = ['quick-start', 'coordinates', 'rules', 'protocol', 'timing', 'logs', 'statuses', 'package', 'qualifying', 'rating', 'local'] as const
const STATUS_IDS = ['ok', 'crashed', 'timeout', 'invalid'] as const

const UL = 'flex list-disc flex-col gap-1.5 pl-5'
const H = 'font-semibold text-foreground'

export default async function TanksDocsPage() {
  const t = await getT(m)
  const rich = (k: Parameters<typeof t>[0], href?: string, vars?: Record<string, string>) => <Rich s={t(k)} href={href} vars={vars} />
  return (
    <div className="mx-auto max-w-3xl px-4 py-10 sm:px-6 sm:py-14">
      <PageHeader title={t('title')}>{t('intro')}</PageHeader>

      <nav aria-label={t('onThisPage')} className="mt-8 flex flex-wrap gap-2">
        {TOC_IDS.map((id) => (
          <a key={id} href={`#${id}`} className="rounded-full border border-border px-3 py-1.5 text-sm font-semibold text-muted-foreground hover:text-foreground">
            {t(`toc.${id}`)}
          </a>
        ))}
      </nav>

      <Section id="quick-start" title={t('qs.title')}>
        <p>{rich('qs.lead', '/app/tanks')}</p>
        <p className={H}>{t('qs.h1')}</p>
        <p>{rich('qs.p1')}</p>
        <p className={H}>{t('qs.h2')}</p>
        <p>{rich('qs.p2')}</p>
        <p className={H}>{t('qs.h3')}</p>
        <p>{rich('qs.p3')}</p>
      </Section>

      <Section id="coordinates" title={t('coord.title')}>
        <p>{rich('coord.p')}</p>
      </Section>

      <Section id="rules" title={t('rules.title')}>
        <div className="overflow-hidden rounded-[10px] border border-border">
          <table className="w-full text-left text-sm">
            <tbody>
              {Array.from({ length: RULE_COUNT }, (_, i) => (
                <tr key={i} className="border-b border-border last:border-0 odd:bg-muted/40">
                  <td className="px-3 py-2 font-medium text-foreground">{t(`rule.${i}.l` as 'rule.0.l')}</td>
                  <td className="px-3 py-2 font-mono text-xs text-muted-foreground">{t(`rule.${i}.v` as 'rule.0.v')}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p>{rich('rules.p1')}</p>
        <p>{t('rules.p2')}</p>
      </Section>

      <Section id="protocol" title={t('proto.title')}>
        <p>{t('proto.p0')}</p>
        <p className={H}>{t('proto.hStart')}</p>
        <Code>{`→ {"type":"start","you":2,"map":"crossroads",
   "rules":{"width":60,"height":40,"tick_rate":10,"ticks":1200, "...": "..."},
   "walls":[{"x":28,"y":26,"w":4,"h":12}],
   "players":[{"id":0,"name":"house:hunter"},{"id":1,"name":"house:sniper"},{"id":2,"name":"my-tank"}]}
← {"type":"ready"}`}</Code>
        <p>{rich('proto.pStart')}</p>
        <p className={H}>{t('proto.hTick')}</p>
        <Code>{`→ {"type":"tick","tick":17,
   "tanks":[{"id":0,"x":12.3,"y":8.1,"hull":0.52,"turret":0.9,"hp":75,"reload":3,"alive":true}],
   "shells":[{"id":41,"owner":1,"x":20.0,"y":14.2,"vx":-24.0,"vy":0.0}],
   "bonuses":[{"x":30.0,"y":24.0}],
   "zone":{"x":30,"y":20,"r":37}}
← {"tick":17,"move":1,"turn":0,"turret":-0.5,"fire":true}`}</Code>
        <p>{rich('proto.pTick')}</p>
        <p className={H}>{t('proto.hEnd')}</p>
        <Code>{`→ {"type":"end","place":2,
   "players":[{"slot":0,"place":1,"kills":2,"damage":150,"death_tick":null,"status":"ok"}]}`}</Code>
        <p>{t('proto.pEnd')}</p>
      </Section>

      <Section id="timing" title={t('timing.title')}>
        <ul className={UL}>
          <li>{rich('timing.1')}</li>
          <li>{rich('timing.2')}</li>
          <li>{rich('timing.3')}</li>
          <li>{rich('timing.4')}</li>
        </ul>
      </Section>

      <Section id="logs" title={t('logs.title')}>
        <p>{rich('logs.p1')}</p>
        <p>{t('logs.p2')}</p>
      </Section>

      <Section id="statuses" title={t('status.title')}>
        <div className="overflow-hidden rounded-[10px] border border-border">
          <table className="w-full text-left text-sm">
            <tbody>
              {STATUS_IDS.map((status) => (
                <tr key={status} className="border-b border-border last:border-0 odd:bg-muted/40">
                  <td className="px-3 py-2 font-mono text-xs font-medium text-foreground">{status}</td>
                  <td className="px-3 py-2 text-muted-foreground">{t(`status.${status}`)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p>{t('status.note')}</p>
      </Section>

      <Section id="package" title={t('pkg.title')}>
        <p>{rich('pkg.lead')}</p>
        <Code>{`{"name": "my-tank", "language": "python", "entry": "bot.py"}`}</Code>
        <p>{rich('pkg.p1')}</p>
        <ul className={UL}>
          <li>{rich('pkg.1')}</li>
          <li>{rich('pkg.2')}</li>
          <li>{rich('pkg.3')}</li>
          <li>{rich('pkg.4')}</li>
        </ul>
      </Section>

      <Section id="qualifying" title={t('qual.title')}>
        <p>{t('qual.lead')}</p>
        <ul className={UL}>
          <li>{rich('qual.1')}</li>
          <li>{rich('qual.2')}</li>
          <li>{rich('qual.3')}</li>
          <li>{rich('qual.4')}</li>
        </ul>
        <p>{t('qual.outro')}</p>
      </Section>

      <Section id="rating" title={t('rating.title')}>
        <p>{t('rating.p1')}</p>
        <p>{t('rating.p2')}</p>
        <Code>{'displayed_rating = round(1000 + 40 × (μ − 3σ))'}</Code>
        <p>{t('rating.p3')}</p>
      </Section>

      <Section id="local" title={t('local.title')}>
        <p>{rich('local.p1', undefined, { cli: CLI })}</p>
        <InstallCli />
        <p>{t('local.then')}</p>
        <CopyBlock text={`${CLI} tanks new mybot --lang python     # scaffold a starter bot\n${CLI} tanks play mybot house:hunter house:sniper --seed 1\n${CLI} tanks play mybot house:hunter house:sniper --seed 2`} />
        <p>{rich('local.p2', '/tanks/replay')}</p>
        <p>{rich('local.p3')}</p>
      </Section>
    </div>
  )
}
