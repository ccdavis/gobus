# GoBus Deployment Guide

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

For **ARM servers** (e.g., Raspberry Pi), replace the Go download URL:

```bash
curl -fsSL https://go.dev/dl/go1.24.0.linux-arm64.tar.gz | sudo tar -C /usr/local -xzf -
```

### Manual setup — macOS

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

### Running the binary

The binary is self-contained — static assets are embedded. Just copy it
to the server and run:

```bash
# Copy to server
scp gobus you@server:~/gobus

# On the server
./gobus                          # starts on :8080, auto-downloads GTFS
./gobus -port 3000               # custom port
GOBUS_DB_PATH=/data/gobus.db ./gobus   # custom database location
```

On first run it downloads ~24 MB of GTFS data and imports it (~30 seconds).
The cookie secret is auto-generated and saved to `.cookie_secret` next to
the database.

### Cross-compiling

To build on macOS for a Linux server:

```bash
# For x86_64 Linux servers
GOOS=linux GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-linux-gnu-gcc go build -o gobus-linux ./cmd/gobus/

# For ARM Linux servers (Raspberry Pi)
GOOS=linux GOARCH=arm64 CGO_ENABLED=1 CC=aarch64-linux-gnu-gcc go build -o gobus-linux-arm64 ./cmd/gobus/
```

Cross-compiling with CGo requires a cross-compiler (`brew install FiloSottile/musl-cross/musl-cross`).
The easiest alternative: build directly on the target machine, or use Docker
(the Dockerfile handles all build dependencies).

---

# Deploying to Fly.io

Step-by-step guide to deploy GoBus as a public site with a custom domain.
Fly.io builds the Docker image remotely, so you don't need build tools
locally — just `flyctl`.

## Prerequisites

- A Fly.io account (free at [fly.io/app/sign-up](https://fly.io/app/sign-up))
- A credit card on file (required by Fly.io, but the free tier covers this app)

## 1. Install flyctl

```bash
# macOS
brew install flyctl

# Linux
curl -L https://fly.io/install.sh | sh

# Windows
powershell -Command "iwr https://fly.io/install.ps1 -useb | iex"
```

Then authenticate:

```bash
fly auth login
```

## 2. Launch the app

From the project root:

```bash
fly launch
```

This will detect the `Dockerfile` and prompt you with questions:

- **App name**: Pick something like `gobus` or `gobus-transit`
- **Region**: Pick the closest to Minneapolis — `ord` (Chicago) is a good choice
- **Database**: Say **no** (we use SQLite, not Postgres)
- **Redis**: Say **no**
- **Deploy now**: Say **no** (we need to set up the volume first)

This creates a `fly.toml` file. Edit it to make sure it has these settings:

```toml
[http_service]
  internal_port = 8080
  force_https = true
  auto_stop_machines = "stop"
  auto_start_machines = true
  min_machines_running = 0

[mounts]
  source = "gobus_data"
  destination = "/data"
```

The `[mounts]` section is critical — it connects a persistent volume to `/data`
where SQLite and GTFS files live. Without this, your data disappears on every
deploy.

## 3. Create a persistent volume

```bash
fly volumes create gobus_data --region ord --size 1
```

This creates a 1 GB persistent disk in Chicago. Adjust the region to match
what you chose in step 2. The free tier includes 1 GB of volume storage.

## 4. Set the cookie secret

Generate a random secret and set it as an environment variable:

```bash
# Generate a random 32-byte hex secret
fly secrets set GOBUS_COOKIE_SECRET=$(openssl rand -hex 32)
```

This ensures user sessions survive app restarts. Without it, the app
generates a random secret on each start and all users get logged out.

You can also set other config if you want non-default values:

```bash
fly secrets set GOBUS_MAX_USERS=50           # default: 100
fly secrets set GOBUS_MAX_DEVICES_RECENT=3   # default: 3
```

## 5. Deploy

```bash
fly deploy
```

This builds the Docker image remotely on Fly.io's builders, pushes it, and
starts the app. First deploy takes 2-3 minutes (subsequent deploys are
faster due to layer caching).

Watch the logs to verify startup:

```bash
fly logs
```

You should see:

```
asset version computed version=abc123de
no GOBUS_COOKIE_SECRET set — ...    # only if you skipped step 4
server starting addr=:8080
```

Your app is now live at `https://your-app-name.fly.dev`

## 6. Set up a custom domain

### Option A: Register a new domain through a registrar

Buy a domain from any registrar (Namecheap, Cloudflare, Google Domains, etc.).

### Option B: Use a domain you already own

You can add a subdomain like `bus.yourdomain.com`.

### Point the domain at Fly.io

1. Tell Fly.io about your domain:

```bash
fly certs add bus.yourdomain.com
```

2. Fly.io will show you the DNS records to create. Typically:

```
CNAME  bus.yourdomain.com  →  your-app-name.fly.dev
```

Or for a root domain (no subdomain):

```
A      yourdomain.com  →  <Fly.io IPv4 address>
AAAA   yourdomain.com  →  <Fly.io IPv6 address>
```

3. Add these records in your domain registrar's DNS settings.

4. Wait for DNS propagation (usually 5-30 minutes). Check status:

```bash
fly certs show bus.yourdomain.com
```

Once it shows `Ready`, Fly.io has automatically provisioned a Let's Encrypt
SSL certificate. Your site is live at `https://bus.yourdomain.com`.

## 7. First-time data setup

On first startup, GoBus automatically downloads the Metro Transit GTFS feed
(~24 MB) and imports it into SQLite. This takes about 30 seconds. During
this time, visitors see a "Please wait, downloading route data..." loading
page that auto-refreshes every 5 seconds. Once the import finishes, the
next refresh lands on the login page.

The GTFS data is stored on the persistent volume, so it survives deploys.
GoBus automatically checks for GTFS updates daily.

## 8. Register your first user

1. Visit `https://your-app-name.fly.dev`
2. You'll be redirected to the login page
3. Click "Register"
4. Pick a username (3-30 chars) and passphrase (8+ chars)
5. You're in! The app will ask for location permission to show nearby stops.

## Ongoing operations

### Redeploy after code changes

```bash
fly deploy
```

### View logs

```bash
fly logs            # stream live logs
fly logs --app gobus  # if you have multiple apps
```

### SSH into the running machine

```bash
fly ssh console
```

Useful for inspecting the database:

```bash
sqlite3 /data/gobus.db "SELECT COUNT(*) FROM users;"
sqlite3 /data/gobus.db "SELECT username, created_at FROM users;"
```

### Scale (if needed)

The free tier runs 1 shared CPU VM. If you need more:

```bash
fly scale vm shared-cpu-2x    # double CPU/RAM
fly scale count 1              # stay at 1 instance (SQLite can't do multi-instance)
```

**Important:** Do NOT scale to multiple instances. SQLite does not support
concurrent writes from multiple processes. Always keep `count=1`.

### Force GTFS re-import

```bash
fly ssh console -C "gobus --import-gtfs"
```

### Backup the database

```bash
fly ssh sftp get /data/gobus.db ./gobus-backup.db
```

## Cost

Fly.io's free tier includes:
- 3 shared-cpu-1x VMs (you use 1)
- 256 MB RAM per VM
- 3 GB persistent volume storage (you use 1 GB)
- Unlimited bandwidth (within reason)
- Auto-SSL certificates
- Custom domains

For a transit app with <100 users, this costs **$0/month**.

If you exceed the free tier, the smallest paid plan is ~$1.94/month.

## Troubleshooting

### "Error: No volumes found"
You forgot step 3. Create the volume before deploying.

### App starts but immediately stops
Check `fly logs`. Common causes:
- Missing `GOBUS_COOKIE_SECRET` (warning, not fatal)
- Can't create database file (volume not mounted — check `fly.toml` mounts)

### "502 Bad Gateway" after deploy
The app might still be importing GTFS data. Wait 30 seconds and try again.
Check `fly logs` to see import progress.

### Custom domain shows certificate error
DNS hasn't propagated yet. Run `fly certs show yourdomain.com` to check
status. It can take up to 30 minutes.

### Users getting logged out after deploy
You didn't set `GOBUS_COOKIE_SECRET`. Without it, a new random secret is
generated on each restart. Set it with `fly secrets set` (step 4).
