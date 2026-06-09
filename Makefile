.PHONY: build dev generate test test-e2e test-all clean import-gtfs prebuilt-db ios-framework

# CGo is required for mattn/go-sqlite3
export CGO_ENABLED := 1

# Build the binary
build: generate
	go build -o gobus ./cmd/gobus

# Run dev server
dev: generate
	go run ./cmd/gobus

# Generate templ files
generate:
	$(HOME)/go/bin/templ generate

# Go unit tests
test:
	go test ./...

# Playwright E2E tests
test-e2e: build
	cd e2e && npm test

# All tests
test-all: test test-e2e

# Force GTFS download and import
import-gtfs: build
	./gobus --import-gtfs

# Build a prebuilt SQLite DB (with GTFS schedule data) to bundle into the
# iPhone app. Output: dist/gobus.db
prebuilt-db: build
	mkdir -p dist
	GOBUS_DB_PATH=dist/gobus.db GOBUS_GTFS_DIR=dist/gtfs ./gobus --import-gtfs
	@echo "Prebuilt DB ready at dist/gobus.db"

# Build the iOS xcframework from the mobile package.
# Requires macOS + Xcode + gomobile:
#   go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init
# See NATIVE_APP_PLAN.md Phase 2. (Will not run on Linux/WSL2.)
ios-framework:
	mkdir -p build
	gomobile bind -target=ios -o build/Gobus.xcframework ./mobile
	@echo "Built build/Gobus.xcframework"

# Clean build artifacts
clean:
	rm -f gobus
	rm -rf dist build
	find . -name '*_templ.go' -delete
