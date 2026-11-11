# FRACTAL frontend redesign

A calm, PayPal-inspired interface for the payment-exception workflow. This document
records the design decisions, the shared components, and the places where the UI stops
because the backend does not expose what the screen needs yet.

The redesign is visual and interaction-only. Routes, API calls, data models, auth,
role logic, and business rules are unchanged.

## Principles

- **One job per screen.** The case list helps you triage. The case page helps you
  resolve one exception. Nothing competes for attention.
- **Calm by default.** White surfaces on a soft gray canvas, one accent blue, and a
  single yellow primary action per screen. Yellow is never used for status.
- **Status is never color-only.** Every status, risk and decision pill pairs a tinted
  background with a dot and a text label. Lists also carry a left rule for cases that
  need action.
- **Plain language.** No `SNAKE_CASE`, no full UUIDs, sentence case throughout. Raw
  enums are mapped in `src/lib/format.ts` before they reach the DOM.
- **Progressive detail.** Evidence and permissions are collapsed until asked for.
  Destructive or state-changing demo actions ask for confirmation.
- **Motion is feedback, not decoration.** Only short fades for status changes, menus
  and message sends. All motion is disabled under `prefers-reduced-motion`.

## Design tokens

Everything lives in `src/index.css` and `tailwind.config.js`; components use the
semantic names, never raw colors.

| Token | Light | Dark |
| --- | --- | --- |
| `--bg` | `#f5f7fa` | `#0b1220` |
| `--surface` | `#ffffff` | `#121a2b` |
| `--surface-muted` | `#f1f4f8` | `#182238` |
| `--border` / `--border-strong` | `#e1e7ec` / `#cbd2d9` | `#243049` / `#33415e` |
| `--text` / `--text-muted` | `#2c2e2f` / `#6c7378` | `#e8ecf3` / `#94a0b5` |
| `--brand-navy` | `#003087` | `#8fb4ff` |
| `--brand-blue` | `#0070ba` | `#4da3e8` |
| `--brand-yellow` | `#ffc439` | `#ffc439` |
| `--success` / `--warning` / `--danger` / `--info` | `#0b7a43` / `#a15c00` / `#c5221f` / `#0070ba` | `#4cc38a` / `#f5b94a` / `#f28b87` / `#4da3e8` |

Type is Inter only (`400`, `500`, `600`), with five sizes: `meta` 13/18, `body` 14/22,
`section` 18/26, `title` 28/36, `hero` 32/40. Radii are 6/10/16px plus a pill.
Numbers that can change use `tabular-nums`; identifiers use the system mono stack.

Theme follows `prefers-color-scheme` on first load, then a toggle stored in
`localStorage` under `fractal-theme` (wrapped in `try/catch`). A tiny inline script in
`index.html` sets `data-theme` before first paint so there is no flash.

## Shell

`src/components/Layout.tsx` owns the whole frame:

- A single search field in the top bar. It feeds a React context; the dashboards filter
  their existing payload client-side. `/` focuses it, `Esc` clears it.
- One nav entry per role destination. Full sidebar at `lg`, an icon rail at `sm`–`lg`,
  and a bottom tab bar below `sm`. `g c` jumps to the case list.
- Theme toggle, account menu with keyboard shortcuts and sign out, and a
  **Demo mode** tag on `/demo`.
- Login renders outside the shell.

The avatar menu and the case-detail "more" menu close on outside click and `Esc`.
Dialogs trap focus, restore focus on close, and close on `Esc`.

## Components

New or rebuilt primitives in `src/components/ui/`:

- `Button` / `IconButton` — pill buttons; `primary` is the only yellow variant.
- `Input` / `Textarea` / `Select` — labelled fields with hint and error wiring.
- `StatusPill` / `Tag` — tinted status with a dot and label.
- `Panel` / `PageHeader` / `PanelHeader` — page and section structure.
- `Disclosure` — height-animated evidence and permission groups.
- `CopyChip` — truncated identifier plus copy-to-clipboard, with an icon-only mode.
- `Dialog` / `ConfirmDialog` — focus-managed modal and destructive confirmation.
- `Skeleton`, `EmptyState`, `ErrorNotice` — loading, empty and failure states.
- `Timeline`, `Stepper`, `Highlight`, `Tooltip` — supporting views.
- `Conversation` (shared) — chat bubbles, optimistic send, composer, and a disabled
  attachment affordance.

Old `Card`, `Badge` and `Icons` components were removed; `lucide-react` is now the only
icon source.

## Screens

### Merchant case list

- A four-cell summary strip replaces stat cards. Each cell is a filter toggle:
  total, needs approval, investigating, escalated. Zero counts stay visible but muted.
- Toolbar with status, issue type and sort. "Needs attention first" is the default and
  puts escalated cases above approval requests, then most recently updated.
- Search is debounced and highlights matches in the case number, customer and order.
- Table columns: case number, customer, order (+ store status), issue type, status,
  updated. Below `xl`, the same data becomes a two-column card grid.
- Action-needed rows get a warning/danger left rule and are ordered first.
- Skeleton rows while loading; a distinct empty state for "no cases" vs "no matches".

### Merchant case detail

- Sticky header with case number, status, opened time and the single primary
  **Reconcile order** action. The action is disabled with an explanatory tooltip when
  the case is resolved or not reconcilable.
- Two-column layout (`1fr` + 320px) above `xl`; one column below.
- **Payment vs order** panel: amounts, statuses, provider order/capture and order total,
  with an "In sync" / "Out of sync" summary and highlighted differences.
- **AI investigation** panel: category, hypothesis, suggested risk and a collapsed
  evidence list. Marked advisory.
- **PolicyGuard decision** panel: outcome, rule, revision, checks, pending approvals and
  inline failure recovery. Reconcile runs through the existing action endpoint.
- **Conversation** and **activity** timeline; merchant-only events are labelled and can
  be hidden. Permissions open in a small popover instead of taking a card.

### Customer case list / detail

- The same list system in customer language: merchant, payment, order, status.
- Detail opens with a plain-language headline for the current status and a four-step
  stepper (opened → evidence → investigating → resolved).
- Payment and store order sit side by side.
- Conversation supports optimistic sending. The merchant-visible timeline only shows
  customer-visible events.
- "What you can do" explains permissions in a popover.

### Report an issue

- Issue type is chosen with four plain-language radio cards. The API still receives the
  same case-type values.
- Order reference, optional store ID, email and claim code retain their existing fields
  and validation. Description has a character count.
- Success toasts and opens the newly created case.

### Demo controls

- Grouped into webhook handling, replay, seed scenarios and charts.
- Every state-changing or destructive demo action asks for confirmation, with the
  wording stating what will and will not change.
- Charts use the design tokens; simulated behaviour is unchanged.

### Login

- Role selection, email, and one yellow **Continue** action.
- Demo accounts fill the form; no password is requested in dev mode.
- Errors are surfaced inline in plain language.

## Accessibility

- Semantic landmarks, one `h1` per page, labelled form fields, and a skip link.
- Focus is always visible; dialogs trap and restore focus.
- Status uses text plus dot, never color alone; risk and decision pills follow the same
  rule. Timelines mark merchant-only entries with a lock and text.
- `aria-live` on status changes and the conversation log.
- Reduced motion is respected globally.
- Contrast targets WCAG AA for body and meta text in both themes.

## Responsive notes

Captured and checked at 1440, 1024, 768 and 360 in both themes.

- `xl` and up: sidebar, tables and the two-column case layout.
- `sm`–`lg`: icon rail, card grids, single-column case pages.
- below `sm`: bottom tab bar, stacked filters, single-column cards.

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
- **Demo side effects** remain simulated as before; the confirmation dialogs describe
  that nothing external is changed.

## Screenshots

Side-by-side comparisons (1440px, light and dark) are in
[`docs/design/comparisons`](design/comparisons):

- `login-light.png`, `login-dark.png`
- `merchant-cases-light.png`, `merchant-cases-dark.png`
- `merchant-case-detail-light.png`, `merchant-case-detail-dark.png`
- `customer-case-detail-light.png`, `customer-case-detail-dark.png`
- `report-issue-light.png`, `report-issue-dark.png`
- `demo-controls-light.png`, `demo-controls-dark.png`

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
