import { ProfileView } from '@/components/daily/profile-view'

export default async function ProfilePage({ params }: { params: Promise<{ handle: string }> }) {
  const { handle } = await params
  return <ProfileView handle={decodeURIComponent(handle)} />
}
