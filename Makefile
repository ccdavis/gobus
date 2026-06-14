.PHONY: build dev generate test test-e2e test-all clean import-gtfs prebuilt-db ios-framework ios-app ios-run

# iOS Simulator device used by the ios-run target.
IOS_SIM ?= iPhone 16 Pro

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
# See ios/README.md. (Will not run on Linux/WSL2.)
ios-framework:
	mkdir -p build
	gomobile bind -target=ios -o build/GobusKit.xcframework ./mobile
	@echo "Built build/GobusKit.xcframework"

# Generate the Xcode project and build the app for the iOS Simulator.
# Requires xcodegen + full Xcode. Builds the prebuilt DB only if missing
# (prebuilt-db force-reimports GTFS, which is slow); delete dist/gobus.db to refresh.
ios-app: ios-framework
	@test -f dist/gobus.db || $(MAKE) prebuilt-db
	cd ios && xcodegen generate
	cd ios && xcodebuild -project Gobus.xcodeproj -scheme Gobus \
		-sdk iphonesimulator -configuration Debug \
		-destination 'platform=iOS Simulator,name=$(IOS_SIM)' \
		-derivedDataPath build/dd CODE_SIGNING_ALLOWED=NO build
	@echo "Built ios/build/dd/Build/Products/Debug-iphonesimulator/Gobus.app"

# Boot the simulator, install, and launch the app.
ios-run: ios-app
	xcrun simctl boot '$(IOS_SIM)' || true
	open -a Simulator
	xcrun simctl install '$(IOS_SIM)' ios/build/dd/Build/Products/Debug-iphonesimulator/Gobus.app
	xcrun simctl launch --console-pty '$(IOS_SIM)' com.gobus.app

# Clean build artifacts
clean:
	rm -f gobus
	rm -rf dist build
	find . -name '*_templ.go' -delete
