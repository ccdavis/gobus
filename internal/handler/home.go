package handler

import (
	"net/http"
)

// Home redirects to the nearby departures page.
func (h *Handler) Home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		http.Redirect(w, r, "/nearby", http.StatusFound)
		return
	}
	http.NotFound(w, r)
}
