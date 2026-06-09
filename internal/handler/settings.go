package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"gobus/internal/storage"
)

// settingDistanceUnit is the settings key for the distance unit preference.
const settingDistanceUnit = "distance_unit"

// defaultDistanceUnit is used when the user has not chosen a unit.
const defaultDistanceUnit = "metric"

// distanceUnit returns the user's distance-unit preference, defaulting to metric.
func (h *Handler) distanceUnit(r *http.Request) string {
	v, ok, err := h.db.GetSetting(r.Context(), settingDistanceUnit)
	if err != nil {
		h.logger.Error("reading distance unit", "error", err)
	}
	if !ok || (v != "metric" && v != "imperial") {
		return defaultDistanceUnit
	}
	return v
}

// SavedList returns all saved locations as JSON.
func (h *Handler) SavedList(w http.ResponseWriter, r *http.Request) {
	locs, err := h.db.ListSavedLocations(r.Context())
	if err != nil {
		h.logger.Error("listing saved locations", "error", err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	if locs == nil {
		locs = []storage.SavedLocation{}
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(locs); err != nil {
		h.logger.Error("encoding saved locations", "error", err)
	}
}

// SavedAdd adds (or updates) a saved location from a form-encoded POST.
func (h *Handler) SavedAdd(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	loc := storage.SavedLocation{
		StopID: r.FormValue("stopID"),
		Name:   r.FormValue("name"),
		Label:  r.FormValue("label"),
	}
	loc.Lat, _ = strconv.ParseFloat(r.FormValue("lat"), 64)
	loc.Lon, _ = strconv.ParseFloat(r.FormValue("lon"), 64)
	if loc.StopID == "" || loc.Name == "" {
		http.Error(w, "stopID and name required", http.StatusBadRequest)
		return
	}
	if loc.Label == "" {
		loc.Label = loc.Name
	}
	if err := h.db.AddSavedLocation(r.Context(), loc); err != nil {
		h.logger.Error("adding saved location", "error", err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SavedRemove deletes a saved location by stop ID.
func (h *Handler) SavedRemove(w http.ResponseWriter, r *http.Request) {
	stopID := r.PathValue("stopID")
	if stopID == "" {
		http.Error(w, "stopID required", http.StatusBadRequest)
		return
	}
	if err := h.db.RemoveSavedLocation(r.Context(), stopID); err != nil {
		h.logger.Error("removing saved location", "error", err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UnitSet persists the distance-unit preference (metric or imperial).
func (h *Handler) UnitSet(w http.ResponseWriter, r *http.Request) {
	unit := r.FormValue("unit")
	if unit != "metric" && unit != "imperial" {
		http.Error(w, "unit must be metric or imperial", http.StatusBadRequest)
		return
	}
	if err := h.db.SetSetting(r.Context(), settingDistanceUnit, unit); err != nil {
		h.logger.Error("setting distance unit", "error", err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
