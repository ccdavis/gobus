package handler

import (
	"context"
	"fmt"
	"sort"
	"time"

	"gobus/internal/nextrip"
	"gobus/internal/templates"
)

// fetchDepartures gets merged scheduled + realtime departures for a stop.
// Returns up to `limit` departures sorted by time.
func (h *Handler) fetchDepartures(ctx context.Context, stopID string, now time.Time, limit int) []templates.DepartureInfo {
	// 1. Get scheduled departures from GTFS
	afterTime := now.Format("15:04:05")
	schedRows, err := h.db.DeparturesForStop(ctx, stopID, now, afterTime, limit*2)
	if err != nil {
		h.logger.Error("fetching scheduled departures", "stop", stopID, "error", err)
	}

	// 2. Get realtime departures from NexTrip API
	var rtDeps []nextrip.Departure
	ntResp, err := h.nt.DeparturesForStop(ctx, stopID)
	if err != nil {
		h.logger.Warn("NexTrip API unavailable, using schedule only", "stop", stopID, "error", err)
	} else {
		rtDeps = ntResp.Departures
	}

	// 3. Build a map of realtime data keyed by trip_id for merging
	rtByTrip := make(map[string]nextrip.Departure)
	for _, d := range rtDeps {
		rtByTrip[d.TripID] = d
	}

	// 4. Merge: start with scheduled, overlay realtime where available
	seen := make(map[string]bool) // track trip_ids we've already included
	var result []templates.DepartureInfo

	for _, sched := range schedRows {
		scheduledTime := formatGTFSTime(sched.DepartureTime)
		minutesAway := minutesUntil(sched.DepartureTime, now)

		// Use route_short_name, fall back to route_long_name
		routeShort := sched.RouteShort
		if routeShort == "" {
			routeShort = sched.RouteLong
		}

		dep := templates.DepartureInfo{
			RouteID:     sched.RouteID,
			RouteShort:  routeShort,
			RouteColor:  sched.RouteColor,
			Headsign:    sched.TripHeadsign,
			Scheduled:   scheduledTime,
			MinutesAway: minutesAway,
		}

		// Overlay realtime data if available for this trip
		if rt, ok := rtByTrip[sched.TripID]; ok {
			dep.IsRealtime = rt.Actual
			// Use NexTrip short name if GTFS short name was empty
			if sched.RouteShort == "" && rt.RouteShortName != "" {
				dep.RouteShort = rt.RouteShortName
			}
			if rt.Actual {
				rtTime := time.Unix(rt.DepartureTime, 0).In(now.Location())
				dep.Realtime = rtTime.Format("3:04 PM")
				dep.MinutesAway = int(time.Until(rtTime).Minutes())
				if dep.MinutesAway < 0 {
					dep.MinutesAway = 0
				}
				// Check if late (realtime > scheduled by 2+ minutes)
				schedMins := minutesUntil(sched.DepartureTime, now)
				dep.IsLate = dep.MinutesAway > schedMins+2
			}
			seen[sched.TripID] = true
		}

		result = append(result, dep)
	}

	// 5. Add any realtime-only departures not in the schedule (extra trips)
	for _, rt := range rtDeps {
		if seen[rt.TripID] {
			continue
		}
		rtTime := time.Unix(rt.DepartureTime, 0).In(now.Location())
		minutesAway := int(time.Until(rtTime).Minutes())
		if minutesAway < 0 {
			continue
		}

		result = append(result, templates.DepartureInfo{
			RouteID:    rt.RouteID,
			RouteShort: rt.RouteShortName,
			Headsign:   rt.Description,
			Scheduled:  rtTime.Format("3:04 PM"),
			Realtime:   rtTime.Format("3:04 PM"),
			MinutesAway: minutesAway,
			IsRealtime: rt.Actual,
		})
	}

	// 6. Sort by minutes away
	sort.Slice(result, func(i, j int) bool {
		return result[i].MinutesAway < result[j].MinutesAway
	})

	// 7. Limit
	if len(result) > limit {
		result = result[:limit]
	}

	return result
}

// fetchDeparturesGrouped returns departures grouped by route,
// with up to 3 next arrivals per route. Used for the nearby view.
func (h *Handler) fetchDeparturesGrouped(ctx context.Context, stopID string, now time.Time) []templates.DepartureInfo {
	allDeps := h.fetchDepartures(ctx, stopID, now, 30)

	// Group by route+direction, take first 3 per group
	type routeKey struct {
		routeID   string
		headsign  string
	}
	groups := make(map[routeKey][]templates.DepartureInfo)
	var order []routeKey

	for _, dep := range allDeps {
		key := routeKey{dep.RouteID, dep.Headsign}
		if _, exists := groups[key]; !exists {
			order = append(order, key)
		}
		if len(groups[key]) < 3 {
			groups[key] = append(groups[key], dep)
		}
	}

	// Flatten: return the first departure per route (for sorting the stop),
	// but include the next 2 as additional times in a formatted string
	var result []templates.DepartureInfo
	for _, key := range order {
		deps := groups[key]
		if len(deps) == 0 {
			continue
		}
		primary := deps[0]
		// Format additional times into the headsign line
		if len(deps) > 1 {
			also := "Also: "
			for i := 1; i < len(deps); i++ {
				if i > 1 {
					also += ", "
				}
				if deps[i].IsRealtime && deps[i].Realtime != "" {
					also += deps[i].Realtime
				} else {
					also += deps[i].Scheduled
				}
				also += fmt.Sprintf(" (%d min)", deps[i].MinutesAway)
			}
			primary.Headsign = primary.Headsign + " — " + also
		}
		result = append(result, primary)
	}

	return result
}
