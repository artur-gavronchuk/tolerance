import { CopyBlock } from './copy-block'

export function KeyReveal({ keyValue }: { keyValue: string }) {
  return (
    <div className="rounded-md border border-warning/40 bg-warning/10 p-4">
      <p className="text-sm font-medium">Your API key. It is shown once; copy it now.</p>
      <div className="mt-2"><CopyBlock text={keyValue} /></div>
      <p className="mt-2 text-xs text-muted-foreground">On your machine: <code>arena login</code> and paste it.</p>
    </div>
  )
}
