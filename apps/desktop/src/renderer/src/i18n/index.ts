import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import { en } from './en';
import { ru } from './ru';

export const languages = ['ru', 'en'] as const;
export type Language = (typeof languages)[number];

const STORAGE_KEY = 'studio.language';

function storedLanguage(): Language {
  try {
    const value = localStorage.getItem(STORAGE_KEY);
    if (value === 'ru' || value === 'en') return value;
  } catch {
    // Storage may be unavailable; fall back to the default.
  }
  return 'ru';
}

export function setLanguage(language: Language): void {
  void i18n.changeLanguage(language);
  document.documentElement.lang = language;
  try {
    localStorage.setItem(STORAGE_KEY, language);
  } catch {
    // Not persisted; the choice still applies for this session.
  }
}

void i18n.use(initReactI18next).init({
  resources: { ru: { translation: ru }, en: { translation: en } },
  lng: storedLanguage(),
  fallbackLng: 'ru',
  interpolation: { escapeValue: false },
});
document.documentElement.lang = i18n.language;

declare module 'i18next' {
  interface CustomTypeOptions {
    resources: { translation: typeof ru };
  }
}

export default i18n;
