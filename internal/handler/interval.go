package handler

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// upcomingRouteInstants returns the absolute instants of all remaining
// departures for a route/stop/direction, merged across the service days that
// can still produce departures at `now` (see scheduledDeparturesForStop).
func (h *Handler) upcomingRouteInstants(ctx context.Context, stopID, routeID string, directionID int, now time.Time) []time.Time {
	var future []time.Time

	times, err := h.db.AllDeparturesForStopRoute(ctx, stopID, routeID, directionID, now)
	if err == nil {
		cutoff := now.Format("15:04:05")
		for _, t := range times {
			if t < cutoff {
				continue
			}
			future = append(future, gtfsInstant(t, now))
		}
	}

	if now.Hour() < prevServiceDayWindowEndHour {
		prevDay := now.AddDate(0, 0, -1)
		cutoff := fmt.Sprintf("%02d:%02d:%02d", now.Hour()+24, now.Minute(), now.Second())
		times, err := h.db.AllDeparturesForStopRoute(ctx, stopID, routeID, directionID, prevDay)
		if err == nil {
			for _, t := range times {
				if t < cutoff {
					continue
				}
				future = append(future, gtfsInstant(t, prevDay))
			}
		}
	}

	sort.Slice(future, func(i, j int) bool { return future[i].Before(future[j]) })
	return future
}

// detectInterval examines all remaining departures today for a route/stop/direction
// and returns a human-readable interval string like "Every 20 min until 8:00 PM".
// Returns empty string if no regular interval is detected.
func (h *Handler) detectInterval(ctx context.Context, stopID, routeID string, directionID int, now time.Time) string {
	futureTimes := h.upcomingRouteInstants(ctx, stopID, routeID, directionID, now)
	if len(futureTimes) < 3 {
		return ""
	}

	// Calculate intervals between consecutive departures
	var intervals []int
	for i := 1; i < len(futureTimes); i++ {
		diff := int(futureTimes[i].Sub(futureTimes[i-1]).Minutes())
		if diff > 0 {
			intervals = append(intervals, diff)
		}
	}

	if len(intervals) < 2 {
		return ""
	}

	// Find the longest run of consistent intervals (within ±2 min tolerance)
	bestStart := 0
	bestLen := 1
	curStart := 0
	curLen := 1

	for i := 1; i < len(intervals); i++ {
		if abs(intervals[i]-intervals[curStart]) <= 2 {
			curLen++
		} else {
			if curLen > bestLen {
				bestStart = curStart
				bestLen = curLen
			}
			curStart = i
			curLen = 1
		}
	}
	if curLen > bestLen {
		bestStart = curStart
		bestLen = curLen
	}

	// Need at least 3 consistent intervals to call it a pattern
	if bestLen < 3 {
		return ""
	}

	// Calculate the average interval
	sum := 0
	for i := bestStart; i < bestStart+bestLen; i++ {
		sum += intervals[i]
	}
	avgInterval := sum / bestLen

	// Round to nearest 5 minutes for cleaner display
	rounded := ((avgInterval + 2) / 5) * 5
	if rounded == 0 {
		rounded = avgInterval
	}

	// Find when the pattern ends
	// The last departure in the pattern is at index bestStart+bestLen in futureTimes
	endIdx := bestStart + bestLen
	if endIdx >= len(futureTimes) {
		endIdx = len(futureTimes) - 1
	}
	endTime := futureTimes[endIdx]

	return fmt.Sprintf("Every %d min until %s", rounded, endTime.Format("3:04 PM"))
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
