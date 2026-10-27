// Themes after Gerry Themes for JetBrains IDEs. Colors live in styles.css; `swatches` repeats
// background and accent so the picker can show each theme without switching to it.
export const themes = ['system', 'light', 'dark', 'darcula', 'oceanic', 'storm', 'violet'] as const;
export type Theme = (typeof themes)[number];

export const swatches: Record<Exclude<Theme, 'system'>, { bg: string; accent: string }> = {
  light: { bg: '#ffffff', accent: '#78a0ff' },
  dark: { bg: '#1e1f22', accent: '#5378c8' },
  darcula: { bg: '#2b2b2b', accent: '#217de1' },
  oceanic: { bg: '#1a2426', accent: '#009688' },
  storm: { bg: '#17233d', accent: '#0082a0' },
  violet: { bg: '#1a1f30', accent: '#a062cb' },
};

const STORAGE_KEY = 'studio.theme';

function isTheme(value: unknown): value is Theme {
  return themes.includes(value as Theme);
}

export function storedTheme(): Theme {
  try {
    const value = localStorage.getItem(STORAGE_KEY);
    if (isTheme(value)) return value;
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
