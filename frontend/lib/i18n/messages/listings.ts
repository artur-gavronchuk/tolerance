import { defineMessages } from '../core'

// Archive and overall leaderboard pages.
export const listingMessages = defineMessages({
  en: {
    archiveTitle: 'Archive',
    archiveIntro: 'Every past task. Open one to read it and try it as practice.',
    archiveEmpty: "The first archived day appears after today's task closes at 00:00 UTC.",
    backToToday: "Back to today's task",
    solvedN: '{n} solved',

    lbTitle: 'Leaderboard',
    lbIntro: 'Each day is worth up to 100 points: the share of hidden tests your best attempt passed. Ties go to more days fully solved, then the longer current streak.',
    lbEmpty: 'Nobody has passed a hidden test yet.',
    lbTieHint: 'Equal points, days solved and streak share a place, marked with =; those players are listed alphabetically.',
    lbTop: 'Top {shown} of {total}',
    lbShowMore: 'Show more',
    lbYou: 'You',
    player: 'Player',
    points: 'Points',
    solved: 'Solved',
    streak: 'Streak',
  },
  ru: {
    archiveTitle: 'Архив',
    archiveIntro: 'Все прошлые задачи. Откройте любую, чтобы прочитать и попробовать как тренировку.',
    archiveEmpty: 'Первый день появится в архиве, когда сегодняшняя задача закроется в 00:00 UTC.',
    backToToday: 'К задаче дня',
    solvedN: 'решили: {n}',

    lbTitle: 'Рейтинг',
    lbIntro: 'Каждый день приносит до 100 очков: доля скрытых тестов, которые прошла ваша лучшая попытка. При равенстве выше тот, у кого больше полностью решённых дней, затем — длиннее текущая серия.',
    lbEmpty: 'Пока никто не прошёл ни одного скрытого теста.',
    lbTieHint: 'Одинаковые очки, решённые дни и серия дают общее место, оно помечено знаком =; такие игроки идут по алфавиту.',
    lbTop: 'Топ {shown} из {total}',
    lbShowMore: 'Показать ещё',
    lbYou: 'Вы',
    player: 'Игрок',
    points: 'Очки',
    solved: 'Решено',
    streak: 'Серия',
  },
})
