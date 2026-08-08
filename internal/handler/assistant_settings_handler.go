// Package handler adapts HTTP requests to the domain services.
package handler

import (
	"net/http"

	"github.com/cymonevo/go_template/internal/domain/assistant_settings"
	appmw "github.com/cymonevo/go_template/internal/middleware"
	"github.com/cymonevo/go_template/pkg/response"
	"github.com/cymonevo/go_template/pkg/validator"
	"github.com/go-chi/chi/v5"
)

// AssistantSettingsHandler exposes per-user assistant preference endpoints.
type AssistantSettingsHandler struct {
	svc      *assistantsettings.Service
	validate *validator.Validator
}

// NewAssistantSettingsHandler constructs an AssistantSettingsHandler.
func NewAssistantSettingsHandler(svc *assistantsettings.Service, validate *validator.Validator) *AssistantSettingsHandler {
	return &AssistantSettingsHandler{svc: svc, validate: validate}
}

// Register mounts authenticated assistant settings routes.
func (h *AssistantSettingsHandler) Register(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Group(func(pr chi.Router) {
		pr.Use(authMiddleware)
		pr.Get("/api/v1/assistant/settings", h.Get)
		pr.Put("/api/v1/assistant/settings", h.Update)
	})
}

// Get godoc
// @Summary      Get assistant settings
// @Tags         assistant
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Envelope{data=assistantsettings.Response}
// @Failure      401  {object}  response.Envelope
// @Router       /api/v1/assistant/settings [get]
func (h *AssistantSettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID := appmw.UserIDFrom(r.Context())
	settings, err := h.svc.GetOrCreate(r.Context(), userID)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, assistantsettings.ToResponse(settings))
}

// Update godoc
// @Summary      Update assistant settings
// @Tags         assistant
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      assistantsettings.UpdateInput  true  "Settings payload"
// @Success      200   {object}  response.Envelope{data=assistantsettings.Response}
// @Failure      401   {object}  response.Envelope
// @Failure      422   {object}  response.Envelope
// @Router       /api/v1/assistant/settings [put]
func (h *AssistantSettingsHandler) Update(w http.ResponseWriter, r *http.Request) {
	var in assistantsettings.UpdateInput
	if err := h.validate.BindJSON(r, &in); err != nil {
		response.Error(w, err)
		return
	}

	userID := appmw.UserIDFrom(r.Context())
	settings, err := h.svc.Update(r.Context(), userID, in)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, assistantsettings.ToResponse(settings))
}
