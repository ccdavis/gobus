# Deploying an iOS app to a real iPhone for testing — a playbook

A project-agnostic guide for getting any iOS app onto a **physical iPhone for QA
testing**, using a **free Apple ID** (no paid Developer Program). Written for a
future Claude session driving the build from the command line on the user's
behalf. It captures the non-obvious failures we actually hit, in order, with the
fixes that worked.

> Scope: own-device testing/QA. TestFlight and App Store distribution **do**
> require the paid Apple Developer Program ($99/yr) — that's the only thing the
> free account can't do here.

---

## The single most important fact

**A free Apple ID is sufficient to run your own app on your own iPhone.** Xcode
creates a "Personal Team" automatically. Limitations vs. the paid program:

- Provisioning profile **expires after ~7 days** → just rebuild + reinstall.
- **No TestFlight / App Store**, no distribution to other people's devices.
- A few entitlements (push, associated domains, etc.) are unavailable.

Many docs (including this project's, before we fixed them) wrongly claim a paid
account is required for device testing. It isn't.

---

## Prerequisites

- macOS with **full Xcode** installed (not just Command Line Tools):
  `sudo xcode-select -s /Applications/Xcode.app/Contents/Developer`
- The user's **Apple ID added to Xcode**: Xcode → Settings → Accounts → `+` →
  Apple ID. This is the one step that *must* happen in the GUI; there is no
  supported CLI to add an account. Once added, Xcode silently creates the
  Personal Team.
- A **unique bundle identifier**. Free personal teams can't reuse a bundle ID
  already registered to another team, so avoid generic ones like `com.app.app`.
  Use something like `com.<username>.<app>`.

---

## Finding the values you need (no GUI digging)

**Team ID** — after the Apple ID is added in Xcode, it's cached in prefs even
before any cert exists:

```bash
defaults read com.apple.dt.Xcode 2>/dev/null | grep -iE "teamID|teamName|isFree"
# → teamID = XXXXXXXXXX;  teamName = "<Name> (Personal Team)";  isFreeProvisioningTeam = 1
```

**Device UDID** — plug in the iPhone (unlocked), then:

```bash
xcrun xctrace list devices    # entries under "== Devices ==" (ignore Simulators)
xcrun devicectl list devices  # alternative; shows connection state
```

**Device readiness** (developer mode, pairing, lock state):

```bash
xcrun devicectl device info details --device <UDID> 2>&1 \
  | grep -iE "developerMode|pairingState|bootState|tunnelState"
```

---

## Wire signing into the project (so it survives project regeneration)

If the project uses **XcodeGen** (`project.yml`), put signing in the target's
`settings.base` so `xcodegen generate` never wipes it:

```yaml
settings:
  base:
    PRODUCT_BUNDLE_IDENTIFIER: com.<username>.<app>   # unique!
    CODE_SIGN_STYLE: Automatic
    DEVELOPMENT_TEAM: XXXXXXXXXX                        # the Team ID from above
```

For a raw `.xcodeproj`, the equivalent build settings are
`CODE_SIGN_STYLE = Automatic` and `DEVELOPMENT_TEAM = XXXXXXXXXX`. Then
`xcodegen generate` (if applicable).

---

## Build, sign, install

```bash
cd <ios-project-dir>

# 1. Build + sign for the device. -allowProvisioningUpdates lets Xcode create
#    the development cert + provisioning profile on first run.
xcodebuild -project <App>.xcodeproj -scheme <App> \
  -sdk iphoneos -configuration Debug \
  -destination 'id=<UDID>' \
  -derivedDataPath build/dd -allowProvisioningUpdates build

# 2. Install (uses device pairing, NOT the keychain — safe from any session).
xcrun devicectl device install app --device <UDID> \
  build/dd/Build/Products/Debug-iphoneos/<App>.app

# 3. (optional) Launch it.
xcrun devicectl device process launch --device <UDID> <bundle-id>
```

Verify a build's signature without the keychain (read-only, works anywhere):

```bash
codesign --verify --verbose=2 build/dd/Build/Products/Debug-iphoneos/<App>.app
```

---

## The gotchas we actually hit (and the fixes)

### 1. `Device is busy (Waiting to reconnect)` / `connected (no DDI)`
**Cause:** **Developer Mode is disabled** on the iPhone (required on iOS 16+).
The Developer Mode toggle only *appears* after a device build has been attempted
at least once, so the sequence is: attempt a build (it'll fail), then enable it.

**Fix, on the phone:** Settings → Privacy & Security → **Developer Mode** → on →
reboot → after reboot, confirm "Turn On" + passcode. Verify:

```bash
xcrun devicectl device info details --device <UDID> 2>&1 | grep developerMode
# want: developerModeStatus: enabled
```

### 2. `errSecInternalComponent` at the CodeSign step — the big one
Symptom: build gets all the way to `CodeSign .../<framework>` and dies with
`errSecInternalComponent`. The signing identity is valid and visible
(`security find-identity -v -p codesigning` lists it), yet signing fails.

**Root cause:** `codesign` needs to *use the private key*, which requires the
user's **unlocked login keychain**. An automated/detached/sandboxed shell (e.g.
an agent's tool-run shell, SSH, CI without setup) is in a different security
session where the login keychain is **locked and cannot prompt**. Tell-tale
sign — this fails in the detached session:

```bash
security show-keychain-info ~/Library/Keychains/login.keychain-db
# → "User interaction is not allowed."   (means: locked / no interactive session)
```

Reading the keychain (listing identities) works from such a session, but
**using a key for signing does not** — which is why the error is so confusing.

**Fix that worked:** have the **user run the build command in their own
Terminal.app**, i.e. their real GUI login session, where the login keychain is
already unlocked. If you're an agent driving this:

- Don't rely on your own tool shell for the signing build — it's the locked
  session. Hand the user the exact command to paste into Terminal.
- The Claude Code `!`-prefix runs in the chat input, **not** a raw shell — don't
  tell the user to paste `! ...` into Terminal.app (zsh treats leading `!` as
  pipeline negation / history expansion and mangles it). The `!` prefix is only
  for the Claude prompt itself.
- To get full visibility into the user's run, have them redirect output to a
  file you can then read:
  ```bash
  cd <ios-project-dir> && xcodebuild ... build > /tmp/ios_build.log 2>&1; echo "DONE exit=$?"
  ```
  Then read `/tmp/ios_build.log` and grep for `BUILD SUCCEEDED|BUILD FAILED|errSec`.

Other fixes people cite (lower priority, try only if the above is impossible):
- Unlock the keychain in that session: `security unlock-keychain ~/Library/Keychains/login.keychain-db`.
- Open the key's partition list (CI pattern):
  `security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k <pw> ~/Library/Keychains/login.keychain-db`.
  In our case this alone did **not** fix it — the session being locked/non-
  interactive was the real blocker, so running in the user's own Terminal was
  the reliable path.

### 3. App installs but won't launch — untrusted developer
**Fix, on the phone (first install per cert only):** Settings → General → VPN &
Device Management → Developer App → your Apple Development cert → **Trust**.

### 4. Profile expired after a week
Free-team profiles last ~7 days. The app silently stops launching. Just re-run
the build + install steps; no settings change needed.

---

## VoiceOver / accessibility notes (for blind or low-vision users)

This project's user drives everything with VoiceOver. Things that tripped us up:

- **Xcode's "Signing & Capabilities" editor is very hard with VoiceOver.** Avoid
  it entirely by setting `DEVELOPMENT_TEAM` + `CODE_SIGN_STYLE` in the project
  config and building from the command line, as above. The only unavoidable GUI
  is **Settings → Accounts** to add the Apple ID (a far simpler dialog).
- **iOS boot/secure screens may ignore the VoiceOver triple-click shortcut**,
  because the accessibility shortcut isn't fully active until the first unlock
  after a reboot. The Developer Mode confirmation is one such screen.
- On the "swipe up to continue" Developer Mode screen: VoiceOver may report a
  "swipe up" *button* that does nothing when activated, and the three-finger
  VoiceOver scroll may not work either. What worked was the **single-finger
  home-screen swipe-up** gesture. (Siri "turn off VoiceOver" is a fallback only
  if Siri is already set up.)
- Suggest enabling Settings → Accessibility → **Accessibility Shortcut →
  VoiceOver** so a triple-click of the side button toggles VoiceOver for the
  rare screens where a gesture must be done sighted-style.

---

## Quick reference — the whole happy path

```bash
# one-time: add Apple ID in Xcode → Settings → Accounts (GUI)
TEAM=$(defaults read com.apple.dt.Xcode 2>/dev/null | grep teamID | head -1 | grep -oE '[A-Z0-9]{10}')
UDID=<from `xcrun xctrace list devices`>
# set DEVELOPMENT_TEAM=$TEAM, CODE_SIGN_STYLE=Automatic, unique bundle id in project config
# enable Developer Mode on the phone (Settings → Privacy & Security)

# build IN THE USER'S OWN TERMINAL (unlocked keychain):
xcodebuild -project <App>.xcodeproj -scheme <App> -sdk iphoneos -configuration Debug \
  -destination "id=$UDID" -derivedDataPath build/dd -allowProvisioningUpdates build

# install + launch (any session is fine):
xcrun devicectl device install app --device "$UDID" build/dd/Build/Products/Debug-iphoneos/<App>.app
xcrun devicectl device process launch --device "$UDID" <bundle-id>
# then: Trust the cert on the phone (Settings → General → VPN & Device Management)
```
