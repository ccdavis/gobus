package geocode

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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

// Reverse performs reverse geocoding: lat/lon → a nearby street name.
//
// Privacy: the label only needs to orient the user ("you're near Lake St"),
// so coordinates are rounded to 3 decimal places (~110 m) before leaving the
// device, and the result is a road-level label — never a house number, which
// would imply precision we deliberately didn't send.
func (c *Client) Reverse(ctx context.Context, lat, lon float64) (string, error) {
	u := "https://nominatim.openstreetmap.org/reverse?" + url.Values{
		"lat":            {strconv.FormatFloat(coarse(lat), 'f', 3, 64)},
		"lon":            {strconv.FormatFloat(coarse(lon), 'f', 3, 64)},
		"format":         {"jsonv2"},
		"zoom":           {"17"}, // street level, no building resolution
		"addressdetails": {"1"},
	}.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("nominatim reverse: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("nominatim reverse status %d", resp.StatusCode)
	}

	var result struct {
		DisplayName string `json:"display_name"`
		Address     struct {
			Road          string `json:"road"`
			Neighbourhood string `json:"neighbourhood"`
			Suburb        string `json:"suburb"`
		} `json:"address"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("nominatim reverse decode: %w", err)
	}

	// Road-level label: "Near Lake St"
	if result.Address.Road != "" {
		return "Near " + result.Address.Road, nil
	}
	if result.Address.Neighbourhood != "" {
		return result.Address.Neighbourhood, nil
	}
	if result.Address.Suburb != "" {
		return result.Address.Suburb, nil
	}
	// Fallback to first part of display name (before first comma)
	if result.DisplayName != "" {
		if i := strings.Index(result.DisplayName, ","); i > 0 {
			return result.DisplayName[:i], nil
		}
		return result.DisplayName, nil
	}
	return "", fmt.Errorf("no address found")
}

// coarse rounds a coordinate to 3 decimal places (~110 m) so the geocoding
// service never receives the user's exact position.
func coarse(v float64) float64 {
	return math.Round(v*1000) / 1000
}
