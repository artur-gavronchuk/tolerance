# Habit tracker

Build a **habit tracker** as a static web page: a list of habits, a grid of the last 14 days for each, and
streaks and a completion rate that are always right, whatever the date, month, year or time zone.

## Must have

- Add a habit (text input + Add button; Enter also adds; the name is trimmed, a blank name adds nothing, the
  input is cleared afterwards). Delete a habit with a button on it.
- Each habit shows **14 day buttons: today and the 13 days before it, in order, and no future days**. A day button
  toggles "done" for that habit and day.
- "Today" is the visitor's **local** calendar date (`new Date()` in the browser), never the UTC date.
- Each habit shows:
  - its **current streak**: the number of consecutive done days ending today. If today is not done yet, the
    streak that ends yesterday still counts (it is not broken until the day is over); if neither today nor
    yesterday is done, it is 0;
  - its **best streak**: the longest run of consecutive done days in the habit's whole history, recomputed from
    the data (unticking a day can lower it);
  - its **completion rate**: done days among the 14 visible days, as a whole percent rounded to the nearest
    integer (`3/14` is `21%`, `1/14` is `7%`).
- A page-level summary of how many habits are done today, like `2/3 done today`.
- Consecutive means consecutive calendar days: month ends, 29 February, New Year and the days when the
  clocks change (23- and 25-hour days) must not break or invent a streak.
- Habits and ticks are saved in `localStorage`; after a reload (even on a later day) streaks and rates are
  recalculated for the current date.
- Habit names are plain text; `<i>x</i>` must be shown literally, never interpreted as HTML.
- Works on a phone screen (375 px wide) without horizontal scrolling, with several habits and long names.

## Nice to have

- Week labels, a month view, a heat map, a nice "streak!" moment.
- The page updates itself when midnight passes while it stays open.
- Export or import of the data, dark mode.

## Rules

Static files only: an `index.html` at the root of the zip plus any CSS, JS and images. No server, no
external API keys, no external CDNs or fonts (the checks run without a network). Use relative paths
(`./app.js`, not `/app.js`). Up to 5 MB.

## Automated checks

Your page is also opened in Chromium (no network) with a **fake clock and time zone** and driven through
acceptance scenarios; the share it passes is shown next to the votes. The scenarios find elements by
`data-testid`:

| `data-testid` | What it is |
|---------------|------------|
| `habit-name-input`, `add-habit` | the name `<input>` and the Add button |
| `habit` | one habit's container (several on the page) |
| `habit-name` | inside a habit; its text is exactly the name |
| `day-YYYY-MM-DD` | inside a habit; a `<button>` for that local date, one per visible day, e.g. `day-2026-03-10`; `aria-pressed` is `"true"` when the day is done and `"false"` otherwise |
| `streak`, `best-streak` | inside a habit; the text is just the number (`3`) |
| `rate` | inside a habit; the text is the percent (`21%`) |
| `delete-habit` | inside a habit; click deletes it |
| `today-summary` | the page summary; its text contains `done/total` such as `2/3` |

Day buttons are real `<button>` elements, so Enter and Space toggle them from the keyboard. The scenarios
set the clock and time zone, load the page, and tick days in the past and today; they also change the date
and reload to check that the streak moves on.

## Judging

Votes decide. After the deadline every site is shown to everyone and people vote; the automated score is
shown beside the votes and breaks ties. Up to 3 uploads; your latest counts. A scored upload appears after
the checks finish (a minute or so).
