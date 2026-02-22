package nextrip

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Client is an HTTP client for the Metro Transit NexTrip API.
type Client struct {
	baseURL string
	client  *http.Client
	cache   *Cache
	logger  *slog.Logger
}

// NewClient creates a NexTrip API client.
func NewClient(baseURL string, logger *slog.Logger) *Client {
	return &Client{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		cache:  NewCache(60 * time.Second),
		logger: logger,
	}
}

// DeparturesForStop fetches realtime departure predictions for a stop.
func (c *Client) DeparturesForStop(ctx context.Context, stopID string) (*Response, error) {
	cacheKey := "stop:" + stopID
	if cached, ok := c.cache.Get(cacheKey); ok {
		return cached.(*Response), nil
	}

	url := fmt.Sprintf("%s/%s", c.baseURL, stopID)
	resp, err := c.doGet(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("departures for stop %s: %w", stopID, err)
	}
	defer resp.Body.Close()

	var result Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	c.cache.Set(cacheKey, &result)
	return &result, nil
}

// DeparturesForRouteStop fetches departures for a specific route/direction/stop.
func (c *Client) DeparturesForRouteStop(ctx context.Context, routeID string, directionID int, placeCode string) (*Response, error) {
	cacheKey := fmt.Sprintf("route:%s:%d:%s", routeID, directionID, placeCode)
	if cached, ok := c.cache.Get(cacheKey); ok {
		return cached.(*Response), nil
	}

	url := fmt.Sprintf("%s/%s/%d/%s", c.baseURL, routeID, directionID, placeCode)
	resp, err := c.doGet(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("departures for route stop: %w", err)
	}
	defer resp.Body.Close()

	var result Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	c.cache.Set(cacheKey, &result)
	return &result, nil
}

// Routes fetches all available routes.
func (c *Client) Routes(ctx context.Context) ([]RouteResponse, error) {
	if cached, ok := c.cache.Get("routes"); ok {
		return cached.([]RouteResponse), nil
	}

	url := fmt.Sprintf("%s/routes", c.baseURL)
	resp, err := c.doGet(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("fetch routes: %w", err)
	}
	defer resp.Body.Close()

	var result []RouteResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode routes: %w", err)
	}

	c.cache.Set("routes", result)
	return result, nil
}

// Directions fetches directions for a route.
func (c *Client) Directions(ctx context.Context, routeID string) ([]DirectionResponse, error) {
	cacheKey := "dirs:" + routeID
	if cached, ok := c.cache.Get(cacheKey); ok {
		return cached.([]DirectionResponse), nil
	}

	url := fmt.Sprintf("%s/directions/%s", c.baseURL, routeID)
	resp, err := c.doGet(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("fetch directions: %w", err)
	}
	defer resp.Body.Close()

	var result []DirectionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode directions: %w", err)
	}

	c.cache.Set(cacheKey, result)
	return result, nil
}

func (c *Client) doGet(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	return resp, nil
}
