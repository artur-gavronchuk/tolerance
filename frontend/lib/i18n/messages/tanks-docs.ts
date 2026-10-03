import { defineMessages } from '../core'

// /tanks/docs. Markup in values: <c>code</c>, <b>emphasis</b>, <l>link</l> (the link target is set in the page).
export const tanksDocsMessages = defineMessages({
  en: {
    metaTitle: 'Docs',
    metaDescription: 'How to get a bot into the tanks ladder, the engine rules, the bot protocol, and how rating works.',
    title: 'Docs',
    intro:
      'Your bot is a process. The platform sends it the match state once per tick over stdin and reads your move back over stdout. Everyone plays with full information — there is no fog of war.',
    onThisPage: 'On this page',
    'toc.quick-start': 'Quick start',
    'toc.coordinates': 'Coordinates',
    'toc.rules': 'Rules',
    'toc.protocol': 'Protocol',
    'toc.timing': 'Timing',
    'toc.logs': 'Logs',
    'toc.statuses': 'Statuses',
    'toc.package': 'Package and limits',
    'toc.qualifying': 'Qualifying checks',
    'toc.rating': 'Rating',
    'toc.local': 'Playing locally',

    'qs.title': 'Quick start',
    'qs.lead': 'Nothing to install. Everything happens on the <l>My bot</l> page.',
    'qs.h1': '1. Download a starter kit',
    'qs.p1': "Pick Python or JavaScript. The zip holds a working bot, <c>bot.json</c>, this game's rules as <c>GAME.md</c> and a short README.",
    'qs.h2': '2. Give the folder to your coding agent',
    'qs.p2':
      'Open it in Claude Code, Cursor, Codex or any agent and ask it to read <c>GAME.md</c> and improve the bot: keep <c>bot.json</c> valid, use only the standard library, aim to beat as many house bots as you can. The exact prompt is on the My bot page. Or edit the bot by hand.',
    'qs.h3': '3. Zip it and upload',
    'qs.p3':
      'Zip the folder (a <c>.tar.gz</c> works too) and drop it on the My bot page. Each version gets a trial match against a house bot, then plays the ladder; the check results and replays are right there, so the platform is your test harness. Paste what went wrong back to your agent and upload the next version.',

    'coord.title': 'Coordinate system',
    'coord.p':
      'The field is 60 (width, x) by 40 (height, y) units. <c>(0,0)</c> is the bottom-left corner; x grows right, y grows up. Angles are radians in <c>(-π, π]</c>: 0 points along +x, positive angles turn counter-clockwise. Walls (including the field edge) are axis-aligned rectangles: <c>{x, y, w, h}</c>, with <c>x, y</c> at the bottom-left corner.',

    'rules.title': 'Rules (engine tanks/1)',
    'rule.0.l': 'Field size', 'rule.0.v': '60 × 40',
    'rule.1.l': 'Tick rate', 'rule.1.v': '10 ticks/s',
    'rule.2.l': 'Match length', 'rule.2.v': '1200 ticks (2 minutes)',
    'rule.3.l': 'Physics substeps per tick', 'rule.3.v': '4',
    'rule.4.l': 'Tank radius', 'rule.4.v': '1.0',
    'rule.5.l': 'Forward speed', 'rule.5.v': '5 units/s',
    'rule.6.l': 'Reverse speed', 'rule.6.v': '3 units/s',
    'rule.7.l': 'Hull turn rate', 'rule.7.v': '2.5 rad/s',
    'rule.8.l': 'Turret turn rate', 'rule.8.v': '4 rad/s',
    'rule.9.l': 'Reload time', 'rule.9.v': '10 ticks',
    'rule.10.l': 'Muzzle offset (shell spawn point)', 'rule.10.v': '1.3 from tank centre',
    'rule.11.l': 'Shell speed', 'rule.11.v': '24 units/s',
    'rule.12.l': 'Shell lifetime', 'rule.12.v': '30 ticks',
    'rule.13.l': 'Shell damage', 'rule.13.v': '25',
    'rule.14.l': 'Max HP', 'rule.14.v': '100',
    'rule.15.l': 'Heal pickup amount', 'rule.15.v': '+35 HP (capped at max HP)',
    'rule.16.l': 'Heal pickup respawn', 'rule.16.v': '150 ticks after being taken',
    'rule.17.l': 'Shrinking zone', 'rule.17.v': 'starts tick 800, radius 37 → radius 6 by tick 1100, then holds',
    'rule.18.l': 'Zone damage', 'rule.18.v': '1 HP/tick while outside it',
    'rules.p1':
      "There is no inertia: your effective speed is <c>move × max speed</c> every tick, not a force. A dead tank's wreck takes no part in tank-tank or tank-wall pushing, and shells pass through it. Your own shells never hit you.",
    'rules.p2':
      'Placement once the match ends: alive beats dead; among the alive, higher HP wins, then higher damage dealt; among the dead, whoever died later wins, then higher damage dealt. Exact ties share a place.',

    'proto.title': 'Protocol',
    'proto.p0': 'One JSON object per line, UTF-8, at most 64 KiB per line. The platform writes lines to your stdin; you write lines to your stdout.',
    'proto.hStart': 'start (once, before the first tick)',
    'proto.pStart':
      "<c>you</c> is your tank's id — use it to find yourself in every later <c>tick</c>'s <c>tanks</c> array. Reply <c>ready</c> within 5 seconds of receiving <c>start</c> (this covers your interpreter's startup time too), or your tank sits still for the whole match with status <c>timeout</c>.",
    'proto.hTick': 'tick (once per tick, only while you are alive)',
    'proto.pTick':
      "<c>move</c>, <c>turn</c>, <c>turret</c> are clamped to <c>[-1, 1]</c>. <c>move</c>: 1 full speed forward, -1 full speed reverse. <c>turn</c>: hull turn rate fraction, positive counter-clockwise. <c>turret</c>: turret turn rate fraction, same sign convention, absolute angle (not relative to the hull). <c>fire: true</c> shoots if your reload is 0 this tick; otherwise it's a no-op, not an error.",
    'proto.hEnd': 'end (once, after the match is over)',
    'proto.pEnd': 'Exit after this — the platform closes your stdin right after sending it.',

    'timing.title': 'Timing and the time budget',
    'timing.1': '<b>Ready deadline</b>: 5 seconds from <c>start</c> to <c>ready</c>.',
    'timing.2': '<b>Per-tick deadline</b>: 200 ms to answer a <c>tick</c>. Miss it and that tick is skipped — you are not disconnected for one slow tick.',
    'timing.3':
      '<b>Time budget</b>: the first 20 ms of thinking time per tick is free; time spent beyond that is paid out of a shared 20-second budget for the whole match. Run it out and your bot is disconnected for the rest of the match (<c>timeout</c>).',
    'timing.4': 'A reply carrying the wrong <c>tick</c> (a stale answer to an earlier tick) is discarded, same as a missed tick.',

    'logs.title': 'Logs and stray output',
    'logs.p1':
      "stdout is only for protocol replies. A very common mistake is a debug <c>print(...)</c> left in before your JSON — that line isn't a JSON object with a numeric <c>tick</c> field, so it's ignored, not read as your move, but it is counted. Your move for that tick still counts if the real reply also arrives in time. More than 1000 such stray lines in one match disable your bot for the rest of it (status <c>invalid</c>).",
    'logs.p2': "Write logs to stderr instead — up to 16 KiB per match, sanitized on the server, visible only to you (your bot's owner) on the match log page.",

    'status.title': 'Statuses',
    'status.ok': 'Answered normally for the whole match (or until it died).',
    'status.crashed': 'The process exited before the match ended.',
    'status.timeout': 'Missed the ready deadline, or ran out of the time budget.',
    'status.invalid': 'Sent more than 1000 non-command stdout lines.',
    'status.note': "In every case your tank stays on the field — it just stops moving from that point on, so the match doesn't get one-sided by disconnects alone.",

    'pkg.title': 'Bot package and limits',
    'pkg.lead': 'A directory with a <c>bot.json</c> manifest at its root:',
    'pkg.p1': '<c>language</c> is <c>python</c> or <c>javascript</c>. The platform runs your bot as <c>python3 -u &lt;entry&gt;</c> or <c>node &lt;entry&gt;</c>.',
    'pkg.1': 'Archive at most 1 MiB compressed, 4 MiB uncompressed, 200 regular files, no absolute paths or <c>..</c>.',
    'pkg.2': 'Only the standard library — the run image is <c>python:3.12-slim</c> plus Node 22, no installed third-party packages.',
    'pkg.3': '<c>GAME.md</c> and <c>RESULTS.md</c>, if present, are stripped before packing.',
    'pkg.4': '20 version uploads per day per bot.',

    'qual.title': 'Qualifying checks',
    'qual.lead': 'Every uploaded version goes through a check before it can play in the ladder:',
    'qual.1': '<b>package</b> — the archive is well-formed, <c>bot.json</c> parses, and <c>entry</c> exists.',
    'qual.2': '<b>starts</b> — the bot answers <c>ready</c> within 5 seconds.',
    'qual.3':
      "<b>stable</b> — in a 600-tick, 1-on-1 trial match against <c>house:idle</c>, it answers at least 95% of the ticks it was alive for, and doesn't crash. Stray stdout lines don't fail this check on their own, but they show up in the report with a hint to use stderr instead.",
    'qual.4':
      '<b>beats_idle</b> — it finishes above <c>house:idle</c> in that same trial match. A bot that does nothing ties <c>house:idle</c> for first place and fails this check.',
    'qual.outro': 'All four pass: the version goes active and plays in the ladder. Any one fails: the version is rejected and your previous active version, if any, keeps playing.',

    'rating.title': 'Rating',
    'rating.p1':
      "Matches are rated with Weng–Lin (Plackett–Luce), the idea behind TrueSkill/OpenSkill: every bot has a skill estimate μ and an uncertainty σ, both updated from where it placed relative to everyone else in the match. Starting values are μ₀ = 25, σ₀ = 25/3; the model's own parameters are β = σ₀ / 2 and κ = 0.0001. A new version of an existing bot keeps its rating, with its uncertainty raised back to at least 5.0.",
    'rating.p2': 'The number shown on the ladder is a conservative estimate that starts low and climbs as the bot proves itself:',
    'rating.p3': 'A bot with fewer than 10 season matches is marked provisional: its rating can still move a lot, and tournament seeding prefers bots that are not provisional.',

    'local.title': 'Playing locally',
    'local.p1': 'Advanced and optional: if you want faster iteration than uploading, the <c>{cli}</c> command plays matches on your own machine, using the exact same engine as the server. Install it:',
    'local.then': 'Then:',
    'local.p2':
      'Each run prints a results table (place, kills, damage, status, and the stderr tail of anyone who crashed) and writes a replay file you can open at <l>/tanks/replay</l>. Try a handful of different <c>--seed</c> values — a strategy that only wins on one seed is fragile.',
    'local.p3':
      'The house bots, easiest to hardest: <c>house:idle</c> (never moves), <c>house:hunter</c> (charges and shoots), <c>house:sniper</c> (keeps its distance, leads shots), <c>house:duelist</c> (strafes, dodges, leads shots), <c>house:warden</c> (also picks targets, heals, avoids crossfire and the shrinking zone) and <c>house:ace</c> (plans its dodges against the shots you are about to fire). All but idle play in the ladder, so a new bot starts near the bottom of the table and climbs as it beats them.',
  },
  ru: {
    metaTitle: 'Документация',
    metaDescription: 'Как попасть в лестницу танков, правила движка, протокол бота и как считается рейтинг.',
    title: 'Документация',
    intro:
      'Ваш бот — это процесс. Платформа раз в тик присылает ему состояние матча в stdin и читает ход из stdout. У всех полная информация — тумана войны нет.',
    onThisPage: 'На этой странице',
    'toc.quick-start': 'Быстрый старт',
    'toc.coordinates': 'Координаты',
    'toc.rules': 'Правила',
    'toc.protocol': 'Протокол',
    'toc.timing': 'Время',
    'toc.logs': 'Логи',
    'toc.statuses': 'Статусы',
    'toc.package': 'Пакет и лимиты',
    'toc.qualifying': 'Проверки допуска',
    'toc.rating': 'Рейтинг',
    'toc.local': 'Игра локально',

    'qs.title': 'Быстрый старт',
    'qs.lead': 'Ничего ставить не нужно. Всё происходит на странице <l>Мой бот</l>.',
    'qs.h1': '1. Скачайте стартовый набор',
    'qs.p1': 'Выберите Python или JavaScript. В архиве рабочий бот, <c>bot.json</c>, правила игры в <c>GAME.md</c> и короткий README.',
    'qs.h2': '2. Отдайте папку своему coding-агенту',
    'qs.p2':
      'Откройте её в Claude Code, Cursor, Codex или любом агенте и попросите прочитать <c>GAME.md</c> и улучшить бота: <c>bot.json</c> должен остаться валидным, только стандартная библиотека, цель — обыграть как можно больше house-ботов. Точный промпт — на странице «Мой бот». Или правьте бота руками.',
    'qs.h3': '3. Заархивируйте и загрузите',
    'qs.p3':
      'Заархивируйте папку (подойдёт и <c>.tar.gz</c>) и загрузите на странице «Мой бот». Каждая версия сначала играет пробный матч с house-ботом, затем выходит на лестницу; результаты проверок и повторы матчей — там же, так что платформа и есть ваш тестовый стенд. Скопируйте, что пошло не так, агенту и загрузите следующую версию.',

    'coord.title': 'Система координат',
    'coord.p':
      'Поле — 60 (ширина, x) на 40 (высота, y) единиц. <c>(0,0)</c> — левый нижний угол; x растёт вправо, y — вверх. Углы в радианах из <c>(-π, π]</c>: 0 направлен вдоль +x, положительные углы — против часовой стрелки. Стены (включая границу поля) — прямоугольники вдоль осей: <c>{x, y, w, h}</c>, где <c>x, y</c> — левый нижний угол.',

    'rules.title': 'Правила (движок tanks/1)',
    'rule.0.l': 'Размер поля', 'rule.0.v': '60 × 40',
    'rule.1.l': 'Частота тиков', 'rule.1.v': '10 тиков/с',
    'rule.2.l': 'Длина матча', 'rule.2.v': '1200 тиков (2 минуты)',
    'rule.3.l': 'Подшагов физики на тик', 'rule.3.v': '4',
    'rule.4.l': 'Радиус танка', 'rule.4.v': '1.0',
    'rule.5.l': 'Скорость вперёд', 'rule.5.v': '5 ед/с',
    'rule.6.l': 'Скорость назад', 'rule.6.v': '3 ед/с',
    'rule.7.l': 'Скорость поворота корпуса', 'rule.7.v': '2.5 рад/с',
    'rule.8.l': 'Скорость поворота башни', 'rule.8.v': '4 рад/с',
    'rule.9.l': 'Перезарядка', 'rule.9.v': '10 тиков',
    'rule.10.l': 'Вынос дула (точка появления снаряда)', 'rule.10.v': '1.3 от центра танка',
    'rule.11.l': 'Скорость снаряда', 'rule.11.v': '24 ед/с',
    'rule.12.l': 'Время жизни снаряда', 'rule.12.v': '30 тиков',
    'rule.13.l': 'Урон снаряда', 'rule.13.v': '25',
    'rule.14.l': 'Максимум HP', 'rule.14.v': '100',
    'rule.15.l': 'Лечение с аптечки', 'rule.15.v': '+35 HP (не выше максимума)',
    'rule.16.l': 'Возрождение аптечки', 'rule.16.v': 'через 150 тиков после подбора',
    'rule.17.l': 'Сужающаяся зона', 'rule.17.v': 'с тика 800, радиус 37 → 6 к тику 1100, затем не меняется',
    'rule.18.l': 'Урон зоны', 'rule.18.v': '1 HP/тик вне зоны',
    'rules.p1':
      'Инерции нет: фактическая скорость каждый тик равна <c>move × макс. скорость</c>, а не силе. Обломки погибшего танка не участвуют в толкании с другими танками и стенами, снаряды проходят сквозь них. Ваши снаряды вас не задевают.',
    'rules.p2':
      'Места после матча: живой выше мёртвого; среди живых выигрывает больше HP, затем больше нанесённого урона; среди мёртвых — кто погиб позже, затем больше урона. При полном равенстве место делится.',

    'proto.title': 'Протокол',
    'proto.p0': 'Один JSON-объект на строку, UTF-8, не больше 64 КиБ на строку. Платформа пишет строки в ваш stdin, вы пишете строки в stdout.',
    'proto.hStart': 'start (один раз, перед первым тиком)',
    'proto.pStart':
      '<c>you</c> — id вашего танка; по нему вы находите себя в массиве <c>tanks</c> каждого следующего <c>tick</c>. Ответьте <c>ready</c> в течение 5 секунд после <c>start</c> (сюда входит и запуск интерпретатора), иначе танк простоит весь матч со статусом <c>timeout</c>.',
    'proto.hTick': 'tick (раз в тик, пока вы живы)',
    'proto.pTick':
      '<c>move</c>, <c>turn</c>, <c>turret</c> ограничиваются диапазоном <c>[-1, 1]</c>. <c>move</c>: 1 — полный вперёд, -1 — полный назад. <c>turn</c>: доля скорости поворота корпуса, положительное — против часовой. <c>turret</c>: доля скорости поворота башни, знак тот же, угол абсолютный (не относительно корпуса). <c>fire: true</c> стреляет, если в этом тике перезарядка равна 0; иначе ничего не происходит, и это не ошибка.',
    'proto.hEnd': 'end (один раз, после конца матча)',
    'proto.pEnd': 'После этого завершайтесь — платформа закрывает ваш stdin сразу после отправки.',

    'timing.title': 'Время и бюджет времени',
    'timing.1': '<b>Дедлайн готовности</b>: 5 секунд от <c>start</c> до <c>ready</c>.',
    'timing.2': '<b>Дедлайн на тик</b>: 200 мс на ответ на <c>tick</c>. Не успели — тик пропускается; из-за одного медленного тика бота не отключают.',
    'timing.3':
      '<b>Бюджет времени</b>: первые 20 мс на размышления в каждом тике бесплатны; всё сверх этого списывается с общего бюджета в 20 секунд на матч. Исчерпаете его — бот отключается до конца матча (<c>timeout</c>).',
    'timing.4': 'Ответ с неверным <c>tick</c> (запоздавший ответ на прошлый тик) отбрасывается, как и пропущенный тик.',

    'logs.title': 'Логи и лишний вывод',
    'logs.p1':
      'stdout — только для ответов по протоколу. Частая ошибка — забытый отладочный <c>print(...)</c> перед вашим JSON: такая строка не JSON-объект с числовым полем <c>tick</c>, поэтому она игнорируется и не читается как ход, но учитывается в счётчике. Ваш ход в этом тике всё равно засчитается, если настоящий ответ тоже пришёл вовремя. Больше 1000 таких лишних строк за матч отключают бота до конца матча (статус <c>invalid</c>).',
    'logs.p2': 'Пишите логи в stderr — до 16 КиБ за матч, на сервере они очищаются и видны только вам (владельцу бота) на странице лога матча.',

    'status.title': 'Статусы',
    'status.ok': 'Нормально отвечал весь матч (или до гибели).',
    'status.crashed': 'Процесс завершился до конца матча.',
    'status.timeout': 'Не успел к дедлайну готовности или исчерпал бюджет времени.',
    'status.invalid': 'Отправил больше 1000 строк в stdout, не являющихся командами.',
    'status.note': 'В любом случае танк остаётся на поле — просто перестаёт двигаться, чтобы матч не становился односторонним из-за одних только отключений.',

    'pkg.title': 'Пакет бота и лимиты',
    'pkg.lead': 'Папка с манифестом <c>bot.json</c> в корне:',
    'pkg.p1': '<c>language</c> — <c>python</c> или <c>javascript</c>. Платформа запускает бота как <c>python3 -u &lt;entry&gt;</c> или <c>node &lt;entry&gt;</c>.',
    'pkg.1': 'Архив не больше 1 МиБ в сжатом виде и 4 МиБ в распакованном, до 200 обычных файлов, без абсолютных путей и <c>..</c>.',
    'pkg.2': 'Только стандартная библиотека — образ запуска это <c>python:3.12-slim</c> плюс Node 22, сторонних пакетов нет.',
    'pkg.3': '<c>GAME.md</c> и <c>RESULTS.md</c>, если есть, удаляются перед упаковкой.',
    'pkg.4': '20 загрузок версий в день на одного бота.',

    'qual.title': 'Проверки допуска',
    'qual.lead': 'Каждая загруженная версия проходит проверку, прежде чем выйти на лестницу:',
    'qual.1': '<b>package</b> — архив корректен, <c>bot.json</c> разбирается, а <c>entry</c> существует.',
    'qual.2': '<b>starts</b> — бот отвечает <c>ready</c> в течение 5 секунд.',
    'qual.3':
      '<b>stable</b> — в пробном матче 1 на 1 из 600 тиков против <c>house:idle</c> отвечает минимум на 95% тиков, пока жив, и не падает. Лишние строки в stdout сами по себе проверку не валят, но попадают в отчёт с советом писать в stderr.',
    'qual.4':
      '<b>beats_idle</b> — в том же пробном матче заканчивает выше <c>house:idle</c>. Бот, который ничего не делает, делит с <c>house:idle</c> первое место и эту проверку не проходит.',
    'qual.outro': 'Все четыре пройдены: версия становится активной и играет на лестнице. Хоть одна провалена: версия отклоняется, а предыдущая активная (если была) продолжает играть.',

    'rating.title': 'Рейтинг',
    'rating.p1':
      'Матчи оцениваются по Weng–Lin (Plackett–Luce) — идея та же, что в TrueSkill/OpenSkill: у каждого бота есть оценка навыка μ и неопределённость σ, обе обновляются по занятому месту относительно остальных в матче. Начальные значения: μ₀ = 25, σ₀ = 25/3; параметры самой модели: β = σ₀ / 2 и κ = 0.0001. Новая версия существующего бота сохраняет рейтинг, а неопределённость поднимается обратно минимум до 5.0.',
    'rating.p2': 'Число на лестнице — консервативная оценка: сначала низкая, растёт по мере того, как бот себя показывает:',
    'rating.p3': 'Бот с менее чем 10 матчами в сезоне считается предварительным: его рейтинг ещё может сильно меняться, а при посеве турнира предпочитают ботов без этой пометки.',

    'local.title': 'Игра локально',
    'local.p1': 'Для продвинутых, по желанию: если нужна итерация быстрее загрузки, команда <c>{cli}</c> играет матчи на вашей машине на том же движке, что и сервер. Установка:',
    'local.then': 'Затем:',
    'local.p2':
      'Каждый запуск печатает таблицу результатов (место, убийства, урон, статус и хвост stderr у тех, кто упал) и записывает файл повтора, который можно открыть на <l>/tanks/replay</l>. Попробуйте несколько разных значений <c>--seed</c> — стратегия, выигрывающая только на одном seed, хрупка.',
    'local.p3':
      'House-боты от простого к сложному: <c>house:idle</c> (не двигается), <c>house:hunter</c> (бросается в атаку и стреляет), <c>house:sniper</c> (держит дистанцию, стреляет с упреждением), <c>house:duelist</c> (маневрирует, уклоняется, стреляет с упреждением), <c>house:warden</c> (ещё выбирает цели, лечится, избегает перекрёстного огня и сужающейся зоны) и <c>house:ace</c> (планирует уклонения с учётом ваших будущих выстрелов). Все, кроме idle, играют на лестнице, поэтому новый бот начинает у низа таблицы и поднимается, обыгрывая их.',
  },
})
