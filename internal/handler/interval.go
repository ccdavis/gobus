package handler

import (
	"context"
	"fmt"
	"time"
)

// detectInterval examines all remaining departures today for a route/stop/direction
// and returns a human-readable interval string like "Every 20 min until 8:00 PM".
// Returns empty string if no regular interval is detected.
func (h *Handler) detectInterval(ctx context.Context, stopID, routeID string, directionID int, now time.Time) string {
	times, err := h.db.AllDeparturesForStopRoute(ctx, stopID, routeID, directionID, now)
	if err != nil || len(times) < 3 {
		return ""
	}

	// Parse times and filter to future only
	currentTime := now.Format("15:04:05")
	var futureTimes []time.Time
	for _, t := range times {
		if t < currentTime {
			continue
		}
		parsed := parseGTFSTime(t, now)
		futureTimes = append(futureTimes, parsed)
	}

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

// parseGTFSTime converts "HH:MM:SS" (possibly >24) to a time.Time on the given day.
func parseGTFSTime(gtfsTime string, now time.Time) time.Time {
	var h, m, s int
	fmt.Sscanf(gtfsTime, "%d:%d:%d", &h, &m, &s)
	t := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return t.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second)
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
