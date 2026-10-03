import type { Metadata } from 'next'
import { Brand } from '@/components/brand'
import { PRODUCT } from '@/lib/brand'
import { getT } from '@/lib/i18n/server'
import { termsMessages } from '@/lib/i18n/messages/terms'

export async function generateMetadata(): Promise<Metadata> {
  const t = await getT(termsMessages)
  return { title: t('title'), description: t('description') }
}

// Baked in at build time, like API_URL: set ARENA_CONTACT_EMAIL in .env.
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

export default async function TermsPage() {
  const t = await getT(termsMessages)
  return (
    <main className="mx-auto max-w-2xl px-4 py-6 sm:py-8">
      <Brand />
      <h1 className="display mt-12 text-[2.3rem] sm:text-[2.75rem]">{t('title')}</h1>
      <p className="mt-2 text-sm text-muted-foreground">{t('lastUpdated', { date: t('updated') })}</p>
      <p className="mt-5 text-[0.95rem] leading-7 text-muted-foreground">{t('intro', { product: PRODUCT })}</p>

      <Section title={t('runsTitle')}>
        <p>{t('runs1')}</p>
        <p>{t('runs2')}</p>
      </Section>

      <Section title={t('provesTitle')}>
        <p>{t('proves1')}</p>
      </Section>

      <Section title={t('fairTitle')}>
        <p>{t('fairLead')}</p>
        <List items={[t('fair0'), t('fair1'), t('fair2'), t('fair3'), t('fair4')]} />
        <p>{t('fairAfter')}</p>
      </Section>

      <Section title={t('fairFlagsTitle')}>
        <p>{t('fairFlags1')}</p>
        <p>{t('fairFlags2')}</p>
      </Section>

      <Section title={t('publicTitle')}>
        <p>{t('public1')}</p>
        <p>{t('public2')}</p>
      </Section>

      <Section title={t('keepTitle')}>
        <List items={[t('keep1'), t('keep2'), t('keep3')]} />
        <p>{t('keepAfter')}</p>
      </Section>

      <Section title={t('deleteTitle')}>
        <p>{t('deletePre')}<Contact fallback={t('operator')} />{t('deletePost')}</p>
      </Section>

      <Section title={t('warrantyTitle')}>
        <p>{t('warranty1')}</p>
      </Section>
    </main>
  )
}
