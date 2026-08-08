package handler

import (
	"net/http"

	"github.com/cymonevo/go_template/internal/domain/user_plugin"
	appmw "github.com/cymonevo/go_template/internal/middleware"
	"github.com/cymonevo/go_template/pkg/response"
	"github.com/cymonevo/go_template/pkg/validator"
	"github.com/go-chi/chi/v5"
)

// UserPluginHandler exposes per-user plugin install and management endpoints.
type UserPluginHandler struct {
	svc      *userplugin.Service
	validate *validator.Validator
}

// NewUserPluginHandler constructs a UserPluginHandler.
func NewUserPluginHandler(svc *userplugin.Service, validate *validator.Validator) *UserPluginHandler {
	return &UserPluginHandler{svc: svc, validate: validate}
}

// Register mounts authenticated user plugin routes under /api/v1/users/me/plugins.
func (h *UserPluginHandler) Register(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Group(func(pr chi.Router) {
		pr.Use(authMiddleware)
		pr.Get("/api/v1/users/me/plugins", h.List)
		pr.Post("/api/v1/users/me/plugins", h.Install)
		pr.Delete("/api/v1/users/me/plugins/{pluginId}", h.Uninstall)
		pr.Patch("/api/v1/users/me/plugins/{pluginId}", h.SetEnabled)
	})
}

// List godoc
// @Summary      List installed plugins
// @Tags         user-plugins
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Envelope{data=[]userplugin.InstalledResponse}
// @Failure      401  {object}  response.Envelope
// @Router       /api/v1/users/me/plugins [get]
func (h *UserPluginHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := appmw.UserIDFrom(r.Context())
	items, err := h.svc.ListInstalled(r.Context(), userID)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, items)
}

// Install godoc
// @Summary      Install a plugin
// @Tags         user-plugins
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      userplugin.InstallInput  true  "Install payload"
// @Success      201   {object}  response.Envelope{data=userplugin.InstalledResponse}
// @Failure      401   {object}  response.Envelope
// @Failure      404   {object}  response.Envelope
// @Failure      422   {object}  response.Envelope
// @Router       /api/v1/users/me/plugins [post]
func (h *UserPluginHandler) Install(w http.ResponseWriter, r *http.Request) {
	var in userplugin.InstallInput
	if err := h.validate.BindJSON(r, &in); err != nil {
		response.Error(w, err)
		return
	}

	userID := appmw.UserIDFrom(r.Context())
	item, err := h.svc.Install(r.Context(), userID, in.PluginSlug)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.Created(w, item)
}

// Uninstall godoc
// @Summary      Uninstall a plugin
// @Tags         user-plugins
// @Produce      json
// @Security     BearerAuth
// @Param        pluginId  path  string  true  "Installed plugin ID"
// @Success      204
// @Failure      401  {object}  response.Envelope
// @Failure      403  {object}  response.Envelope
// @Failure      404  {object}  response.Envelope
// @Router       /api/v1/users/me/plugins/{pluginId} [delete]
func (h *UserPluginHandler) Uninstall(w http.ResponseWriter, r *http.Request) {
	userID := appmw.UserIDFrom(r.Context())
	installID := chi.URLParam(r, "pluginId")
	if err := h.svc.Uninstall(r.Context(), userID, installID); err != nil {
		response.Error(w, err)
		return
	}
	response.NoContent(w)
}

// SetEnabled godoc
// @Summary      Enable or disable an installed plugin
// @Tags         user-plugins
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        pluginId  path  string  true  "Installed plugin ID"
// @Param        body      body  userplugin.SetEnabledInput  true  "Enable payload"
// @Success      200  {object}  response.Envelope{data=userplugin.InstalledResponse}
// @Failure      401  {object}  response.Envelope
// @Failure      403  {object}  response.Envelope
// @Failure      404  {object}  response.Envelope
// @Router       /api/v1/users/me/plugins/{pluginId} [patch]
func (h *UserPluginHandler) SetEnabled(w http.ResponseWriter, r *http.Request) {
	var in userplugin.SetEnabledInput
	if err := h.validate.BindJSON(r, &in); err != nil {
		response.Error(w, err)
		return
	}

	userID := appmw.UserIDFrom(r.Context())
	installID := chi.URLParam(r, "pluginId")
	item, err := h.svc.SetEnabled(r.Context(), userID, installID, in.Enabled)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, item)
}
