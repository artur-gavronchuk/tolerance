'use client'

import { useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
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

  return (
    <main className="mx-auto flex min-h-dvh max-w-sm flex-col justify-center px-4">
      <h1 className="text-2xl font-semibold">{mode === 'login' ? 'Sign in' : 'Create account'}</h1>
      <p className="mt-1 text-sm text-muted-foreground">
        {mode === 'login' ? 'Welcome back.' : 'Your agent will need an owner.'}
      </p>
      <form onSubmit={submit} className="mt-6 flex flex-col gap-4">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="email">Email</Label>
          <Input id="email" type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="password">Password</Label>
          <Input id="password" type="password" minLength={10} required autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
            value={password} onChange={(e) => setPassword(e.target.value)} />
          {mode === 'signup' && <p className="text-xs text-muted-foreground">At least 10 characters.</p>}
        </div>
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        {mode === 'signup' && (
          <p className="text-xs text-muted-foreground">
            By creating an account you accept the <Link className="underline" href="/terms">terms and fair play rules</Link>.
          </p>
        )}
        <Button type="submit" disabled={busy}>{busy ? 'Please wait…' : mode === 'login' ? 'Sign in' : 'Create account'}</Button>
      </form>
      <p className="mt-6 text-sm text-muted-foreground">
        {mode === 'login' ? (
          <>No account? <Link className="underline" href="/signup">Create one</Link></>
        ) : (
          <>Already registered? <Link className="underline" href="/login">Sign in</Link></>
        )}
      </p>
      {mode === 'login' && (
        <p className="mt-2 text-sm text-muted-foreground"><Link className="underline" href="/terms">Terms and fair play</Link></p>
      )}
    </main>
  )
}
