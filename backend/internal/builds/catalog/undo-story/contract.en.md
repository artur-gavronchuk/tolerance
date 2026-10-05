# The world remembers undo

Build an **interactive story for a phone**: a short branching tale in which every decision can be taken back
with an **Undo** button, but the world keeps count of how many times you did it. Genre, plot, voice and look
are entirely yours (a noir, a fairy tale, a sci-fi log, a cooking disaster); what is fixed is the engine below.
The gallery shows your entry in a phone frame, so design for a **390 px wide** screen and touch.

People vote for the story that makes them *feel* the memory of their own undos, so use the counter: let the
narrator, the scenery and the choices notice it.

## The story

- A story is a **graph of scenes**. Between **6 and 12 scenes** are reachable, and **at least 2 of them are
  endings**. An ending is a scene with no choices.
- Every scene has an **id**: a string matching `[a-z0-9-]+`. Every choice has an **id** of the same form
  (so `choice-<id>` is a valid `data-testid`); ids of choices are unique within one scene. A scene id and a
  choice id may coincide.
- The **start scene** is the one shown when the page opens and again after a restart. It is not an ending and
  offers at least one choice (before any undo).
- The graph is **fixed**: taking choice `x` in scene `S` always leads to the same scene, and never to `S`
  itself (a path may loop back to an earlier scene). Which choices a scene offers depends only on the scene;
  the single exception is the start scene (see "The extra choice"). The *words* on the screen may depend on
  anything you like, including the undo count.
- Choosing the **first listed choice** over and over, from a fresh start, reaches an ending after **at least 3
  and at most 12** choices.

## State and API: `window.__story`

Available as soon as the page has loaded, with the story at its start scene.

```js
window.__story = {
  getState() {},   // { nodeId, choices: [id, ...], undoCount, historyDepth, terminal }
  choose(id) {},   // true | false
  undo() {},       // true | false
  restart() {},    // return value is ignored
}
```

`getState()` returns a **new plain object each call** (changing it must not change the story) with at least:

| field | meaning |
|-------|---------|
| `nodeId` | string, the current scene's id |
| `choices` | array of the ids offered **right now**, in display order; `[]` in an ending |
| `undoCount` | integer, how many successful undos since the last restart |
| `historyDepth` | integer, how many chosen steps can still be undone (the length of the path behind you) |
| `terminal` | boolean, `true` exactly when the scene is an ending, i.e. when `choices` is empty |

Everything is **synchronous**: right after `choose`, `undo` or `restart` returns, `getState()` is up to date.

- **`choose(id)`** returns `true` and moves to the target scene when `id` is one of the current `choices`;
  `historyDepth` grows by 1, `undoCount` does not change. For anything else (an unknown id, an id that only
  another scene offers, a non-string, no argument, any id in an ending) it returns `false` and **changes nothing**.
- **`undo()`** returns `true` and goes back to the scene you chose from; `historyDepth` shrinks by 1 and
  **`undoCount` grows by 1**. It also works on an ending. **`undoCount` never goes down** because of an undo,
  not even when you undo the step that was taken after an earlier undo: it counts every undo ever made since the last restart.
  With an **empty history** (`historyDepth` 0) it returns `false` and **changes nothing, `undoCount` included**.
- **`restart()`** works from any scene, ending included. It returns to the start scene with `historyDepth` 0
  and `undoCount` **0**. A restart is the only way `undoCount` goes back to zero.
- Choosing after an undo works as usual and keeps the count (`undoCount` 1, then a choice: still 1).
- Persisting the state across reloads is up to you (not tested). If you do, a first-time visitor starts clean.

## The extra choice

The world notices that you hesitate. While you are in the **start scene** and `undoCount` is **1 or more**
(including when you come back to it by undoing from deeper in the story), the start scene offers **all the
choices it offered at `undoCount` 0 plus at least one extra choice** that was not there at the start. The extra
choice has its own id, can be chosen like any other (it leads to a scene other than the start scene), and
once it appears it stays until a restart (`undoCount` only grows, so the set only grows). After a restart
the start scene again offers exactly its original choices. The extra choice may appear anywhere in the
list (even first), so do not assume that `choices[0]` is the same before and after an undo.

## On the screen

Elements are found by `data-testid`:

| `data-testid` | What it is |
|---------------|------------|
| `scene` | **exactly one** element holding the current scene's text (never empty), with the attribute `data-node="<nodeId>"` |
| `choice-<id>` | one native `<button>` for **each** id in `choices`, in the same order, visible and tappable. **No other element** may use a `data-testid` starting with `choice-`; an ending shows no such element |
| `undo` | a `<button>` that does what `undo()` does; always on the page, and it may be disabled while `historyDepth` is 0 |
| `undo-count` | **exactly one** element whose text, trimmed, is exactly the decimal `undoCount` (`0`, `1`, `12`), and which is always visible. Put any label ("times", an icon) **outside** this element |
| `restart` | a `<button>` that does what `restart()` does; always on the page and enabled |

The screen and the API are two views of **one** story: after every tap or API call, the scene, the choice
buttons (same ids, same order) and the counter match `getState()` within half a second. While you animate a
transition, do not keep the previous scene's choice buttons in the DOM.

On a 390 px wide touch screen nothing may scroll sideways, in any scene, an ending included. Vertical
scrolling is fine; keep the controls easy to reach and tap targets generous.

## Yours to invent

Everything that is not a rule: the genre, the plot, the names, the prose (the best entries have a voice),
what the *extra choice* means in your world, how the undo count changes the telling (the narrator getting
tired, a character keeping score, scenery that fades, endings that read differently), typography, art
(inline SVG or CSS, no network), animation, sound with a mute toggle, haptics, a title screen inside the
start scene. Respect `prefers-reduced-motion`. Make the first minute delightful: that is when people decide
whether to vote.
