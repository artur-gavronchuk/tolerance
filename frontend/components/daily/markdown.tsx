import ReactMarkdown, { type Components } from 'react-markdown'
import remarkGfm from 'remark-gfm'

const components: Components = {
  h1: (p) => <h2 className="heading mt-6 mb-2 text-xl first:mt-0" {...p} />,
  h2: (p) => <h3 className="heading mt-6 mb-2 text-lg first:mt-0" {...p} />,
  h3: (p) => <h4 className="mt-5 mb-1.5 font-bold first:mt-0" {...p} />,
  h4: (p) => <h5 className="mt-4 mb-1 font-bold first:mt-0" {...p} />,
  p: (p) => <p className="my-3 leading-7 first:mt-0 last:mb-0" {...p} />,
  ul: (p) => <ul className="my-3 list-disc space-y-1 pl-6 leading-7" {...p} />,
  ol: (p) => <ol className="my-3 list-decimal space-y-1 pl-6 leading-7" {...p} />,
  a: (p) => <a className="font-semibold text-primary hover:underline" target="_blank" rel="noreferrer noopener" {...p} />,
  blockquote: (p) => <blockquote className="my-3 border-l-2 border-border pl-4 text-muted-foreground" {...p} />,
  hr: () => <hr className="my-6 border-border" />,
  pre: (p) => <pre className="my-3 overflow-x-auto rounded-lg bg-terminal p-4 font-mono text-code text-terminal-foreground [&_code]:bg-transparent [&_code]:p-0 [&_code]:text-inherit" {...p} />,
  code: (p) => <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-[0.85em]" {...p} />,
  table: (p) => <div className="my-3 overflow-x-auto"><table className="w-full border-collapse text-sm" {...p} /></div>,
  th: (p) => <th className="border border-border bg-muted px-3 py-1.5 text-left font-bold" {...p} />,
  td: (p) => <td className="border border-border px-3 py-1.5" {...p} />,
}

// Renders TASK.md. Raw HTML in the source is not rendered.
export function Markdown({ children }: { children: string }) {
  return (
    <div className="min-w-0 text-lede break-words">
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>{children}</ReactMarkdown>
    </div>
  )
}
