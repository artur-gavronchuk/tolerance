import type { ReactNode } from 'react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { type Language, languages, setLanguage } from './i18n';
import { applyTheme, storedTheme, type Theme, themes } from './theme';

const steps = ['map', 'plan', 'run', 'verify', 'review'] as const;

export function App() {
  return (
    <div className="flex h-full">
      <Sidebar />
      <main className="flex min-w-0 flex-1 flex-col">
        <div className="drag h-10 shrink-0 border-b border-line" />
        <Welcome />
        <StatusBar />
      </main>
    </div>
  );
}

function Sidebar() {
  const { t } = useTranslation();
  return (
    <aside className="flex w-60 shrink-0 flex-col border-r border-line bg-panel">
      {/* Leaves room for the macOS window buttons. */}
      <div className="drag flex h-10 shrink-0 items-center pl-20">
        <span className="text-[13px] font-semibold tracking-tight">Studio</span>
      </div>
      <div className="flex items-center justify-between px-3 pt-3 pb-1">
        <span className="text-[12px] font-medium text-muted">{t('sidebar.projects')}</span>
        <button
          type="button"
          disabled
          title={t('welcome.addProjectSoon')}
          aria-label={t('sidebar.addProject')}
          className="no-drag grid h-5 w-5 place-items-center rounded text-muted enabled:hover:bg-line disabled:opacity-40"
        >
          <svg width="12" height="12" viewBox="0 0 12 12" aria-hidden="true">
            <path d="M6 1.5v9M1.5 6h9" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
          </svg>
        </button>
      </div>
      <p className="px-3 py-1 text-[12px] text-faint">{t('sidebar.noProjects')}</p>
      <div className="mt-auto space-y-3 border-t border-line p-3">
        <Preferences />
      </div>
    </aside>
  );
}

function Preferences() {
  const { t, i18n } = useTranslation();
  const [theme, setThemeState] = useState<Theme>(storedTheme);
  const themeLabel: Record<Theme, string> = {
    system: t('settings.themeSystem'),
    light: t('settings.themeLight'),
    dark: t('settings.themeDark'),
  };
  return (
    <>
      <Segmented
        label={t('settings.language')}
        value={i18n.language as Language}
        options={languages.map((l) => ({ value: l, label: l.toUpperCase() }))}
        onChange={setLanguage}
      />
      <Segmented
        label={t('settings.theme')}
        value={theme}
        options={themes.map((v) => ({ value: v, label: themeLabel[v] }))}
        onChange={(v) => {
          applyTheme(v);
          setThemeState(v);
        }}
      />
    </>
  );
}

function Segmented<T extends string>(props: {
  label: string;
  value: T;
  options: { value: T; label: string }[];
  onChange: (value: T) => void;
}) {
  return (
    <fieldset className="space-y-1">
      <legend className="text-[11px] text-faint">{props.label}</legend>
      <div className="flex rounded-md border border-line bg-bg p-0.5">
        {props.options.map((o) => (
          <label
            key={o.value}
            className="flex-1 cursor-default rounded px-1.5 py-0.5 text-center text-[12px] text-muted has-checked:bg-raised has-checked:text-ink has-checked:shadow-[0_0_0_1px_var(--line)] has-focus-visible:outline-2 has-focus-visible:outline-accent"
          >
            <input
              type="radio"
              name={props.label}
              value={o.value}
              checked={o.value === props.value}
              onChange={() => props.onChange(o.value)}
              className="sr-only"
            />
            {o.label}
          </label>
        ))}
      </div>
    </fieldset>
  );
}

function Welcome() {
  const { t } = useTranslation();
  return (
    <section className="flex flex-1 items-center overflow-auto px-10 py-12">
      <div className="mx-auto w-full max-w-3xl">
        <h1 className="max-w-xl text-[22px] font-semibold leading-tight tracking-tight">
          {t('welcome.title')}
        </h1>
        <p className="mt-3 max-w-xl text-[14px] leading-relaxed text-muted">{t('welcome.lead')}</p>

        <ol className="mt-8 grid grid-cols-5 border-y border-line">
          {steps.map((step, i) => (
            <Step key={step} index={i + 1} title={t(`welcome.steps.${step}.title`)}>
              {t(`welcome.steps.${step}.text`)}
            </Step>
          ))}
        </ol>

        <div className="mt-8 flex items-center gap-3">
          <button
            type="button"
            disabled
            className="rounded-md bg-accent px-3 py-1.5 text-[13px] font-medium text-accent-ink disabled:opacity-50"
          >
            {t('welcome.addProject')}
          </button>
          <span className="text-[12px] text-faint">{t('welcome.addProjectSoon')}</span>
        </div>
      </div>
    </section>
  );
}

function Step(props: { index: number; title: string; children: ReactNode }) {
  return (
    <li className="border-line py-3 pr-3 not-first:border-l not-first:pl-3">
      <div className="flex items-baseline gap-1.5">
        <span className="font-mono text-[11px] text-faint">{props.index}</span>
        <span className="text-[13px] font-medium">{props.title}</span>
      </div>
      <p className="mt-1 text-[12px] leading-snug text-muted">{props.children}</p>
    </li>
  );
}

function StatusBar() {
  const { t } = useTranslation();
  const { electron, node } = window.studio.versions;
  return (
    <footer className="flex h-6 shrink-0 items-center border-t border-line px-3 text-[11px] text-faint">
      {t('status.runtime', { electron, node })}
    </footer>
  );
}
