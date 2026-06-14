# GoBus — iOS app

A thin SwiftUI shell that runs the shared Go core on-device and points a
`WKWebView` at the local server (`http://127.0.0.1:<port>`). See
`../NATIVE_APP_PLAN.md` for the overall design.

## What's here (checked in)

- `project.yml` — [XcodeGen](https://github.com/yonyz/XcodeGen) spec; the source
  of truth for the Xcode project. The `.xcodeproj` is generated, not committed.
- `Gobus/` — Swift sources:
  - `GobusApp.swift` — app entry; starts the Go core, loads the WebView.
  - `GobusServer.swift` — first-launch DB copy + `MobileStart`/`MobileStop`.
  - `WebView.swift` — `WKWebView` wrapper.
  - `LocationPrimer.swift` — primes the When-In-Use location prompt.
  - `Info.plist` — ATS localhost exception + location usage description.

## Build inputs (generated, git-ignored)

- `../build/GobusKit.xcframework` — the Go core, gomobile-bound for iOS. The
  framework module is `GobusKit` (kept distinct from the `Gobus` app target to
  avoid a Swift module-name collision); the exported funcs are `MobileStart` /
  `MobileStop`. Build with `make ios-framework` from the repo root.
- `../dist/gobus.db` — prebuilt SQLite schedule DB, bundled as a read-only
  resource and copied to Application Support on first launch. Build with
  `make prebuilt-db`.

## Prerequisites (macOS)

```bash
brew install xcodegen
go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init
# Xcode (full, not just Command Line Tools) with an iOS SDK:
sudo xcode-select -s /Applications/Xcode.app/Contents/Developer
```

## Build & run (Simulator — no Apple Developer account needed)

```bash
# From the repo root:
make prebuilt-db        # -> dist/gobus.db   (downloads + imports GTFS; slow once)
make ios-framework      # -> build/GobusKit.xcframework

cd ios
xcodegen generate       # -> Gobus.xcodeproj
xcodebuild -project Gobus.xcodeproj -scheme Gobus \
  -sdk iphonesimulator -configuration Debug \
  -destination 'platform=iOS Simulator,name=iPhone 16 Pro' \
  -derivedDataPath build/dd CODE_SIGNING_ALLOWED=NO build

DEV="iPhone 16 Pro"
xcrun simctl boot "$DEV"; open -a Simulator
xcrun simctl install "$DEV" build/dd/Build/Products/Debug-iphonesimulator/Gobus.app
xcrun simctl launch --console-pty "$DEV" com.gobus.app
```

The first time the page requests location, WebKit shows a geolocation prompt —
tap **Allow While Using App**; the choice is remembered. To exercise nearby
departures in the Simulator, set a location, e.g.:

```bash
xcrun simctl location "iPhone 16 Pro" set 44.9778,-93.2650   # downtown Minneapolis
```

## Running on a real device

Requires an Apple Developer account for signing/provisioning. In `project.yml`
set `DEVELOPMENT_TEAM` (and a unique `PRODUCT_BUNDLE_IDENTIFIER`), regenerate,
then build with `-destination 'generic/platform=iOS'` and code signing enabled.
