# GoBus

Accessible Metro Transit (Twin Cities) PWA. Go backend with templ + HTMX frontend.

## Tech Stack

- **Backend**: Go 1.22+ with `net/http` routing
- **Templates**: `templ` (type-safe HTML components compiled to Go)
- **Frontend**: HTMX (vendored) + minimal vanilla JS (~3KB)
- **Database**: SQLite with R-Tree (`mattn/go-sqlite3`, requires CGo)
- **Testing**: Go `testing` (unit) + Playwright (E2E)

## Commands

```bash
make build       # Build the binary
make dev         # Run dev server with auto-reload
make generate    # Generate templ files
make test        # Go unit tests
make test-e2e    # Playwright E2E tests (starts Go server)
make test-all    # All tests
make import-gtfs # Force GTFS re-download and import
```

## Project Structure

- `cmd/gobus/` — Entry point
- `internal/config/` — Environment-based configuration
- `internal/server/` — HTTP server, middleware
- `internal/handler/` — HTTP handlers (thin: call transit/storage, render templ)
- `internal/gtfs/` — GTFS download, parse, import pipeline
- `internal/nextrip/` — NexTrip REST API client + cache
- `internal/realtime/` — GTFS-RT protobuf feed polling
- `internal/geo/` — Haversine, bounding box, nearest-stop logic
- `internal/transit/` — Core business logic (**no HTTP/SQL/templ deps** — gomobile-exportable)
- `internal/storage/` — SQLite connection, migrations, queries
- `internal/templates/` — templ components
- `web/static/` — CSS, JS, icons
- `e2e/` — Playwright test suite (Node.js project)

## Architecture Notes

- Server-rendered HTML for accessibility. Screen readers see real DOM, not JS-rendered content.
- HTMX swaps HTML fragments for dynamic behavior. SSE for realtime departure updates.
- `internal/transit/` must stay dependency-free (no HTTP, SQL, or templ imports) — it will become a gomobile library for native iOS/Android apps.
- SQLite R-Tree index on stops for O(log n) geospatial "nearest stop" queries.
- Dark mode is the default theme. Minimum 4.5:1 contrast (WCAG AA), 7:1 for departure times (AAA).

## Conventions

- Use `log/slog` for all logging
- Configuration via environment variables (prefix: `GOBUS_`)
- No external HTTP router — use Go 1.22 `net/http` patterns
- Handlers are thin: business logic goes in `transit/`, data access in `storage/`
- All templ components must use semantic HTML and ARIA attributes
- Test GTFS fixtures live in `e2e/fixtures/gtfs/`
