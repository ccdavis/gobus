# GoBus → Native App Plan

Turning GoBus from a public client/server PWA into a **local-first app** that runs
the Go backend on-device, with two thin front-end shells over one shared core:

- **Desktop build** — run the binary, open a browser at `http://127.0.0.1:<port>`.
  This is also the day-to-day **dev/test harness**: iterate on features with the
  full templ + HTMX UI, no iOS simulator required.
- **iPhone build** — the same Go core, compiled to an `xcframework` via gomobile,
  started inside a native Swift app that points a `WKWebView` at the local server.

Both are **single-user, no login**, and store all user settings in the **local
SQLite database file** (the same model on both platforms). We are giving up the
public HTTP server / multi-user model entirely.

---

## Architecture

```
┌─────────────────────────────┐     ┌─────────────────────────────┐
│  Desktop shell              │     │  iPhone shell (Swift)       │
│  cmd/gobus → local server   │     │  SwiftUI + WKWebView        │
│  open in any browser        │     │  + CoreLocation             │
└──────────────┬──────────────┘     └──────────────┬──────────────┘
               │  http://127.0.0.1:<port>          │  http://127.0.0.1:<port>
               ▼                                    ▼
        ┌──────────────────────────────────────────────────┐
        │  Shared Go core (unchanged business logic)        │
        │  server · handler · templ · storage(SQLite) ·     │
        │  nextrip · realtime · geo · gtfs · geocode        │
        └──────────────────────────────────────────────────┘
                               │
                               ▼
                 Local SQLite DB (schedule data + user settings)
```

The iPhone build adds exactly one new Go entry point (`mobile` package, gomobile
bind target). Everything below it is shared, byte-for-byte, with the desktop build.

### What changes vs. today
- **Remove user auth** entirely: login/register/logout, device-session limiting,
  cookie signing, the `requireAuth` middleware, and related config.
- **Settings move from `localStorage` → SQLite.** Saved locations and unit
  preference become tables in the DB file, owned by the Go core. This makes
  settings durable (survives a WebView cache clear) and identical on both
  platforms. (`localStorage` still functions in the WebView, but SQLite is the
  source of truth.)
- **Bind to `127.0.0.1` only**, OS-assigned port (`:0`), never a public interface.
- **Ship a prebuilt SQLite DB** instead of downloading + importing 24MB / 1.67M
  stop_times on first launch.

---

## Phase 1 — Go refactor (do on WSL2; produces a working desktop app)

Everything here is verifiable without a Mac by running the desktop build.

1. **Rip out auth.**
   - Delete the auth routes from `server.New` (`/login`, `/register`, `/logout`).
   - Remove `requireAuth` from `withMiddleware` and the device-session upsert.
   - Remove `handler/auth.go`, `templates/auth*.templ`, cookie signing, and the
     device-session storage queries + the auth-related `config` fields
     (`CookieSecret`, `MaxUsers`, `MaxDevicesTotal`, `MaxDevicesRecent`,
     `DeviceWindowMin`).
   - `LocationLabel` currently keys its reverse-geocode cache on `userID`; key it
     on a single constant (one user per device).

2. **Make the server embeddable + local-only.**
   - Add `Serve(ln net.Listener)` to `server.Server` (keep `ListenAndServe` as a
     thin wrapper for the desktop build).
   - Bind to `127.0.0.1:<port>`; support port `0` (OS picks a free port) and
     return the chosen port to the caller.

3. **Settings in SQLite.**
   - New migration: `settings` (key/value) + `saved_locations` tables.
   - Handlers/endpoints to read/write them; repoint the existing saved-locations
     and unit-preference UI at these endpoints instead of `localStorage`.
   - (Optional) one-time best-effort import of existing `localStorage` values for
     current PWA users.

4. **Skip GTFS download when data already present.**
   - On startup, if `db.HasData()` is true, do **not** download/import — just
     serve. (The scheduler already checks `HasData`; wire startup to honor it.)
   - Keep the background daily-update scheduler optional/configurable.

5. **`mobile` package (the gomobile entry point).**
   - `gobus/mobile` exporting gomobile-friendly funcs:
     - `Start(dbPath, dataDir string, port int) (int, error)` — opens the DB,
       starts the realtime alerts fetcher, serves on `127.0.0.1:port`, returns the
       actual bound port.
     - `Stop()` — graceful shutdown.
   - Keep the exported surface tiny and primitive-typed (gomobile marshals
     primitives + simple structs cleanly).

6. **Prebuilt DB tooling.**
   - Makefile target that runs the existing `-import-gtfs` path to emit a
     versioned `gobus.db` artifact to bundle into the iPhone app (and to seed the
     desktop build).

**Exit criteria for Phase 1:** `make build` + run → open browser at localhost →
nearby, routes, stop detail, SSE updates, saved locations, and unit preference all
work with **no login** and settings persisted in the SQLite file.

---

## Phase 2 — gomobile bind (must run on the Mac Mini)

iOS `xcframework` builds require macOS + Xcode (clang/lipo/iOS SDK); this step
cannot run on WSL2.

1. Install: `go install golang.org/x/mobile/cmd/gomobile@latest`, then
   `gomobile init`.
2. **De-risk first:** bind a trivial throwaway function to confirm the toolchain +
   **CGo SQLite under gomobile** works on `-target=ios` before wiring real UI.
   (CGo `mattn/go-sqlite3` under gomobile is the single biggest integration risk;
   prove it early.)
3. `gomobile bind -target=ios -o Gobus.xcframework ./mobile` (CGO enabled, correct
   sqlite build tags).

**Exit criteria:** `Gobus.xcframework` builds and exposes `Start`/`Stop` to Swift.

---

## Phase 3 — Xcode app (Mac Mini)

1. New SwiftUI app; add `Gobus.xcframework`.
2. **First-launch DB copy:** the app bundle is read-only and SQLite WAL needs a
   writable dir — copy the bundled `gobus.db` into Application Support, hand that
   path to `Start()`.
3. On launch: `let port = GobusStart(dbPath, dataDir, 0)`, then load
   `http://127.0.0.1:\(port)/` in a `WKWebView`.
4. **Localhost networking:** add the ATS exception (`NSAllowsLocalNetworking` /
   localhost) so the WebView may hit the local server.
5. **Location:** request When-In-Use permission and add
   `NSLocationWhenInUseUsageDescription`. Since iOS 15, WKWebView bridges the JS
   Geolocation API (`navigator.geolocation` in `app.js`) to the host app's
   CoreLocation permission — so the existing JS likely needs **no changes**.
   *Fallback:* inject coordinates via a JS bridge if the native bridge misbehaves.
6. App lifecycle: stop/restart (or keep alive) the server on background/foreground.
7. VoiceOver pass; app icon; launch screen.

**Exit criteria:** app launches, shows the UI in the WebView, location prompt works,
nearby departures render on a real device.

---

## Phase 4 — Data refresh & polish

- **GTFS refresh:** background-download a fresh prebuilt DB (or run the importer in
  the background and swap the file). Realtime (NexTrip departures, GTFS-RT alerts)
  is already live over the network and needs no bundled data.
- App Store prep: an app bundling an offline transit DB + native location is well
  past guideline 4.2 ("minimum functionality") for webview apps — low risk.
- Apple Developer account ($99/yr) for device runs + distribution.

---

## Key technical notes

- **Geolocation in WKWebView** likely works out of the box on iOS 15+; treat the
  JS bridge as a fallback, not the primary path.
- **SQLite/WAL** requires a writable DB location → copy out of the bundle on first
  launch on iOS.
- **Local-only binding** (`127.0.0.1`, port `:0`) avoids port conflicts and keeps
  the server unreachable off-device.
- **gomobile + CGo SQLite** is the main risk; prove it with a trivial bind in
  Phase 2 step 2 before building anything on top.
- **Go runtime** adds ~10–15MB to the iPhone app — fine for the App Store.

---

## Cross-machine workflow

- **WSL2 (here):** all of Phase 1 — refactor, auth removal, settings-in-SQLite,
  `mobile` package, prebuilt-DB tooling. Verified via the desktop build.
- **Branch + push**, then continue on the **Mac Mini** for Phases 2–3 (gomobile
  bind + Xcode), since the iOS toolchain only runs on macOS.
- This plan doc lives in the repo so it travels with the branch.

---

## Open questions / decisions to confirm

- Desktop build: bind strictly to `127.0.0.1`, or allow opt-in LAN binding for
  testing the web UI from a phone over Wi-Fi during development?
- GTFS refresh cadence on device (weekly? on launch if >N days old?).
- Delete the auth code outright vs. keep it unwired behind a build tag (plan
  assumes **delete** for a clean tree).
