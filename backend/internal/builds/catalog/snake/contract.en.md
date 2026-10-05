# Snake

Build **Snake** as a static web page — the classic game, made so good that people keep playing it.
Everyone who opens the gallery plays your entry and votes for the one they like most, so the feel
matters as much as the rules: crisp controls, a satisfying moment when the snake eats, a game-over screen
you want to restart from, a look of your own.

## The rules (exact — they are checked)

- The field is **20 × 20** cells. `x` goes 0…19 left to right, `y` goes 0…19 top to bottom.
- A new game starts with the snake `[[10,10],[9,10],[8,10]]` (head first) moving **right**, score 0.
- Every tick the head moves one cell in the current direction.
  - Leaving the field (no wrap-around) or running into the snake's own body ends the game.
  - Moving into the cell the **tail is leaving on this tick** is allowed (when the snake is not eating on it).
  - Moving onto the food: the snake grows by one (its tail stays where it was on that tick), the score goes
    up by 1 and new food appears.
- **Food placement** is deterministic, from the game's seed, with this exact generator:

  ```js
  function mulberry32(a) {
    return function () {
      a |= 0; a = (a + 0x6D2B79F5) | 0
      let t = Math.imul(a ^ (a >>> 15), 1 | a)
      t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
      return ((t ^ (t >>> 14)) >>> 0) / 4294967296
    }
  }
  ```

  A game seeded with `s` creates `rng = mulberry32(s)`. To place food, list the cells not covered by the
  snake in row-major order (`y` first, then `x`) and take the one at index `Math.floor(rng() * count)`.
  The first food of a game is placed this way right after the snake is set up (unless the test hook gives a
  starting food, see below). If no cell is free, the game is over.
- **Controls**: arrow keys and WASD turn the snake. Turns are **buffered**: up to two pending turns are
  kept and each tick uses one. A key that asks for the direction the snake would already be going (after the
  pending turns) or its exact opposite is ignored, so pressing Up then Left quickly between two ticks turns
  up and then left, while Up then Down only turns up.
- **Space** pauses and resumes. **R** (or the restart button) starts a new game with the same seed as the
  previous one.
- The **best score** is kept in `localStorage` and survives a reload; it is updated as soon as the score
  beats it.
- Outside test mode the game starts on the first turn key (or a click on a start button, if you have one)
  and the snake moves **at least 4 cells per second**. Speeding up as the score grows is welcome.
- On a phone (375 px wide) the whole game fits without horizontal scrolling and has **on-screen direction
  buttons** (swipes are a bonus).

## Nice to have

Juice: animation, particles or a flash when eating, screen shake on death, sound with a mute toggle,
a speed ramp, a nice start screen, themes, a "new best!" moment, anything that makes it fun. Respect
`prefers-reduced-motion`.

## Rules for the upload

Static files only: an `index.html` at the root of the zip plus any CSS, JS, images and sounds. No server,
no external API keys, no CDNs or web fonts (the checks run without a network). Use relative paths
(`./game.js`, not `/game.js`). Up to 5 MB. A single `.html` file is fine too.

## Automated checks

Your page is opened in Chromium (no network) and driven through acceptance scenarios; the share it passes
is most of the platform score. With **`?test=1`** in the URL the game must not tick on its own (no timers
moving the snake) and must expose this hook:

```js
window.snake = {
  // Start a new game with this seed. opts (all optional): snake: [[x,y], ...] head first,
  // dir: 'up' | 'down' | 'left' | 'right', food: [x, y] (then no food is drawn from rng until it is eaten).
  // The game is playing right away (no start screen in the way).
  reset(seed, opts) {},
  step() {},   // one tick; does nothing when the game is over or paused
  state() {},  // { snake: [[x,y], ...], food: [x,y] | null, dir, score, best, over: boolean }
}
```

Keys go to the page (the `document`), so listen on `window` or `document`. Elements found by `data-testid`:

| `data-testid` | What it is |
|---------------|------------|
| `score` | the current score (its text contains the number) |
| `best` | the best score (its text contains the number) |
| `game-over` | shown when the game is over, hidden otherwise |
| `restart` | a button, visible on game over, that starts a new game (same seed) |
| `ctl-up`, `ctl-down`, `ctl-left`, `ctl-right` | on-screen direction buttons, visible at 375 px wide |

The screen and the hook must agree: the score shown is `state().score`, the snake drawn is `state().snake`.

## Judging

Upload a zip with `index.html` at its root (or a single `.html` file). Within a minute or two the platform
opens your page, takes a screenshot and gives it a **platform score** out of 100:

- 70 points: the share of the acceptance scenarios above that pass;
- 30 points: quality — accessibility (axe-core), no horizontal scroll and big enough tap targets at 375 px,
  no JavaScript errors, a fast and light first load.

Your entry then appears in the public gallery where everyone can play it, and people vote for the ones
they like. You can upload again at any time; your latest upload replaces the previous one and keeps its
votes.
