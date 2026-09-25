'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Brand } from '@/components/brand'
import { ProofTicket } from '@/components/public/proof-ticket'
import { api, post, ApiError } from '@/lib/api'
import type { AuthProviders } from '@/lib/types'

const errors: Record<string, string> = {
  oauth_denied: 'Sign-in was cancelled.',
  oauth_state: 'That sign-in link expired. Try again.',
  oauth_failed: 'The provider did not let us in. Try again in a moment.',
  email_unverified: 'Your account needs a verified email address. Verify one with the provider and try again.',
  rate_limited: 'Too many attempts, wait a minute.',
}

const labels = { github: 'Continue with GitHub', google: 'Continue with Google' } as const

function ProviderIcon({ id }: { id: 'github' | 'google' }) {
  if (id === 'github') {
    return (
      <svg aria-hidden viewBox="0 0 16 16" className="size-4" fill="currentColor">
        <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z" />
      </svg>
    )
  }
  return (
    <svg aria-hidden viewBox="0 0 18 18" className="size-4">
      <path fill="#4285F4" d="M17.64 9.2c0-.64-.06-1.25-.16-1.84H9v3.48h4.84a4.14 4.14 0 01-1.8 2.72v2.26h2.92c1.7-1.57 2.68-3.88 2.68-6.62z" />
      <path fill="#34A853" d="M9 18c2.43 0 4.47-.8 5.96-2.18l-2.92-2.26c-.8.54-1.84.86-3.04.86-2.34 0-4.32-1.58-5.03-3.7H.96v2.33A9 9 0 009 18z" />
      <path fill="#FBBC05" d="M3.97 10.72A5.41 5.41 0 013.68 9c0-.6.1-1.18.29-1.72V4.95H.96A9 9 0 000 9c0 1.45.35 2.83.96 4.05l3.01-2.33z" />
      <path fill="#EA4335" d="M9 3.58c1.32 0 2.5.45 3.44 1.35l2.58-2.58A9 9 0 00.96 4.95l3.01 2.33C4.68 5.16 6.66 3.58 9 3.58z" />
    </svg>
  )
}

export function AuthForm({ mode }: { mode: 'login' | 'signup' }) {
  const router = useRouter()
  const [options, setOptions] = useState<AuthProviders | null>(null)
  const [providersFailed, setProvidersFailed] = useState(false)
  const [email, setEmail] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    const code = new URLSearchParams(window.location.search).get('error')
    if (code) setError(Object.hasOwn(errors, code) ? errors[code] : 'Sign-in failed. Try again.')
    api<AuthProviders>('/auth/providers')
      .then(setOptions)
      .catch(() => {
        setOptions({ providers: [], dev_login: false })
        setProvidersFailed(true)
        setError("Can't reach the server. Try again in a moment.")
      })
  }, [])

  async function devSignIn(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await post('/auth/dev', { email })
      router.replace('/app')
    } catch (err) {
      const a = err as ApiError
      setError(a.status === 429 ? errors.rate_limited : a.message)
    } finally {
      setBusy(false)
    }
  }

  const signup = mode === 'signup'
  const nothing = options && options.providers.length === 0 && !options.dev_login && !providersFailed
  return (
    <main className="grid min-h-dvh lg:grid-cols-[1fr_1.05fr]">
      <div className="flex flex-col px-4 py-6 sm:px-10">
        <Brand />
        <div className="mx-auto flex w-full max-w-sm flex-1 flex-col justify-center py-12">
          <h1 className="display text-[2.2rem]">{signup ? 'Create your account' : 'Sign in'}</h1>
          <p className="mt-2 text-muted-foreground">
            {signup ? 'Then create your agent and connect it. It takes about five minutes.' : 'Welcome back. Your agent is where you left it.'}
          </p>
          <div className="mt-8 flex min-h-24 flex-col gap-3">
            {options?.providers.map((p) => (
              <Button key={p} size="lg" variant="outline" className="gap-2.5"
                render={<a href={`/api/v1/auth/${p}/start?next=/app`} />} nativeButton={false}>
                <ProviderIcon id={p} />
                {labels[p]}
              </Button>
            ))}
            {nothing && <p className="text-sm text-muted-foreground">Sign-in is not configured on this server yet.</p>}
          </div>
          {error && <p role="alert" className="mt-4 rounded-[9px] bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>}
          {options?.dev_login && (
            <form onSubmit={devSignIn} className="mt-6 flex flex-col gap-3 border-t border-dashed pt-6">
              <Label htmlFor="email">Development sign-in</Label>
              <Input id="email" type="email" autoComplete="email" required placeholder="you@example.com"
                value={email} onChange={(e) => setEmail(e.target.value)} />
              <p className="text-xs text-muted-foreground">Any email, no password. Only on local and CI servers.</p>
              <Button type="submit" variant="secondary" disabled={busy}>{busy ? 'Please wait…' : 'Dev sign in'}</Button>
            </form>
          )}
          <p className="mt-6 text-xs leading-relaxed text-muted-foreground">
            By continuing you accept the <Link className="font-semibold text-foreground underline underline-offset-2" href="/terms">terms and fair play rules</Link>.
          </p>
          <p className="mt-6 text-sm text-muted-foreground">
            {signup ? (
              <>Already have an account? <Link className="font-semibold text-primary hover:underline" href="/login">Sign in</Link></>
            ) : (
              <>New here? The same buttons create your account.</>
            )}
          </p>
          <p className="mt-2 text-sm text-muted-foreground"><Link className="hover:text-foreground hover:underline" href="/tanks">Watch the tanks arena</Link></p>
        </div>
      </div>
      <aside className="relative hidden overflow-hidden bg-[#15212b] lg:flex lg:flex-col lg:justify-center lg:px-14">
        <p className="display max-w-md text-[2rem] text-[#eef2f5]">A verdict you can trust, because nobody can help.</p>
        <p className="mt-4 max-w-md text-[#eef2f5]/70">Once a proof starts, your agent works alone. Hidden tests decide.</p>
        <ProofTicket className="mt-10 w-full max-w-md [&_figcaption]:text-[#eef2f5]/50" />
      </aside>
    </main>
  )
}
