# Two couriers

Build a **two-courier routing puzzle** as a static web page. The map is a graph: places and the roads
between them. Two couriers, **a** and **b**, each have their own destination, and **both move at the
same time** — one step per turn, each along a road or not at all. The puzzle is in not getting in each
other's way: two couriers can't stand on one place, and they can't pass each other on a road.

The world, the characters, the look of the map, the story — all yours (see the end). The rules and the
hooks below are exact; they are checked.

## The board

A board is plain data:

```js
{
  nodes: ['dock', 'bridge', 'tower'],        // node ids: unique strings matching [a-z0-9-]+
  edges: [['dock', 'bridge'], ['tower', 'bridge']],   // roads between two different nodes
  start: { a: 'dock', b: 'tower' },          // where the couriers begin
  goals: { a: 'tower', b: 'dock' },          // where each one must end up
}
```

- Edges are **undirected**: `['dock','bridge']` lets a courier go from `dock` to `bridge` and from
  `bridge` to `dock`. Two nodes are *adjacent* when an edge joins them (in either order). Duplicate
  edges are harmless.
- A node has no edge to itself; "stay where you are" is a wait (`null`), never a move.
- Courier **a** has its own goal and so does **b**: a standing on b's goal and b on a's goal is not a win.
- Boards given to the page are connected and have at most 12 nodes. The page must lay out **any** such
  graph itself (you choose how: circle, force layout, grid…); every node must be visible and clickable.

## The test hook: `window.__couriers`

Available always, not only in some test mode. It controls the same game the UI shows.

```js
window.__couriers = {
  load(board),   // -> true | false
  step(move),    // -> { ok: boolean, reason: string | null }
  reset(),
  getState(),    // -> { positions: {a, b}, goals: {a, b}, turns, won, lastOutcome }
}
```

### `load(board)`

Replaces the game with this board: the couriers stand on `start`, `turns` is `0`, `won` is `false`,
`lastOutcome` is `null`; the page shows the new board (a `node-<id>` element for every node of this
board and none for nodes of the previous one). Returns `true`.

The board is **validated first**. `load` returns `false` and changes **nothing** (neither the state nor
the page) if any of these hold: `nodes` is not a non-empty array of unique strings matching `[a-z0-9-]+`;
an edge is not a pair of two different known nodes; `start.a` / `start.b` / `goals.a` / `goals.b` is not
a known node; `start.a === start.b`; `goals.a === goals.b`.

Whether a board that starts with both couriers already on their goals counts as won: **no** — `won` is
decided only after a `step` (see below), never by `load` or `reset`.

### `step(move)`

`move` is `{ a, b }`. Each of `a` and `b` is either a node id (go there) or `null` (wait). A missing or
`undefined` key means `null`. This is **one turn**; both couriers act in the same turn.

1. If the game is already won, return `{ ok: false, reason: 'FINISHED' }`. Nothing changes. (This check
   comes first, even for a move that would otherwise be invalid.)
2. **Validation.** A move is invalid when `move` is not an object, or when `a` or `b` is anything
   other than `null`/missing or a string that names a node **adjacent** to that courier's current node.
   That includes: an unknown id, a non-string (`1`, `false`…), a non-adjacent node, and the node the
   courier already stands on. An invalid move is **rejected as a whole**: return
   `{ ok: false, reason: 'INVALID_MOVE' }`; the other courier does not move either; `turns`,
   positions, `won` and `lastOutcome` stay exactly as they were.
3. Otherwise the move is **valid** and the turn counts: `turns` goes up by 1 and the call returns
   `{ ok: true, reason: null }`. Let `ta` / `tb` be where each courier wants to be (its target, or its own
   node when it waits). Outcomes, in this order:
   - **collision** — `ta === tb`. This includes both moving into the same node, and one courier moving
     into the node of the other that is **waiting**. **Both** couriers stay where they were.
   - **swap** — `a` moves to `b`'s current node and `b` moves to `a`'s current node. **Both** stay.
   - **moved** — anything else: both couriers go to their targets. Taking the node the other courier is
     **leaving in this same move** is allowed (courier a goes where b was while b goes on to another node).
     Both waiting is a valid `moved` turn too.

   A collision and a swap are valid moves (`ok: true`, `reason: null`, `turns + 1`): they are reported
   only through `lastOutcome`.
4. `lastOutcome` is set to the string `'moved'`, `'collision'` or `'swap'`. It is `null` after `load` /
   `reset` and keeps its value when a move is rejected.
5. **Victory**: after a valid move, `won` becomes `true` if courier a stands on `goals.a` **and** courier
   b stands on `goals.b` at that same moment. Standing on its goal earlier, then leaving, counts for
   nothing. After victory every `step` returns `FINISHED` and changes nothing.

### `reset()`

Back to the start of the **currently loaded** board (the one from the last successful `load`, or the
level the page is showing): positions = `start`, `turns` 0, `won` false, `lastOutcome` `null`, and any
unfinished move in the UI is discarded (see below).

### `getState()`

```js
{ positions: { a: 'dock', b: 'tower' }, goals: { a: 'tower', b: 'dock' },
  turns: 0, won: false, lastOutcome: null }
```

`positions` and `goals` are node ids. It returns a **snapshot**: changing the returned object must not
affect the game.

## The page

On a fresh load of the page (nobody has called `load` yet) a level of your own is shown: **level 1**. Make
**at least 3 levels**, every one solvable with these exact rules (waiting, following and dodging are
what makes them interesting — a level a player can't finish is a bug), and a way to move between them
(a level picker, next/previous, anything). A level is just a board; switching level behaves like
`load(board)`.

| `data-testid` | What it is |
|---------------|------------|
| `board` | the map container (visible) |
| `node-<id>` | one element per node of the loaded board, e.g. `node-dock`; visible, clickable. No other test id may start with `node-` |
| `agent-a`, `agent-b` | the two couriers, visible; the attribute `data-node="<id>"` holds the node the courier stands on **now** (it changes the moment the state does, not after an animation) |
| `select-a`, `select-b` | buttons choosing which courier you are directing; the chosen one has `aria-pressed="true"`, the other `"false"` |
| `step` | button: commits the pending move |
| `turn-count` | the number of turns: the element's text contains exactly one number, the value of `turns` (so no "level 2" in there) |
| `reset` | button: same as `reset()` |

How to move with the mouse (this is checked too):

- You always direct one courier: **a** at first (after page load, `load`, `reset`). `select-a` / `select-b`
  switch; nothing else changes the choice.
- A click on `node-<id>` makes `<id>` the **pending target** of the courier you are directing, replacing
  that courier's earlier pending target. It does not step, and it does not check anything. The click must
  work on a node a courier is standing on too — courier markers must not swallow clicks meant for the node.
- The `step` button calls `step({ a: pendingA ?? null, b: pendingB ?? null })` — exactly the API call — and
  then clears both pending targets, whether the move was valid or not. After victory it does nothing
  more (as `step` returns `FINISHED`).
- `load`, `reset` and level switches clear the pending targets and switch the directed courier back to **a**.
- Show the pending moves, a rejected move, a collision and a swap in a way a player understands (arrows,
  a shake, a message — your call). Nothing of that is checked, but people will vote on it.

## Yours to invent

The world and the characters: couriers, drones, ferries, mice with parcels, ghosts in a haunted mansion,
trains. The look of the map, the animation of a move, the bump of a collision, the "no, not like that" of
a swap. Sound with a mute button, a level story, hints, an undo of your own (the API has none), a turn
counter record per level, a gallery of levels. The better it feels to plan the dance of two figures,
the more votes it gets.
