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
	// Also build a map of direction text by route+direction_id for fallback
	dirTextByRouteDir := make(map[string]string)
	for _, d := range rtDeps {
		rtByTrip[d.TripID] = d
		if d.DirectionText != "" {
			key := fmt.Sprintf("%s:%d", d.RouteID, d.DirectionID)
			dirTextByRouteDir[key] = expandDirectionText(d.DirectionText)
		}
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
			DirectionID: sched.DirectionID,
			Scheduled:   scheduledTime,
			MinutesAway: minutesAway,
		}

		// Try to get direction text from NexTrip data for this route+direction
		dirKey := fmt.Sprintf("%s:%d", sched.RouteID, sched.DirectionID)
		dep.DirectionText = dirTextByRouteDir[dirKey]

		// Overlay realtime data if available for this trip
		if rt, ok := rtByTrip[sched.TripID]; ok {
			dep.IsRealtime = rt.Actual
			dep.DirectionText = expandDirectionText(rt.DirectionText)
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
			RouteID:       rt.RouteID,
			RouteShort:    rt.RouteShortName,
			Headsign:      rt.Description,
			DirectionText: expandDirectionText(rt.DirectionText),
			DirectionID:   rt.DirectionID,
			Scheduled:     rtTime.Format("3:04 PM"),
			Realtime:      rtTime.Format("3:04 PM"),
			MinutesAway:   minutesAway,
			IsRealtime:    rt.Actual,
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

// fetchDeparturesForStopView returns departures grouped by route+direction
// with individual time entries (for the stops-centric nearby view).
func (h *Handler) fetchDeparturesForStopView(ctx context.Context, stopID string, now time.Time) []templates.StopRouteGroup {
	allDeps := h.fetchDepartures(ctx, stopID, now, 30)

	type routeKey struct {
		routeID     string
		directionID int
	}
	groups := make(map[routeKey][]templates.DepartureInfo)
	var order []routeKey

	for _, dep := range allDeps {
		key := routeKey{dep.RouteID, dep.DirectionID}
		if _, exists := groups[key]; !exists {
			order = append(order, key)
		}
		if len(groups[key]) < 3 {
			groups[key] = append(groups[key], dep)
		}
	}

	var result []templates.StopRouteGroup
	for _, key := range order {
		deps := groups[key]
		if len(deps) == 0 {
			continue
		}
		rg := templates.StopRouteGroup{
			RouteID:        deps[0].RouteID,
			RouteShort:     deps[0].RouteShort,
			RouteColor:     deps[0].RouteColor,
			RouteTextColor: deps[0].RouteTextColor,
			DirectionText:  deps[0].DirectionText,
			Headsign:       deps[0].Headsign,
		}
		for _, dep := range deps {
			rg.Times = append(rg.Times, templates.StopRouteDeparture{
				Scheduled:   dep.Scheduled,
				Realtime:    dep.Realtime,
				MinutesAway: dep.MinutesAway,
				IsRealtime:  dep.IsRealtime,
				IsLate:      dep.IsLate,
			})
		}
		result = append(result, rg)
	}
	return result
}

// expandDirectionText converts NexTrip direction abbreviations to full words.
func expandDirectionText(abbr string) string {
	switch abbr {
	case "NB":
		return "Northbound"
	case "SB":
		return "Southbound"
	case "EB":
		return "Eastbound"
	case "WB":
		return "Westbound"
	default:
		return abbr
	}
}
