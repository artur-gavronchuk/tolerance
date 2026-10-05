# Split the bill

Build **Split the bill** as a static web page — a phone app for the moment the dinner is over and somebody
has to work out who owes what. Add the people at the table, add what was ordered and who shared each dish,
pick a tip, and every person sees their exact share. The shares must add up to the cent.

Make it feel like a real native app: it is used with one thumb, so big touch targets, the important
numbers big and always in view (a bottom bar is a good home for the total), inputs that bring up the right
keyboard, instant feedback, small moments of delight. Everyone who opens the gallery uses your entry live,
inside a phone frame, and votes for the one they like most, so the look and feel matter as much as the math.

## The rules (exact — they are checked)

**One screen.** Everything in the table below is reachable by scrolling one page: no tabs, no modals, no
`alert()`/`confirm()` dialogs (they cannot be answered in the checks). Phone first: it must work from
**375 px to 430 px** wide with no horizontal scrolling, and every button, input and checkbox is at least
**44 × 44 px**.

**People**
- Add a person by typing a name and pressing the add button (or Enter in the field). The name is
  **trimmed** (spaces at both ends); it must be 1–24 characters. Otherwise it is rejected: the field keeps its
  text and the error element is shown.
- Names must be **unique ignoring case** (`Ann` and ` ann ` clash). A clash is rejected with the error element too.
- A successful add clears the field and hides the error. The name is shown as typed (trimmed), people stay
  in the order they were added.
- A person can be removed with a button. Removing a person takes them off **every** item (an item's sharers
  just lose that person) and their row disappears.

**Items**
- An item is a name (trimmed, 1–40 characters) and a price typed as text. A price is **digits, optionally
  followed by one `.` or `,` and one or two more digits**: `12`, `12.5`, `12.50`, `12,50`, `0,05`, `007.10`
  are valid, and surrounding spaces are ignored. Everything else is invalid: `abc`, `1.234`, `1,2,3`, `-5`,
  `$5`, `1 000`, `.5`, `5.`, `1e3`, `1,000.00`. The price must be **at least 0.01** and have **at most six
  digits before the separator** (so `0` and `0,00` and `1000000` are invalid; `999999.99` is the largest).
- An invalid name or price is rejected: **no item is added**, the fields keep their text and the error
  element is shown. A successful add clears both fields and hides the error. The same item name may be added
  twice. Items stay in the order they were added.
- Each item is shared by a chosen subset of the people. A **new item is shared by everyone at the table at
  that moment**. Every item shows one checkbox per person (in table order) to toggle that person in or out,
  independently per item. A person who joins **later** is not added to items that already exist (their
  checkbox is unchecked) but is on the next new item.
- An item can be removed with a button.
- An item with **no sharers** (everyone toggled off, or it was added when nobody was at the table, or the
  last sharer was removed) is **flagged** and counts for nothing: it is left out of the subtotal, the tip
  and every share until somebody shares it.

**Tip**
- Presets **0 %, 10 %, 15 %, 20 %**; a new bill starts at **0 %**. A preset button shows `aria-pressed="true"`
  exactly when the current tip percent equals its value, `"false"` otherwise.
- A **custom percent** field takes the same notation as a price (`12.5` or `12,5`, at most two decimals) from
  `0` to `100` inclusive. A valid value is applied immediately. An invalid, non-empty value (`abc`, `101`,
  `-5`, `1.234`, `12%`) shows the tip error element and **leaves the tip unchanged**; emptying the field hides
  the error and leaves the tip unchanged. Choosing a preset empties the custom field.
- **tip = subtotal × percent, rounded half up to the cent**, computed on whole cents — not on floating-point
  dollars (`$10.10` at 15 % is `$1.52`; `$12.30` at 15 % is `$1.85`; `$0.05` at 10 % is `$0.01`).

**Each person's share** (all in cents)
1. A person's *items* are the sum, over the items they share, of the item's price divided **equally** among
   its sharers. A person's *exact share* is their items plus the tip split **in proportion to their items**
   (so someone who ordered nothing pays nothing). Compute these as exact fractions of a cent.
2. Round every exact share **down** to a whole cent.
3. The rounded-down shares are short of `subtotal + tip` by a few cents. Hand them out **one cent each** to
   the people with the **largest fractional remainder** (the part that was cut off); on equal remainders, the
   **person who was added earlier** comes first.
4. The shares therefore **always add up exactly to the grand total = subtotal + tip**. For example `$10.00`
   among three people is `$3.34`, `$3.33`, `$3.33`; one cent between two people is `$0.01` and `$0.00`.

**Display.** Money is shown as `$` + whole dollars with `,` thousands separators + `.` + two digits:
`$0.00`, `$7.10`, `$1,234.56`, `$999,999.99`. Each person's row shows their share; the grand total is always visible.

**Persistence.** The whole bill — people, items, sharers, tip — is kept in `localStorage` and is exactly the
same after a reload. A **New bill** button clears everything at once (no confirmation dialog): nobody, no
items, tip back to 0 %, and it stays cleared after a reload.

## Nice to have

Avatars or colours per person, a bar showing each person's part of the bill, quick "everyone / nobody" toggles
per item, an undo after removing or starting a new bill, a "copy the split to paste into the group chat"
button, haptic-feeling animation when the total changes, a sample bill on the very first launch (outside test
mode, see below) so your gallery card looks alive, a night theme. Respect `prefers-reduced-motion`.

## Rules for the upload

Static files only: an `index.html` at the root of the zip plus any CSS, JS, images and sounds. No server,
no external API keys, no CDNs or web fonts (the checks run without a network). Use relative paths
(`./app.js`, not `/app.js`). Up to 5 MB. A single `.html` file is fine too.

## Automated checks

Your page is opened in Chromium on a phone-sized screen (390 px wide, touch) and driven through acceptance
scenarios; the share it passes is most of the platform score. With **`?test=1`** in the URL a fresh browser
(empty `localStorage`) must start with an **empty bill** — no sample data, no welcome screen in the way. The page must expose
this read-only hook, always (also without `?test=1`):

```js
window.bill = {
  // The current bill, all money in whole cents, people and sharers in table order:
  state() {
    return {
      people: [{ name: 'Ann', share: 334 }, ...],                  // share = the person's share after rounding (rules above)
      items: [{ name: 'Pizza', cents: 1000, sharers: ['Ann', ...] }, ...],  // flagged items have sharers: []
      tipPercent: 15,      // number, e.g. 12.5
      subtotal: 1000,      // only items that have sharers
      tip: 150,
      total: 1150,         // subtotal + tip
    }
  },
}
```

Elements are found by `data-testid`. Where the table says "text is exactly", the element's text (trimmed)
contains nothing but that value. Lists keep the order in which things were added.

| `data-testid` | What it is |
|---------------|------------|
| `new-person` | the text input for a name |
| `add-person` | the button that adds it (Enter in the field does the same) |
| `person-error` | the error for a rejected name; visible only after a rejection |
| `person` | one per person in the list; contains the next three |
| `person-name` | inside `person`: text is exactly the name |
| `person-total` | inside `person`: text is exactly the person's share, like `$3.34` |
| `remove-person` | inside `person`: a button that removes them |
| `new-item-name` | the text input for an item's name |
| `new-item-price` | the text input for its price (Enter in this field adds the item) |
| `add-item` | the button that adds the item |
| `item-error` | the error for a rejected item; visible only after a rejection |
| `item` | one per item in the list; contains the next five |
| `item-title` | inside `item`: text is exactly the item name |
| `item-price` | inside `item`: text is exactly the price, like `$12.50` |
| `remove-item` | inside `item`: a button that removes it |
| `item-flag` | inside `item`: present and visible only while the item has no sharers |
| `sharer` | inside `item`, one **per person** in table order: a checkbox (`<input type="checkbox">`, or an element with `role="checkbox"` and `aria-checked`) that is checked when that person shares the item; it must be clickable and at least 44 × 44 px |
| `tip-0`, `tip-10`, `tip-15`, `tip-20` | the preset buttons, with `aria-pressed` as described |
| `tip-custom` | the custom percent text input |
| `tip-error` | the error for a rejected custom percent; visible only after a rejection |
| `subtotal` | text is exactly the subtotal, like `$30.00` |
| `tip-amount` | text is exactly the tip, like `$4.50` |
| `grand-total` | text is exactly the grand total, like `$34.50` |
| `new-bill` | the button that starts a new bill |

Errors may be hidden with the `hidden` attribute or `display: none`, as long as they are not visible.
A sticky bottom bar must not stop controls from being clicked: scroll them into view above it
(`scroll-padding-bottom` helps). The screen and the hook must agree: what is drawn is what `state()` says.

## Judging

Upload a zip with `index.html` at its root (or a single `.html` file). Within a minute or two the platform
opens your page on a phone-sized screen, takes a screenshot and gives it a **platform score** out of 100:

- 70 points: the share of the acceptance scenarios above that pass;
- 30 points: quality — accessibility (axe-core), no horizontal scroll and big enough tap targets at 375 px,
  no JavaScript errors, a fast and light first load.

Your entry then appears in the public gallery where everyone can use it, inside a phone frame, and people
vote for the ones they like. You can upload again at any time; your latest upload replaces the previous one
and keeps its votes.
