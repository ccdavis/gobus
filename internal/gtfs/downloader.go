package gtfs

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
)

// Downloader handles GTFS zip file downloads with conditional requests.
type Downloader struct {
	client *http.Client
	url    string
	dir    string // Directory to store downloaded files
	logger *slog.Logger
}

// NewDownloader creates a Downloader for the given GTFS URL.
func NewDownloader(url, dir string, logger *slog.Logger) *Downloader {
	return &Downloader{
		client: &http.Client{},
		url:    url,
		dir:    dir,
		logger: logger,
	}
}

// CheckResult holds the result of a conditional check.
type CheckResult struct {
	NeedsUpdate  bool
	LastModified string
	ETag         string
}

// Check sends a HEAD request with If-Modified-Since to see if the feed has changed.
func (d *Downloader) Check(ctx context.Context, lastModified, etag string) (*CheckResult, error) {
	req, err := http.NewRequestWithContext(ctx, "HEAD", d.url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HEAD request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		d.logger.Info("GTFS feed not modified")
		return &CheckResult{NeedsUpdate: false}, nil
	}

	return &CheckResult{
		NeedsUpdate:  true,
		LastModified: resp.Header.Get("Last-Modified"),
		ETag:         resp.Header.Get("ETag"),
	}, nil
}

// Download fetches the GTFS zip and saves it to a temp file.
// Returns the path to the downloaded file and the response headers.
func (d *Downloader) Download(ctx context.Context) (path string, lastModified string, etag string, err error) {
	if err := os.MkdirAll(d.dir, 0755); err != nil {
		return "", "", "", fmt.Errorf("create dir: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", d.url, nil)
	if err != nil {
		return "", "", "", fmt.Errorf("create request: %w", err)
	}

	d.logger.Info("downloading GTFS feed", "url", d.url)
	resp, err := d.client.Do(req)
	if err != nil {
		return "", "", "", fmt.Errorf("GET request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp(d.dir, "gtfs-*.zip")
	if err != nil {
		return "", "", "", fmt.Errorf("create temp file: %w", err)
	}
	defer tmpFile.Close()

	written, err := io.Copy(tmpFile, resp.Body)
	if err != nil {
		os.Remove(tmpFile.Name())
		return "", "", "", fmt.Errorf("write file: %w", err)
	}

	path = tmpFile.Name()
	lastModified = resp.Header.Get("Last-Modified")
	etag = resp.Header.Get("ETag")

	d.logger.Info("GTFS feed downloaded",
		"path", filepath.Base(path),
		"size_mb", fmt.Sprintf("%.1f", float64(written)/(1024*1024)),
	)
	return path, lastModified, etag, nil
}
