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

const defaultGoogleBaseURL = "https://maps.googleapis.com/maps/api"

// GoogleProvider uses the Google Geocoding and Places APIs.
type GoogleProvider struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// NewGoogleProvider constructs a GoogleProvider.
func NewGoogleProvider(cfg Config) *GoogleProvider {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultGoogleBaseURL
	}
	return &GoogleProvider{
		apiKey:     cfg.APIKey,
		baseURL:    baseURL,
		httpClient: cfg.HTTPClient,
	}
}

type googleGeocodeResponse struct {
	Status  string `json:"status"`
	Results []struct {
		FormattedAddress string `json:"formatted_address"`
		Geometry         struct {
			Location struct {
				Lat float64 `json:"lat"`
				Lng float64 `json:"lng"`
			} `json:"location"`
		} `json:"geometry"`
	} `json:"results"`
	ErrorMessage string `json:"error_message"`
}

type googleNearbyResponse struct {
	Status       string `json:"status"`
	Results      []googlePlaceResult
	ErrorMessage string `json:"error_message"`
}

type googlePlaceResult struct {
	Name     string `json:"name"`
	PlaceID  string `json:"place_id"`
	Geometry struct {
		Location struct {
			Lat float64 `json:"lat"`
			Lng float64 `json:"lng"`
		} `json:"location"`
	} `json:"geometry"`
}

// Geocode resolves a free-text address to coordinates.
func (g *GoogleProvider) Geocode(ctx context.Context, address string) (Result, error) {
	trimmed := strings.TrimSpace(address)
	if trimmed == "" {
		return Result{}, ErrEmptyAddress
	}

	endpoint := fmt.Sprintf("%s/geocode/json?%s", g.baseURL, url.Values{
		"address": {trimmed},
		"key":     {g.apiKey},
	}.Encode())

	var payload googleGeocodeResponse
	if err := g.getJSON(ctx, endpoint, &payload); err != nil {
		return Result{}, err
	}

	switch payload.Status {
	case "OK":
		if len(payload.Results) == 0 {
			return Result{}, ErrAddressNotFound
		}
		first := payload.Results[0]
		return Result{
			Latitude:         first.Geometry.Location.Lat,
			Longitude:        first.Geometry.Location.Lng,
			FormattedAddress: first.FormattedAddress,
		}, nil
	case "ZERO_RESULTS":
		return Result{}, ErrAddressNotFound
	default:
		msg := payload.ErrorMessage
		if msg == "" {
			msg = payload.Status
		}
		return Result{}, fmt.Errorf("places: google geocode: %s", msg)
	}
}

// SearchNearby finds places matching keyword within radiusMeters of lat/lng.
func (g *GoogleProvider) SearchNearby(ctx context.Context, keyword string, lat, lng float64, radiusMeters int) ([]Place, error) {
	if strings.TrimSpace(keyword) == "" {
		return []Place{}, nil
	}
	if radiusMeters <= 0 {
		radiusMeters = 500
	}

	endpoint := fmt.Sprintf("%s/place/nearbysearch/json?%s", g.baseURL, url.Values{
		"location": {fmt.Sprintf("%f,%f", lat, lng)},
		"radius":   {fmt.Sprintf("%d", radiusMeters)},
		"keyword":  {keyword},
		"key":      {g.apiKey},
	}.Encode())

	var payload googleNearbyResponse
	if err := g.getJSON(ctx, endpoint, &payload); err != nil {
		return nil, err
	}

	switch payload.Status {
	case "OK", "ZERO_RESULTS":
		places := make([]Place, 0, len(payload.Results))
		for _, item := range payload.Results {
			places = append(places, Place{
				Name:      item.Name,
				Latitude:  item.Geometry.Location.Lat,
				Longitude: item.Geometry.Location.Lng,
				PlaceID:   item.PlaceID,
			})
		}
		return places, nil
	default:
		msg := payload.ErrorMessage
		if msg == "" {
			msg = payload.Status
		}
		return nil, fmt.Errorf("places: google nearby search: %s", msg)
	}
}

func (g *GoogleProvider) getJSON(ctx context.Context, endpoint string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("places: google request: %w", err)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("places: google request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("places: google read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("places: google http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("places: google decode: %w", err)
	}
	return nil
}
