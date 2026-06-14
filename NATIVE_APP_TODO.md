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
  copy, async startup, location prompt, ATS localhost exception — Phase 3.
- Verified on the iPhone 16 Pro Simulator: UI renders, geolocation resolves,
  nearby departures populate. See `ios/README.md` to build/run.

## Remaining

### Ship on a real device (blocks device testing + distribution)
- [ ] Apple Developer account ($99/yr).
- [ ] Set `DEVELOPMENT_TEAM` and a unique `PRODUCT_BUNDLE_IDENTIFIER` in
      `ios/project.yml`; regenerate and build with code signing for
      `-destination 'generic/platform=iOS'`.
- [ ] Run on a physical iPhone; confirm CoreLocation + nearby departures on
      device (not just Simulator).

### App polish
- [ ] App icon (asset catalog `AppIcon`, referenced by
      `ASSETCATALOG_COMPILER_APPICON_NAME`).
- [ ] Launch screen (currently an empty `UILaunchScreen`; give it a dark
      background to avoid a white flash before the WebView paints).
- [ ] VoiceOver pass on device — the UI is server-rendered semantic HTML, so it
      should largely work, but verify focus order and the WebView container.

### Data freshness (Phase 4)
- [ ] Decide GTFS refresh cadence on device (e.g. on launch if data older than
      N days). The background scheduler already runs and updates schedule tables
      in place; confirm it behaves on device and define the trigger.
- [ ] Optionally background-download a fresh prebuilt DB and swap, vs. running
      the importer in-process. Must not touch user-settings rows in `gobus.db`.
- [ ] Consider shrinking the bundled DB (~153 MB): VACUUM, and drop tables not
      needed at runtime (e.g. `shapes`) to cut app size.

### Lifecycle / robustness
- [ ] App lifecycle: optionally stop/restart (or explicitly keep alive) the
      server on background/foreground via `scenePhase`. Currently left running;
      iOS suspends it with the app.
- [ ] Optional promptless geolocation: inject CoreLocation coordinates into
      `navigator.geolocation` via a JS bridge, removing the one-time WebKit
      geolocation permission prompt. Fallback only — the native bridge works.

### App Store prep
- [ ] An app bundling an offline transit DB + native location is well past
      guideline 4.2 ("minimum functionality") for WebView apps — low risk.
- [ ] Screenshots, privacy nutrition label (location: used on-device, not
      collected), and submission.
