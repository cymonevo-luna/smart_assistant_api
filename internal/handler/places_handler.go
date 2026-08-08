package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/cymonevo/go_template/pkg/places"
	"github.com/cymonevo/go_template/pkg/response"
	"github.com/cymonevo/go_template/pkg/validator"
	"github.com/go-chi/chi/v5"
)

// PlacesHandler exposes authenticated places proxy endpoints.
type PlacesHandler struct {
	provider places.Provider
	validate *validator.Validator
}

// NewPlacesHandler constructs a PlacesHandler.
func NewPlacesHandler(provider places.Provider, validate *validator.Validator) *PlacesHandler {
	return &PlacesHandler{provider: provider, validate: validate}
}

// Register mounts authenticated places routes.
func (h *PlacesHandler) Register(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Group(func(pr chi.Router) {
		pr.Use(authMiddleware)
		pr.Get("/api/v1/places/nearby", h.SearchNearby)
	})
}

type nearbyQuery struct {
	Keyword      string  `validate:"required"`
	Latitude     float64 `validate:"required"`
	Longitude    float64 `validate:"required"`
	RadiusMeters int     `validate:"omitempty,min=1,max=50000"`
}

type nearbyPlaceResponse struct {
	Name      string  `json:"name"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	PlaceID   string  `json:"place_id"`
}

type nearbyResponse struct {
	Places []nearbyPlaceResponse `json:"places"`
}

// SearchNearby godoc
// @Summary      Search nearby places by keyword
// @Description  Proxies nearby place search to the configured provider. Keyword nearby accuracy depends on the external provider.
// @Tags         places
// @Produce      json
// @Security     BearerAuth
// @Param        keyword        query  string   true   "Place keyword"
// @Param        latitude       query  number   true   "Latitude"
// @Param        longitude      query  number   true   "Longitude"
// @Param        radius_meters  query  int      false  "Search radius in meters (default 500)"
// @Success      200  {object}  response.Envelope{data=handler.nearbyResponse}
// @Failure      401  {object}  response.Envelope
// @Failure      422  {object}  response.Envelope
// @Router       /api/v1/places/nearby [get]
func (h *PlacesHandler) SearchNearby(w http.ResponseWriter, r *http.Request) {
	query, err := parseNearbyQuery(r)
	if err != nil {
		response.Error(w, err)
		return
	}
	if err := h.validate.Struct(query); err != nil {
		response.Error(w, err)
		return
	}

	results, err := h.provider.SearchNearby(r.Context(), query.Keyword, query.Latitude, query.Longitude, query.RadiusMeters)
	if err != nil {
		response.Error(w, response.NewInternal("failed to search nearby places").Wrap(err))
		return
	}

	places := make([]nearbyPlaceResponse, 0, len(results))
	for _, item := range results {
		places = append(places, nearbyPlaceResponse{
			Name:      item.Name,
			Latitude:  item.Latitude,
			Longitude: item.Longitude,
			PlaceID:   item.PlaceID,
		})
	}

	response.OK(w, nearbyResponse{Places: places})
}

func parseNearbyQuery(r *http.Request) (nearbyQuery, error) {
	values := r.URL.Query()
	query := nearbyQuery{
		Keyword:      strings.TrimSpace(values.Get("keyword")),
		RadiusMeters: 500,
	}

	latRaw := strings.TrimSpace(values.Get("latitude"))
	if latRaw == "" {
		return nearbyQuery{}, response.NewValidation(map[string]string{
			"latitude": "is required",
		}).WithMessage("validation failed: latitude is required")
	}
	lat, err := strconv.ParseFloat(latRaw, 64)
	if err != nil {
		return nearbyQuery{}, response.NewValidation(map[string]string{
			"latitude": "must be a valid number",
		}).WithMessage("validation failed: latitude must be a valid number")
	}
	query.Latitude = lat

	lngRaw := strings.TrimSpace(values.Get("longitude"))
	if lngRaw == "" {
		return nearbyQuery{}, response.NewValidation(map[string]string{
			"longitude": "is required",
		}).WithMessage("validation failed: longitude is required")
	}
	lng, err := strconv.ParseFloat(lngRaw, 64)
	if err != nil {
		return nearbyQuery{}, response.NewValidation(map[string]string{
			"longitude": "must be a valid number",
		}).WithMessage("validation failed: longitude must be a valid number")
	}
	query.Longitude = lng

	if radiusRaw := strings.TrimSpace(values.Get("radius_meters")); radiusRaw != "" {
		radius, err := strconv.Atoi(radiusRaw)
		if err != nil {
			return nearbyQuery{}, response.NewValidation(map[string]string{
				"radius_meters": "must be a valid integer",
			}).WithMessage("validation failed: radius_meters must be a valid integer")
		}
		query.RadiusMeters = radius
	}

	return query, nil
}
