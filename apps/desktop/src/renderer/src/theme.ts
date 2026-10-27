export const themes = ['system', 'light', 'dark'] as const;
export type Theme = (typeof themes)[number];

const STORAGE_KEY = 'studio.theme';

export function storedTheme(): Theme {
  try {
    const value = localStorage.getItem(STORAGE_KEY);
    if (value === 'light' || value === 'dark' || value === 'system') return value;
  } catch {
    // Storage may be unavailable; fall back to the system theme.
  }
  return 'system';
}

export function applyTheme(theme: Theme): void {
  const root = document.documentElement;
  if (theme === 'system') root.removeAttribute('data-theme');
  else root.setAttribute('data-theme', theme);
  try {
    localStorage.setItem(STORAGE_KEY, theme);
  } catch {
    // Not persisted; the choice still applies for this session.
  }
}
