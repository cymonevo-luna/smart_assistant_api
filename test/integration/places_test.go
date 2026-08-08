//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/cymonevo/go_template/pkg/auth"
	"github.com/google/uuid"
)

func TestPlacesNearbyAuthenticated(t *testing.T) {
	token := userAccessToken(t)
	client := newClient(t).authed(token)

	res := client.get("/api/v1/places/nearby?keyword=Alfamart&latitude=-6.17&longitude=106.86&radius_meters=500")
	res.requireStatus(t, http.StatusOK)
	if !res.Envelope.Success {
		t.Fatalf("expected success envelope, got %+v", res.Envelope)
	}

	var data struct {
		Places []struct {
			Name      string  `json:"name"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
			PlaceID   string  `json:"place_id"`
		} `json:"places"`
	}
	res.decode(t, &data)
	if len(data.Places) == 0 {
		t.Fatal("expected at least one place in response")
	}
}

func TestPlacesNearbyMissingKeyword(t *testing.T) {
	token := userAccessToken(t)
	client := newClient(t).authed(token)

	res := client.get("/api/v1/places/nearby?latitude=-6.17&longitude=106.86")
	res.requireStatus(t, http.StatusUnprocessableEntity)
	if res.Envelope.Error == nil || res.Envelope.Error.Code != "validation_error" {
		t.Fatalf("expected validation_error, got %+v", res.Envelope.Error)
	}
}

func TestPlacesNearbyUnauthenticated(t *testing.T) {
	res := newClient(t).get("/api/v1/places/nearby?keyword=Alfamart&latitude=-6.17&longitude=106.86")
	res.requireStatus(t, http.StatusUnauthorized)
}

func userAccessToken(t *testing.T) string {
	t.Helper()
	pair, err := application.Container().Tokens.GeneratePair(uuid.NewString(), fmt.Sprintf("places-%s@integration.test", uuid.NewString()), auth.RoleUser)
	if err != nil {
		t.Fatalf("mint user token: %v", err)
	}
	return pair.AccessToken
}
