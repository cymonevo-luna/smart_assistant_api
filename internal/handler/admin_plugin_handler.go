package handler

import (
	"net/http"

	"github.com/cymonevo/go_template/internal/domain/plugin"
	"github.com/cymonevo/go_template/pkg/response"
	"github.com/cymonevo/go_template/pkg/validator"
	"github.com/go-chi/chi/v5"
)

// AdminPluginHandler exposes admin-only plugin catalog management endpoints.
type AdminPluginHandler struct {
	svc      *plugin.Service
	validate *validator.Validator
}

// NewAdminPluginHandler constructs an AdminPluginHandler.
func NewAdminPluginHandler(svc *plugin.Service, validate *validator.Validator) *AdminPluginHandler {
	return &AdminPluginHandler{svc: svc, validate: validate}
}

// Register mounts the admin plugin routes under "/api/admin/plugins".
func (h *AdminPluginHandler) Register(r chi.Router, authMiddleware, adminMiddleware func(http.Handler) http.Handler) {
	r.Group(func(ar chi.Router) {
		ar.Use(authMiddleware)
		ar.Use(adminMiddleware)

		ar.Post("/api/admin/plugins", h.RegisterPlugin)
		ar.Put("/api/admin/plugins/{slug}", h.Update)
	})
}

// RegisterPlugin godoc
// @Summary      Register a plugin in the catalog (admin only)
// @Tags         plugins
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      plugin.RegisterPluginInput  true  "Plugin registration payload"
// @Success      201   {object}  response.Envelope{data=plugin.DetailResponse}
// @Failure      403   {object}  response.Envelope
// @Failure      409   {object}  response.Envelope
// @Failure      422   {object}  response.Envelope
// @Router       /api/admin/plugins [post]
func (h *AdminPluginHandler) RegisterPlugin(w http.ResponseWriter, r *http.Request) {
	var in plugin.RegisterPluginInput
	if err := h.validate.BindJSON(r, &in); err != nil {
		response.Error(w, err)
		return
	}

	p, err := h.svc.Register(r.Context(), in)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.Created(w, plugin.ToDetailResponse(p))
}

// Update godoc
// @Summary      Update a catalog plugin (admin only)
// @Tags         plugins
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        slug  path      string                    true  "Plugin slug"
// @Param        body  body      plugin.UpdatePluginInput  true  "Plugin update payload"
// @Success      200   {object}  response.Envelope{data=plugin.DetailResponse}
// @Failure      403   {object}  response.Envelope
// @Failure      404   {object}  response.Envelope
// @Failure      422   {object}  response.Envelope
// @Router       /api/admin/plugins/{slug} [put]
func (h *AdminPluginHandler) Update(w http.ResponseWriter, r *http.Request) {
	var in plugin.UpdatePluginInput
	if err := h.validate.BindJSON(r, &in); err != nil {
		response.Error(w, err)
		return
	}

	p, err := h.svc.Update(r.Context(), chi.URLParam(r, "slug"), in)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, plugin.ToDetailResponse(p))
}
