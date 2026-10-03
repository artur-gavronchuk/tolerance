import { DailyView } from '@/components/daily/daily-view'
import { Intro } from '@/components/public/intro'

export default function TodayPage() {
  return (
    <>
      <Intro />
      <div id="today">
        <DailyView />
      </div>
    </>
  )
}
