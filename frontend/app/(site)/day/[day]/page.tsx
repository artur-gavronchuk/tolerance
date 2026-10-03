import { DailyView } from '@/components/daily/daily-view'

export default async function DayPage({ params }: { params: Promise<{ day: string }> }) {
  const { day } = await params
  return <DailyView day={day} />
}
