# Creature lab

Build a **creature lab**: a page with one creature that is made of six numbers, mutates at the press of a
button and lives entirely in its link. Send someone the link and they see exactly the creature you see. What
the creature is (a jellyfish, a robot, a flower, a tiny god) and how each number shapes it is yours to invent;
the rules below are what every lab has in common.

## The genome

- A creature's **genome** is **six integers, each from 0 to 7** (inclusive). The order matters. Written as text
  it is the six numbers joined by dots: `3.1.4.1.5.2`.
- The **default genome** is `0.0.0.0.0.0`.
- **Every number must change the look.** Changing any one of the six numbers to any other value changes what the
  creature looks like (whatever the other five numbers are), so all 8 values of every number give 8 different
  looks. How is up to you: colour, size, shape, number of eyes, a pattern, a mood.
- **The look is a function of the genome and nothing else.** The same genome always looks the same: after a
  mutation, after `setGenome`, after a reload, when the link is opened in a new tab, a minute later. No random
  decoration chosen when the creature is drawn, nothing that depends on the history of earlier mutations.
- All visible differences between creatures live inside the element `data-testid="creature"` (see below).
  Idle motion is welcome (breathing, blinking, drifting), but only with CSS animations, CSS transitions or the Web
  Animations API, which the checks freeze before looking. Anything else inside the creature element (canvas
  redrawn on a timer, script-driven motion, random values) must not change what the creature looks like for a
  given genome.

## The genome lives in the URL

The URL fragment is the single source of truth for the creature, the way a share link needs.

- A valid fragment is **exactly** `#g=` followed by six single digits `0`-`7` separated by single dots:
  `#g=3.1.4.1.5.2`. Nothing before it in the fragment, nothing after it. Everything else is **invalid**:
  a missing or empty fragment, `#g=`, letters (`a.b.c.d.e.f`), numbers outside 0-7 (`99`, `8`, `-1`), fewer or more
  than six numbers, a leading zero (`03`), other separators (`,` or none), a trailing dot, extra parameters
  (`#g=1.2.3.4.5.6&x=1`), a different key (`#G=` or `#x=`). There is no decoding and no repair, so a
  fragment is either exactly valid or invalid.
- On load, a valid fragment sets the creature. An **invalid or missing fragment shows the default creature
  `0.0.0.0.0.0`**, the page keeps working and logs no errors (no uncaught exception, no `console.error`).
  Nothing may become `NaN`, `null` or `undefined`.
- **The address bar always shows the current creature.** Whenever the creature is not what `location.hash` says
  (the page opened without a fragment or with an invalid one), the fragment is rewritten to
  `#g=` + the genome, so after load, after every `setGenome` and after every mutation
  `location.hash === "#g=" + genome` (for example `"#g=0.0.0.0.0.0"` on a bare page). Only the fragment changes:
  the path and the query string stay as they are. Whether the change adds a history entry is up to you.
- Reloading the page therefore restores the same creature. Do not keep the genome anywhere else (storage, cookies)
  as a replacement for the fragment.
- **The page follows the address bar.** If the fragment changes while the page is open (the user edits the URL, a
  script sets `location.hash`, the browser goes back), the creature, the genome label, the link and the API follow:
  a valid new fragment shows that creature, an invalid one shows the default creature and the address is
  rewritten to `#g=0.0.0.0.0.0`. No reload happens.

## The API

```js
window.__creature = {
  getGenome() {},      // -> array of 6 numbers, e.g. [3, 1, 4, 1, 5, 2]
  setGenome(arr) {},   // -> true if applied, false if refused
}
```

It exists by the time the page's `load` event has fired, and `getGenome` is correct from the first moment.

- `getGenome()` returns a **new array of six JavaScript numbers** (not strings) every time. Changing the returned
  array does not change the creature.
- `setGenome(arr)` accepts only a real **array of exactly six values, each a number that is an integer from 0 to 7**.
  Then it applies it and returns `true`: the creature, the genome label and the link update and the fragment becomes
  `#g=` + the genome. Setting the genome that is already shown is fine and returns `true`. The array is copied:
  changing it afterwards changes nothing.
- For **anything else** `setGenome` returns **`false`**, changes nothing at all (creature, label, link, URL) and
  **does not throw**. That covers: not an array (a string `"1.2.3.4.5.6"`, `null`, `undefined`, an object, no
  argument), wrong length (5 or 7), values that are not numbers (`"3"`, `true`, `null`, `undefined`, a hole in a sparse
  array), numbers outside 0-7, and numbers that are not integers (`1.5`, `NaN`, `Infinity`). Nothing is rounded,
  clamped or coerced.

## Elements

Found by `data-testid`; the page is checked in a desktop-size window.

| `data-testid` | What it is |
|---------------|------------|
| `creature` | the creature itself, visible, at least 96 x 96 px. Everything that makes creatures differ is drawn inside it (DOM, SVG, CSS or canvas) |
| `genome` | an element whose text is exactly the genome as `a.b.c.d.e.f`, for example `3.1.4.1.5.2` (surrounding whitespace is ignored, nothing else in it). Always the current genome |
| `mutate` | a button |
| `copy-link` | an element (a button, an input, whatever you like) that **shows the ready link**: its text, or its value if it is an input, contains the full address of the page, exactly `location.href`, including `#g=…`. It is always up to date and keeps showing the link after it is clicked. Clicking it should copy the link to the clipboard; if the clipboard is unavailable that must fail quietly (no uncaught errors). A "Copied!" note next to or inside it is welcome |

The genome label and the link follow every change of the genome, from any source: mutate, `setGenome`, the
address bar, your own controls.

## Mutation

Pressing `mutate` makes a **mutation** of the current creature:

- The new genome **differs from the old one in at least one number**: a click never does nothing, however many
  times in a row and wherever you start, including `7.7.7.7.7.7` and `0.0.0.0.0.0`.
- Every number of the result is an integer from 0 to 7, always.
- Mutations keep exploring: twenty in a row from any genome visit at least five different genomes (a stuck
  button or a loop between two creatures is not a mutation).
- The effect is **immediate**: by the time the click has been handled `getGenome()`, the label, the link and
  the fragment already show the new genome, and the button works again right away (animate the creature as much as
  you like, but don't lock the button). Because the look depends on the genome, a mutation always changes how the
  creature looks.
- Which numbers change, by how much, randomly or not: yours to design. Small drifts of one or two numbers feel
  like genetics; a total reshuffle feels like a new animal.

## Yours to invent

The theme and everything people will vote on: what kind of creature this is and what the six numbers mean, the
art (SVG, CSS, canvas, emoji, pixels), the look of the lab around it, copy and a name for the species, animation
and sound, a gene editor, a gallery of favourite genomes, a family tree, a way to breed two links. The more
alive it feels, the better. Respect `prefers-reduced-motion`.
