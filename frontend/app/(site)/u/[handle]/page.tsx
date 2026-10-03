import { ProfileView } from '@/components/daily/profile-view'
import { ReportButton } from '@/components/fairplay/report-button'
import { ActivityError, ActivityProvider, MainStack, ProductsSection, TanksSection } from '@/components/profile/activity'

export default async function ProfilePage({ params }: { params: Promise<{ handle: string }> }) {
  const { handle } = await params
  const h = decodeURIComponent(handle)
  return (
    <ActivityProvider handle={h}>
      <MainStack />
      <ProfileView handle={h} />
      <div className="mt-4"><ReportButton handle={h} /></div>
      <div className="mt-10 space-y-10">
        <ActivityError />
        <ProductsSection />
        <TanksSection />
      </div>
    </ActivityProvider>
  )
}
