/** A unified diff as plain text, lines tinted by kind. Monaco only if this turns out not to be enough. */
export function DiffText({ patch, maxHeight = true }: { patch: string; maxHeight?: boolean }) {
  const lines = patch.split('\n');
  return (
    <pre
      className={`${maxHeight ? 'max-h-[480px] ' : ''}overflow-auto rounded-md border border-line bg-panel py-1.5 font-mono text-[11.5px] leading-[1.45] select-text`}
    >
      {lines.map((line, i) => (
        // biome-ignore lint/suspicious/noArrayIndexKey: static text, never reordered
        <div key={i} className={`px-2 ${lineClass(line)}`}>
          {line || ' '}
        </div>
      ))}
    </pre>
  );
}

function lineClass(line: string): string {
  if (line.startsWith('diff --git')) return 'mt-2 font-semibold text-ink first:mt-0';
  if (line.startsWith('+++') || line.startsWith('---') || line.startsWith('index ')) return 'text-faint';
  if (line.startsWith('@@')) return 'text-accent-fg';
  if (line.startsWith('+')) return 'bg-add/10 text-add';
  if (line.startsWith('-')) return 'bg-remove/10 text-remove';
  return 'text-muted';
}

/** The part of a unified diff for each file, keyed by its path (the "b/" side, else "a/"). */
export function splitPatch(patch: string): Map<string, string> {
  const files = new Map<string, string>();
  for (const chunk of patch.split(/^(?=diff --git )/m)) {
    const header = /^diff --git a\/(.+?) b\/(.+)$/m.exec(chunk);
    if (header) files.set(header[2] ?? header[1] ?? '', chunk.trimEnd());
  }
  return files;
}
