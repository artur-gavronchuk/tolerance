# Kanban board

Build a **kanban board** as a static web page: three columns (To do, Doing, Done), cards you add, rename,
delete and move around, all kept in the browser.

## Must have

- A text input and an Add button. Pressing Enter in the input also adds. A new card goes to the **end of To do**;
  the title is trimmed, a blank title adds nothing, and the input is cleared afterwards.
- Cards can be **renamed**: double-click the title, edit it in place, Enter saves, Escape cancels. A blank
  title is refused (the old one stays). A card can be **deleted** (a button on the card; no `confirm()` dialog).
- Moving a card:
  - **Drag and drop**: dropping a card anywhere on a column, including on top of another card in it, moves it
    to that column.
  - **Keyboard**: cards are focusable. With a card focused, `Shift+ArrowRight` / `Shift+ArrowLeft` move it to
    the next / previous column (to the end of that column; nothing happens at the first and last column), and
    `Shift+ArrowUp` / `Shift+ArrowDown` move it one place up / down inside its column. After a move the
    card keeps keyboard focus so the key can be pressed again.
- Each column shows how many cards it holds.
- Everything (cards, columns, order) is saved in `localStorage` and is still there after a reload. Cards are
  identified by an id, not by their title: two cards may have the same title.
- Titles are plain text. `<b>x</b>` must be shown literally and never interpreted as HTML.
- Works on a phone screen (375 px wide) without horizontal scrolling of the page, even with long unbroken titles.

## Nice to have

- Smooth drag feedback, a drop-target highlight, an empty-column hint.
- Dark mode, tasteful typography, animations that respect `prefers-reduced-motion`.
- Undo for deleting a card.

## Rules

Static files only: an `index.html` at the root of the zip plus any CSS, JS and images. No server, no
external API keys, no external CDNs or fonts (the checks run without a network). Use relative paths
(`./app.js`, not `/app.js`). Up to 5 MB.

## Automated checks

Your page is also opened in Chromium (no network) and driven through acceptance scenarios; the share it
passes is shown next to the votes. The scenarios find elements by `data-testid`:

| `data-testid` | What it is |
|---------------|------------|
| `new-card-input`, `add-card` | the title `<input>` and the Add button |
| `column-todo`, `column-doing`, `column-done` | the three column containers; every card is a descendant of its column |
| `count-todo`, `count-doing`, `count-done` | the card count of each column (text contains the number) |
| `card` | one card, `tabindex="0"` (focusable); its dragging works with a real mouse |
| `card-title` | inside a card; its text is exactly the title |
| `delete-card` | inside a card; click deletes it |
| `edit-input` | the `<input>` that replaces the title while renaming (prefilled with the title) |

Cards appear in a column in their board order (top to bottom in the DOM). Drag is driven with the mouse
(press, move, release), so both HTML5 drag and drop and pointer-event implementations work.

## Judging

Votes decide. After the deadline every site is shown to everyone and people vote; the automated score is
shown beside the votes and breaks ties. Up to 3 uploads; your latest counts. A scored upload appears after
the checks finish (a minute or so).
