'use client'

import { useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Brand } from '@/components/brand'
import { ProofTicket } from '@/components/public/proof-ticket'
import { post, ApiError } from '@/lib/api'

export function AuthForm({ mode }: { mode: 'login' | 'signup' }) {
  const router = useRouter()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await post(`/auth/${mode}`, { email, password })
      router.replace('/app')
    } catch (err) {
      const a = err as ApiError
      setError(a.status === 429 ? 'Too many attempts, wait a minute.' : a.message)
    } finally {
      setBusy(false)
    }
  }

  const signup = mode === 'signup'
  return (
    <main className="grid min-h-dvh lg:grid-cols-[1fr_1.05fr]">
      <div className="flex flex-col px-4 py-6 sm:px-10">
        <Brand />
        <div className="mx-auto flex w-full max-w-sm flex-1 flex-col justify-center py-12">
          <h1 className="display text-[2.2rem]">{signup ? 'Create your account' : 'Sign in'}</h1>
          <p className="mt-2 text-muted-foreground">
            {signup ? 'Then create your agent and connect it. It takes about five minutes.' : 'Welcome back. Your agent is where you left it.'}
          </p>
          <form onSubmit={submit} className="mt-8 flex flex-col gap-5">
            <div className="flex flex-col gap-2">
              <Label htmlFor="email">Email</Label>
              <Input id="email" type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="password">Password</Label>
              <Input id="password" type="password" minLength={10} required autoComplete={signup ? 'new-password' : 'current-password'}
                value={password} onChange={(e) => setPassword(e.target.value)} />
              {signup && <p className="text-xs text-muted-foreground">At least 10 characters.</p>}
            </div>
            {error && <p role="alert" className="rounded-[9px] bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>}
            <Button type="submit" size="lg" disabled={busy}>{busy ? 'Please wait…' : signup ? 'Create account' : 'Sign in'}</Button>
          </form>
          <p className="mt-8 text-sm text-muted-foreground">
            {signup ? (
              <>Already have an account? <Link className="font-semibold text-primary hover:underline" href="/login">Sign in</Link></>
            ) : (
              <>No account yet? <Link className="font-semibold text-primary hover:underline" href="/signup">Create one</Link></>
            )}
          </p>
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
