# GoBus

Accessible real-time transit app for Metro Transit (Minneapolis/St. Paul). Built as a PWA with a Go backend, designed for screen reader users and anyone who wants a fast, clean departure board.

This is a PWA (Progressive web application,) basically a web page that your phone can put on its app menu and treat it like it is a real app. For iPhone you need to open initially with Safari and add to home screen; Android will give you the option even more easily.

As a user you must register with a user name and pass-phrase, but no email or phone number is required. Registration is simply a way to keep your preferences separate from other users and to limit bots singing up. Once you register and sign in, your browser remembers your login unless you don't open the app for more than thirty days.

I "built" this app completely with Claude Code / Opus 4.6. It was a means to an end: I wanted a more friendly bus and train schedule app. I could do it by hand in Rust or C++ or Python but I think it would have taken several weeks at least. This took me three evenings.

## Features

- **Nearby departures** — uses your location to show the closest stops with scheduled and real-time arrival times
- **Route explorer** — browse all 123 Metro Transit routes, see every stop in each direction
- **Stop detail** — live-updating departures via SSE, service alerts, interval detection ("Every 15 min until 9:00 PM")
- **Service alerts** — full-text GTFS-RT alerts and NexTrip alerts on affected stops and routes
- **Saved locations** — save frequently used stops as "Home", "Work", etc. for one-tap access
- **PWA** — installable on mobile, works offline with cached pages, dark mode default

Directions  from this poihnt on are for developers and for hosting the app, not running it as a end user.

## Build Requirements

- **Go 1.22+** with CGo enabled (for SQLite)
- **GCC** or another C compiler (required by `mattn/go-sqlite3`)
- **[templ](https://templ.guide/)** CLI — install with `go install github.com/a-h/templ/cmd/templ@latest`

## Quick start

```bash
# Clone and build
git clone https://github.com/your-user/gobus.git
cd gobus
make build

# Run — downloads GTFS data on first launch (~24MB, takes ~30s to import)
./gobus
```

The server starts on `http://localhost:8080`. Open it in a browser and allow location access to see nearby departures.

## Build and run

```bash
make build       # Generate templ files + compile binary
make dev         # Run without building a binary (go run)
make import-gtfs # Force re-download of GTFS schedule data
make test        # Run Go unit tests
make clean       # Remove binary and generated files
```

### Configuration

All settings are via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `GOBUS_PORT` | `8080` | HTTP server port |
| `GOBUS_DB_PATH` | `./gobus.db` | SQLite database path |
| `GOBUS_GTFS_DIR` | `./data` | Directory for GTFS zip downloads |
| `GOBUS_GTFS_URL` | Metro Transit URL | GTFS feed URL |
| `GOBUS_NEXTRIP_URL` | `https://svc.metrotransit.org/nextrip/` | NexTrip API base URL |

### CLI flags

```bash
./gobus --port 3000        # Override port
./gobus --import-gtfs      # Download GTFS and exit
./gobus --test-mode        # Use test configuration
```

## How it works

### Data sources

- **GTFS static schedule** — downloaded from Metro Transit on first run, checked daily and at 3 AM for updates. Uses `If-Modified-Since` to avoid redundant downloads.
- **NexTrip REST API** — real-time departure predictions, polled per-stop with a 60-second in-memory cache.
- **GTFS-RT protobuf feed** — service alerts polled every 60 seconds.

### Architecture

Server-rendered HTML with HTMX for dynamic updates. No JavaScript framework — the frontend is ~17KB total (14KB HTMX + 3KB app.js). This keeps things fast and makes the app fully accessible to screen readers, since the DOM is real semantic HTML from the server.

```
Go server (net/http 1.22)
├── templ templates → HTML responses
├── HTMX → partial page updates
├── SSE → live departure streaming (60s tick)
├── SQLite + R-Tree → geospatial nearest-stop queries
├── NexTrip client → real-time departures
└── GTFS-RT fetcher → service alerts
```

### Project structure

```
cmd/gobus/          Entry point, CLI flags, wiring
internal/
  config/           Environment-based configuration
  server/           HTTP server, middleware, routing
  handler/          HTTP handlers (thin layer over storage/nextrip)
  storage/          SQLite connection, migrations, queries
  gtfs/             GTFS download, streaming CSV parse, bulk import
  nextrip/          NexTrip REST API client + TTL cache
  realtime/         GTFS-RT protobuf alert fetcher + store
  geo/              Haversine distance, bounding box math
  templates/        templ components (layout, nearby, stop, routes)
web/static/
  css/main.css      Dark-mode-first styles, high contrast
  js/app.js         Geolocation, saved locations, idle timeout, install prompt
  js/sw.js          Service worker (shell + page caching, offline fallback)
  js/htmx.min.js    Vendored HTMX
  js/htmx-sse.js    Vendored HTMX SSE extension
  icons/            PWA icons (192x192, 512x512)
```

## Accessibility

- Dark mode default with WCAG AA contrast minimums (4.5:1 body text, 7:1 departure times)
- Respects `prefers-color-scheme` and `prefers-contrast` media queries
- Skip navigation link, ARIA landmarks, `aria-live` regions for dynamic content
- All interactive elements keyboard-accessible with visible focus indicators
- Service alerts announced via `role="alert"`

## License

Apache 2.0 — see [LICENSE](LICENSE).
