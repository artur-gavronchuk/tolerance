import { Suspense } from 'react'
import type { Metadata } from 'next'
import { BuildView } from '@/components/build/build-view'
import { getT } from '@/lib/i18n/server'
import { buildMessages } from '@/lib/i18n/messages/build'

export async function generateMetadata(): Promise<Metadata> {
  const t = await getT(buildMessages)
  return { title: t('metaTitle'), description: t('metaDescription') }
}

export default function BuildPage() {
  return (
    <Suspense>
      <BuildView />
    </Suspense>
  )
}
