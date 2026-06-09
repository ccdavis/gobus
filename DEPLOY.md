# GoBus Build & Run Guide

GoBus is a **local-first, single-user** app: the Go core runs on your own device
and you reach it from a local browser (desktop build) or, on iPhone, from an
in-app `WKWebView` over the same core (see [`NATIVE_APP_PLAN.md`](NATIVE_APP_PLAN.md)).
There is no public server to deploy and no login.

> **Note:** Earlier versions documented a multi-user public deployment to Fly.io.
> That model has been retired — GoBus no longer has user accounts, cookies, or a
> hosted server. This guide now covers building and running the local binary.

## Building from source

GoBus requires specific build tools because it uses CGo (for SQLite) and
templ (for type-safe HTML templates). Use the setup script below or follow
the manual steps.

### Quick setup (Linux and macOS)

Run from the project root:

```bash
./scripts/setup-build.sh
```

This installs Go 1.24, templ, and a C compiler (if missing), then builds
the binary. See below for what it does.

### Manual setup — Linux (Ubuntu/Debian)

```bash
# 1. Install C compiler (required for CGo / SQLite)
sudo apt-get update && sudo apt-get install -y gcc build-essential

# 2. Install Go 1.24
#    Remove any old system Go first
sudo rm -rf /usr/local/go
curl -fsSL https://go.dev/dl/go1.24.0.linux-amd64.tar.gz | sudo tar -C /usr/local -xzf -

# 3. Add Go to PATH (add to ~/.bashrc or ~/.profile for persistence)
export PATH=/usr/local/go/bin:$HOME/go/bin:$PATH

# 4. Verify
go version    # should show go1.24.0
gcc --version # should show gcc

# 5. Install templ CLI
go install github.com/a-h/templ/cmd/templ@v0.3.977

# 6. Build GoBus
cd /path/to/gobus
templ generate
CGO_ENABLED=1 go build -o gobus ./cmd/gobus/

# 7. Verify — should print usage and exit
./gobus --help
```

For **ARM machines** (e.g., Raspberry Pi), replace the Go download URL:

```bash
curl -fsSL https://go.dev/dl/go1.24.0.linux-arm64.tar.gz | sudo tar -C /usr/local -xzf -
```

### Manual setup — macOS

macOS is also where the native iPhone build happens (gomobile + Xcode require
macOS); see [`NATIVE_APP_PLAN.md`](NATIVE_APP_PLAN.md).

```bash
# 1. Install Xcode command line tools (provides C compiler)
xcode-select --install    # skip if already installed

# 2. Install Go via Homebrew
brew install go@1.24
# Or download directly:
# curl -fsSL https://go.dev/dl/go1.24.0.darwin-arm64.tar.gz | sudo tar -C /usr/local -xzf -

# 3. Add Go to PATH (add to ~/.zshrc for persistence)
export PATH=$(brew --prefix go@1.24)/bin:$HOME/go/bin:$PATH

# 4. Install templ CLI
go install github.com/a-h/templ/cmd/templ@v0.3.977

# 5. Build
cd /path/to/gobus
templ generate
CGO_ENABLED=1 go build -o gobus ./cmd/gobus/
```

## Running the binary

The binary is self-contained — static assets are embedded. By default it binds
to `127.0.0.1` (local-only) and no login is required:

```bash
./gobus                          # starts on http://127.0.0.1:8080
./gobus -port 3000               # custom port
./gobus -host 0.0.0.0            # expose on the LAN (e.g. to test from a phone)
GOBUS_DB_PATH=/data/gobus.db ./gobus   # custom database location
```

On first run it downloads ~24 MB of GTFS data and imports it (~30 seconds),
showing a loading page that auto-refreshes until the data is ready. User
settings (saved locations, units) are stored in the SQLite database file.

To skip the on-device download, point `GOBUS_DB_PATH` at a prebuilt database
(produced by `./gobus --import-gtfs`); the app serves immediately when data is
already present.

## Cross-compiling

To build on one machine for another (e.g. on macOS for a Linux box):

```bash
# For x86_64 Linux
GOOS=linux GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-linux-gnu-gcc go build -o gobus-linux ./cmd/gobus/

# For ARM Linux (Raspberry Pi)
GOOS=linux GOARCH=arm64 CGO_ENABLED=1 CC=aarch64-linux-gnu-gcc go build -o gobus-linux-arm64 ./cmd/gobus/
```

Cross-compiling with CGo requires a cross-compiler
(`brew install FiloSottile/musl-cross/musl-cross`). The easiest alternative is
to build directly on the target machine.

## Inspecting the database

```bash
sqlite3 gobus.db "SELECT COUNT(*) FROM stops;"
sqlite3 gobus.db "SELECT route_id, route_long_name FROM routes LIMIT 10;"
```

## Force a GTFS re-import

```bash
./gobus --import-gtfs    # re-download the feed and rebuild, then exit
```
