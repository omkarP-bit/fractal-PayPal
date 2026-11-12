# FRACTAL frontend redesign (v2)

A fluid, neo-brutalist interface for the payment-exception workflow: warm paper,
PayPal blue and yellow, ink outlines, hard offset shadows, and tables that use the
whole window. This document records the v2 tokens, layout rules, components, and the
places where the UI stops because the backend does not expose what the screen needs.

The redesign is visual and interaction-only. Routes, API calls, data models, auth,
role logic, and business rules are unchanged.

## Principles

- **One job per screen.** The case list helps you triage. The case page helps you
  resolve one exception. Nothing competes for attention.
- **Fill the window, don't centre a column.** Data pages stretch to the app cap; only
  short forms and sign-in keep a reading width.
- **Flat, then outlined.** Status is a saturated fill with dark ink text, a square dot
  and a text label. Surfaces are paper or ink-outlined panels, never blurred glass.
- **Primary action is loud.** Yellow is reserved for the one primary action per screen
  and always keeps ink text at full opacity.
- **Status is never color-only.** Every status, risk and decision tag carries a text
  label; tables also mark action-needed rows with a left rule.
- **Plain language.** No `SNAKE_CASE`, no full UUIDs, sentence case throughout. Raw
  enums are mapped in `src/lib/format.ts` before they reach the DOM.
- **Motion is feedback, not decoration.** Buttons lift 2px and press 4px, menus fade,
  skeletons step. All translation is removed under `prefers-reduced-motion`.

## Design tokens

Everything lives in `src/index.css` and `tailwind.config.js`; components use the
semantic names, never raw colors. Legacy utility names (`surface-muted`, `line`, radii)
are mapped to v2 values so no screen keeps the old soft look by accident.

| Token | Light | Dark |
| --- | --- | --- |
| `--bg` | `#f4f1e8` | `#0a0e17` |
| `--surface` | `#ffffff` | `#131a2a` |
| `--surface-alt` (alias `--surface-muted`) | `#e9eef6` | `#1a2338` |
| `--ink` | `#0b0f19` | `#e8ecf3` |
| `--text` / `--text-muted` | `#0b0f19` / `#4a5568` | `#f2f5fa` / `#a3aec2` |
| `--brand-navy` | `#003087` | `#8fb4ff` |
| `--brand-blue` | `#0070ba` | `#4da3e8` |
| `--brand-sky` | `#009cde` | `#009cde` |
| `--brand-yellow` | `#ffc439` | `#ffc439` |
| `--success` / `--warning` / `--danger` / `--info` / `--neutral` | `#00a86b` / `#ffc439` / `#ff4d4f` / `#009cde` / `#d5dce6` | same fills, dark ink pairing |
| `--shadow-hard` | `4px 4px 0 #0b0f19` | `4px 4px 0 #009cde` |
| `--shadow-hard-lg` | `6px 6px 0 #0b0f19` | `6px 6px 0 #009cde` |

Shape rules: **2px** ink borders, **3px** on panels at `1440px` and up; **4px** radius
(2px on tags); zero-blur shadows. Status text uses the matching `*-ink` token so the
flat fills clear WCAG AA. `--danger-text` exists for error copy on paper/surface.

Type is Inter (`500`, `700`, `800`) with JetBrains Mono for case IDs, order refs, times
and key hints. `meta` 12/16, `body` 14/22 (15/24 at `1440px`+), `section` 20/28,
`title` 36/40 (44/48 at `1440px`+), `hero` 40/44. Numbers use `tabular-nums`.

Theme follows `prefers-color-scheme` on first load, then a toggle stored in
`localStorage` under `fractal-theme` (wrapped in `try/catch`). A tiny inline script in
`index.html` sets `data-theme` before first paint so there is no flash.

## Layout

`src/index.css` owns the fluid rules:

- `.app-frame` caps the whole app at **2200px** and stays left-aligned above that.
- `.app-shell` is a CSS grid: `64px` icon rail from `640px`, `232px` sidebar from
  `1024px`, `minmax(0, 1fr)` for the page.
- `.app-main` uses `padding: clamp(16px, 3vw, 48px)` (48px from ~1600px) and
  `min-width: 0`.
- `.page-grid` is a 12-column grid with a 24px gap; detail pages use **8 / 4** columns
  from `1024px` and stack below it.
- `.list-layout` is one column with a **320px insight rail** from `1920px`; the rail is
  omitted below that so the table keeps the full width.
- `.page-sticky-header` pins the case header under the 4rem top bar.
- Row heights step `56px` → `64px` (`1440px`) → `72px` (`1920px`).
- Stat blocks use container queries: 4-up when the strip has 720px, 2-up below,
  and a snap-scrolling row under 520px.
- Payment-vs-order uses a container query to split at 640px and stack with a 2px rule
  below it.

The page primary action sits in the same column as the table, so its right edge lines
up with the table's right edge at every width, including beside the 1920px rail.
List footers (`Showing x of y`, load more, page count) live inside the table panel.
A dashed hint block anchors the bottom of the list column on tall screens.

## Shell

`src/components/Layout.tsx` owns the whole frame:

- A rectangular search field in the top bar. It feeds a React context; the dashboards
  filter their existing payload client-side. `/` focuses it, `Esc` clears it.
- One nav entry per role destination. Full sidebar at `lg`, an icon rail at `sm`–`lg`,
  and a bottom tab bar below `sm`. `g c` jumps to the case list.
- A brutalist brand square, a flat **Demo mode** tag on `/demo`, theme toggle, account
  menu with keyboard shortcuts and sign out.
- Login renders outside the shell.

The avatar menu and the case-detail "more" menu close on outside click and `Esc`.
Dialogs trap focus, restore focus on close, and close on `Esc`.

## Components

New or rebuilt primitives in `src/components/ui/`:

- `Button` / `IconButton` — 44px controls; `primary` is the only yellow variant and
  always uses ink text. Disabled primaries fall back to the neutral fill with a dashed
  border and no shadow. Raised buttons lift/sink; text buttons stay flat.
- `Input` / `Textarea` / `Select` — 2px fields; focus adds a 3px sky outline plus a
  hard sky shadow. Errors use `--danger-text` plus an icon.
- `StatusPill` / `Tag` — flat fill, ink text, square dot, uppercase label.
- `StatBlock` — clickable filter summary; zero counts are dashed and muted, the active
  filter gets a sky outline.
- `Panel` / `PageHeader` / `PanelHeader` — paper panels with hard shadows and heading
  hierarchy.
- `InsightRail` — "Needs your attention" and "Recent activity" panels for `1920px`+.
- `Disclosure` — height-animated evidence and permission groups.
- `CopyChip` — truncated identifier plus copy-to-clipboard, with an icon-only mode.
- `Dialog` / `ConfirmDialog` — focus-managed modal and destructive confirmation.
- `Skeleton`, `EmptyState`, `ErrorNotice` — loading, empty and failure states.
- `Timeline`, `Stepper`, `Highlight`, `Tooltip` — supporting views.
- `Conversation` (shared) — bordered chat bubbles (navy for your own messages),
  optimistic send, composer, and a disabled attachment affordance.

Old `Card`, `Badge` and `Icons` components were removed; `lucide-react` is now the only
icon source.

## Screens

### Merchant case list

- Four stat blocks (total, investigating, needs approval, escalated) are also filter
  toggles. Zero counts stay visible but dashed and muted.
- Filters (status, issue type, sort) sit below the stat strip so the strip never
  scrolls on desktop. "Needs attention first" is the default and puts escalated cases
  above approval requests, then most recently updated.
- Search is debounced and highlights matches in the case number, customer and order.
- Table columns: case number, customer (+ email), order (+ store status), issue type,
  status, updated. Below `xl`, the same data becomes a two-column card grid.
- Action-needed rows get a warning/danger left rule. Footer inside the panel shows the
  visible count and a load-more control.
- At `1920px`+, a 320px rail lists the cases that need a decision and recent activity.

### Merchant case detail

- Sticky header with case number, status, opened time and the single primary
  **Reconcile order** action. The action is disabled with an explanatory tooltip when
  the case is resolved or not reconcilable.
- 12-column layout: main 8 / side 4 with a 24px gap from `1024px`; one column below.
- **Payment vs order** panel with a navy title strip and an "In sync" / "Out of sync"
  tag. Amounts, statuses, provider order/capture and order total sit in a split panel
  with a 2px divider; differences get a flat warning band.
- **AI investigation** panel: category, hypothesis, suggested risk and a collapsed
  evidence list. Marked advisory.
- **PolicyGuard decision** panel: a flat outcome block, rule, revision, checks, pending
  approvals and inline failure recovery. Reconcile runs through the existing endpoint.
- **Conversation** and **activity** timeline; merchant-only events are labelled and can
  be hidden. Permissions open in a small bordered popover instead of taking a card.

### Customer case list / detail

- The same list system in customer language: merchant, payment, order, status. The
  primary **Report an issue** action aligns with the table's right edge.
- Detail opens with a plain-language headline and a four-step stepper
  (opened → evidence → investigating → resolved).
- Payment and store order share a container-query split with large tabular amounts.
- Conversation supports optimistic sending; your messages use a navy fill. The
  customer timeline only shows customer-visible events.
- "What you can do" explains permissions in a popover.

### Report an issue

- Left-aligned 720px form; "What happens next" and a pre-submit checklist sit beside it
  from `1280px`.
- Issue type is chosen with four plain-language radio cards. The API still receives the
  same case-type values.
- Order reference, optional store ID, email and claim code retain their existing fields
  and validation. Description has a character count.
- Evidence upload is shown as a dashed unavailable block with an explanation.
- Success toasts and opens the newly created case.

### Demo controls

- Grouped panels with flat title strips: webhook handling, concurrency, replay, seed
  scenarios, live webhook activity and a danger-bordered reset section.
- The old mock observability charts are gone. **Live webhook activity** polls real,
  merchant-scoped event data every 10 seconds; **Live Kibana** links to the real
  dashboard instead of drawing fake latency or replay charts.
- Every state-changing or destructive demo action asks for confirmation, with wording
  that states what will and will not change. Those scenario buttons remain simulated.

### Merchant onboarding

- `/signup` uses the same paper panel, yellow primary action, ink borders and hard
  shadows as the rest of v2. A three-step `Stepper` shows business/region, Connect
  PayPal and webhook live.
- Region is a required select. Currency is derived from the region and is not silently
  defaulted to USD; cross-border-only regions show a warning strip with the limitation.
- The Connect PayPal panel links to the PayPal Developer Dashboard and states that
  FRACTAL consumes an app the merchant creates; it cannot generate Client IDs or
  Secrets. Webhook registration requires a public HTTPS origin, so the panel explains
  `FRACTAL_PUBLIC_URL` / `PUBLIC_TUNNEL_URL` instead of surfacing PayPal's opaque
  "Invalid data provided" error.
- The authenticated `/merchant/onboarding` screen reuses the detail-page pattern: a
  main column for business/region and webhook status, plus a "What happens next" rail.
- Secrets use `type="password"`, are cleared after submit, and are never echoed back.
  Connected state shows only the public webhook URL, webhook ID, environment and dates.
- Long webhook URLs truncate inside their row and keep a copy chip.

### Login

- A 440px paper panel: role tiles, email, and one yellow **Continue** action.
- Demo accounts fill the form; no password is requested in dev mode.
- Merchant accounts show an optional password field for self-service signups. Seeded
  demo accounts leave it blank and keep the passwordless dev path.
- A plain signup link leads to `/signup`. Errors are surfaced inline in plain language.

## Accessibility

- Semantic landmarks, one `h1` per page, labelled form fields, and a skip link.
- Focus is always visible with a **3px sky** outline and 3px offset; table rows carry
  their own inset focus ring. Dialogs trap and restore focus.
- Status uses text plus a square dot, never color alone; risk and decision tags follow
  the same rule. Timelines mark merchant-only entries with a lock and text.
- `aria-live` on search result counts, status changes and the conversation log.
- Touch targets are at least 44px (nav tabs, icon buttons, primary controls).
- Reduced motion removes every translate and animation.
- Contrast checks: body `16.9:1`, muted `6.7:1`, yellow/ink `12.0:1`, success ink `5.3:1`,
  danger ink `5.3:1`, info ink `5.1:1`, dark muted `8.6:1`, dark danger copy `9.5:1`.
  All status pairs clear WCAG 2.2 AA in both themes.

## Responsive notes

Captured and checked at **360, 768, 1024, 1440, 1920 and 2560** in light and dark
(`docs/design/v2`). No horizontal page overflow at any tested width.

- `1920px`+: 48px page padding, 72px rows, 320px insight rail, 2200px app cap.
- `1440px`–`1919px`: 64px rows, 3px panel borders, rail hidden, table full width.
- `1024px`–`1279px`: 232px sidebar, 12-column detail grid, card lists.
- `640px`–`1023px`: 64px icon rail, stacked detail pages, two-column cards.
- below `640px`: bottom tab bar, stacked actions, snap-scrolling stat strips,
  single-column cards.

## Backend TODOs

These are marked with `// TODO(backend):` in the source:

- Evidence upload from the conversation composer (`src/components/Conversation.tsx`).
- A searchable order picker to replace the manual order-reference field
  (`src/pages/customer/NewCase.tsx`).
- Evidence upload/drag-and-drop on the report form (`src/pages/customer/NewCase.tsx`).

## Deliberate omissions

- **Evidence upload** is disabled until an upload endpoint exists.
- **Order picker** is not faked; the manual reference remains.
- **Undo** for reconcile is not offered because the API has no undo operation.
- **Pending approvals** are read-only here; there is no approval endpoint in this UI, so
  the panel explains where to decide.
- **Row preview drawer** was skipped in favour of direct navigation.
- **Demo side effects** remain simulated as before; confirmation dialogs describe that
  nothing external is changed.
- **Rail below 1920px** is hidden rather than stacked, so the table keeps the full
  width and the page does not repeat the stat strip.
- **Mobile table** becomes cards below `1280px`; columns are not horizontally scrolled.

## Screenshots

The v2 set is in [`docs/design/v2`](design/v2), named
`{screen}-{width}-{theme}.png`:

- `merchant-cases`, `merchant-case-detail`, `demo-controls`
- `customer-cases`, `customer-case-detail`, `report-issue`, `login`
- full light/dark pairs at `1440`, `1920`, `2560`
- light spot checks at `360`, `768`, `1024`

The earlier v1 comparisons remain in [`docs/design/comparisons`](design/comparisons)
for before/after reference.

## Verifying

```bash
cd web
npm run build
cd ..
docker compose build web && docker compose up -d web
```

Open <http://localhost:5173>. The dev logins are `buyer@example.com` (customer) and
`ops@acme.com` (merchant). Use `/` to search, `g c` to jump to cases, and `Esc` to clear
search or close a dialog.
