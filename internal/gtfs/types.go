package gtfs

// Feed holds all parsed data from a GTFS zip file.
type Feed struct {
	Agencies      []Agency
	Routes        []Route
	Stops         []Stop
	Trips         []Trip
	StopTimes     []StopTime
	Calendar      []CalendarEntry
	CalendarDates []CalendarDate
	Shapes        []ShapePoint
	LastModified  string // From HTTP response header
	ETag          string // From HTTP response header
}

type Agency struct {
	AgencyID       string `csv:"agency_id"`
	AgencyName     string `csv:"agency_name"`
	AgencyURL      string `csv:"agency_url"`
	AgencyTimezone string `csv:"agency_timezone"`
}

type Route struct {
	RouteID        string `csv:"route_id"`
	AgencyID       string `csv:"agency_id"`
	RouteShortName string `csv:"route_short_name"`
	RouteLongName  string `csv:"route_long_name"`
	RouteType      string `csv:"route_type"`
	RouteColor     string `csv:"route_color"`
	RouteTextColor string `csv:"route_text_color"`
	RouteSortOrder string `csv:"route_sort_order"`
}

type Stop struct {
	StopID             string `csv:"stop_id"`
	StopCode           string `csv:"stop_code"`
	StopName           string `csv:"stop_name"`
	StopDesc           string `csv:"stop_desc"`
	StopLat            string `csv:"stop_lat"`
	StopLon            string `csv:"stop_lon"`
	ZoneID             string `csv:"zone_id"`
	StopURL            string `csv:"stop_url"`
	LocationType       string `csv:"location_type"`
	ParentStation      string `csv:"parent_station"`
	WheelchairBoarding string `csv:"wheelchair_boarding"`
}

type Trip struct {
	TripID       string `csv:"trip_id"`
	RouteID      string `csv:"route_id"`
	ServiceID    string `csv:"service_id"`
	TripHeadsign string `csv:"trip_headsign"`
	DirectionID  string `csv:"direction_id"`
	BlockID      string `csv:"block_id"`
	ShapeID      string `csv:"shape_id"`
}

type StopTime struct {
	TripID        string `csv:"trip_id"`
	ArrivalTime   string `csv:"arrival_time"`
	DepartureTime string `csv:"departure_time"`
	StopID        string `csv:"stop_id"`
	StopSequence  string `csv:"stop_sequence"`
	PickupType    string `csv:"pickup_type"`
	DropOffType   string `csv:"drop_off_type"`
	Timepoint     string `csv:"timepoint"`
}

type CalendarEntry struct {
	ServiceID string `csv:"service_id"`
	Monday    string `csv:"monday"`
	Tuesday   string `csv:"tuesday"`
	Wednesday string `csv:"wednesday"`
	Thursday  string `csv:"thursday"`
	Friday    string `csv:"friday"`
	Saturday  string `csv:"saturday"`
	Sunday    string `csv:"sunday"`
	StartDate string `csv:"start_date"`
	EndDate   string `csv:"end_date"`
}

type CalendarDate struct {
	ServiceID     string `csv:"service_id"`
	Date          string `csv:"date"`
	ExceptionType string `csv:"exception_type"`
}

type ShapePoint struct {
	ShapeID           string `csv:"shape_id"`
	ShapePtLat        string `csv:"shape_pt_lat"`
	ShapePtLon        string `csv:"shape_pt_lon"`
	ShapePtSequence   string `csv:"shape_pt_sequence"`
	ShapeDistTraveled string `csv:"shape_dist_traveled"`
}
