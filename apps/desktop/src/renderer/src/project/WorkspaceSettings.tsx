import type { Project } from '@studio/schema';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { type FormEvent, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useTRPC } from '../lib/trpc';
import { userError } from '../workspace/shared';
import { Section } from './shared';

const editors = ['code', 'cursor', 'idea'];
const terminals = ['Terminal', 'iTerm', 'Ghostty', 'Warp'];

/** Setup command, editor and terminal for the project's workspaces (daily mode, D1). */
export function WorkspaceSettings({ project }: { project: Project }) {
  const trpc = useTRPC();
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const defaults = useQuery(trpc.projects.openDefaults.queryOptions());
  const [setup, setSetup] = useState(project.setupCommand ?? '');
  const [editor, setEditor] = useState(project.editorCommand ?? '');
  const [terminal, setTerminal] = useState(project.terminalApp ?? '');
  const save = useMutation(
    trpc.projects.updateSettings.mutationOptions({
      onSuccess: () => queryClient.invalidateQueries({ queryKey: trpc.projects.list.queryKey() }),
    }),
  );
  const dirty =
    setup !== (project.setupCommand ?? '') ||
    editor !== (project.editorCommand ?? '') ||
    terminal !== (project.terminalApp ?? '');
  const mac = window.studio.platform === 'darwin';

  function submit(e: FormEvent) {
    e.preventDefault();
    save.mutate({
      projectId: project.id,
      settings: { setupCommand: setup, editorCommand: editor, terminalApp: terminal },
    });
  }

  const field =
    'w-full rounded-md border border-line bg-raised px-2 py-1 font-mono text-[12px] placeholder:text-faint focus:border-accent';

  return (
    <Section title={t('projectSettings.title')} lead={t('projectSettings.lead')}>
      <form onSubmit={submit} className="space-y-3">
        <div>
          <label htmlFor="setup-command" className="text-[12px] font-medium">
            {t('projectSettings.setup')}
          </label>
          <textarea
            id="setup-command"
            rows={2}
            value={setup}
            onChange={(e) => setSetup(e.target.value)}
            placeholder={t('projectSettings.setupPlaceholder')}
            className={`mt-1 block resize-y ${field}`}
            style={{ outline: 'none' }}
          />
          <p className="mt-1 text-[11.5px] text-faint">{t('projectSettings.setupHint')}</p>
        </div>
        <div className="grid grid-cols-2 gap-4">
          <div>
            <label htmlFor="editor-command" className="text-[12px] font-medium">
              {t('projectSettings.editor')}
            </label>
            <input
              id="editor-command"
              list="editor-options"
              value={editor}
              onChange={(e) => setEditor(e.target.value)}
              placeholder={t('projectSettings.byDefault', { value: defaults.data?.editor ?? '' })}
              className={`mt-1 ${field}`}
              style={{ outline: 'none' }}
            />
            <datalist id="editor-options">
              {editors.map((v) => (
                <option key={v} value={v} />
              ))}
            </datalist>
            <p className="mt-1 text-[11.5px] text-faint">{t('projectSettings.editorHint')}</p>
          </div>
          <div>
            <label htmlFor="terminal-app" className="text-[12px] font-medium">
              {t('projectSettings.terminal')}
            </label>
            <input
              id="terminal-app"
              list={mac ? 'terminal-options' : undefined}
              value={terminal}
              onChange={(e) => setTerminal(e.target.value)}
              placeholder={t('projectSettings.byDefault', { value: defaults.data?.terminal ?? '' })}
              className={`mt-1 ${field}`}
              style={{ outline: 'none' }}
            />
            <datalist id="terminal-options">
              {terminals.map((v) => (
                <option key={v} value={v} />
              ))}
            </datalist>
            <p className="mt-1 text-[11.5px] text-faint">
              {mac ? t('projectSettings.terminalHint') : t('projectSettings.terminalHintLinux')}
            </p>
          </div>
        </div>
        <div className="flex items-center gap-3">
          <button
            type="submit"
            disabled={!dirty || save.isPending}
            className="rounded-md border border-accent px-3 py-1 text-[12.5px] font-medium text-accent-fg hover:bg-accent/10 disabled:opacity-50"
          >
            {t('projectSettings.save')}
          </button>
          {save.isSuccess && !dirty && (
            <span className="text-[12px] text-add">{t('projectSettings.saved')}</span>
          )}
          {save.error && <span className="text-[12px] text-remove">{userError(t, save.error)}</span>}
        </div>
      </form>
    </Section>
  );
}
