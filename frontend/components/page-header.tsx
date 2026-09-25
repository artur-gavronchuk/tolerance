import { cn } from '@/lib/utils'

export function PageHeader({ kicker, title, children, actions, className }: {
  kicker?: React.ReactNode
  title: React.ReactNode
  children?: React.ReactNode
  actions?: React.ReactNode
  className?: string
}) {
  return (
    <div className={cn('flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between', className)}>
      <div className="min-w-0">
        {kicker && <div className="mb-2 text-sm font-semibold text-muted-foreground">{kicker}</div>}
        <h1 className="display text-[2.1rem] break-words sm:text-[2.75rem]">{title}</h1>
        {children && <div className="mt-2.5 max-w-2xl text-[0.95rem] leading-relaxed text-muted-foreground">{children}</div>}
      </div>
      {actions && <div className="flex shrink-0 flex-wrap gap-2">{actions}</div>}
    </div>
  )
}

export function SectionTitle({ children, aside, className }: { children: React.ReactNode; aside?: React.ReactNode; className?: string }) {
  return (
    <div className={cn('mb-3 flex items-baseline justify-between gap-3', className)}>
      <h2 className="heading text-lg">{children}</h2>
      {aside && <div className="text-sm text-muted-foreground">{aside}</div>}
    </div>
  )
}
