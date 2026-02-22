package handler

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gobus/internal/geo"
	"gobus/internal/templates"
)

// radiusTiers defines the progressive search half-sides in meters.
// Each tier represents a square box with side = 2 * radius.
// Tuned for Minneapolis grid: ~201m N-S blocks, ~101m E-W blocks.
var radiusTiers = []float64{450, 900, 1800, 3600, 7200, 14400}

// nextRadius returns the next radius tier above the given radius.
// Returns 0, false if already at or above the maximum.
func nextRadius(current float64) (float64, bool) {
	for _, tier := range radiusTiers {
		if tier > current {
			return tier, true
		}
	}
	return 0, false
}

// dbLimitForRadius returns the R-Tree query limit and display stop limit
// for a given search radius.
func dbLimitForRadius(halfSideMeters float64) (dbLimit, displayLimit int) {
	switch {
	case halfSideMeters <= 450:
		return 15, 10
	case halfSideMeters <= 900:
		return 40, 25
	case halfSideMeters <= 1800:
		return 80, 50
	case halfSideMeters <= 3600:
		return 150, 100
	case halfSideMeters <= 7200:
		return 300, 200
	default:
		return 500, 300
	}
}

// buildRoutesMoreURL constructs the "show more" URL for the routes view.
func buildRoutesMoreURL(lat, lon string, offset int, radius float64) string {
	return fmt.Sprintf("/nearby?view=routes&lat=%s&lon=%s&offset=%d&radius=%.0f&partial=1",
		lat, lon, offset, radius)
}

// buildStopsMoreURL constructs the "show more" URL for the stops view.
func buildStopsMoreURL(lat, lon string, offset int, radius float64) string {
	return fmt.Sprintf("/nearby?view=stops&lat=%s&lon=%s&offset=%d&radius=%.0f&partial=1",
		lat, lon, offset, radius)
}

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

	radius, _ := strconv.ParseFloat(r.URL.Query().Get("radius"), 64)
	if radius <= 0 {
		radius = radiusTiers[0]
	}

	data := templates.NearbyData{
		Page:  h.page("Nearby Departures", "/nearby"),
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
				limit := 5
				stopViews, hasMore, err := h.findNearbyStopsView(r, lat, lon, offset, limit, radius)
				if err != nil {
					h.logger.Error("finding nearby stops (stop view)", "error", err)
				} else {
					// Auto-advance through empty radius tiers
					newOffset := offset + len(stopViews)
					for !hasMore && len(stopViews) == 0 && newOffset > 0 {
						nextR, ok := nextRadius(radius)
						if !ok {
							break
						}
						radius = nextR
						stopViews, hasMore, err = h.findNearbyStopsView(r, lat, lon, newOffset, limit, radius)
						if err != nil {
							h.logger.Error("finding nearby stops (stop view)", "error", err)
							break
						}
						newOffset += len(stopViews)
					}
					data.StopViews = stopViews
					data.HasStops = len(stopViews) > 0 || offset > 0
					if hasMore {
						data.HasMore = true
						data.MoreURL = buildStopsMoreURL(latStr, lonStr, newOffset, radius)
					} else if newOffset > 0 {
						if nextR, ok := nextRadius(radius); ok {
							data.HasMore = true
							data.MoreURL = buildStopsMoreURL(latStr, lonStr, newOffset, nextR)
						}
					}
				}
			default:
				limit := 5
				if partial {
					limit = 10
				}
				routes, hasMore, err := h.findNearbyRoutes(r, lat, lon, offset, limit, radius)
				if err != nil {
					h.logger.Error("finding nearby routes", "error", err)
				} else {
					// Auto-advance through empty radius tiers
					newOffset := offset + len(routes)
					for !hasMore && len(routes) == 0 && newOffset > 0 {
						nextR, ok := nextRadius(radius)
						if !ok {
							break
						}
						radius = nextR
						routes, hasMore, err = h.findNearbyRoutes(r, lat, lon, newOffset, limit, radius)
						if err != nil {
							h.logger.Error("finding nearby routes", "error", err)
							break
						}
						newOffset += len(routes)
					}
					data.Routes = routes
					data.HasStops = len(routes) > 0 || offset > 0
					if hasMore {
						data.HasMore = true
						data.MoreURL = buildRoutesMoreURL(latStr, lonStr, newOffset, radius)
					} else if newOffset > 0 {
						if nextR, ok := nextRadius(radius); ok {
							data.HasMore = true
							data.MoreURL = buildRoutesMoreURL(latStr, lonStr, newOffset, nextR)
						}
					}
				}
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if partial {
		if view == "stops" {
			moreURL := ""
			if data.HasMore {
				moreURL = data.MoreURL
			}
			if err := templates.StopNearbyPartial(data.StopViews, data.HasMore, moreURL).Render(r.Context(), w); err != nil {
				h.logger.Error("rendering stop partial", "error", err)
			}
		} else {
			moreURL := ""
			if data.HasMore {
				moreURL = data.MoreURL
			}
			if err := templates.RouteNearbyPartial(data.Routes, data.HasMore, moreURL).Render(r.Context(), w); err != nil {
				h.logger.Error("rendering route partial", "error", err)
			}
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
func (h *Handler) findNearbyRoutes(r *http.Request, lat, lon float64, offset, limit int, halfSide float64) ([]templates.RouteNearbyRow, bool, error) {
	ctx := r.Context()
	now := time.Now()

	const companionRadius = 50.0
	dbLimit, displayLimit := dbLimitForRadius(halfSide)
	latDeg, lonDeg := geo.BoundingBoxRadius(lat, halfSide)

	rows, err := h.db.NearbyStops(ctx, lat, lon, latDeg, lonDeg, dbLimit)
	if err != nil {
		return nil, false, fmt.Errorf("query nearby stops: %w", err)
	}

	// Compute distances for ordering (Haversine for display accuracy)
	type stopWithDist struct {
		row      int
		distance float64
	}
	var allStops []stopWithDist
	for i, row := range rows {
		dist := geo.Haversine(lat, lon, row.StopLat, row.StopLon)
		allStops = append(allStops, stopWithDist{row: i, distance: dist})
	}

	// Take top display stops (scaled by radius)
	displayStops := allStops
	if len(displayStops) > displayLimit {
		displayStops = allStops[:displayLimit]
	}
	displayStopIDs := make(map[string]bool)
	for _, sd := range displayStops {
		displayStopIDs[rows[sd.row].StopID] = true
	}

	// Companion stops: remaining stops within 50m of a display stop
	var companionStops []stopWithDist
	for _, candidate := range allStops[len(displayStops):] {
		cRow := rows[candidate.row]
		if displayStopIDs[cRow.StopID] {
			continue
		}
		for _, ds := range displayStops {
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

	fetchStops := append(displayStops, companionStops...)
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
			DistanceM:      geo.Haversine(lat, lon, g.stopLat, g.stopLon),
			WalkDistM:      geo.ManhattanDistance(lat, lon, g.stopLat, g.stopLon),
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
			allRoutes[pi].AltDistanceM = allRoutes[ai].DistanceM
			allRoutes[pi].AltWalkDistM = allRoutes[ai].WalkDistM
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

// findNearbyStopsView builds the stop-first view data with pagination.
// Each stop shows all routes serving it, with no cross-stop pairing.
func (h *Handler) findNearbyStopsView(r *http.Request, lat, lon float64, offset, limit int, halfSide float64) ([]templates.StopViewData, bool, error) {
	ctx := r.Context()
	now := time.Now()

	dbLimit, _ := dbLimitForRadius(halfSide)
	latDeg, lonDeg := geo.BoundingBoxRadius(lat, halfSide)

	rows, err := h.db.NearbyStops(ctx, lat, lon, latDeg, lonDeg, dbLimit)
	if err != nil {
		return nil, false, fmt.Errorf("query nearby stops: %w", err)
	}

	// Compute distances for ordering (Haversine for display accuracy)
	type stopWithDist struct {
		row      int
		distance float64
	}
	var allStops []stopWithDist
	for i, row := range rows {
		dist := geo.Haversine(lat, lon, row.StopLat, row.StopLon)
		allStops = append(allStops, stopWithDist{row: i, distance: dist})
	}

	// Paginate
	if offset >= len(allStops) {
		return nil, false, nil
	}
	end := offset + limit
	hasMore := end < len(allStops)
	if end > len(allStops) {
		end = len(allStops)
	}
	pageStops := allStops[offset:end]

	// Count stop name occurrences within the page for disambiguation
	nameCounts := make(map[string]int)
	for _, s := range pageStops {
		nameCounts[rows[s.row].StopName]++
	}

	var result []templates.StopViewData
	for _, s := range pageStops {
		row := rows[s.row]
		rg := h.fetchDeparturesForStopView(ctx, row.StopID, now)

		sv := templates.StopViewData{
			StopID:      row.StopID,
			StopName:    row.StopName,
			DistanceM:   s.distance,
			WalkDistM:   geo.ManhattanDistance(lat, lon, row.StopLat, row.StopLon),
			RouteGroups: rg,
		}

		// Disambiguate if multiple stops share the same name
		if nameCounts[row.StopName] > 1 {
			sv.StopDesc = formatStopDesc(row.StopDesc)
		}

		result = append(result, sv)
	}

	return result, hasMore, nil
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

// LocationLabel handles async reverse geocoding for the nearby page location label.
// Returns an HTML span with the street address, or 204 if unavailable.
// Caches the result per user — skips the Nominatim call if the user hasn't moved >25m.
func (h *Handler) LocationLabel(w http.ResponseWriter, r *http.Request) {
	latStr := r.URL.Query().Get("lat")
	lonStr := r.URL.Query().Get("lon")
	lat, err1 := strconv.ParseFloat(latStr, 64)
	lon, err2 := strconv.ParseFloat(lonStr, 64)
	if err1 != nil || err2 != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Identify user from session cookie for caching
	userID := int64(0)
	if cookie, err := r.Cookie(cookieName); err == nil {
		userID = h.verifyCookie(cookie.Value)
	}

	// Check cache: if user hasn't moved >25m, return cached address
	if userID > 0 {
		if cached, ok := h.locationCache.Load(userID); ok {
			cl := cached.(*cachedLocation)
			if geo.Haversine(lat, lon, cl.Lat, cl.Lon) < 25 {
				h.renderLocationLabel(w, cl.Address)
				return
			}
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()

	addr, err := h.geo.Reverse(ctx, lat, lon)
	if err != nil || addr == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Cache the result for this user
	if userID > 0 {
		h.locationCache.Store(userID, &cachedLocation{Lat: lat, Lon: lon, Address: addr})
	}

	h.renderLocationLabel(w, addr)
}

func (h *Handler) renderLocationLabel(w http.ResponseWriter, addr string) {
	escaped := html.EscapeString(addr)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<span class="location-label" role="status" aria-label="Current location: %s">%s</span>`, escaped, escaped)
}
