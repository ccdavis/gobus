package handler

import (
	"testing"

	"gobus/internal/storage"
)

func TestClusterSearchResults_SingleResult(t *testing.T) {
	results := []storage.StopSearchResult{
		{Name: "Lake St & Lyndale Ave", Lat: 44.9485, Lon: -93.2885},
	}

	clusters := clusterSearchResults(results, 500)
	if len(clusters) != 1 {
		t.Fatalf("got %d clusters, want 1", len(clusters))
	}
	if clusters[0].Name != "Lake St & Lyndale Ave" {
		t.Errorf("name = %q", clusters[0].Name)
	}
}

func TestClusterSearchResults_NearbyMerged(t *testing.T) {
	// Two stops at the same intersection (opposite sides of street)
	results := []storage.StopSearchResult{
		{Name: "Lake St & Lyndale Ave", Lat: 44.94850, Lon: -93.28850},
		{Name: "Lake St & Lyndale Ave", Lat: 44.94855, Lon: -93.28845},
	}

	clusters := clusterSearchResults(results, 500)
	if len(clusters) != 1 {
		t.Fatalf("got %d clusters, want 1 (stops should merge)", len(clusters))
	}
	// Centroid should be the average
	if clusters[0].n != 2 {
		t.Errorf("cluster count = %d, want 2", clusters[0].n)
	}
}

func TestClusterSearchResults_FarApart(t *testing.T) {
	// Two stops far apart (different intersections)
	results := []storage.StopSearchResult{
		{Name: "Lake St & Lyndale Ave", Lat: 44.9485, Lon: -93.2885},
		{Name: "Lake St & Nicollet Ave", Lat: 44.9485, Lon: -93.2780},
	}

	clusters := clusterSearchResults(results, 500)
	if len(clusters) != 2 {
		t.Fatalf("got %d clusters, want 2 (stops are ~800m apart)", len(clusters))
	}
}

func TestClusterSearchResults_Empty(t *testing.T) {
	clusters := clusterSearchResults(nil, 500)
	if len(clusters) != 0 {
		t.Fatalf("got %d clusters, want 0", len(clusters))
	}
}

func TestClusterSearchResults_CentroidAccuracy(t *testing.T) {
	results := []storage.StopSearchResult{
		{Name: "A", Lat: 10.0, Lon: 20.0},
		{Name: "B", Lat: 10.0, Lon: 20.0}, // exact same location
		{Name: "C", Lat: 10.0, Lon: 20.0},
	}

	clusters := clusterSearchResults(results, 500)
	if len(clusters) != 1 {
		t.Fatalf("got %d clusters, want 1", len(clusters))
	}
	// Centroid of 3 identical points should be that point
	c := clusters[0]
	if c.Lat != 10.0 || c.Lon != 20.0 {
		t.Errorf("centroid = (%.6f, %.6f), want (10.0, 20.0)", c.Lat, c.Lon)
	}
}

func TestClusterSearchResults_UsesFirstName(t *testing.T) {
	results := []storage.StopSearchResult{
		{Name: "First", Lat: 44.9485, Lon: -93.2885},
		{Name: "Second", Lat: 44.9486, Lon: -93.2885}, // close enough to merge
	}

	clusters := clusterSearchResults(results, 500)
	if len(clusters) != 1 {
		t.Fatalf("got %d clusters, want 1", len(clusters))
	}
	if clusters[0].Name != "First" {
		t.Errorf("cluster name = %q, want 'First' (should use first result's name)", clusters[0].Name)
	}
}
