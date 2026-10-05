import { BuildView } from '@/components/build/build-view'

export default async function ChallengePage({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params
  return <BuildView slug={slug} />
}
