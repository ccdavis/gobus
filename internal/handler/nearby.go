package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"gobus/internal/geo"
	"gobus/internal/templates"
)

// Nearby serves the nearby departures page.
func (h *Handler) Nearby(w http.ResponseWriter, r *http.Request) {
	latStr := r.URL.Query().Get("lat")
	lonStr := r.URL.Query().Get("lon")
	query := r.URL.Query().Get("q")

	data := templates.NearbyData{
		Page: templates.Page{
			Title:       "Nearby Departures",
			CurrentPath: "/nearby",
		},
		Lat:   latStr,
		Lon:   lonStr,
		Query: query,
	}

	// If we have coordinates, find nearby stops
	if latStr != "" && lonStr != "" {
		lat, err1 := strconv.ParseFloat(latStr, 64)
		lon, err2 := strconv.ParseFloat(lonStr, 64)
		if err1 == nil && err2 == nil {
			stops, err := h.findNearbyStops(r, lat, lon)
			if err != nil {
				h.logger.Error("finding nearby stops", "error", err)
			} else {
				data.Stops = stops
				data.HasStops = len(stops) > 0
			}
		}
	}

	// TODO: handle manual address query geocoding

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.NearbyPage(data).Render(r.Context(), w); err != nil {
		h.logger.Error("rendering nearby page", "error", err)
	}
}

// findNearbyStops queries the database for stops near the given coordinates
// and builds the template data with departure information.
func (h *Handler) findNearbyStops(r *http.Request, lat, lon float64) ([]templates.NearbyStop, error) {
	ctx := r.Context()
	now := time.Now()

	// Start with 2 mile radius (display rule: show up to 5 within 2 miles)
	const maxRadiusMeters = 3218.0 // 2 miles
	latDeg, lonDeg := geo.BoundingBoxRadius(lat, maxRadiusMeters)

	// Use the larger of latDeg/lonDeg for the bounding box query
	radiusDeg := latDeg
	if lonDeg > radiusDeg {
		radiusDeg = lonDeg
	}

	rows, err := h.db.NearbyStops(ctx, lat, lon, radiusDeg, 50)
	if err != nil {
		return nil, fmt.Errorf("query nearby stops: %w", err)
	}

	// Compute Haversine distances and apply display rules
	type stopWithDist struct {
		row      int
		distance float64
	}
	var stopsWithDist []stopWithDist
	for i, row := range rows {
		dist := geo.Haversine(lat, lon, row.StopLat, row.StopLon)
		if dist <= maxRadiusMeters {
			stopsWithDist = append(stopsWithDist, stopWithDist{row: i, distance: dist})
		}
	}

	// Count stops within 100 meters
	within100m := 0
	for _, s := range stopsWithDist {
		if s.distance <= 100 {
			within100m++
		}
	}

	// Apply display rules: show 5, "more" button if >5 within 100m
	limit := 5
	showMore := within100m > 5
	if len(stopsWithDist) > limit {
		stopsWithDist = stopsWithDist[:limit]
	}

	var result []templates.NearbyStop
	for i, sd := range stopsWithDist {
		row := rows[sd.row]
		distStr := formatDistance(sd.distance)

		// Fetch departures grouped by route (up to 3 next arrivals per route)
		deps := h.fetchDeparturesGrouped(ctx, row.StopID, now)

		ns := templates.NearbyStop{
			StopID:     row.StopID,
			StopName:   row.StopName,
			Distance:   distStr,
			Departures: deps,
			ShowMore:   showMore && i == len(stopsWithDist)-1,
		}
		result = append(result, ns)
	}

	return result, nil
}

func formatDistance(meters float64) string {
	if meters < 1000 {
		return fmt.Sprintf("%d m", int(meters))
	}
	miles := geo.MetersToMiles(meters)
	return fmt.Sprintf("%.1f mi", miles)
}
