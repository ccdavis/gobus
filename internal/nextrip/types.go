package nextrip

// Response is the top-level NexTrip API response for a stop.
type Response struct {
	Stops      []Stop      `json:"stops"`
	Alerts     []Alert     `json:"alerts"`
	Departures []Departure `json:"departures"`
}

// Stop is a stop in the NexTrip response.
type Stop struct {
	StopID      int     `json:"stop_id"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Description string  `json:"description"`
}

// Alert is a service alert for a stop.
type Alert struct {
	StopClosed bool   `json:"stop_closed"`
	AlertText  string `json:"alert_text"`
}

// Departure is a single departure prediction from NexTrip.
type Departure struct {
	Actual               bool   `json:"actual"`    // true = realtime, false = scheduled
	TripID               string `json:"trip_id"`
	StopID               int    `json:"stop_id"`
	DepartureText        string `json:"departure_text"`  // "3 Min", "11:26", etc.
	DepartureTime        int64  `json:"departure_time"`  // Unix timestamp
	Description          string `json:"description"`     // Headsign / destination
	RouteID              string `json:"route_id"`
	RouteShortName       string `json:"route_short_name"`
	DirectionID          int    `json:"direction_id"`
	DirectionText        string `json:"direction_text"` // "NB", "SB", "EB", "WB"
	Terminal             string `json:"terminal,omitempty"`
	AgencyID             int    `json:"agency_id"`
	ScheduleRelationship string `json:"schedule_relationship"`
}

// RouteResponse is the response from the routes endpoint.
type RouteResponse struct {
	RouteID    string `json:"route_id"`
	AgencyID   int    `json:"agency_id"`
	RouteLabel string `json:"route_label"`
}

// DirectionResponse is the response from the directions endpoint.
type DirectionResponse struct {
	DirectionID   int    `json:"direction_id"`
	DirectionName string `json:"direction_name"`
}

// PlaceResponse is the response from the stops endpoint.
type PlaceResponse struct {
	PlaceCode   string `json:"place_code"`
	Description string `json:"description"`
}
