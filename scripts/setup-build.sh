#!/usr/bin/env bash
#
# GoBus build environment setup
# Installs Go 1.24, templ CLI, and builds the binary.
# Works on Linux (x86_64/arm64) and macOS (arm64/x86_64).
#
set -euo pipefail

GO_VERSION="1.24.0"
TEMPL_VERSION="v0.3.977"

# --- Detect platform ---

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
    x86_64)  GOARCH="amd64" ;;
    aarch64) GOARCH="arm64" ;;
    arm64)   GOARCH="arm64" ;;
    *)       echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

echo "==> Platform: $OS/$GOARCH"

# --- Check for C compiler ---

if ! command -v gcc &>/dev/null && ! command -v cc &>/dev/null; then
    echo "==> No C compiler found (required for SQLite)"
    if [ "$OS" = "linux" ]; then
        echo "    Installing gcc..."
        if command -v apt-get &>/dev/null; then
            sudo apt-get update -qq && sudo apt-get install -y -qq gcc build-essential
        elif command -v dnf &>/dev/null; then
            sudo dnf install -y gcc
        elif command -v yum &>/dev/null; then
            sudo yum install -y gcc
        else
            echo "    Please install gcc manually and re-run this script."
            exit 1
        fi
    elif [ "$OS" = "darwin" ]; then
        echo "    Installing Xcode command line tools..."
        xcode-select --install 2>/dev/null || true
        echo "    If a dialog appeared, click Install, wait for it to finish, then re-run this script."
        exit 1
    fi
fi

echo "==> C compiler: $(cc --version 2>&1 | head -1)"

# --- Install Go ---

install_go() {
    local tarball="go${GO_VERSION}.${OS}-${GOARCH}.tar.gz"
    local url="https://go.dev/dl/${tarball}"

    echo "==> Downloading Go ${GO_VERSION}..."
    curl -fsSL "$url" -o "/tmp/${tarball}"

    echo "==> Installing to /usr/local/go..."
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf "/tmp/${tarball}"
    rm -f "/tmp/${tarball}"
}

# Check if Go is installed and at the right version
if command -v go &>/dev/null; then
    CURRENT_GO=$(go version | grep -oP 'go\K[0-9]+\.[0-9]+' || go version | sed 's/.*go\([0-9]*\.[0-9]*\).*/\1/')
    WANT_GO=$(echo "$GO_VERSION" | grep -oP '^[0-9]+\.[0-9]+')
    if [ "$CURRENT_GO" = "$WANT_GO" ]; then
        echo "==> Go ${CURRENT_GO} already installed"
    else
        echo "==> Go ${CURRENT_GO} found, need ${WANT_GO}"
        install_go
    fi
else
    install_go
fi

# Ensure Go is in PATH for the rest of this script
export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"
echo "==> Using: $(go version)"

# --- Install templ ---

if command -v templ &>/dev/null; then
    echo "==> templ already installed: $(templ version 2>/dev/null || echo 'unknown version')"
else
    echo "==> Installing templ ${TEMPL_VERSION}..."
    go install "github.com/a-h/templ/cmd/templ@${TEMPL_VERSION}"
fi

# --- Build ---

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

echo "==> Building GoBus..."
cd "$PROJECT_DIR"
templ generate
CGO_ENABLED=1 go build -o gobus ./cmd/gobus/

echo ""
echo "==> Build complete!"
echo "    Binary: ${PROJECT_DIR}/gobus ($(du -h gobus | cut -f1))"
echo ""
echo "    Run with: ./gobus"
echo ""
echo "    To add Go to your PATH permanently, add this to your shell profile:"
echo "      export PATH=/usr/local/go/bin:\$HOME/go/bin:\$PATH"
