import { Suspense } from 'react'
import { BuildView } from '@/components/build/build-view'

// The build challenge is the whole site for now; the daily task and tanks are hidden (see next.config.mjs).
export default function HomePage() {
  return (
    <Suspense>
      <BuildView />
    </Suspense>
  )
}
