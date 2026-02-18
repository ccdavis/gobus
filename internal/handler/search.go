package handler

import (
	"fmt"
	"net/http"
	"net/url"

	"gobus/internal/geo"
	"gobus/internal/storage"
	"gobus/internal/templates"
)

// Search serves the location search page.
// On GET with no query: shows the search form.
// On GET with q= parameter: geocodes and redirects to /nearby on success,
// or shows errors/disambiguation on the search page.
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	view := r.URL.Query().Get("view")
	if view != "stops" {
		view = "routes"
	}

	data := templates.SearchData{
		Page: h.page("Search Location", "/search"),
		Query: query,
		View:  view,
	}

	// If no query, just show the form
	if query == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := templates.SearchPage(data).Render(r.Context(), w); err != nil {
			h.logger.Error("rendering search page", "error", err)
		}
		return
	}

	// Try GTFS cross-street search first (works offline)
	results, err := h.db.SearchStops(r.Context(), query)
	if err != nil {
		h.logger.Error("search stops", "query", query, "error", err)
	}

	if len(results) > 0 {
		clusters := clusterSearchResults(results, 500)
		if len(clusters) == 1 {
			// Single match — redirect to nearby
			redirectURL := fmt.Sprintf("/nearby?view=%s&lat=%.6f&lon=%.6f&q=%s",
				view, clusters[0].Lat, clusters[0].Lon, url.QueryEscape(query))
			http.Redirect(w, r, redirectURL, http.StatusFound)
			return
		}
		// Multiple matches — show disambiguation
		data.SearchResults = make([]templates.SearchResult, len(clusters))
		for i, c := range clusters {
			data.SearchResults[i] = templates.SearchResult{
				Name: c.Name,
				Lat:  fmt.Sprintf("%.6f", c.Lat),
				Lon:  fmt.Sprintf("%.6f", c.Lon),
			}
		}
	} else {
		// No GTFS match — fall back to Nominatim address geocoding
		geoResult, err := h.geo.Search(r.Context(), query+", Minneapolis, MN")
		if err != nil {
			h.logger.Warn("nominatim geocoding failed", "query", query, "error", err)
			data.SearchError = "Address lookup is unavailable right now. Try entering cross streets instead (e.g. \"Lake & Lyndale\") — cross-street search works offline."
		} else if geoResult == nil {
			data.SearchError = "No results found for \"" + query + "\". Try nearby cross streets instead (e.g. \"Lake & Lyndale\") — cross-street search works even without internet."
		} else {
			// Nominatim success — redirect to nearby
			redirectURL := fmt.Sprintf("/nearby?view=%s&lat=%.6f&lon=%.6f&q=%s",
				view, geoResult.Lat, geoResult.Lon, url.QueryEscape(query))
			http.Redirect(w, r, redirectURL, http.StatusFound)
			return
		}
	}

	// Show form with errors or disambiguation
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.SearchPage(data).Render(r.Context(), w); err != nil {
		h.logger.Error("rendering search page", "error", err)
	}
}

// clusterSearchResults groups stop search results by proximity.
// Results within radiusMeters of each other are merged into one cluster,
// using the first result's name and the centroid of all members.
type searchCluster struct {
	Name string
	Lat  float64
	Lon  float64
	n    int
}

func clusterSearchResults(results []storage.StopSearchResult, radiusMeters float64) []searchCluster {
	var clusters []searchCluster
	for _, r := range results {
		merged := false
		for i := range clusters {
			dist := geo.Haversine(clusters[i].Lat, clusters[i].Lon, r.Lat, r.Lon)
			if dist <= radiusMeters {
				n := float64(clusters[i].n)
				clusters[i].Lat = (clusters[i].Lat*n + r.Lat) / (n + 1)
				clusters[i].Lon = (clusters[i].Lon*n + r.Lon) / (n + 1)
				clusters[i].n++
				merged = true
				break
			}
		}
		if !merged {
			clusters = append(clusters, searchCluster{
				Name: r.Name,
				Lat:  r.Lat,
				Lon:  r.Lon,
				n:    1,
			})
		}
	}
	return clusters
}
