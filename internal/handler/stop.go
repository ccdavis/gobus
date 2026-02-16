package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"gobus/internal/templates"
)

// StopDetail serves the detail page for a single stop.
func (h *Handler) StopDetail(w http.ResponseWriter, r *http.Request) {
	stopID := r.PathValue("id")
	ctx := r.Context()
	now := time.Now()

	// Get stop info
	var stopName, stopCode string
	var stopLat, stopLon float64
	err := h.db.QueryRowContext(ctx,
		`SELECT stop_name, stop_code, stop_lat, stop_lon FROM stops WHERE stop_id = ?`,
		stopID).Scan(&stopName, &stopCode, &stopLat, &stopLon)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.logger.Error("fetching stop", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	// Get merged scheduled + realtime departures
	departures := h.fetchDepartures(ctx, stopID, now, 15)

	// Detect service interval from the first departure's route
	var interval string
	if len(departures) > 0 {
		dep := departures[0]
		// Look up direction from the scheduled data
		depRows, _ := h.db.DeparturesForStop(ctx, stopID, now, now.Format("15:04:05"), 1)
		if len(depRows) > 0 {
			interval = h.detectInterval(ctx, stopID, dep.RouteID, depRows[0].DirectionID, now)
		}
	}

	// Get alerts for this stop (from GTFS-RT feed + NexTrip)
	alerts := h.alertsForStop(ctx, stopID)

	data := templates.StopDetailData{
		Page: templates.Page{
			Title:       fmt.Sprintf("Stop %s", stopName),
			CurrentPath: "",
		},
		StopID:     stopID,
		StopName:   stopName,
		StopCode:   stopCode,
		Lat:        stopLat,
		Lon:        stopLon,
		Departures: departures,
		Interval:   interval,
		Alerts:     alerts,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.StopDetailPage(data).Render(ctx, w); err != nil {
		h.logger.Error("rendering stop detail page", "error", err)
	}
}

// formatGTFSTime converts "HH:MM:SS" (possibly >24h) to a user-friendly time string.
func formatGTFSTime(gtfsTime string) string {
	var h, m, s int
	fmt.Sscanf(gtfsTime, "%d:%d:%d", &h, &m, &s)

	// Handle times past midnight (e.g. 25:30:00 = 1:30 AM)
	displayHour := h % 24
	period := "AM"
	if displayHour >= 12 {
		period = "PM"
	}
	if displayHour == 0 {
		displayHour = 12
	} else if displayHour > 12 {
		displayHour -= 12
	}

	return fmt.Sprintf("%d:%02d %s", displayHour, m, period)
}

// minutesUntil calculates minutes from now until a GTFS time on the current day.
func minutesUntil(gtfsTime string, now time.Time) int {
	var h, m, s int
	fmt.Sscanf(gtfsTime, "%d:%d:%d", &h, &m, &s)

	depTime := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	depTime = depTime.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second)

	diff := depTime.Sub(now)
	if diff < 0 {
		return 0
	}
	return int(diff.Minutes())
}
