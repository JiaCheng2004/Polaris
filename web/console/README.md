# Polaris Console

Optional admin and observability SPA for a Polaris gateway. It talks to the gateway's
`/v1` API from the browser and holds no state of its own. Deployment and authentication
are covered in [`docs/CONSOLE.md`](../../docs/CONSOLE.md).

## Run

```bash
npm install
npm run dev          # http://localhost:5173
```

Connect with your gateway base URL and an admin token — the bootstrap admin key, or an
`is_admin` virtual key. (`/v1/admin/*` analytics need `control_plane.enabled`.)

## Build

```bash
npm run build        # static assets → dist/ ; serve from any static host
```

Or embed it in the gateway binary and serve it with `--console`:

```bash
make build-console   # from the repo root
```

## Stack

- React 18 + Vite + TypeScript. TanStack Query for server state; React Router for routing.
- GSAP (`@gsap/react`) for Standard-mode motion; Phosphor for icons.
- Charts are hand-rolled SVG (no chart library) drawn from the validated,
  colorblind-safe dataviz palette in `src/design/tokens.css`.

## Design system — two independent axes

Both are set as data-attributes on `<html>` and stored in `localStorage`:

- **`data-theme`**: `light` | `dark`.
- **`data-mode`**: `standard` (minimalist, experience-first — curated, airy, animated)
  | `advanced` (industrial telemetry, information-first — raw, dense, static).

Panels read the mode (`useTheme()`) and render genuinely different content per mode:
Standard shows curated highlights with count-ups and staggered reveals; Advanced shows
exact figures, more charts, and full tables in a hairline grid with no motion. Chrome
tokens vary by mode × theme; chart tokens vary by theme only, so data reads as one
accessible system in both aesthetics.

## Layout

```
src/
  api/         HTTP client, SSE stream, resource calls, response normalizers, types
  components/  shell (Sidebar / TopBar / nav), DataTable, Modal, Toast, Filters, ui primitives
  design/      tokens.css (theme × mode) + base.css (element/shell styles)
  motion/      GSAP hooks (reveal + count-up), gated to Standard mode
  panels/      one file per route (Overview, Usage, Providers, Models, Audit,
               Projects, Keys, Policies, Budgets, Tools, MCP, Playground, Settings)
  state/       theme/mode context + connection context (base URL + session token)
  viz/         SVG chart primitives (StatTile, TimeSeries, BarList, Meter, Sparkline)
```

## Checks

```bash
npm run typecheck    # tsc --noEmit (strict, noUnusedLocals/Parameters)
npm test             # vitest
```
