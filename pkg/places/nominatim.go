package places

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const defaultNominatimBaseURL = "https://nominatim.openstreetmap.org"

// NominatimProvider uses OpenStreetMap Nominatim for geocoding and search.
// Nominatim requires a valid User-Agent per its usage policy.
type NominatimProvider struct {
	baseURL    string
	userAgent  string
	httpClient *http.Client
}

// NewNominatimProvider constructs a NominatimProvider.
func NewNominatimProvider(cfg Config) *NominatimProvider {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultNominatimBaseURL
	}
	return &NominatimProvider{
		baseURL:    baseURL,
		userAgent:  cfg.UserAgent,
		httpClient: cfg.HTTPClient,
	}
}

type nominatimSearchResult struct {
	PlaceID     int64   `json:"place_id"`
	DisplayName string  `json:"display_name"`
	Lat         string  `json:"lat"`
	Lon         string  `json:"lon"`
	Importance  float64 `json:"importance"`
}

// Geocode resolves a free-text address to coordinates.
func (n *NominatimProvider) Geocode(ctx context.Context, address string) (Result, error) {
	trimmed := strings.TrimSpace(address)
	if trimmed == "" {
		return Result{}, ErrEmptyAddress
	}

	endpoint := fmt.Sprintf("%s/search?%s", n.baseURL, url.Values{
		"q":              {trimmed},
		"format":         {"json"},
		"limit":          {"1"},
		"addressdetails": {"0"},
	}.Encode())

	var results []nominatimSearchResult
	if err := n.getJSON(ctx, endpoint, &results); err != nil {
		return Result{}, err
	}
	if len(results) == 0 {
		return Result{}, ErrAddressNotFound
	}

	lat, lng, err := parseLatLon(results[0].Lat, results[0].Lon)
	if err != nil {
		return Result{}, fmt.Errorf("places: nominatim geocode: %w", err)
	}

	return Result{
		Latitude:         lat,
		Longitude:        lng,
		FormattedAddress: results[0].DisplayName,
	}, nil
}

// SearchNearby finds places matching keyword near lat/lng within radiusMeters.
func (n *NominatimProvider) SearchNearby(ctx context.Context, keyword string, lat, lng float64, radiusMeters int) ([]Place, error) {
	if strings.TrimSpace(keyword) == "" {
		return []Place{}, nil
	}
	if radiusMeters <= 0 {
		radiusMeters = 500
	}

	endpoint := fmt.Sprintf("%s/search?%s", n.baseURL, url.Values{
		"q":      {keyword},
		"format": {"json"},
		"limit":  {"20"},
		"lat":    {fmt.Sprintf("%f", lat)},
		"lon":    {fmt.Sprintf("%f", lng)},
	}.Encode())

	var results []nominatimSearchResult
	if err := n.getJSON(ctx, endpoint, &results); err != nil {
		return nil, err
	}

	places := make([]Place, 0, len(results))
	for _, item := range results {
		itemLat, itemLng, err := parseLatLon(item.Lat, item.Lon)
		if err != nil {
			continue
		}
		if haversineMeters(lat, lng, itemLat, itemLng) > float64(radiusMeters) {
			continue
		}
		places = append(places, Place{
			Name:      firstToken(item.DisplayName),
			Latitude:  itemLat,
			Longitude: itemLng,
			PlaceID:   fmt.Sprintf("%d", item.PlaceID),
		})
	}
	return places, nil
}

func (n *NominatimProvider) getJSON(ctx context.Context, endpoint string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("places: nominatim request: %w", err)
	}
	req.Header.Set("User-Agent", n.userAgent)

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("places: nominatim request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("places: nominatim read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("places: nominatim http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("places: nominatim decode: %w", err)
	}
	return nil
}

func parseLatLon(lat, lon string) (float64, float64, error) {
	var latVal, lonVal float64
	if _, err := fmt.Sscanf(lat, "%f", &latVal); err != nil {
		return 0, 0, fmt.Errorf("parse latitude %q: %w", lat, err)
	}
	if _, err := fmt.Sscanf(lon, "%f", &lonVal); err != nil {
		return 0, 0, fmt.Errorf("parse longitude %q: %w", lon, err)
	}
	return latVal, lonVal, nil
}

func firstToken(displayName string) string {
	if displayName == "" {
		return ""
	}
	if idx := strings.Index(displayName, ","); idx >= 0 {
		return strings.TrimSpace(displayName[:idx])
	}
	return displayName
}
