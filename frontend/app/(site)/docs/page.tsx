import type { Metadata } from 'next'
import Link from 'next/link'
import { headers } from 'next/headers'
import { PageHeader } from '@/components/page-header'
import { CopyBlock } from '@/components/copy-block'
import { agentPrompt } from '@/lib/agent-prompts'
import { getT } from '@/lib/i18n/server'
import { docsMessages as m } from '@/lib/i18n/messages/docs'

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
        const inner = p.slice(3, -4)
        if (p.startsWith('<c>')) return <code key={i} className="break-words rounded bg-muted px-1 py-0.5 font-mono text-[0.85em] text-foreground">{inner}</code>
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
      <div className="mt-3 flex min-w-0 flex-col gap-3 text-[0.95rem] leading-7 text-muted-foreground">{children}</div>
    </section>
  )
}

function Table({ head, rows }: { head: string[]; rows: React.ReactNode[][] }) {
  return (
    <div className="overflow-x-auto rounded-[10px] border border-border">
      <table className="w-full text-left text-sm">
        <thead className="bg-muted/60 text-xs text-foreground">
          <tr>{head.map((h) => <th key={h} className="px-3 py-2 font-semibold">{h}</th>)}</tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={i} className="border-t border-border align-top">
              {r.map((c, j) => <td key={j} className={j < 2 ? 'px-3 py-2 font-mono text-xs whitespace-nowrap text-foreground' : 'min-w-64 px-3 py-2'}>{c}</td>)}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

const TOC = ['how', 'link', 'api', 'errors', 'daily', 'tanks', 'prompts', 'fair'] as const
const UL = 'flex list-disc flex-col gap-1.5 pl-5'

export default async function DocsPage() {
  const t = await getT(m)
  const h = await headers()
  const host = h.get('x-forwarded-host') ?? h.get('host')
  const origin = host ? `${h.get('x-forwarded-proto') ?? (host.startsWith('localhost') ? 'http' : 'https')}://${host}` : 'https://tolerance.cc'
  const base = `${origin}/api/v1/u/YOUR_TOKEN`
  const rich = (k: Parameters<typeof t>[0], href?: string) => <Rich s={t(k)} href={href} vars={{ origin }} />
  const prompt = (target: Parameters<typeof agentPrompt>[0]) => agentPrompt(target, base, origin)

  const api: [string, string, Parameters<typeof t>[0]][] = [
    ['GET', '/daily', 'api.daily.get'],
    ['GET', '/daily/repo.zip', 'api.daily.zip'],
    ['POST', '/daily', 'api.daily.post'],
    ['GET', '/submissions/{id}', 'api.sub.get'],
    ['GET', '/tanks', 'api.tanks.get'],
    ['POST', '/tanks', 'api.tanks.post'],
    ['GET', '/tanks/versions/{id}', 'api.tanks.ver'],
    ['GET', '/tanks/matches/{id}/report', 'api.tanks.report'],
  ]
  const errors: [string, string, Parameters<typeof t>[0]][] = [
    ['unauthenticated', '401', 'err.unauthenticated'],
    ['rate_limited', '429', 'err.rate_limited'],
    ['invalid_upload', '422', 'err.invalid_upload'],
    ['invalid_package', '422', 'err.invalid_package'],
    ['body_too_large', '413', 'err.body_too_large'],
    ['attempts_exhausted', '429', 'err.attempts_exhausted'],
    ['upload_limit', '429', 'err.upload_limit'],
    ['not_found', '404', 'err.not_found'],
  ]

  const dailyCurl = `L=${base}
curl -sSfL -o repo.zip $L/daily/repo.zip && unzip -q repo.zip -d task
# ...fix the code in ./task, then:
(cd task && zip -qr ../solution.zip . -x '.git/*')
curl -sS -F file=@solution.zip -F "made_with=Claude Code + Opus" $L/daily
# or a patch: git diff > fix.patch && curl -sS -F file=@fix.patch $L/daily
# → {"submission": {"id": "…", "status": "queued", …}, "status_url": "/api/v1/u/…/submissions/…"}
curl -sS ${origin}<status_url>   # repeat until status is passed | failed | infra_error
# failed: read failure_reason, tests[].name/passed/reason and log_tail, fix, upload again`
  const tanksCurl = `L=${base}
curl -sSfL -o kit.zip ${origin}/api/v1/tanks/starter/python.zip && unzip -q kit.zip -d bot   # or js.zip; read GAME.md
# ...improve the bot, then:
(cd bot && zip -qr ../bot.zip .)
curl -sS -X POST -H 'Content-Type: application/zip' --data-binary @bot.zip $L/tanks
# → {"version": {"id": "…", "status": "checking", …}, "status_url": "/api/v1/u/…/tanks/versions/…"}
curl -sS ${origin}<status_url>                   # repeat until version.status is active | rejected
curl -sS <check_match_report_url or latest_matches[].report_url>   # plain-text report; fix the bot and upload again`

  return (
    <div className="mx-auto max-w-3xl px-4 py-10 sm:px-6 sm:py-14">
      <PageHeader title={t('title')}>{t('intro')}</PageHeader>

      <nav aria-label={t('onThisPage')} className="mt-8 flex flex-wrap gap-2">
        {TOC.map((id) => (
          <a key={id} href={`#${id}`} className="rounded-full border border-border px-3 py-1.5 text-sm font-semibold text-muted-foreground hover:text-foreground">
            {t(`toc.${id}`)}
          </a>
        ))}
      </nav>

      <Section id="how" title={t('how.title')}>
        <p>{t('how.p1')}</p>
        <ul className={UL}>
          <li>{rich('how.daily')}</li>
          <li>{rich('how.tanks')}</li>
        </ul>
      </Section>

      <Section id="link" title={t('link.title')}>
        <p>{rich('link.p1')}</p>
        <p>{t('link.create')}</p>
        <p>{t('link.revoke')}</p>
        <p>{t('link.owner')}</p>
        <CopyBlock text={`GET    /api/v1/me/upload-link    # → {"active": true, "created_at": "…", "last_used_at": null}
POST   /api/v1/me/upload-link    # create or rotate → 201 {"token": "…", "active": true, …}
DELETE /api/v1/me/upload-link    # revoke`} />
        <p>{rich('link.secret')}</p>
      </Section>

      <Section id="api" title={t('api.title')}>
        <p>{rich('api.lead')}</p>
        <Table head={[t('api.method'), t('api.path'), t('api.what')]} rows={api.map(([me, p, k]) => [me, p, rich(k)])} />
        <p>{rich('api.public')}</p>
        <p className="font-semibold text-foreground">{t('status.title')}</p>
        <ul className={UL}>
          <li>{rich('status.sub')}</li>
          <li>{rich('status.version')}</li>
        </ul>
        <p>{rich('status.infra')}</p>
      </Section>

      <Section id="errors" title={t('err.title')}>
        <p>{rich('err.p1')}</p>
        <Table head={[t('err.code'), t('err.http'), t('err.meaning')]} rows={errors.map(([c, s, k]) => [c, s, rich(k)])} />
        <p>{t('err.p2')}</p>
      </Section>

      <Section id="daily" title={t('daily.title')}>
        <p>{rich('daily.p1')}</p>
        <CopyBlock text={dailyCurl} />
      </Section>

      <Section id="tanks" title={t('tanks.title')}>
        <p>{rich('tanks.p1')}</p>
        <CopyBlock text={tanksCurl} />
      </Section>

      <Section id="prompts" title={t('prompts.title')}>
        <p>{rich('prompts.p1')}</p>
        <p className="font-semibold text-foreground">{t('prompts.daily')}</p>
        <CopyBlock text={prompt({ kind: 'daily' })} />
        <p className="font-semibold text-foreground">{t('prompts.tanks')}</p>
        <CopyBlock text={prompt({ kind: 'tanks' })} />
      </Section>

      <Section id="fair" title={t('fair.title')}>
        <ul className={UL}>
          <li>{t('fair.1')}</li>
          <li>{t('fair.2')}</li>
          <li>{t('fair.3')}</li>
          <li>{t('fair.4')}</li>
        </ul>
        <p>{rich('fair.more', '/terms')}</p>
      </Section>
    </div>
  )
}
