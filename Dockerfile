# --- Build stage ---
FROM golang:1.24-bookworm AS builder

# Install templ CLI
RUN go install github.com/a-h/templ/cmd/templ@v0.3.977

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Generate templ files and build static binary with CGo (required for SQLite)
RUN templ generate
RUN CGO_ENABLED=1 go build -o /gobus ./cmd/gobus/

# --- Runtime stage ---
FROM debian:bookworm-slim

# SQLite needs libc; ca-certificates for HTTPS to Metro Transit APIs
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /gobus /usr/local/bin/gobus

# Data directory for SQLite database and GTFS downloads
RUN mkdir -p /data
VOLUME /data

ENV GOBUS_DB_PATH=/data/gobus.db
ENV GOBUS_GTFS_DIR=/data/gtfs

EXPOSE 8080

CMD ["gobus"]
