import type { Metadata } from 'next'
import { Brand } from '@/components/brand'
import { PRODUCT } from '@/lib/brand'

export const metadata: Metadata = {
  title: 'Terms and fair play',
  description: 'What runs where, what a result proves, the rules, and what we keep about you.',
}

// Baked in at build time, like API_URL: set ARENA_CONTACT_EMAIL in .env.
const contact = process.env.NEXT_PUBLIC_CONTACT_EMAIL

const UPDATED = '3 October 2026'

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

function Contact() {
  if (!contact) return <>the operator of this site</>
  return <a className="break-all text-foreground underline" href={`mailto:${contact}`}>{contact}</a>
}

export default function TermsPage() {
  return (
    <main className="mx-auto max-w-2xl px-4 py-6 sm:py-8">
      <Brand />
      <h1 className="display mt-12 text-[2.3rem] sm:text-[2.75rem]">Terms and fair play</h1>
      <p className="mt-2 text-sm text-muted-foreground">Last updated {UPDATED}</p>
      <p className="mt-5 text-[0.95rem] leading-7 text-muted-foreground">
        {PRODUCT} publishes one coding task a day and checks your solution against hidden tests. Creating an
        account means you accept what is written here. It is short on purpose.
      </p>

      <Section title="What runs where">
        <p>
          You download the task repository and solve it however you like, with your own tools and your own model
          keys; none of that touches us. You upload the edited repository (a zip) or a patch.
        </p>
        <p>
          We apply it to a clean copy of the repository and run tests you never saw, in a container with no network
          access and hard limits on time, memory and processes.
        </p>
      </Section>

      <Section title="What a result proves, and what it does not">
        <p>
          A passed task means one thing: your upload passed the hidden tests within the time limit. We cannot see how
          you made it, so the &ldquo;made with&rdquo; label is whatever you typed. Read results as a game score, not as
          a certificate.
        </p>
      </Section>

      <Section title="Fair play">
        <p>You agree not to:</p>
        <List items={[
          'tamper with tests, the test runner or the reported results. Uploads that touch test files are refused, and any other way of fooling the check counts the same;',
          'attack the platform, the sandbox, the match servers or other users, or try to escape a sandbox;',
          'publish hidden tests or task solutions while a day is open;',
          'open several accounts to get around attempt limits.',
        ]} />
        <p>
          We may remove results, bots or accounts that break these rules, and recompute leaderboards when a task
          turns out to be broken.
        </p>
      </Section>

      <Section title="What is public">
        <p>
          Public: your handle, your best result per day, the &ldquo;made with&rdquo; text you typed, your streak, and,
          for tank bots, the bot&apos;s name, rating, matches and replays. Pick names you are happy to see on a
          leaderboard.
        </p>
        <p>Private, visible only to you: your email, your uploaded files, logs, and a bot&apos;s code and its debug log.</p>
      </Section>

      <Section title="What we keep">
        <List items={[
          'Account: your email and account id at GitHub or Google (and your GitHub login), and sessions stored as hashes of their tokens. We never see or store a password.',
          'Submissions: the uploaded file, the sandbox log tail and test results. Text that looks like a secret is removed from logs before it is stored, but do not put secrets in your solution.',
          'Daily database backups, each kept for 7 days.',
        ]} />
        <p>We do not sell or share this data.</p>
      </Section>

      <Section title="Deleting your data">
        <p>
          There is no delete button yet. Write to <Contact /> from your account&apos;s email and we will delete the
          account and everything you uploaded within 30 days. Backups that still hold it expire within 7 days
          after that.
        </p>
      </Section>

      <Section title="No warranty">
        <p>
          The service is free and provided as is. It may be down, change, or reset leaderboards when the rules or
          tasks change. When these terms change, the date above changes with them.
        </p>
      </Section>
    </main>
  )
}
