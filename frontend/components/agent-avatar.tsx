import Link from "next/link"
import { cn } from "@/lib/utils"
import { agentAvatarClass } from "@/lib/data"

const sizeClasses = {
  xs: "size-5 text-[10px]",
  sm: "size-6 text-[11px]",
  md: "size-8 text-xs",
  lg: "size-12 text-base",
}

export function AgentAvatar({
  agent,
  size = "sm",
  className,
}: {
  agent: string
  size?: keyof typeof sizeClasses
  className?: string
}) {
  return (
    <span
      className={cn(
        "flex shrink-0 items-center justify-center rounded-full font-semibold",
        sizeClasses[size],
        agentAvatarClass(agent),
        className,
      )}
    >
      {agent.charAt(0)}
    </span>
  )
}

export function AgentLink({
  agent,
  author,
  size = "sm",
  className,
  showAuthor = false,
}: {
  agent: string
  author?: string
  size?: keyof typeof sizeClasses
  className?: string
  showAuthor?: boolean
}) {
  return (
    <Link
      href={`/agents/${encodeURIComponent(agent)}`}
      className={cn("group inline-flex items-center gap-2 hover:underline", className)}
    >
      <AgentAvatar agent={agent} size={size} />
      <span className="flex flex-col leading-tight">
        <span className="text-sm font-medium group-hover:underline">{agent}</span>
        {showAuthor && author && (
          <span className="text-xs text-muted-foreground">@{author}</span>
        )}
      </span>
    </Link>
  )
}
