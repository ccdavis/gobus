package handler

import (
	"fmt"
	"net/http"
	"time"

	"gobus/internal/templates"
)

// RouteList serves the route explorer page.
func (h *Handler) RouteList(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.AllRoutes(r.Context())
	if err != nil {
		h.logger.Error("fetching routes", "error", err)
	}

	var routes []templates.RouteInfo
	for _, row := range rows {
		short := row.RouteShort
		if short == "" {
			short = row.RouteLong
		}
		routes = append(routes, templates.RouteInfo{
			RouteID:        row.RouteID,
			RouteShort:     short,
			RouteLong:      row.RouteLong,
			RouteColor:     row.RouteColor,
			RouteTextColor: row.RouteTextColor,
			RouteType:      row.RouteType,
		})
	}

	data := templates.RouteListData{
		Page: h.page("Route Explorer", "/routes"),
		Routes: routes,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.RouteListPage(data).Render(r.Context(), w); err != nil {
		h.logger.Error("rendering route list page", "error", err)
	}
}

// RouteDetail serves the detail page for a single route.
func (h *Handler) RouteDetail(w http.ResponseWriter, r *http.Request) {
	routeID := r.PathValue("id")
	now := time.Now()

	// Get route info
	routes, err := h.db.AllRoutes(r.Context())
	if err != nil {
		h.logger.Error("fetching route", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	var routeInfo templates.RouteInfo
	found := false
	for _, row := range routes {
		if row.RouteID == routeID {
			routeInfo = templates.RouteInfo{
				RouteID:        row.RouteID,
				RouteShort:     row.RouteShort,
				RouteLong:      row.RouteLong,
				RouteColor:     row.RouteColor,
				RouteTextColor: row.RouteTextColor,
				RouteType:      row.RouteType,
			}
			found = true
			break
		}
	}
	if !found {
		http.NotFound(w, r)
		return
	}

	// Get stops for each direction
	var directions []templates.DirectionStops
	for _, dirID := range []int{0, 1} {
		stops, err := h.db.StopsForRoute(r.Context(), routeID, dirID, now)
		if err != nil {
			continue // No stops in this direction
		}
		if len(stops) == 0 {
			continue
		}

		dirName := directionName(dirID)
		var routeStops []templates.RouteStop
		for _, s := range stops {
			routeStops = append(routeStops, templates.RouteStop{
				StopID:   s.StopID,
				StopName: s.StopName,
				Sequence: s.StopSequence,
			})
		}

		directions = append(directions, templates.DirectionStops{
			DirectionID:   dirID,
			DirectionName: dirName,
			Stops:         routeStops,
		})
	}

	// Get alerts for this route
	routeAlerts := h.alertsForRoute(routeID)

	data := templates.RouteDetailData{
		Page: h.page(fmt.Sprintf("Route %s", routeInfo.RouteShort), "/routes"),
		RouteID:        routeInfo.RouteID,
		RouteShort:     routeInfo.RouteShort,
		RouteLong:      routeInfo.RouteLong,
		RouteColor:     routeInfo.RouteColor,
		RouteTextColor: routeInfo.RouteTextColor,
		RouteType:      routeInfo.RouteType,
		Directions:     directions,
		Alerts:         routeAlerts,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.RouteDetailPage(data).Render(r.Context(), w); err != nil {
		h.logger.Error("rendering route detail page", "error", err)
	}
}

func directionName(id int) string {
	switch id {
	case 0:
		return "Outbound"
	case 1:
		return "Inbound"
	default:
		return fmt.Sprintf("Direction %d", id)
	}
}
