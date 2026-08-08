// Package places provides address geocoding and nearby place search behind a
// pluggable provider interface. Keyword nearby accuracy depends on the
// configured external provider.
package places

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Provider resolves addresses to coordinates and searches for nearby places.
type Provider interface {
	Geocode(ctx context.Context, address string) (Result, error)
	SearchNearby(ctx context.Context, keyword string, lat, lng float64, radiusMeters int) ([]Place, error)
}

// Result is a resolved geographic location.
type Result struct {
	Latitude         float64
	Longitude        float64
	FormattedAddress string
}

// Place is a nearby point of interest.
type Place struct {
	Name      string
	Latitude  float64
	Longitude float64
	PlaceID   string
}

// Config configures a places provider.
type Config struct {
	Provider   string
	APIKey     string
	BaseURL    string
	UserAgent  string
	HTTPClient *http.Client
}

// NewProvider builds a Provider from configuration.
func NewProvider(cfg Config) (Provider, error) {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "smart_assistant_api"
	}

	switch cfg.Provider {
	case "", "mock":
		return NewMockProvider(), nil
	case "google":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("places: PLACES_API_KEY is required when PLACES_PROVIDER=google")
		}
		return NewGoogleProvider(cfg), nil
	case "nominatim":
		return NewNominatimProvider(cfg), nil
	default:
		return nil, fmt.Errorf("places: unsupported PLACES_PROVIDER %q", cfg.Provider)
	}
}
