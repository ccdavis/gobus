package handler

import (
	"context"

	"gobus/internal/realtime"
	"gobus/internal/templates"
)

// alertsForStop returns alerts from the GTFS-RT feed and NexTrip for a given stop.
func (h *Handler) alertsForStop(ctx context.Context, stopID string) []templates.AlertDisplay {
	var alerts []templates.AlertDisplay

	// 1. GTFS-RT alerts (from background fetcher)
	rtAlerts := h.rt.AlertsForStop(stopID)
	for _, a := range rtAlerts {
		alerts = append(alerts, templates.AlertDisplay{
			HeaderText: a.HeaderText,
			DescText:   a.DescText,
			Effect:     realtime.FormatAlertEffect(a.Effect),
		})
	}

	// 2. NexTrip per-stop alerts (from API response, already fetched for departures)
	ntResp, err := h.nt.DeparturesForStop(ctx, stopID)
	if err == nil && ntResp != nil {
		for _, a := range ntResp.Alerts {
			// Deduplicate: skip if we already have an alert with the same text
			if !alertExists(alerts, a.AlertText) {
				effect := ""
				if a.StopClosed {
					effect = "No Service"
				}
				alerts = append(alerts, templates.AlertDisplay{
					HeaderText: a.AlertText,
					Effect:     effect,
				})
			}
		}
	}

	return alerts
}

// alertsForRoute returns alerts from the GTFS-RT feed for a given route.
func (h *Handler) alertsForRoute(routeID string) []templates.AlertDisplay {
	var alerts []templates.AlertDisplay
	rtAlerts := h.rt.AlertsForRoute(routeID)
	for _, a := range rtAlerts {
		alerts = append(alerts, templates.AlertDisplay{
			HeaderText: a.HeaderText,
			DescText:   a.DescText,
			Effect:     realtime.FormatAlertEffect(a.Effect),
		})
	}
	return alerts
}

func alertExists(alerts []templates.AlertDisplay, text string) bool {
	for _, a := range alerts {
		if a.HeaderText == text {
			return true
		}
	}
	return false
}
