package handler

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"gobus/internal/templates"
)

// SSEDepartures streams live departure updates for a stop via Server-Sent Events.
// The HTMX SSE extension on the client listens for "departures" events and swaps the HTML.
func (h *Handler) SSEDepartures(w http.ResponseWriter, r *http.Request) {
	stopID := r.PathValue("id")
	ctx := r.Context()

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// Send initial data immediately
	h.sendDepartureEvent(ctx, w, flusher, stopID)

	// Tick every 60 seconds per user spec
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.sendDepartureEvent(ctx, w, flusher, stopID)
		case <-ctx.Done():
			return
		}
	}
}

// sendDepartureEvent renders the departure list as HTML and sends it as an SSE event.
func (h *Handler) sendDepartureEvent(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, stopID string) {
	now := time.Now()
	departures := h.fetchDepartures(ctx, stopID, now, 15)

	var buf bytes.Buffer
	if err := templates.DepartureList(departures).Render(ctx, &buf); err != nil {
		h.logger.Error("rendering SSE departure list", "error", err)
		return
	}

	// SSE format: event name, then data lines (each line prefixed with "data: ")
	fmt.Fprintf(w, "event: departures\n")
	for _, line := range bytes.Split(buf.Bytes(), []byte("\n")) {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	fmt.Fprintf(w, "\n")
	flusher.Flush()
}
