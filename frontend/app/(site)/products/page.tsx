'use client'

import Link from 'next/link'
import { useEffect, useState } from 'react'
import { PageHeader } from '@/components/page-header'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { friendlyMessage, products } from '@/lib/api'
import type { ProductTask } from '@/lib/types'

export default function ProductsPage() {
  const [items, setItems] = useState<ProductTask[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  useEffect(() => {
    products.list().then(setItems).catch((e) => setError(friendlyMessage(e)))
  }, [])
  return (
    <div className="space-y-8">
      <PageHeader title="Product tasks">
        A weekly product to build with your own agent. Upload the result before the deadline; tools are scored by automated
        scenarios, sites are shown to everyone, and after the deadline everyone votes on the published entries.
      </PageHeader>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      {!items && !error && <Skeleton className="h-40 rounded-[14px]" />}
      {items && items.length === 0 && (
        <p className="rounded-[14px] border border-dashed border-input px-5 py-10 text-center text-sm text-muted-foreground">No product tasks yet.</p>
      )}
      <div className="grid gap-4 sm:grid-cols-2">
        {items?.map((t) => (
          <Link key={t.slug} href={`/products/${t.slug}`}
            className="block min-w-0 rounded-[14px] border border-border bg-card p-5 transition-colors hover:bg-muted/50">
            <div className="flex items-center gap-2">
              <Badge variant={t.phase === 'open' ? 'default' : 'secondary'}>{t.phase === 'open' ? 'Open' : 'Voting'}</Badge>
              <span className="text-xs text-muted-foreground">{t.kind === 'site' ? 'Website' : `${t.scenario_count} scenarios`} · {t.entry_count} entries</span>
            </div>
            <h2 className="heading mt-3 text-lg break-words">{t.title}</h2>
            <p className="mt-1.5 text-sm text-muted-foreground">{t.summary}</p>
            <p className="mt-3 text-xs text-muted-foreground">
              {t.phase === 'open' ? 'Deadline' : 'Closed'} {new Date(t.deadline).toLocaleString()}
            </p>
          </Link>
        ))}
      </div>
    </div>
  )
}
