package geocode

import (
	"context"
	"encoding/json"
	"fmt"
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

// Reverse performs reverse geocoding: lat/lon → nearest address.
// Returns a short address string (house number + road), or the full
// display name if those fields are missing.
func (c *Client) Reverse(ctx context.Context, lat, lon float64) (string, error) {
	u := "https://nominatim.openstreetmap.org/reverse?" + url.Values{
		"lat":            {strconv.FormatFloat(lat, 'f', 6, 64)},
		"lon":            {strconv.FormatFloat(lon, 'f', 6, 64)},
		"format":         {"jsonv2"},
		"zoom":           {"18"}, // street-level
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
			HouseNumber string `json:"house_number"`
			Road        string `json:"road"`
		} `json:"address"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("nominatim reverse decode: %w", err)
	}

	// Build a short address: "123 Main St"
	if result.Address.Road != "" {
		if result.Address.HouseNumber != "" {
			return result.Address.HouseNumber + " " + result.Address.Road, nil
		}
		return result.Address.Road, nil
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
