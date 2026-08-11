# GoBus iPhone App Code Review

**Review date:** 2026-08-10  
**Status:** Read-only audit; findings have not yet been fixed  
**Scope:** SwiftUI/WKWebView shell, gomobile entry point, embedded Go server,
GTFS storage and refresh paths, server-rendered web UI, JavaScript behavior,
accessibility, release configuration, and test tooling.

## Executive summary

The app has an appealingly small native shell and a sensible local-first core,
but it is not ready for App Store distribution yet. The most important issues
are correctness and lifecycle problems rather than cosmetic polish:

1. Exact location is sent to Nominatim even though the planned privacy label
   says location is used only on-device.
2. The bundled schedule database normally will not refresh during typical iOS
   use and will eventually expire.
3. Scheduled departures around midnight are associated with the wrong GTFS
   service day.
4. The nearby-routes view can present departure times from one stop as though
   they belong to another stop.
5. Realtime requests are made serially, allowing a nearby page to take minutes
   to render during poor connectivity.

There are also several native integration gaps: failed startup cannot be
retried, external/new-window navigation is not handled, PWA caching is run under
a random local origin, and the location permission flow is abrupt. The current
end-to-end test target does not run a test suite.

## Priority 1: correctness, privacy, and availability

### 1. Exact coordinates leave the device

**Severity:** High / App Store disclosure risk

`Handler.LocationLabel` automatically reverse-geocodes the current coordinates:

- `internal/handler/nearby.go:490-522`
- `internal/geocode/nominatim.go:93-115`

`geocode.Client.Reverse` sends latitude and longitude, rounded to six decimal
places, to `https://nominatim.openstreetmap.org/reverse`. Six decimal places are
far more precise than needed for a general neighborhood label.

This conflicts with the planned statement in `NATIVE_APP_TODO.md:61-62`:

> privacy nutrition label (location: used on-device, not collected)

It also is not explained by the current `NSLocationWhenInUseUsageDescription`,
which only says location is used to find nearby stops and departures.

**Impact**

- The proposed privacy declaration is factually incomplete: location is
  transmitted off-device to a third party.
- Users cannot opt out of reverse geocoding while retaining nearby departures.
- The third party may receive the coordinate, request time, IP address, and the
  app's static user-agent string.

**Recommended resolution**

Choose one of these deliberately:

1. Remove reverse geocoding on native iOS and display a nearest-stop or generic
   location label computed locally.
2. Use Apple Core Location's native geocoder behind an explicit native bridge,
   after reviewing its privacy implications.
3. Keep Nominatim, but make the network lookup optional, reduce precision, add
   a clear disclosure, and complete the App Store privacy assessment based on
   Nominatim's current data-handling policy.

Add a test that verifies nearby departures still work when reverse geocoding is
disabled or unavailable.

### 2. Schedule data normally never refreshes on iPhone

**Severity:** High

Relevant code:

- `ios/Gobus/GobusServer.swift:31-36`
- `mobile/mobile.go:82-91`
- `internal/gtfs/scheduler.go:34-42`
- `internal/gtfs/scheduler.go:71-91`

On startup, `EnsureData` returns immediately whenever the database contains at
least one route. It does not check `imported_at`, `Last-Modified`, or `ETag`.
`StartBackground` then schedules the first refresh for 3:00 AM Central.

This works for an always-running desktop process, but not for a normal iPhone
app. iOS ordinarily suspends backgrounded apps, so the process is unlikely to
execute its timer at 3 AM. The existing database is also never replaced by a
new bundled database during an app upgrade because it contains user settings.

The comment in `GobusServer.swift` that schedule data “is kept current by the
background GTFS refresh” is therefore misleading for iOS.

**Impact**

- Calendar ranges eventually expire and scheduled departures disappear.
- Added/removed routes and stops remain stale.
- The problem can persist across app upgrades because the first-launch copy is
  intentionally never repeated.

**Recommended resolution**

- On app launch or foreground, check `imported_at` and trigger a conditional
  refresh when data is older than a defined threshold, such as 24 hours.
- Do not block initial rendering when usable existing data is present. Refresh
  asynchronously and atomically.
- Retry a failed check later rather than marking the day as checked before the
  network operation succeeds. `CheckAndUpdate` currently sets `lastCheckDate`
  at `scheduler.go:54` before doing any I/O.
- Give the downloader explicit connection/overall timeouts; it currently uses
  `&http.Client{}` in `internal/gtfs/downloader.go:24`.
- Consider a real iOS `BGAppRefreshTask` only as a supplement. Foreground
  refresh must be sufficient because background execution is not guaranteed.

Longer term, split the large schedule database from the small user database.
That would let app updates replace schedule data safely without touching saved
locations or preferences.

### 3. GTFS service-day handling is wrong after midnight

**Severity:** High / transit-information correctness

Relevant code:

- `internal/storage/queries.go:158-210`
- `internal/handler/departures.go:15-18`
- `internal/handler/stop.go:89-111`

GTFS permits times over 24 hours. For example, a Sunday service trip at
`25:00:00` represents Monday at 1:00 AM.

At Monday 12:30 AM, the current implementation:

1. Queries Monday's service calendar with `afterTime = "00:30:00"`.
2. Includes Monday service's `25:00:00`, which is actually Tuesday at 1 AM.
3. Does not query Sunday's service for its `25:00:00`, which is the trip that
   actually occurs in 30 minutes.
4. Interprets all selected times relative to `now`'s calendar date.

This can both omit the correct departure and display a next-day departure as
imminent. Interval detection has the same service-day assumption.

**Recommended resolution**

- Query both the current service day and the previous service day during the
  post-midnight window.
- Convert every GTFS time into an absolute instant using the service date that
  selected it, not automatically `now.Date()`.
- Filter and sort by absolute instant after combining candidate service days.
- Add table-driven tests spanning at least:
  - `23:30`, `24:30`, and `25:00`;
  - weekday/weekend calendar transitions;
  - `calendar_dates` additions and removals;
  - spring-forward and fall-back DST transitions.

The existing `gtfsInstant` DST work is useful, but its input needs an explicit
service date.

### 4. Nearby “later times” can be attributed to the wrong stop

**Severity:** High

Relevant code:

- `internal/handler/nearby.go:252-287`
- `internal/handler/nearby.go:290-333`

The routes-first view groups departures with this key:

```go
type routeKey struct {
    routeID     string
    directionID int
}
```

When the same route and direction is found at a subsequent nearby stop, its
departure is appended to the existing group's `deps`. The group retains only
the first stop's ID, name, and coordinates. The template then presents every
appended time as a later time at that first stop.

Grouping also ignores headsign and route pattern, so branches with the same
route and `direction_id` may be merged.

**Impact**

A user can be told to wait at Stop A for a time that belongs to Stop B. In a
transit app this is a material correctness problem, not merely imperfect
sorting.

**Recommended resolution**

- First choose the stop/pattern to represent each nearby route direction.
- Collect later departures only from that exact stop and compatible headsign
  or trip pattern.
- Include `stopID` and an appropriate pattern/headsign identifier in grouping
  keys where aggregation is still required.
- Sort times explicitly after realtime merging.
- Add a regression fixture with two nearby stops serving the same route and
  direction at different times.

### 5. Nearby realtime lookups are sequential

**Severity:** High availability/performance risk

Relevant code:

- `internal/handler/nearby.go:267-270`
- `internal/handler/departures.go:23-29`
- `internal/nextrip/client.go:25-30`

The route view may fetch departures for up to 15 stops sequentially. Each
NexTrip call has a ten-second timeout, yielding a theoretical initial-page delay
of roughly 150 seconds when requests fail slowly. Larger radius tiers can do
substantially more work.

**Recommended resolution**

- Render scheduled data without waiting for every realtime request, or fetch
  realtime with bounded concurrency and a short shared page deadline.
- Deduplicate stops before making requests.
- Consider a batch or prefetch layer if the upstream API supports one.
- Preserve partial successes instead of making page latency equal the sum of
  all failures.
- Add a test server that delays/fails responses and assert a maximum handler
  duration.

## Priority 2: functional and native-integration bugs

### 6. Initial empty nearby search never advances radius

**Severity:** Medium

`internal/handler/nearby.go:92-166` describes auto-advancing through empty
radius tiers, but both loops require `newOffset > 0`. On the initial search,
`offset` and the number of results are both zero, so the loop cannot run.

Users in sparse areas receive “No stops found nearby” after only the first
450-meter tier, even though the configured tiers extend to 14.4 km.

Remove the `newOffset > 0` requirement for the initial empty case, while
retaining a finite tier bound. Add tests for empty inner tiers followed by a
nonempty outer tier.

### 7. The SSE idle feature does not close connections

**Severity:** Medium

Relevant code:

- `web/static/js/app.js:461-512`
- `web/static/js/htmx-sse.js:41-60`
- `web/static/js/htmx-sse.js:178-232`

`onIdle` removes the `sse-connect` attribute, but the HTMX extension closes its
`EventSource` only during `htmx:beforeCleanupElement`, when an element is being
removed. Attribute removal does not invoke cleanup.

On wake, `htmx.process(el)` creates another `EventSource` and overwrites the
extension's internal reference without closing the old source. Repeated
idle/wake cycles can accumulate live connections while the UI falsely says
realtime updates are paused.

The subsequent `banner.focus()` also does nothing because the banner is a
non-focusable `div` without `tabindex`.

Use a supported close mechanism, remove/replace the source element so cleanup
runs, or manage the `EventSource` explicitly. Verify connection counts in a
browser test.

### 8. External/new-window navigation is unhandled

**Severity:** Medium

`internal/templates/route_detail.templ:48-56` renders “Show on Map” with
`target="_blank"`. `ios/Gobus/WebView.swift` sets neither a `WKUIDelegate` nor a
`WKNavigationDelegate`, and it provides no SwiftUI coordinator.

There is therefore no app-owned path for handling a new-window navigation or
opening the URL in Safari. More generally, the WebView does not restrict its
main frame to the local GoBus origin or provide a recovery UI for navigation
failures and WebContent process termination.

Apple references:

- <https://developer.apple.com/documentation/webkit/wkuidelegate>
- <https://developer.apple.com/documentation/webkit/wknavigationdelegate>

Recommended native policy:

- Allow `http://127.0.0.1:<bound-port>` navigations in the WebView.
- Open approved `https` links with `UIApplication.open` or an in-app browser.
- Reject unexpected schemes and hosts.
- Handle `targetFrame == nil`, load failures, and WebContent process
  termination.

### 9. Startup failure cannot be retried

**Severity:** Medium

Relevant code:

- `ios/Gobus/GobusApp.swift:28-40`
- `ios/Gobus/GobusApp.swift:63-72`
- `ios/Gobus/GobusServer.swift:28-29`
- `ios/Gobus/GobusServer.swift:56-58`

`didStart` becomes true before startup and is never reset if `MobileStart`
fails. The failure view contains only static text. Transient filesystem,
database, or listener errors therefore require force-quitting the app.

Directory-creation errors are discarded with `try?`, so failures are reported
later and less accurately.

Add a structured error phase, a retry button that resets startup state, and
explicit error handling for both directories. Consider validating or recovering
an existing corrupt database rather than refusing to replace it forever.

### 10. Route text colors are lost in departure views

**Severity:** Medium / accessibility

`DeparturesForStop` selects `route_color` but not `route_text_color` in
`internal/storage/queries.go:165-193`. Consequently,
`internal/handler/departures.go:58-66` cannot populate
`DepartureInfo.RouteTextColor`.

Nearby and stop-detail badges default to white text even when GTFS specifies
dark text for contrast on a light route background. Select and propagate the
text color, validate both values as six-digit hexadecimal colors, and add
contrast-oriented render tests.

### 11. Route explorer selects an arbitrary trip pattern

**Severity:** Medium

`internal/storage/queries.go:261-317` chooses a representative trip with
`LIMIT 1` and no ordering. A route with branches, short turns, or express/local
patterns therefore displays only one arbitrary stop sequence. The chosen
pattern may change after an import or SQLite query-plan change.

Either show each distinct pattern/headsign, choose a documented canonical
pattern deterministically, or build an ordered union with branch information.

### 12. Native PWA state is fragmented by random ports

**Severity:** Medium design/maintenance issue

Relevant code:

- `ios/Gobus/GobusServer.swift:60-69`
- `ios/Gobus/WebView.swift:10-13`
- `web/static/js/app.js:7-17`
- `internal/handler/pwa.go:59-160`

Every native start asks the OS for a random port. Since the port is part of the
web origin, WebKit storage, permissions, service workers, and caches are scoped
to a potentially different origin on each launch. The WebView uses the
persistent default data store, and the shared JavaScript registers a PWA service
worker and page cache.

A later origin cannot clean caches belonging to old port origins. PWA install
UI is also irrelevant inside an already installed native app, and cached transit
pages can present stale information if the Go server fails.

Pass an explicit native-mode signal and disable the PWA manifest, install UI,
service-worker registration, and offline page caching inside the iOS shell. If
WebKit state must persist, consider a stable origin or an explicit native bridge.

## Priority 3: UX, architecture, and coding practices

### 13. Location permission timing is abrupt and duplicative

`AppModel.start` requests native location authorization immediately at launch
(`ios/Gobus/GobusApp.swift:31-35`). The nearby page then automatically calls
`navigator.geolocation` (`web/static/js/app.js:256-343`), which can introduce a
second WebKit-controlled permission surface.

Request location after the user sees the app and taps a clear “Use my location”
action. Search and saved locations should remain available without granting
location. A native Core Location bridge would provide better control over
authorization, error states, and accessibility.

### 14. Schedule data and user data should not share one database

The copied `gobus.db` contains both a large regenerable GTFS dataset and tiny
user-owned settings. This is why app updates cannot simply replace a stale
bundled database.

Separate them into, for example:

- `schedule.db`: replaceable, downloadable, excluded from backup where
  appropriate;
- `user.db`: settings and saved locations, migration-controlled and preserved.

The Go storage layer can attach the second database if cross-database access is
needed. This also reduces risk during large schedule imports and makes backup
and corruption recovery policies clearer.

### 15. Optimistic saved-location operations can lie or lose data

Relevant code:

- `web/static/js/app.js:53-57`
- `web/static/js/app.js:67-78`
- `web/static/js/app.js:400-408`

`removeSavedLocation` removes the local cache after any HTTP response, even a
500 response. Legacy migration fires writes without awaiting their results,
pushes every item into memory, and deletes `localStorage` immediately. Failed
migrations can therefore lose the only durable copy.

Only mutate client state after an `ok` response. Await all migration writes and
remove legacy storage only after every record has been confirmed or reconciled.
Show an accessible error instead of swallowing every failure.

### 16. Appearance and touch-target inconsistencies

- `ios/Gobus/GobusApp.swift:10` forces dark mode, while `README.md:125` says the
  app respects `prefers-color-scheme`.
- `.unit-toggle` is visually cryptic and much smaller than a comfortable iPhone
  touch target (`web/static/css/main.css:930-943`).
- `.direction-toggle` is also rendered as a small inline control while pointer
  users can click the surrounding row; the row itself has no equivalent
  keyboard/button semantics.
- Inline styles are repeated heavily in templates, making responsive and
  accessible redesign harder.

Respect system appearance unless there is a user setting, enlarge touch
targets, and keep the interactive semantics aligned with the visibly clickable
area.

### 17. Release configuration is incomplete and machine-specific

- `ios/project.yml:34` references `AppIcon`, but no asset catalog is checked in.
- `Info.plist` contains an empty launch screen, producing an unfinished startup
  experience.
- `ios/project.yml:32` and `:41` hardcode one bundle identifier and development
  team.
- The version remains `1.0 (1)` with no documented release versioning flow.

The missing icon and launch treatment are already recorded in
`NATIVE_APP_TODO.md`, but should be considered distribution blockers rather than
optional polish. Move signing values to local configuration or documented build
overrides.

### 18. Redundant and stale code remains

- Authentication and logout styling remains in `web/static/css/main.css`, even
  though authentication was removed. The obsolete block starts around line
  1066; `.logout-*` rules appear around line 202.
- The native app renders the PWA manifest, install prompt, offline cache, and
  Apple web-app metadata even though it is already a native installation.
- `Downloader.CheckResult.LastModified` and `.ETag` are populated but not used
  by `CheckAndUpdate`; the subsequent GET supplies its own headers.
- `NearbyStopRow.DistanceMeters` is documented but never assigned.
- `allowsInlineMediaPlayback` is enabled despite no apparent media feature.

Remove dead paths or separate desktop/PWA and native presentation configuration
so future work does not need to reason about behavior that can never execute in
the iPhone app.

## Testing assessment

### Commands run during this audit

- `go test ./...` — passed.
- `go vet ./...` — passed.
- `go test -race ./internal/handler ./internal/storage ./internal/nextrip` —
  passed.
- JavaScript syntax checks for `app.js`, `htmx-sse.js`, and
  `e2e/test-nearby.mjs` — passed.
- `make test-e2e` — failed by configuration.

The iOS target was not compiled because the review environment did not contain
macOS or Xcode.

### End-to-end test target is not functional

`e2e/package.json` defines:

```json
"test": "echo \"Error: no test specified\" && exit 1"
```

Therefore `make test-e2e` and `make test-all` always fail after building the Go
binary. This contradicts the README and project guidance that describe a
working Playwright suite.

`e2e/test-nearby.mjs` is a manual diagnostic script rather than a test suite:

- it assumes a server already exists at fixed port 9990;
- the Makefile does not start that server or load fixtures;
- it prints `YES` or `NO` rather than asserting;
- most failed checks still produce exit status zero;
- it does not test the Swift shell, location permissions, offline behavior,
  startup errors, midnight schedules, saved-location persistence, or SSE idle
  cleanup.

### Recommended tests to add first

1. GTFS service-day regression tests around midnight and DST.
2. Nearby grouping fixture with the same route/direction at multiple stops.
3. Initial empty-radius expansion test.
4. Realtime timeout/partial-success handler test.
5. SSE connection-count test across idle and wake.
6. Schedule freshness and failed-refresh retry tests.
7. XCUITest or native integration coverage for:
   - first launch and database copy;
   - denied/restricted location permission;
   - startup failure and retry;
   - “Show on Map” external navigation;
   - VoiceOver focus and labels;
   - foreground/background transitions;
   - WebContent process termination.

## macOS follow-up checklist

When work resumes on the macOS development machine:

1. Preserve the current app/database/framework artifacts long enough to
   reproduce existing behavior.
2. Generate and build the project with warnings treated seriously:

   ```bash
   make prebuilt-db
   make ios-framework
   cd ios
   xcodegen generate
   xcodebuild -project Gobus.xcodeproj -scheme Gobus \
     -sdk iphonesimulator -configuration Debug \
     -destination 'platform=iOS Simulator,name=iPhone 16 Pro' \
     -derivedDataPath build/dd CODE_SIGNING_ALLOWED=NO build
   ```

3. Inspect Swift concurrency warnings, especially `Task.detached` calling
   shared mutable static state in `GobusServer`.
4. Test on a physical device with network conditions set to offline, high
   latency, and intermittent loss.
5. Set simulator time/location or inject a clock so post-midnight and DST cases
   are deterministic.
6. Confirm whether “Show on Map” currently does nothing when tapped.
7. Inspect WebKit website data after repeated launches to measure random-port
   origin/cache accumulation.
8. Exercise the location flow after resetting both app and website permissions.
9. Run a VoiceOver pass with particular attention to direction toggles, the
   unit toggle, dynamic `aria-live` updates, saved-location editing, and native
   error screens.
10. Revisit the privacy label only after deciding whether exact coordinates
    will continue to be sent to Nominatim.

## Suggested implementation order

1. Resolve location transmission and disclosure.
2. Fix GTFS service-day correctness and add regression tests.
3. Fix nearby grouping and bounded realtime concurrency.
4. Implement launch/foreground schedule freshness.
5. Fix empty-radius expansion and SSE cleanup.
6. Add native WebView navigation/error handling and startup retry.
7. Separate native mode from PWA behavior.
8. Repair the automated test target.
9. Complete accessibility, icon, launch screen, signing, and App Store polish.

This order reduces the risk of shipping incorrect or stale transit information
before investing in distribution polish.
