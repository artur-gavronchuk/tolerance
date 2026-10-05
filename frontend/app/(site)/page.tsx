import { BuildBanner } from '@/components/build/build-banner'
import { DailyView } from '@/components/daily/daily-view'
import { Intro } from '@/components/public/intro'
import { TodayForYou } from '@/components/public/today-for-you'

export default function TodayPage() {
  return (
    <>
      <BuildBanner />
      <Intro />
      <TodayForYou />
      <div id="today">
        <DailyView />
      </div>
    </>
  )
}
