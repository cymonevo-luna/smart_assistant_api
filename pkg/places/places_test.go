package places

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMockProviderGeocodeKnownAddress(t *testing.T) {
	provider := NewMockProvider()
	result, err := provider.Geocode(context.Background(), "Cempaka Putih Tengah 20")
	if err != nil {
		t.Fatalf("Geocode: %v", err)
	}
	if result.Latitude == 0 || result.Longitude == 0 {
		t.Fatalf("expected non-zero coordinates, got lat=%f lng=%f", result.Latitude, result.Longitude)
	}
}

func TestMockProviderGeocodeEmptyAddress(t *testing.T) {
	provider := NewMockProvider()
	_, err := provider.Geocode(context.Background(), "   ")
	if !Unprocessable(err) {
		t.Fatalf("expected unprocessable error, got %v", err)
	}
}

func TestMockProviderSearchNearby(t *testing.T) {
	provider := NewMockProvider()
	places, err := provider.SearchNearby(context.Background(), "Alfamart", -6.17, 106.86, 500)
	if err != nil {
		t.Fatalf("SearchNearby: %v", err)
	}
	if len(places) == 0 {
		t.Fatal("expected at least one place")
	}
}

func TestGoogleProviderSearchNearbyWithMockServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status":"OK",
			"results":[
				{
					"name":"Alfamart",
					"place_id":"abc",
					"geometry":{"location":{"lat":-6.171,"lng":106.861}}
				}
			]
		}`))
	}))
	defer srv.Close()

	provider := NewGoogleProvider(Config{
		APIKey:     "test-key",
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
	})

	places, err := provider.SearchNearby(context.Background(), "Alfamart", -6.17, 106.86, 500)
	if err != nil {
		t.Fatalf("SearchNearby: %v", err)
	}
	if len(places) == 0 {
		t.Fatal("expected at least one place")
	}
	if places[0].Name != "Alfamart" || places[0].PlaceID != "abc" {
		t.Fatalf("unexpected place: %+v", places[0])
	}
}

func TestGoogleProviderGeocodeWithMockServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/geocode/json" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status":"OK",
			"results":[
				{
					"formatted_address":"Jl. Cempaka Putih Tengah 20, Jakarta",
					"geometry":{"location":{"lat":-6.171,"lng":106.861}}
				}
			]
		}`))
	}))
	defer srv.Close()

	provider := NewGoogleProvider(Config{
		APIKey:     "test-key",
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
	})

	result, err := provider.Geocode(context.Background(), "Cempaka Putih Tengah 20")
	if err != nil {
		t.Fatalf("Geocode: %v", err)
	}
	if result.Latitude == 0 || result.Longitude == 0 {
		t.Fatalf("expected non-zero coordinates, got lat=%f lng=%f", result.Latitude, result.Longitude)
	}
}

func TestCachingProviderCachesGeocode(t *testing.T) {
	calls := 0
	inner := &countingProvider{
		geocodeFn: func(context.Context, string) (Result, error) {
			calls++
			return Result{Latitude: 1, Longitude: 2, FormattedAddress: "cached"}, nil
		},
	}
	cached := NewCachingProvider(inner, 5*time.Minute)

	_, err := cached.Geocode(context.Background(), "Same Address")
	if err != nil {
		t.Fatalf("first Geocode: %v", err)
	}
	_, err = cached.Geocode(context.Background(), "same address")
	if err != nil {
		t.Fatalf("second Geocode: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 underlying geocode call, got %d", calls)
	}
}

type countingProvider struct {
	geocodeFn func(context.Context, string) (Result, error)
}

func (c *countingProvider) Geocode(ctx context.Context, address string) (Result, error) {
	return c.geocodeFn(ctx, address)
}

func (c *countingProvider) SearchNearby(context.Context, string, float64, float64, int) ([]Place, error) {
	return nil, nil
}
