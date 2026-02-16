package realtime

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	gtfs "github.com/MobilityData/gtfs-realtime-bindings/golang/gtfs"
	"google.golang.org/protobuf/proto"
)

// Fetcher polls GTFS-RT feeds and updates the store.
type Fetcher struct {
	alertsURL string
	store     *Store
	client    *http.Client
	logger    *slog.Logger
}

// NewFetcher creates a GTFS-RT feed fetcher.
func NewFetcher(alertsURL string, store *Store, logger *slog.Logger) *Fetcher {
	return &Fetcher{
		alertsURL: alertsURL,
		store:     store,
		client:    &http.Client{Timeout: 15 * time.Second},
		logger:    logger,
	}
}

// Start begins polling the alerts feed. Blocks until context is cancelled.
func (f *Fetcher) Start(ctx context.Context) {
	// Fetch immediately on start
	f.fetchAlerts(ctx)

	// Then poll every 60 seconds
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			f.fetchAlerts(ctx)
		case <-ctx.Done():
			f.logger.Info("GTFS-RT fetcher stopped")
			return
		}
	}
}

func (f *Fetcher) fetchAlerts(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, "GET", f.alertsURL, nil)
	if err != nil {
		f.logger.Error("create alerts request", "error", err)
		return
	}

	resp, err := f.client.Do(req)
	if err != nil {
		f.logger.Warn("fetch alerts failed", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		f.logger.Warn("alerts feed returned non-200", "status", resp.StatusCode)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		f.logger.Error("read alerts body", "error", err)
		return
	}

	feed := &gtfs.FeedMessage{}
	if err := proto.Unmarshal(body, feed); err != nil {
		f.logger.Error("parse alerts protobuf", "error", err)
		return
	}

	var alerts []Alert
	for _, entity := range feed.GetEntity() {
		a := entity.GetAlert()
		if a == nil {
			continue
		}

		alert := Alert{
			ID:         entity.GetId(),
			HeaderText: getTranslation(a.GetHeaderText()),
			DescText:   getTranslation(a.GetDescriptionText()),
			Effect:     a.GetEffect().String(),
			Cause:      a.GetCause().String(),
		}

		// Collect affected routes and stops (deduplicated)
		routeSet := make(map[string]bool)
		stopSet := make(map[string]bool)
		for _, ie := range a.GetInformedEntity() {
			if rid := ie.GetRouteId(); rid != "" && !routeSet[rid] {
				alert.RouteIDs = append(alert.RouteIDs, rid)
				routeSet[rid] = true
			}
			if sid := ie.GetStopId(); sid != "" && !stopSet[sid] {
				alert.StopIDs = append(alert.StopIDs, sid)
				stopSet[sid] = true
			}
		}

		alerts = append(alerts, alert)
	}

	f.store.SetAlerts(alerts)
	f.logger.Info("GTFS-RT alerts updated", "count", len(alerts))
}

func getTranslation(ts *gtfs.TranslatedString) string {
	if ts == nil {
		return ""
	}
	for _, t := range ts.GetTranslation() {
		if text := t.GetText(); text != "" {
			return text
		}
	}
	return ""
}

// FormatAlertEffect returns a human-readable effect description.
func FormatAlertEffect(effect string) string {
	switch effect {
	case "NO_SERVICE":
		return "No Service"
	case "REDUCED_SERVICE":
		return "Reduced Service"
	case "SIGNIFICANT_DELAYS":
		return "Significant Delays"
	case "DETOUR":
		return "Detour"
	case "ADDITIONAL_SERVICE":
		return "Additional Service"
	case "MODIFIED_SERVICE":
		return "Modified Service"
	case "STOP_MOVED":
		return "Stop Moved"
	default:
		return fmt.Sprintf("Alert")
	}
}
