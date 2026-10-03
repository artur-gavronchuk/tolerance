import type { Metadata } from 'next'
import { Brand } from '@/components/brand'
import { PRODUCT } from '@/lib/brand'
import { getT } from '@/lib/i18n/server'
import { privacyMessages } from '@/lib/i18n/messages/privacy'

export async function generateMetadata(): Promise<Metadata> {
  const t = await getT(privacyMessages)
  return { title: t('title'), description: t('description') }
}

// Baked in at build time, like API_URL: set ARENA_CONTACT_EMAIL in .env.
// How long request logs (with IP addresses) are kept; the terms page says the same.
const LOG_DAYS = 14

const contact = process.env.NEXT_PUBLIC_CONTACT_EMAIL

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="mt-8">
      <h2 className="heading text-lg text-foreground">{title}</h2>
      <div className="mt-2 flex flex-col gap-3 text-[0.95rem] leading-7 text-muted-foreground">{children}</div>
    </section>
  )
}

function List({ items }: { items: React.ReactNode[] }) {
  return (
    <ul className="flex list-disc flex-col gap-1.5 pl-5">
      {items.map((item, i) => <li key={i}>{item}</li>)}
    </ul>
  )
}

function Contact({ fallback }: { fallback: string }) {
  if (!contact) return <>{fallback}</>
  return <a className="break-all text-foreground underline" href={`mailto:${contact}`}>{contact}</a>
}

export default async function PrivacyPage() {
  const t = await getT(privacyMessages)
  return (
    <main className="mx-auto max-w-2xl px-4 py-6 sm:py-8">
      <Brand />
      <h1 className="display mt-12 text-[2.3rem] sm:text-[2.75rem]">{t('title')}</h1>
      <p className="mt-2 text-sm text-muted-foreground">{t('lastUpdated', { date: t('updated') })}</p>
      <p className="mt-5 text-[0.95rem] leading-7 text-muted-foreground">{t('intro', { product: PRODUCT })}</p>

      <Section title={t('storeTitle')}>
        <List items={[t('store1'), t('store2'), t('store3'), t('store4', { days: LOG_DAYS }), t('store5')]} />
      </Section>

      <Section title={t('whyTitle')}>
        <p>{t('why1')}</p>
      </Section>

      <Section title={t('noSellTitle')}>
        <p>{t('noSell1')}</p>
      </Section>

      <Section title={t('publicTitle')}>
        <p>{t('public1')}</p>
        <p>{t('public2')}</p>
      </Section>

      <Section title={t('cookiesTitle')}>
        <p>{t('cookies1')}</p>
      </Section>

      <Section title={t('rightsTitle')}>
        <List items={[t('rights1'), t('rights2'), t('rights3')]} />
        <p>{t('rights4')}</p>
      </Section>

      <Section title={t('contactTitle')}>
        <p>{t('contactPre')}<Contact fallback={t('operator')} />{t('contactPost')}</p>
      </Section>
    </main>
  )
}
