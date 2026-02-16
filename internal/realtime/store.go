package realtime

import (
	"sync"
)

// Alert represents a parsed service alert.
type Alert struct {
	ID         string
	HeaderText string
	DescText   string
	RouteIDs   []string
	StopIDs    []string
	Effect     string // "NO_SERVICE", "REDUCED_SERVICE", "DETOUR", etc.
	Cause      string
}

// Store holds realtime data in a thread-safe manner.
type Store struct {
	mu     sync.RWMutex
	alerts []Alert
}

// NewStore creates an empty realtime store.
func NewStore() *Store {
	return &Store{}
}

// SetAlerts replaces all alerts.
func (s *Store) SetAlerts(alerts []Alert) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.alerts = alerts
}

// AlertsForRoute returns alerts affecting a specific route.
func (s *Store) AlertsForRoute(routeID string) []Alert {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []Alert
	for _, a := range s.alerts {
		for _, r := range a.RouteIDs {
			if r == routeID {
				result = append(result, a)
				break
			}
		}
	}
	return result
}

// AlertsForStop returns alerts affecting a specific stop.
func (s *Store) AlertsForStop(stopID string) []Alert {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []Alert
	for _, a := range s.alerts {
		for _, sid := range a.StopIDs {
			if sid == stopID {
				result = append(result, a)
				break
			}
		}
	}
	return result
}

// AllAlerts returns all active alerts.
func (s *Store) AllAlerts() []Alert {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Alert, len(s.alerts))
	copy(out, s.alerts)
	return out
}
