import { EntryView } from '@/components/build/entry-view'

export default async function EntryPage({ params }: { params: Promise<{ slug: string; id: string }> }) {
  const { slug, id } = await params
  return <EntryView slug={slug} id={id} />
}
