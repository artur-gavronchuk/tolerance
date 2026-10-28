import { useMutation, useQuery } from '@tanstack/react-query';
import { type FormEvent, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useTRPC } from '../lib/trpc';
import { userError } from '../workspace/shared';

const mac = () => window.studio.platform === 'darwin';

/** ⌘⇧B on macOS, Ctrl+Shift+B elsewhere; by key position, so it works in the Russian layout too. */
function isPainHotkey(e: KeyboardEvent): boolean {
  return e.code === 'KeyB' && e.shiftKey && !e.altKey && (mac() ? e.metaKey && !e.ctrlKey : e.ctrlKey);
}

export const painHotkeyLabel = () => (mac() ? '⌘⇧B' : 'Ctrl+Shift+B');

export interface PainContext {
  projectId: string | null;
  projectName: string | null;
  workspaceId: string | null;
  workspaceName: string | null;
}

/**
 * D5: "what got in the way" in one line, from anywhere — the button in the sidebar or the hotkey, even
 * while typing in a terminal. The line goes to `~/.studio/pains.md` with the time, project and
 * workspace; focus goes back to where it was, so work continues without a click.
 */
export function usePain(context: PainContext) {
  const [open, setOpen] = useState(false);
  const returnTo = useRef<HTMLElement | null>(null);

  const show = () => {
    returnTo.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    setOpen(true);
  };
  const close = () => {
    // Before the box goes away, in the same tick: keys typed right after Enter land where they were going.
    if (returnTo.current?.isConnected) returnTo.current.focus();
    else (document.activeElement as HTMLElement | null)?.blur();
    setOpen(false);
  };

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!isPainHotkey(e)) return;
      // Capture phase: before xterm, which would otherwise send the key to the terminal.
      e.preventDefault();
      e.stopPropagation();
      if (!open) show();
    };
    window.addEventListener('keydown', onKey, true);
    return () => window.removeEventListener('keydown', onKey, true);
  });

  return { open, show, close, context };
}

export function PainButton({ onClick }: { onClick: () => void }) {
  const { t } = useTranslation();
  return (
    <button
      type="button"
      onClick={onClick}
      title={t('pain.buttonHint', { hotkey: painHotkeyLabel() })}
      className="flex w-full items-center gap-2 rounded-md border border-line px-2 py-1 text-[12px] text-muted hover:border-modify/60 hover:text-ink"
    >
      <span className="h-1.5 w-1.5 rounded-full bg-modify" aria-hidden="true" />
      <span className="flex-1 text-left">{t('pain.button')}</span>
      <kbd className="font-sans text-[11px] text-faint">{painHotkeyLabel()}</kbd>
    </button>
  );
}

/** The one-line box over the window, and a short "saved" note after it closes. */
export function PainDialog(props: { open: boolean; onClose: () => void; context: PainContext }) {
  const trpc = useTRPC();
  const { t } = useTranslation();
  const info = useQuery(trpc.system.info.queryOptions());
  // Kept while the box is closed with Esc: a half-written pain is not lost.
  const [text, setText] = useState('');
  const [saved, setSaved] = useState<string | null>(null);
  const add = useMutation(trpc.pains.add.mutationOptions());
  const openFile = useMutation(trpc.pains.open.mutationOptions());

  useEffect(() => {
    if (!saved) return;
    const id = setTimeout(() => setSaved(null), 5000);
    return () => clearTimeout(id);
  }, [saved]);

  const { context } = props;
  const where = [context.projectName, context.workspaceName].filter(Boolean).join(' · ') || t('pain.nowhere');

  function submit(e: FormEvent) {
    e.preventDefault();
    if (!text.trim() || add.isPending) return;
    add.mutate(
      { text, projectId: context.projectId, workspaceId: context.workspaceId },
      {
        onSuccess: ({ path }) => {
          setText('');
          setSaved(path);
          props.onClose();
        },
      },
    );
  }

  return (
    <>
      {props.open && (
        // biome-ignore lint/a11y/noStaticElementInteractions: a click on the backdrop closes the box; Esc does the same from the keyboard
        <div
          className="fixed inset-0 z-50 flex items-start justify-center bg-black/20 pt-[14vh]"
          onMouseDown={(e) => {
            if (e.target === e.currentTarget) props.onClose();
          }}
        >
          <form
            role="dialog"
            aria-label={t('pain.title')}
            onSubmit={submit}
            onKeyDown={(e) => {
              if (e.key === 'Escape') {
                e.preventDefault();
                props.onClose();
              }
            }}
            className="w-[min(560px,calc(100vw-32px))] rounded-lg border border-line bg-raised shadow-xl"
          >
            <div className="flex items-baseline justify-between gap-3 px-3 pt-2.5">
              <span className="text-[12.5px] font-medium">{t('pain.title')}</span>
              <span className="truncate text-[11.5px] text-faint" data-testid="pain-where">
                {where}
              </span>
            </div>
            <label htmlFor="pain-text" className="sr-only">
              {t('pain.title')}
            </label>
            <input
              id="pain-text"
              // biome-ignore lint/a11y/noAutofocus: the box exists only to type one line into it
              autoFocus
              value={text}
              onChange={(e) => setText(e.target.value)}
              maxLength={2000}
              placeholder={t('pain.placeholder')}
              className="block w-full bg-transparent px-3 py-2 text-[13.5px] placeholder:text-faint"
              style={{ outline: 'none' }}
            />
            {add.error && <p className="px-3 pb-1 text-[12px] text-remove">{userError(t, add.error)}</p>}
            <div className="flex items-center gap-3 border-t border-line px-3 py-1.5 text-[11px] text-faint">
              <span className="flex-1">{t('pain.hint')}</span>
              <span title={info.data?.painsPath}>pains.md</span>
            </div>
          </form>
        </div>
      )}
      {saved && (
        <output
          aria-live="polite"
          className="fixed right-4 bottom-8 z-40 flex items-center gap-3 rounded-lg border border-line bg-raised px-3 py-2 text-[12.5px] shadow-lg"
        >
          <span title={saved}>{t('pain.saved')}</span>
          <button
            type="button"
            onClick={() => openFile.mutate()}
            className="text-accent-fg underline-offset-2 hover:underline"
          >
            {t('pain.openFile')}
          </button>
        </output>
      )}
    </>
  );
}
