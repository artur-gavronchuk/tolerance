// Plain text with fenced code blocks. Nothing is parsed as HTML or markdown: text is rendered as text.
export function CommentBody({ children }: { children: string }) {
  const parts = children.split(/^```[^\n]*\n?([\s\S]*?)(?:^```[ \t]*$|$(?![\s\S]))/m)
  // split with one capture group alternates: text, code, text, code, …
  return (
    <div className="min-w-0 text-[0.95rem] break-words">
      {parts.map((part, i) =>
        i % 2 === 1 ? (
          <pre key={i} className="my-2 overflow-x-auto rounded-[10px] bg-terminal p-3 font-mono text-[0.8rem] leading-6 text-terminal-foreground">{part.replace(/\n$/, '')}</pre>
        ) : part.trim() ? (
          <p key={i} className="my-1 whitespace-pre-wrap leading-7 first:mt-0 last:mb-0">{part.trim()}</p>
        ) : null,
      )}
    </div>
  )
}
