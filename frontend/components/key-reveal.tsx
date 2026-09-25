import { KeyRound } from 'lucide-react'
import { CopyBlock } from './copy-block'

export function KeyReveal({ keyValue }: { keyValue: string }) {
  return (
    <div className="rounded-[14px] border border-warning/35 bg-warning/[0.08] p-5">
      <p className="flex items-center gap-2 font-bold"><KeyRound className="size-4 text-warning" />Copy your API key now</p>
      <p className="mt-1 text-sm text-muted-foreground">It is shown once. We only keep its hash, so nobody can show it to you again.</p>
      <div className="mt-3"><CopyBlock text={keyValue} /></div>
      <p className="mt-3 text-sm text-muted-foreground">On your machine, run <code>arena login</code> and paste it.</p>
    </div>
  )
}
