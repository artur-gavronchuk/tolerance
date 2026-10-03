import { defineMessages, makeT, type Locale } from '../core'
import { ApiError } from '../../api'

// API errors: the server answers {code, message, fields?} in English; known codes are shown in the reader's
// language, anything else falls back to the server's text. Keys: `<code>`, `<code>.<field>` for a field error,
// or `<code>.<variant>` where one code carries several messages (see `variant`).
export const errorMessages = defineMessages({
  en: {
    generic: 'Something went wrong. Please try again.',
    rateLimitedIn: 'Too many requests, try again in {n}s.',
    rateLimited: 'Too many requests, try again shortly.',
    tooLarge: 'That request is too large.',
    attempts_exhausted: 'No attempts left today for this task.',
    body_too_large: 'The upload or request is too large.',
    payload_too_large: 'That request is too large.',
    unauthenticated: 'Sign in required.',
    'unauthenticated.expired': 'Your session expired, sign in again.',
    forbidden: 'You do not have access to this.',
    'forbidden.admin': 'Admin role required.',
    'forbidden.source': 'Source and downloads of sites open when voting ends.',
    not_found: 'Not found.',
    internal_error: 'Internal error. Please try again.',
    state_conflict: 'That cannot be done in the current state.',
    invalid_body: 'The request is not valid.',
    'invalid_body.email': 'Enter a valid email address.',
    'invalid_body.days': 'Days must be a number from 1 to 3650.',
    email_unverified: 'Your account has no verified email address.',
    rate_limited: 'Too many attempts, try again in a minute.',
    banned: 'This account has been banned by a moderator.',
    connector_unavailable: 'This server has no prebuilt arena tool for that platform; build it from the repository: cd backend && go build -o arena ./cmd/arena',
    unsupported_platform: 'That platform is not supported.',
    own_entry: 'You cannot vote for your own entry.',
    not_counted: 'That upload is not the one shown in the results.',
    voting_not_open: 'Voting opens after the deadline.',
    voting_closed: 'Voting is over; the results are final.',
    not_a_site_task: 'Only site tasks are judged by comparison.',
    deadline_passed: 'The deadline has passed; uploads are closed.',
    invalid_days: 'Days must be between 1 and 60.',
    invalid_winner: 'The winner must be "a", "b" or "tie".',
    invalid_pair: 'Pick two different entries.',
    no_tasks: 'No task is available right now.',
    day_open: 'Hidden tests and solutions are published when the day closes.',
    invalid_task: 'Unknown task.',
    upload_limit: 'At most 20 uploads per bot per day.',
    upload_conflict: 'Another upload is in progress; try again.',
    name_taken: 'That name is taken.',
    not_enough_bots: 'At least two ranked bots are needed to start a tournament.',
    'validation_failed.name': 'The name must be 2–32 characters: letters, digits, "_" or "-", starting with a letter or digit.',
    'validation_failed.limit': 'The limit must be a positive integer.',
    'validation_failed.size': 'The size must be 2, 4, 8 or 16.',
    'validation_failed.archive_base64': 'The archive is not valid.',
    validation_failed: 'Some of the values are not valid.',
    invalid_upload: 'The upload is not valid.',
    'invalid_upload.multipart': 'Send the file as a form upload.',
    'invalid_upload.file': 'Choose a file to upload.',
    'invalid_upload.utf8': 'The patch must be UTF-8 text.',
    'invalid_upload.format': 'Upload a .zip of the edited repository or a .patch / .diff file.',
    'invalid_upload.format_site': 'Upload a .zip of your project (files at the root).',
    'invalid_upload.nochanges': "The upload has no changes compared to the task's repository.",
    'invalid_upload.diffsize': 'The resulting diff is larger than 1 MiB.',
    'invalid_upload.abspath': 'The zip contains an absolute path: {name}',
    'invalid_upload.dotdot': "The zip contains a path with '..': {name}",
    'invalid_upload.notzip': 'The file is not a valid zip archive.',
    'invalid_upload.entries': 'The zip has too many entries.',
    'invalid_upload.symlink': 'The zip contains a symbolic link: {name}',
    'invalid_upload.bigfile': 'A file in the zip is larger than 5 MiB: {name}',
    'invalid_upload.files': 'The zip has more than 2000 files.',
    'invalid_upload.damaged': 'The zip is damaged.',
    'invalid_upload.damagedFile': 'The zip is damaged or a file is too large: {name}',
    'invalid_upload.unpacked': 'The zip unpacks to more than 50 MiB.',
    'invalid_upload.empty': 'The zip has no files.',
    'invalid_upload.binary': 'The zip changes binary files, which are not supported; remove them and upload again.',
    'invalid_upload.index': 'The zip needs an index.html at its root (or inside a single top-level folder).',
    invalid_package: 'The bot archive was rejected: {detail}',
  },
  ru: {
    generic: 'Что-то пошло не так. Попробуйте ещё раз.',
    rateLimitedIn: 'Слишком много запросов, повторите через {n} с.',
    rateLimited: 'Слишком много запросов, повторите чуть позже.',
    tooLarge: 'Запрос слишком большой.',
    attempts_exhausted: 'На сегодня попыток по этой задаче не осталось.',
    body_too_large: 'Загрузка или запрос слишком большие.',
    payload_too_large: 'Запрос слишком большой.',
    unauthenticated: 'Нужно войти в аккаунт.',
    'unauthenticated.expired': 'Сессия истекла, войдите снова.',
    forbidden: 'У вас нет доступа к этому.',
    'forbidden.admin': 'Нужна роль администратора.',
    'forbidden.source': 'Исходники и скачивание сайтов открываются, когда заканчивается голосование.',
    not_found: 'Не найдено.',
    internal_error: 'Внутренняя ошибка. Попробуйте ещё раз.',
    state_conflict: 'Сейчас это сделать нельзя.',
    invalid_body: 'Некорректный запрос.',
    'invalid_body.email': 'Введите корректный адрес почты.',
    'invalid_body.days': 'Число дней должно быть от 1 до 3650.',
    email_unverified: 'В вашем аккаунте нет подтверждённого адреса почты.',
    rate_limited: 'Слишком много попыток, повторите через минуту.',
    banned: 'Этот аккаунт заблокирован модератором.',
    connector_unavailable: 'На этом сервере нет готового инструмента arena для этой платформы; соберите его из репозитория: cd backend && go build -o arena ./cmd/arena',
    unsupported_platform: 'Эта платформа не поддерживается.',
    own_entry: 'За свою работу голосовать нельзя.',
    not_counted: 'Это не та загрузка, которая показана в результатах.',
    voting_not_open: 'Голосование начнётся после дедлайна.',
    voting_closed: 'Голосование закончено, результаты окончательные.',
    not_a_site_task: 'Сравнением оцениваются только задачи про сайты.',
    deadline_passed: 'Срок вышел, приём работ закрыт.',
    invalid_days: 'Число дней должно быть от 1 до 60.',
    invalid_winner: 'Победитель — «a», «b» или ничья.',
    invalid_pair: 'Выберите две разные работы.',
    no_tasks: 'Сейчас нет доступных задач.',
    day_open: 'Скрытые тесты и решения публикуются, когда день закрывается.',
    invalid_task: 'Неизвестная задача.',
    upload_limit: 'Не больше 20 загрузок на бота в сутки.',
    upload_conflict: 'Другая загрузка ещё идёт, попробуйте снова.',
    name_taken: 'Это имя уже занято.',
    not_enough_bots: 'Чтобы начать турнир, нужно минимум два бота с рейтингом.',
    'validation_failed.name': 'Имя — от 2 до 32 символов: буквы, цифры, «_» или «-», начинается с буквы или цифры.',
    'validation_failed.limit': 'Лимит должен быть положительным целым числом.',
    'validation_failed.size': 'Размер должен быть 2, 4, 8 или 16.',
    'validation_failed.archive_base64': 'Архив некорректен.',
    validation_failed: 'Некоторые значения некорректны.',
    invalid_upload: 'Загрузка не принята.',
    'invalid_upload.multipart': 'Отправьте файл как загрузку формы.',
    'invalid_upload.file': 'Выберите файл для загрузки.',
    'invalid_upload.utf8': 'Патч должен быть текстом в UTF-8.',
    'invalid_upload.format': 'Загрузите .zip с изменённым репозиторием или файл .patch / .diff.',
    'invalid_upload.format_site': 'Загрузите .zip с проектом (файлы в корне).',
    'invalid_upload.nochanges': 'В загрузке нет изменений относительно репозитория задачи.',
    'invalid_upload.diffsize': 'Получившийся дифф больше 1 МиБ.',
    'invalid_upload.abspath': 'В архиве абсолютный путь: {name}',
    'invalid_upload.dotdot': 'В архиве путь с «..»: {name}',
    'invalid_upload.notzip': 'Файл не является корректным zip-архивом.',
    'invalid_upload.entries': 'В архиве слишком много записей.',
    'invalid_upload.symlink': 'В архиве символическая ссылка: {name}',
    'invalid_upload.bigfile': 'Файл в архиве больше 5 МиБ: {name}',
    'invalid_upload.files': 'В архиве больше 2000 файлов.',
    'invalid_upload.damaged': 'Архив повреждён.',
    'invalid_upload.damagedFile': 'Архив повреждён или файл слишком большой: {name}',
    'invalid_upload.unpacked': 'Архив после распаковки больше 50 МиБ.',
    'invalid_upload.empty': 'В архиве нет файлов.',
    'invalid_upload.binary': 'Архив меняет бинарные файлы, это не поддерживается; уберите их и загрузите снова.',
    'invalid_upload.index': 'В архиве нужен index.html в корне (или в единственной папке верхнего уровня).',
    invalid_package: 'Архив бота не принят: {detail}',
  },
})

type K = keyof typeof errorMessages.en

// invalid_upload carries the reason only in its English text, so it is recognised by that text.
const UPLOAD_REASONS: [RegExp, K][] = [
  [/^Send a multipart form/, 'invalid_upload.multipart'],
  [/^The file field is required/, 'invalid_upload.file'],
  [/^The patch must be UTF-8/, 'invalid_upload.utf8'],
  [/^Upload a \.zip of the edited/, 'invalid_upload.format'],
  [/^Upload a \.zip of your project/, 'invalid_upload.format_site'],
  [/^The upload has no changes/, 'invalid_upload.nochanges'],
  [/^The resulting diff is larger/, 'invalid_upload.diffsize'],
  [/^The zip contains an absolute path: /, 'invalid_upload.abspath'],
  [/^The zip contains a path with '\.\.': /, 'invalid_upload.dotdot'],
  [/^The file is not a valid zip/, 'invalid_upload.notzip'],
  [/^The zip has too many entries/, 'invalid_upload.entries'],
  [/^The zip contains a symbolic link: /, 'invalid_upload.symlink'],
  [/^A file in the zip is larger than 5 MiB: /, 'invalid_upload.bigfile'],
  [/^The zip has more than 2000 files/, 'invalid_upload.files'],
  [/^The zip is damaged or a file is too large: /, 'invalid_upload.damagedFile'],
  [/^The zip is damaged$/, 'invalid_upload.damaged'],
  [/^The zip unpacks to more than/, 'invalid_upload.unpacked'],
  [/^The zip has no files/, 'invalid_upload.empty'],
  [/^The zip changes binary files/, 'invalid_upload.binary'],
  [/^The zip needs an index\.html/, 'invalid_upload.index'],
]

function variant(code: string, message: string): K | null {
  let key: string | null = null
  if (code === 'forbidden') key = /^Admin/.test(message) ? 'forbidden.admin' : /^Source and downloads/.test(message) ? 'forbidden.source' : null
  else if (code === 'unauthenticated') key = /expired/i.test(message) ? 'unauthenticated.expired' : null
  else if (code === 'invalid_body') key = /^days must/.test(message) ? 'invalid_body.days' : null
  return key as K | null
}

function has(key: string): key is K {
  return key in errorMessages.en
}

// Any error thrown by api()/post()/upload(), in the reader's language.
export function errorText(err: unknown, locale: Locale = 'en'): string {
  const t = makeT(errorMessages, locale)
  if (!(err instanceof ApiError)) return t('generic')
  const { code, message, status } = err
  if (code !== 'attempts_exhausted' && (status === 429 || code === 'rate_limited') && code !== 'upload_limit') {
    return err.retryAfterSec ? t('rateLimitedIn', { n: err.retryAfterSec }) : code === 'rate_limited' ? t('rate_limited') : t('rateLimited')
  }
  if (status === 413 || code === 'payload_too_large') return code === 'body_too_large' ? t('body_too_large') : t('tooLarge')
  if (err.fields?.length) {
    const key = `${code}.${err.fields[0].path}`
    if (has(key)) return t(key)
  }
  if (code === 'invalid_upload') {
    for (const [re, key] of UPLOAD_REASONS) {
      if (re.test(message)) return t(key, { name: message.slice(message.indexOf(': ') + 2) })
    }
  }
  if (code === 'invalid_package') return t('invalid_package', { detail: message })
  const v = variant(code, message)
  if (v) return t(v)
  if (has(code)) return t(code)
  return message || t('generic')
}
