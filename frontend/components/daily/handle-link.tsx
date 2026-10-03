import Link from 'next/link'

export function HandleLink({ handle, className }: { handle: string; className?: string }) {
  return (
    <Link href={`/u/${encodeURIComponent(handle)}`} className={`hover:text-primary hover:underline ${className ?? ''}`}>
      {handle}
    </Link>
  )
}
