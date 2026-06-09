.PHONY: build dev generate test test-e2e test-all clean import-gtfs

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

# Clean build artifacts
clean:
	rm -f gobus
	find . -name '*_templ.go' -delete
