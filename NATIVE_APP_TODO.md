# GoBus iOS — remaining work

The native iPhone app is built and running. **Phases 1–3 of the original plan
are done** and the old `NATIVE_APP_PLAN.md` is retired; this doc tracks what's
left (Phase 4 + polish).

## Status (done)

- Go core refactored local-first (auth removed, settings in SQLite, embeddable
  `127.0.0.1:0` server) — Phase 1.
- `mobile` package bound to `build/GobusKit.xcframework` via gomobile; CGo
  `mattn/go-sqlite3` builds for device + simulator — Phase 2.
- SwiftUI `WKWebView` shell in `ios/` (XcodeGen `project.yml`), first-launch DB
  copy, async startup, ATS localhost exception — Phase 3.
- Verified on the iPhone 16 Pro Simulator: UI renders, geolocation resolves,
  nearby departures populate. See `ios/README.md` to build/run.
- Code-review fixes (2026-08, `docs/iphone-app-code-review.md`): GTFS
  service-day correctness after midnight; nearby grouping no longer attributes
  another stop's times to a stop; bounded-concurrency realtime fetches; empty
  radius tiers auto-advance on the initial search; schedule freshness on
  launch/foreground (`MobileRefresh`); startup failure retry UI; WebView
  navigation policy (local origin only, external links → Safari, WebContent
  crash recovery); native shell disables PWA manifest/install/service worker;
  location permission prompts in context instead of at launch; reverse
  geocoding disabled on iOS (local nearest-stop label; desktop sends only
  ~110 m-coarse coordinates); SSE connections actually close on idle;
  saved-location writes confirmed before local state changes; Playwright e2e
  suite runs against a fixture DB (`make test-e2e`).

## Remaining

### Ship on a real device
- [x] Run on a physical iPhone for QA. Done with a **free** Apple ID / personal
      team — the paid program is *not* needed for own-device testing. Signing is
      wired into `ios/project.yml` (`CODE_SIGN_STYLE: Automatic`,
      `DEVELOPMENT_TEAM: KKLTANHU6N`, bundle id `com.colindavis.gobus`). Build +
      install steps in `ios/README.md`; first-run war story (Developer Mode,
      keychain session, VoiceOver gotchas) in
      `docs/ios-device-deploy-playbook.md`.
- [ ] Confirm CoreLocation + nearby departures from real GPS on device (in
      progress — verifying during the VoiceOver QA pass).
- [ ] Apple Developer Program ($99/yr) — still required for TestFlight / App
      Store distribution (see "App Store prep" below).

### App polish
- [ ] App icon (asset catalog `AppIcon`, referenced by
      `ASSETCATALOG_COMPILER_APPICON_NAME`).
- [ ] Launch screen (currently an empty `UILaunchScreen`; give it a dark
      background to avoid a white flash before the WebView paints).
- [ ] VoiceOver pass on device — the UI is server-rendered semantic HTML, so it
      should largely work, but verify focus order and the WebView container.

### Data freshness (Phase 4)
- [x] GTFS refresh on device: conditional refresh (imported_at older than 24 h
      → HEAD check → import) runs after startup and on every foreground via
      `MobileRefresh()`; failed checks retry on the next foreground.
- [ ] Split `gobus.db` into a replaceable `schedule.db` and a preserved
      `user.db` (settings, saved locations). That would let app updates ship a
      fresh bundled schedule without wiping user data, simplify backup and
      corruption recovery, and reduce risk during large imports. (Currently a
      single DB: the first-launch copy is never repeated, so a stale bundled
      schedule persists until the in-app refresh replaces it.)
- [ ] Consider shrinking the bundled DB (~153 MB): VACUUM, and drop tables not
      needed at runtime (e.g. `shapes`) to cut app size.

### Lifecycle / robustness
- [x] Foreground hook wired via `scenePhase` (triggers the freshness check).
      The server itself stays running; iOS suspends it with the app.
- [x] Startup failure shows an error message with a working Retry button.
- [ ] Optional promptless geolocation: inject CoreLocation coordinates into
      `navigator.geolocation` via a JS bridge, removing the one-time WebKit
      geolocation permission prompt. Fallback only — the WebKit bridge works.

### App Store prep
- [ ] An app bundling an offline transit DB + native location is well past
      guideline 4.2 ("minimum functionality") for WebView apps — low risk.
- [ ] Screenshots and submission.
- [x] Privacy label groundwork: the iOS build never transmits location
      (reverse geocoding is disabled in the native shell; the nearby label is
      computed locally), so "location: used on-device, not collected" is now
      accurate. NexTrip realtime requests are per-stop-ID only.
