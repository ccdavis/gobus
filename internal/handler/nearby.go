package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gobus/internal/geo"
	"gobus/internal/templates"
)

// Nearby serves the nearby departures page.
func (h *Handler) Nearby(w http.ResponseWriter, r *http.Request) {
	latStr := r.URL.Query().Get("lat")
	lonStr := r.URL.Query().Get("lon")
	query := r.URL.Query().Get("q")
	view := r.URL.Query().Get("view")
	if view != "stops" {
		view = "routes"
	}
	partial := r.URL.Query().Get("partial") == "1"
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	data := templates.NearbyData{
		Page: h.page("Nearby Departures", "/nearby"),
		View:  view,
		Lat:   latStr,
		Lon:   lonStr,
		Query: query,
	}

	// If we have coordinates, find nearby stops/routes
	if latStr != "" && lonStr != "" {
		lat, err1 := strconv.ParseFloat(latStr, 64)
		lon, err2 := strconv.ParseFloat(lonStr, 64)
		if err1 == nil && err2 == nil {
			switch view {
			case "stops":
				stopViews, err := h.findNearbyStopsView(r, lat, lon)
				if err != nil {
					h.logger.Error("finding nearby stops (stop view)", "error", err)
				} else {
					data.StopViews = stopViews
					data.HasStops = len(stopViews) > 0
				}
			default:
				limit := 5
				if partial {
					limit = 10
				}
				routes, hasMore, err := h.findNearbyRoutes(r, lat, lon, offset, limit)
				if err != nil {
					h.logger.Error("finding nearby routes", "error", err)
				} else {
					data.Routes = routes
					data.HasStops = len(routes) > 0
					data.HasMore = hasMore
					if hasMore {
						data.MoreURL = fmt.Sprintf("/nearby?view=routes&lat=%s&lon=%s&offset=%d&partial=1",
							latStr, lonStr, offset+len(routes))
					}
				}
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if partial {
		// HTMX "More" request: return just the rows + OOB load-more update
		moreURL := ""
		if data.HasMore {
			moreURL = data.MoreURL
		}
		if err := templates.RouteNearbyPartial(data.Routes, data.HasMore, moreURL).Render(r.Context(), w); err != nil {
			h.logger.Error("rendering route partial", "error", err)
		}
		return
	}
	if err := templates.NearbyPage(data).Render(r.Context(), w); err != nil {
		h.logger.Error("rendering nearby page", "error", err)
	}
}

// findNearbyRoutes builds the flat route-first nearby view data.
// It queries a wider area than the stop view, groups departures by route+direction,
// pairs opposite directions across nearby stops, computes intervals, and paginates.
func (h *Handler) findNearbyRoutes(r *http.Request, lat, lon float64, offset, limit int) ([]templates.RouteNearbyRow, bool, error) {
	ctx := r.Context()
	now := time.Now()

	const maxRadiusMeters = 3218.0 // 2 miles
	const companionRadius = 50.0
	latDeg, lonDeg := geo.BoundingBoxRadius(lat, maxRadiusMeters)
	radiusDeg := latDeg
	if lonDeg > radiusDeg {
		radiusDeg = lonDeg
	}

	rows, err := h.db.NearbyStops(ctx, lat, lon, radiusDeg, 50)
	if err != nil {
		return nil, false, fmt.Errorf("query nearby stops: %w", err)
	}

	// Compute Haversine distances
	type stopWithDist struct {
		row      int
		distance float64
	}
	var allStops []stopWithDist
	for i, row := range rows {
		dist := geo.Haversine(lat, lon, row.StopLat, row.StopLon)
		if dist <= maxRadiusMeters {
			allStops = append(allStops, stopWithDist{row: i, distance: dist})
		}
	}

	// Take top 20 display stops (wider net than stop view)
	displayLimit := 20
	if len(allStops) > displayLimit {
		allStops = allStops[:displayLimit]
	}
	displayStopIDs := make(map[string]bool)
	for _, sd := range allStops {
		displayStopIDs[rows[sd.row].StopID] = true
	}

	// Companion stops within 50m of display stops
	var companionStops []stopWithDist
	for i := displayLimit; i < len(allStops); i++ {
		candidate := allStops[i]
		cRow := rows[candidate.row]
		if displayStopIDs[cRow.StopID] {
			continue
		}
		for _, ds := range allStops[:displayLimit] {
			dRow := rows[ds.row]
			dist := geo.Haversine(dRow.StopLat, dRow.StopLon, cRow.StopLat, cRow.StopLon)
			if dist <= companionRadius {
				companionStops = append(companionStops, candidate)
				displayStopIDs[cRow.StopID] = true
				break
			}
		}
	}

	// Fetch raw departures for all stops, build route groups
	type routeKey struct {
		routeID     string
		directionID int
	}
	type routeGroup struct {
		deps     []templates.DepartureInfo
		stopID   string
		stopName string
		stopLat  float64
		stopLon  float64
	}
	groups := make(map[routeKey]*routeGroup)
	var order []routeKey

	fetchStops := append(allStops, companionStops...)
	for _, sd := range fetchStops {
		row := rows[sd.row]
		deps := h.fetchDepartures(ctx, row.StopID, now, 30)
		for _, dep := range deps {
			key := routeKey{dep.RouteID, dep.DirectionID}
			if g, ok := groups[key]; ok {
				if len(g.deps) < 3 {
					g.deps = append(g.deps, dep)
				}
			} else {
				groups[key] = &routeGroup{
					deps:     []templates.DepartureInfo{dep},
					stopID:   row.StopID,
					stopName: row.StopName,
					stopLat:  row.StopLat,
					stopLon:  row.StopLon,
				}
				order = append(order, key)
			}
		}
	}

	// Build RouteNearbyRows from groups
	var allRoutes []templates.RouteNearbyRow
	for _, key := range order {
		g := groups[key]
		if len(g.deps) == 0 {
			continue
		}
		dep := g.deps[0]

		row := templates.RouteNearbyRow{
			RouteID:        dep.RouteID,
			RouteShort:     dep.RouteShort,
			RouteColor:     dep.RouteColor,
			RouteTextColor: dep.RouteTextColor,
			RouteName:      dep.Headsign,
			DirectionText:  dep.DirectionText,
			DirectionID:    dep.DirectionID,
			StopID:         g.stopID,
			StopName:       g.stopName,
			Scheduled:      dep.Scheduled,
			Realtime:       dep.Realtime,
			MinutesAway:    dep.MinutesAway,
			IsRealtime:     dep.IsRealtime,
			IsLate:         dep.IsLate,
		}

		// Later times
		for i := 1; i < len(g.deps); i++ {
			d := g.deps[i]
			t := d.Scheduled
			if d.IsRealtime && d.Realtime != "" {
				t = d.Realtime
			}
			row.LaterTimes = append(row.LaterTimes, templates.LaterArrival{
				Time:        t,
				MinutesAway: d.MinutesAway,
				IsRealtime:  d.IsRealtime,
			})
		}

		// Interval detection per route+direction at this stop
		row.Interval = h.detectInterval(ctx, g.stopID, dep.RouteID, dep.DirectionID, now)

		allRoutes = append(allRoutes, row)
	}

	// Cross-stop direction pairing
	for i := range allRoutes {
		if allRoutes[i].HasAlt {
			continue
		}
		// Find opposite direction
		for j := range allRoutes {
			if i == j || allRoutes[j].HasAlt {
				continue
			}
			if allRoutes[i].RouteID != allRoutes[j].RouteID {
				continue
			}
			if allRoutes[i].DirectionID == allRoutes[j].DirectionID {
				continue
			}
			// Same route, opposite direction — pair them
			// Keep the soonest as primary
			pi, ai := i, j
			if allRoutes[j].MinutesAway < allRoutes[i].MinutesAway {
				pi, ai = j, i
			}
			allRoutes[pi].HasAlt = true
			allRoutes[pi].AltDirectionText = allRoutes[ai].DirectionText
			allRoutes[pi].AltStopID = allRoutes[ai].StopID
			allRoutes[pi].AltStopName = allRoutes[ai].StopName
			allRoutes[pi].AltRouteName = allRoutes[ai].RouteName
			allRoutes[pi].AltScheduled = allRoutes[ai].Scheduled
			allRoutes[pi].AltRealtime = allRoutes[ai].Realtime
			allRoutes[pi].AltMinutesAway = allRoutes[ai].MinutesAway
			allRoutes[pi].AltIsRealtime = allRoutes[ai].IsRealtime
			allRoutes[pi].AltIsLate = allRoutes[ai].IsLate
			allRoutes[pi].AltLaterTimes = allRoutes[ai].LaterTimes
			allRoutes[pi].AltInterval = allRoutes[ai].Interval
			// Mark alt for removal
			allRoutes[ai].RouteID = "" // sentinel for removal
			break
		}
	}

	// Remove paired-away rows
	var cleaned []templates.RouteNearbyRow
	for _, row := range allRoutes {
		if row.RouteID != "" {
			cleaned = append(cleaned, row)
		}
	}

	// Paginate
	if offset >= len(cleaned) {
		return nil, false, nil
	}
	end := offset + limit
	hasMore := end < len(cleaned)
	if end > len(cleaned) {
		end = len(cleaned)
	}

	return cleaned[offset:end], hasMore, nil
}

// findNearbyStopsView builds the stop-first view data.
// Each stop shows all routes serving it, with no cross-stop pairing.
func (h *Handler) findNearbyStopsView(r *http.Request, lat, lon float64) ([]templates.StopViewData, error) {
	ctx := r.Context()
	now := time.Now()

	const maxRadiusMeters = 3218.0 // 2 miles
	latDeg, lonDeg := geo.BoundingBoxRadius(lat, maxRadiusMeters)
	radiusDeg := latDeg
	if lonDeg > radiusDeg {
		radiusDeg = lonDeg
	}

	rows, err := h.db.NearbyStops(ctx, lat, lon, radiusDeg, 50)
	if err != nil {
		return nil, fmt.Errorf("query nearby stops: %w", err)
	}

	// Compute Haversine distances, filter to max radius
	type stopWithDist struct {
		row      int
		distance float64
	}
	var allStops []stopWithDist
	for i, row := range rows {
		dist := geo.Haversine(lat, lon, row.StopLat, row.StopLon)
		if dist <= maxRadiusMeters {
			allStops = append(allStops, stopWithDist{row: i, distance: dist})
		}
	}

	// Take top 5 stops
	displayLimit := 5
	if len(allStops) > displayLimit {
		allStops = allStops[:displayLimit]
	}

	// Count stop name occurrences for disambiguation
	nameCounts := make(map[string]int)
	for _, s := range allStops {
		nameCounts[rows[s.row].StopName]++
	}

	var result []templates.StopViewData
	for _, s := range allStops {
		row := rows[s.row]
		rg := h.fetchDeparturesForStopView(ctx, row.StopID, now)

		sv := templates.StopViewData{
			StopID:      row.StopID,
			StopName:    row.StopName,
			Distance:    formatDistance(s.distance),
			RouteGroups: rg,
		}

		// Disambiguate if multiple stops share the same name
		if nameCounts[row.StopName] > 1 {
			sv.StopDesc = formatStopDesc(row.StopDesc)
		}

		result = append(result, sv)
	}

	return result, nil
}

// formatStopDesc converts GTFS stop_desc to a user-friendly label.
// GTFS values like "Nearside S" or "Farside N" → "Southbound side", "Northbound side".
func formatStopDesc(desc string) string {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return ""
	}
	parts := strings.Fields(desc)
	dir := parts[len(parts)-1]
	switch dir {
	case "N":
		return "Northbound side"
	case "S":
		return "Southbound side"
	case "E":
		return "Eastbound side"
	case "W":
		return "Westbound side"
	default:
		return desc
	}
}

func formatDistance(meters float64) string {
	if meters < 1000 {
		return fmt.Sprintf("%d m", int(meters))
	}
	miles := geo.MetersToMiles(meters)
	return fmt.Sprintf("%.1f mi", miles)
}

