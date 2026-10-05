# Space Hotel

Build a landing page for **Apogee**, a (fictional) hotel in low Earth orbit, as a static web page.
Everyone who opens the gallery can open your entry and vote for the one they like most, so the looks
matter as much as the rules: a hero that makes people stop scrolling, rooms with a window onto the Earth,
a booking calculator that feels good to poke at, and a dark theme that is not an afterthought. All the art is
yours to draw with **CSS and inline SVG only** (stars, a station, a planet, a sunrise over the horizon). No
external images, fonts, icon sets or CDNs; the checks run without a network.

The page has a few working parts. They are small, and every one of them is checked exactly.

## The rules (exact — they are checked)

### Page

- One `index.html`. The sections have the ids `rooms`, `booking` and `faq` (the elements the nav links point at).
- The header holds the hotel name, the nav and the theme toggle. The nav has three plain anchor links:
  `#rooms`, `#booking`, `#faq`. Following one puts that section into view and updates `location.hash`.
- The rooms section shows **three room cards** (`room-card`). Each card shows the room's name and its nightly
  price in the same format as below (`$895`, `$1,999`, `$6,495`).
- Nothing scrolls sideways at 375 px wide, whatever is open.

### Header and burger menu

- At **1024 px** wide the three nav links are visible, the burger button (`menu-toggle`) and the mobile menu
  (`mobile-menu`) are not.
- At **375 px** wide the nav links are not shown; the burger button is. The burger has `aria-expanded="false"`
  and the mobile menu is closed. Closed means really not displayed (`display: none`, the `hidden` attribute or
  `visibility: hidden`), not just moved off screen.
- Clicking the burger opens the menu (`aria-expanded="true"`, `mobile-menu` visible, with the three links
  `menu-rooms`, `menu-booking`, `menu-faq`) and clicking it again closes it.
- Clicking a link in the open menu **closes the menu** (`aria-expanded` back to `"false"`) and goes to its
  section. **Escape** also closes the open menu.

### Booking calculator

- `room` is a `<select>` with three options, in this order, with these `value`s and nightly prices in US dollars:

  | value | room | per night |
  |-------|------|-----------|
  | `pod` | Orbit Pod | $895 |
  | `suite` | Horizon Suite | $1,999 |
  | `dome` | Zenith Dome | $6,495 |

- `nights` is a text or number input for a whole number of nights, **1 to 30**. On load it contains `3` and
  `pod` is selected.
- `total` shows the price of the stay. Its text is exactly the amount and nothing else, for example `$5,185`.
  The rule:

  1. `stay = nights × nightly price`
  2. discount on the stay: **0%** for 1–6 nights, **10%** for 7–13 nights, **20%** for 14–30 nights
  3. `discounted = stay × (100 − discount) / 100`, **rounded to the nearest whole dollar, halves round up**
     (`8,158.5` becomes `8,159`; `12,593.7` becomes `12,594`; `19,790.1` becomes `19,790`). Beware of floating
     point: do the arithmetic in integers.
  4. `total = discounted + 2,500`. The **launch fee of $2,500** is added once per booking and is never discounted.

  Examples: Orbit Pod, 7 nights → `$8,139`; Horizon Suite, 14 nights → `$24,889`; Zenith Dome, 30 nights →
  `$158,380`.
- The amount is formatted with a dollar sign and **a comma between thousands**: `$3,395`, `$21,985`,
  `$158,380`. No cents, no spaces.
- The total updates **while the user types** (on every `input` event, no blur and no button needed) and when
  the room changes.
- Invalid nights: empty, `0`, anything above 30, a fraction (`2.5`), a negative number or text. Then
  `nights-error` is visible (with a non-empty message) and `total` shows an em dash, `—` (U+2014), alone.
  With valid nights `nights-error` is hidden. Picking another room while the nights are invalid keeps the dash.

### Newsletter

- `email` is the input and `subscribe` the submit button of a form. The `email-error` and `subscribe-success`
  elements are hidden until the first submit.
- A valid email is **something@something.tld**: no spaces, exactly one `@`, and at least one `.` in the part after
  it with something on both sides of the last dot, i.e. the whole value (after trimming surrounding spaces)
  matches `^[^\s@]+@[^\s@]+\.[^\s@]+$`. So `a@b.c` is valid, `a@b`, `a@b.`, `a b@c.de`, `@x.io` and `a@@b.co`
  are not. (The browser's own `type="email"` check accepts `a@b`, so do not rely on it.)
- Submitting (the button, or Enter in the field) with an invalid email shows `email-error` and hides
  `subscribe-success`; with a valid email it shows `subscribe-success`, **whose text contains the address
  entered**, and hides `email-error`. The newest result always replaces the older one.
- The page does **not reload or navigate**: the same document stays, and the URL gets no query string.

### FAQ

- At least **5** `faq-item`s. Each one has a `faq-question` (a `<button>`, so Enter and Space work) with
  `aria-expanded="true"` or `"false"`, and a `faq-answer` that is visible only while its question is open.
- All of them are closed on load.
- Opening a question **closes the other open one**, so at most one is open at a time. Clicking the open
  question closes it.

### Theme

- `theme-toggle` is a button. The page's `<html>` element always carries `data-theme="light"` or `data-theme="dark"`.
- With nothing saved, the theme follows the system's `prefers-color-scheme`.
- Each click flips the theme and saves it in `localStorage` under the key `theme` with the value `"dark"` or
  `"light"`. A saved value is applied on every load and wins over the system setting, so a reload keeps the choice.
- The toggle really changes the look: the background colour (set on `<html>` or `<body>`) is different in the
  two themes.

## Nice to have

Make it gorgeous: a starfield that twinkles, a station that slowly turns, a planet, aurora in the dome, a
sunrise over the Earth's curve, smooth FAQ and menu animations, a sticky blurred header, a breakdown of the
price under the total, balanced typography and good contrast in both themes. Keep it accessible: labels on the
fields, `aria-invalid` and `role="alert"` on errors, visible focus rings. Respect `prefers-reduced-motion`.

## Rules for the upload

Static files only: an `index.html` at the root of the zip plus any CSS, JS, images and SVG. No server, no
external API keys, no CDNs or web fonts (the checks run without a network). Use relative paths
(`./style.css`, not `/style.css`). Up to 5 MB. A single `.html` file is fine too.

## Automated checks

Your page is opened in Chromium (no network) at 1024 px and 375 px wide and driven through acceptance
scenarios; the share it passes is most of the platform score. There is no test mode and no `window` hook:
everything is checked through the page itself. Elements are found by `data-testid`:

| `data-testid` | What it is |
|---------------|------------|
| `nav-rooms`, `nav-booking`, `nav-faq` | the header links (visible at 1024 px, not shown at 375 px) |
| `menu-toggle` | the burger button, `aria-expanded` `"true"`/`"false"`, shown at 375 px only |
| `mobile-menu` | the menu opened by the burger |
| `menu-rooms`, `menu-booking`, `menu-faq` | the three links in the mobile menu |
| `room-card` | one of the three room cards (name and nightly price inside) |
| `room` | the `<select>` with the options `pod`, `suite`, `dome` |
| `nights` | the nights input, initial value `3` |
| `nights-error` | shown while the nights are invalid, hidden otherwise |
| `total` | the total, text exactly like `$5,185` or `—` |
| `email` | the newsletter email input (inside a `<form>`) |
| `subscribe` | the newsletter submit button |
| `email-error` | shown after a submit with an invalid email |
| `subscribe-success` | shown after a valid submit, contains the email |
| `faq-item` | one FAQ entry, at least 5; contains its `faq-question` and `faq-answer` |
| `faq-question` | the button that opens the entry |
| `faq-answer` | the answer, visible only while open |
| `theme-toggle` | the light/dark button |

The checks type into `nights` and `email` like a person would (so a text input and a number input both work).

## Judging

Upload a zip with `index.html` at its root (or a single `.html` file). Within a minute or two the platform
opens your page, takes a screenshot and gives it a **platform score** out of 100:

- 70 points: the share of the acceptance scenarios above that pass;
- 30 points: quality — accessibility (axe-core), no horizontal scroll and big enough tap targets at 375 px,
  no JavaScript errors, a fast and light first load.

Your entry then appears in the public gallery where everyone can open it, and people vote for the ones
they like. You can upload again at any time; your latest upload replaces the previous one and keeps its
votes.
