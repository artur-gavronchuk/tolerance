import { defineMessages } from '../core'

// Owner-only "Your data" block on the profile: export and delete the account.
export const accountMessages = defineMessages({
  en: {
    title: 'Your data',
    intro: 'Everything we store about you is in one JSON file. Deleting the account is immediate and permanent.',
    privacy: 'Privacy policy',
    download: 'Download my data',
    deleteButton: 'Delete my account',
    deleteWarn: 'This erases your email, sign-in, upload link, submissions and votes, and anonymizes your tank bot. It cannot be undone. Type your handle “{handle}” to confirm.',
    handleLabel: 'Your handle',
    confirm: 'Delete forever',
    deleting: 'Deleting…',
    cancel: 'Cancel',
    failed: 'Could not delete the account: {error}',
  },
  ru: {
    title: 'Ваши данные',
    intro: 'Всё, что мы о вас храним, лежит в одном JSON-файле. Удаление аккаунта мгновенное и необратимое.',
    privacy: 'Политика конфиденциальности',
    download: 'Скачать мои данные',
    deleteButton: 'Удалить аккаунт',
    deleteWarn: 'Будут стёрты email, вход, ссылка для загрузки, решения и голоса, а танковый бот обезличен. Отменить нельзя. Для подтверждения введите свой ник «{handle}».',
    handleLabel: 'Ваш ник',
    confirm: 'Удалить навсегда',
    deleting: 'Удаляем…',
    cancel: 'Отмена',
    failed: 'Не удалось удалить аккаунт: {error}',
  },
})
