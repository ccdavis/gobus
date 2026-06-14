# GoBus

Accessible Metro Transit (Twin Cities) transit app. **Local-first, single-user:**
the Go core runs on-device and is hit by a local browser (desktop build) or an
in-app `WKWebView` (native iPhone build, via gomobile). No login; all user
settings live in the local SQLite file. The native iPhone app lives in `ios/`
(see `ios/README.md`); `NATIVE_APP_TODO.md` tracks remaining work (branch
`phone-app`).

## Tech Stack

- **Backend**: Go 1.24+ with `net/http` routing, bound to `127.0.0.1` (local-only)
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

- `cmd/gobus/` — Entry point (desktop/standalone build)
- `internal/config/` — Environment-based configuration
- `internal/server/` — HTTP server, middleware (`Listen`/`Serve`/`Shutdown`)
- `internal/handler/` — HTTP handlers (thin: call storage/nextrip, render templ)
- `internal/gtfs/` — GTFS download, parse, import pipeline
- `internal/nextrip/` — NexTrip REST API client + cache
- `internal/realtime/` — GTFS-RT protobuf feed polling
- `internal/geo/` — Haversine, bounding box, nearest-stop logic
- `internal/storage/` — SQLite connection, migrations, queries
- `internal/templates/` — templ components
- `web/static/` — CSS, JS, icons
- `e2e/` — Playwright test suite (Node.js project)
- `mobile/` — gomobile bind entry point (`Start`/`Stop`), wraps the server core
- `ios/` — SwiftUI `WKWebView` shell (XcodeGen `project.yml`); see `ios/README.md`

## Architecture Notes

- Server-rendered HTML for accessibility. Screen readers see real DOM, not JS-rendered content.
- HTMX swaps HTML fragments for dynamic behavior. SSE for realtime departure updates.
- Single shared Go core, two thin shells: desktop browser and iPhone `WKWebView`.
  The whole server is packaged for iOS via a small `mobile` gomobile-bind package —
  there is no separate dependency-free `transit/` package.
- The server binds to `127.0.0.1` only (port `0` = OS-assigned on mobile). Set
  `GOBUS_HOST=0.0.0.0` for LAN access during development.
- SQLite R-Tree index on stops for O(log n) geospatial "nearest stop" queries.
- Dark mode is the default theme. Minimum 4.5:1 contrast (WCAG AA), 7:1 for departure times (AAA).

## Conventions

- Use `log/slog` for all logging
- Configuration via environment variables (prefix: `GOBUS_`)
- No external HTTP router — use Go 1.22+ `net/http` patterns
- Handlers are thin: data access in `storage/`, external APIs in `nextrip/`/`realtime/`
- Single-user / no auth: user settings persist in the local SQLite file
- All templ components must use semantic HTML and ARIA attributes
- Test GTFS fixtures live in `e2e/fixtures/gtfs/`
