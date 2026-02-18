package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"gobus/internal/templates"
)

// LaterArrivals serves the later arrivals page for a specific route at a specific stop.
func (h *Handler) LaterArrivals(w http.ResponseWriter, r *http.Request) {
	stopID := r.PathValue("stopID")
	routeID := r.PathValue("routeID")
	directionID, _ := strconv.Atoi(r.URL.Query().Get("dir"))
	ctx := r.Context()
	now := time.Now()

	// Get stop info
	var stopName string
	err := h.db.QueryRowContext(ctx,
		`SELECT stop_name FROM stops WHERE stop_id = ?`,
		stopID).Scan(&stopName)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.logger.Error("fetching stop for later arrivals", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	// Get route info
	var routeShort, routeLong, routeColor, routeTextColor string
	err = h.db.QueryRowContext(ctx,
		`SELECT route_short_name, route_long_name, route_color, route_text_color FROM routes WHERE route_id = ?`,
		routeID).Scan(&routeShort, &routeLong, &routeColor, &routeTextColor)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.logger.Error("fetching route for later arrivals", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	if routeShort == "" {
		routeShort = routeLong
	}

	// Fetch a large number of departures and filter to this route+direction
	allDeps := h.fetchDepartures(ctx, stopID, now, 200)
	var departures []templates.DepartureInfo
	for _, dep := range allDeps {
		if dep.RouteID == routeID && dep.DirectionID == directionID {
			departures = append(departures, dep)
		}
	}

	// Filter to 18 hours
	const maxMinutes = 18 * 60
	var filtered []templates.DepartureInfo
	for _, dep := range departures {
		if dep.MinutesAway <= maxMinutes {
			filtered = append(filtered, dep)
		}
	}
	departures = filtered

	// Detect interval
	interval := h.detectInterval(ctx, stopID, routeID, directionID, now)

	// Get direction text from first departure
	directionText := ""
	if len(departures) > 0 {
		directionText = departures[0].DirectionText
	}

	data := templates.LaterArrivalsData{
		Page: h.page(fmt.Sprintf("Route %s at %s", routeShort, stopName), ""),
		StopID:         stopID,
		StopName:       stopName,
		RouteID:        routeID,
		RouteShort:     routeShort,
		RouteColor:     routeColor,
		RouteTextColor: routeTextColor,
		DirectionText:  directionText,
		DirectionID:    directionID,
		Departures:     departures,
		Interval:       interval,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.LaterArrivalsPage(data).Render(ctx, w); err != nil {
		h.logger.Error("rendering later arrivals page", "error", err)
	}
}
