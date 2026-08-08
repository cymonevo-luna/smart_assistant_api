package places

import (
	"context"
	"math"
	"strings"
)

// MockProvider returns deterministic coordinates and places for tests and CI.
type MockProvider struct{}

// NewMockProvider constructs a MockProvider.
func NewMockProvider() *MockProvider {
	return &MockProvider{}
}

// Geocode resolves known test addresses to fixed coordinates.
func (m *MockProvider) Geocode(_ context.Context, address string) (Result, error) {
	trimmed := strings.TrimSpace(address)
	if trimmed == "" {
		return Result{}, ErrEmptyAddress
	}

	lower := strings.ToLower(trimmed)
	switch {
	case strings.Contains(lower, "cempaka putih tengah 20"):
		return Result{
			Latitude:         -6.171,
			Longitude:        106.861,
			FormattedAddress: "Jl. Cempaka Putih Tengah 20, Jakarta",
		}, nil
	case strings.Contains(lower, "unknown") || strings.Contains(lower, "ambiguous"):
		return Result{}, ErrAddressNotFound
	default:
		return Result{}, ErrAddressNotFound
	}
}

// SearchNearby returns mock places for known keywords near Jakarta.
func (m *MockProvider) SearchNearby(_ context.Context, keyword string, lat, lng float64, radiusMeters int) ([]Place, error) {
	if strings.TrimSpace(keyword) == "" {
		return nil, nil
	}

	lower := strings.ToLower(keyword)
	if !strings.Contains(lower, "alfamart") {
		return []Place{}, nil
	}

	place := Place{
		Name:      "Alfamart",
		Latitude:  -6.171,
		Longitude: 106.861,
		PlaceID:   "mock-alfamart-1",
	}
	if radiusMeters > 0 && haversineMeters(lat, lng, place.Latitude, place.Longitude) > float64(radiusMeters) {
		return []Place{}, nil
	}
	return []Place{place}, nil
}

func haversineMeters(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadius = 6371000.0
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLng := (lng2 - lng1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthRadius * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
