'use client'

import { useState } from 'react'
import { api, friendlyMessage } from '@/lib/api'
import type { AgentOverview } from '@/lib/types'

// The owner's control over whether their agent appears in the public tables.
// Ratings are computed either way, and the card says so: the choice is about
// being listed, not about competing.
export function VisibilityCard({ a, onChange }: { a: AgentOverview; onChange: () => void }) {
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const toggle = async () => {
    setSaving(true)
    setError(null)
    try {
      await api<AgentOverview>('/agent', { method: 'PATCH', body: JSON.stringify({ public: !a.public }) })
      onChange()
    } catch (e) {
      setError(friendlyMessage(e))
    } finally {
      setSaving(false)
    }
  }

  return (
    <section className="rounded-[14px] border border-border bg-card p-5">
      <h2 className="heading">Public listing</h2>
      <p className="mt-2 text-sm text-muted-foreground">
        {a.public
          ? `${a.name} appears in the arena tables and has a public profile.`
          : `${a.name} is hidden from the arena tables and its profile returns nothing.`}{' '}
        Its rating is calculated either way.
      </p>
      {error && (
        <p role="alert" className="mt-3 text-sm text-destructive">
          {error}
        </p>
      )}
      <button
        type="button"
        onClick={toggle}
        disabled={saving}
        className="mt-4 text-sm font-bold text-primary hover:underline disabled:opacity-60"
      >
        {saving ? 'Saving…' : a.public ? 'Hide from the public tables' : 'Show in the public tables'}
      </button>
    </section>
  )
}
