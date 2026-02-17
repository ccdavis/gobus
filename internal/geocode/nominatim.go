package geocode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Result holds a geocoding result.
type Result struct {
	Lat         float64
	Lon         float64
	DisplayName string
}

// Client is a Nominatim geocoding client.
type Client struct {
	httpClient *http.Client
	userAgent  string
}

// New creates a Nominatim geocoding client.
// userAgent is required by Nominatim's usage policy.
func New(userAgent string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		userAgent:  userAgent,
	}
}

// Search geocodes a free-form query, biased toward the Twin Cities area.
// Returns the top result, or nil if nothing found.
func (c *Client) Search(ctx context.Context, query string) (*Result, error) {
	u := "https://nominatim.openstreetmap.org/search?" + url.Values{
		"q":              {query},
		"format":         {"jsonv2"},
		"limit":          {"1"},
		"countrycodes":   {"us"},
		"viewbox":        {"-93.55,44.85,-92.95,45.10"}, // Twin Cities metro
		"bounded":        {"1"},
		"addressdetails": {"0"},
	}.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("nominatim request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nominatim status %d", resp.StatusCode)
	}

	var results []struct {
		Lat         string `json:"lat"`
		Lon         string `json:"lon"`
		DisplayName string `json:"display_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("nominatim decode: %w", err)
	}
	if len(results) == 0 {
		return nil, nil
	}

	lat, err := strconv.ParseFloat(results[0].Lat, 64)
	if err != nil {
		return nil, fmt.Errorf("parse lat: %w", err)
	}
	lon, err := strconv.ParseFloat(results[0].Lon, 64)
	if err != nil {
		return nil, fmt.Errorf("parse lon: %w", err)
	}

	return &Result{
		Lat:         lat,
		Lon:         lon,
		DisplayName: results[0].DisplayName,
	}, nil
}
