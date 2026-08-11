# GoBus — iOS app

A thin SwiftUI shell that runs the shared Go core on-device and points a
`WKWebView` at the local server (`http://127.0.0.1:<port>`). See
`../NATIVE_APP_TODO.md` for status and remaining work.

## What's here (checked in)

- `project.yml` — [XcodeGen](https://github.com/yonyz/XcodeGen) spec; the source
  of truth for the Xcode project. The `.xcodeproj` is generated, not committed.
- `Gobus/` — Swift sources:
  - `GobusApp.swift` — app entry; starts the Go core (with retry on failure),
    loads the WebView, and triggers a schedule-freshness check on foreground.
  - `GobusServer.swift` — first-launch DB copy +
    `MobileStart`/`MobileStop`/`MobileRefresh`.
  - `WebView.swift` — `WKWebView` wrapper; restricts navigation to the local
    origin, opens external links in Safari, recovers from load failures and
    WebContent process termination.
  - `Info.plist` — ATS localhost exception + location usage description. The
    location prompt appears when the nearby page first asks for geolocation
    (WebKit bridges it to the app's When-In-Use permission) — the app doesn't
    pre-prompt at launch.

## Build inputs (generated, git-ignored)

- `../build/GobusKit.xcframework` — the Go core, gomobile-bound for iOS. The
  framework module is `GobusKit` (kept distinct from the `Gobus` app target to
  avoid a Swift module-name collision); the exported funcs are `MobileStart` /
  `MobileStop` / `MobileRefresh`. Build with `make ios-framework` from the repo
  root.
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

A **free Apple ID is enough** for testing on your own iPhone — you do *not* need
the paid ($99/yr) Apple Developer Program. The paid account is only required for
TestFlight / App Store distribution. With a free "personal team" the only
limitations are a **7-day** provisioning expiry (just rebuild + reinstall) and no
distribution to other devices.

Signing is already wired up in `project.yml` (`CODE_SIGN_STYLE: Automatic` +
`DEVELOPMENT_TEAM: KKLTANHU6N`, with a unique `PRODUCT_BUNDLE_IDENTIFIER` of
`com.colindavis.gobus`). To run on a physical device:

```bash
# From repo root: build the framework + DB if you haven't (see above).
cd ios && xcodegen generate

# Find your device's UDID:
xcrun xctrace list devices        # look under "== Devices ==" (not Simulators)

# Build + sign for the device (-allowProvisioningUpdates auto-creates the cert
# + profile on first run). NOTE: codesign needs your *unlocked login keychain*,
# which is only available in your own interactive shell — not in a detached/
# sandboxed session. If signing fails with `errSecInternalComponent`, run this
# command in Terminal.app yourself rather than via an automated runner.
xcodebuild -project Gobus.xcodeproj -scheme Gobus \
  -sdk iphoneos -configuration Debug \
  -destination 'id=<YOUR-UDID>' \
  -derivedDataPath build/dd -allowProvisioningUpdates build

# Install onto the device (uses device pairing, not the keychain):
xcrun devicectl device install app --device <YOUR-UDID> \
  build/dd/Build/Products/Debug-iphoneos/Gobus.app
```

First-time-only device setup:

- **Developer Mode** (iOS 16+): Settings → Privacy & Security → Developer Mode →
  on → reboot → confirm. The toggle only appears *after* a device build has been
  attempted at least once.
- **Trust the cert**: after install, Settings → General → VPN & Device Management
  → your Apple Development cert → **Trust**, before the app will launch.

The full, blow-by-blow account of getting this working the first time (including
the VoiceOver and keychain gotchas) lives in `../docs/ios-device-deploy-playbook.md`.
