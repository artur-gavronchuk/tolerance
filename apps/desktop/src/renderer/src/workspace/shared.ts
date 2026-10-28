import type { TFunction } from 'i18next';
import { useEffect, useState } from 'react';

/**
 * A server error the user can act on, translated. Main sends `workspace.<code>` or
 * `open.<code>:<detail>` (see router.ts); anything else is shown as is.
 */
export function userError(t: TFunction, error: unknown): string {
  const message = error instanceof Error ? error.message : String(error);
  const workspace = /^workspace\.(bad_name|setup_running|no_setup)$/.exec(message);
  if (workspace) return t(`errors.workspace.${workspace[1] as 'bad_name'}`);
  const gitAction =
    /^git\.(nothing_to_commit|empty_message|hook_failed|not_studio_branch|push_failed):(.*)$/s.exec(message);
  if (gitAction) return t(`errors.git.${gitAction[1] as 'hook_failed'}`, { detail: gitAction[2] });
  const gh = /^gh\.(missing|logged_out|failed):(.*)$/s.exec(message);
  if (gh) return t(`errors.gh.${gh[1] as 'failed'}`, { detail: gh[2] });
  const open = /^open\.(editor_not_found|terminal_not_found|folder_failed):(.*)$/s.exec(message);
  if (open) return t(`errors.open.${open[1] as 'editor_not_found'}`, { detail: open[2] });
  return t('errors.generic', { message });
}

/** The current time, updated every `everyMs`, for "5 minutes ago" labels. */
export function useNow(everyMs = 30_000): number {
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), everyMs);
    return () => clearInterval(id);
  }, [everyMs]);
  return now;
}

/** "5 минут назад" / "5 minutes ago"; whole days after a day. */
export function relativeTime(ts: number, now: number, language: string): string {
  const rtf = new Intl.RelativeTimeFormat(language, { numeric: 'auto' });
  const minutes = Math.round((ts - now) / 60_000);
  if (Math.abs(minutes) < 60) return rtf.format(minutes, 'minute');
  const hours = Math.round(minutes / 60);
  if (Math.abs(hours) < 24) return rtf.format(hours, 'hour');
  return rtf.format(Math.round(hours / 24), 'day');
}
