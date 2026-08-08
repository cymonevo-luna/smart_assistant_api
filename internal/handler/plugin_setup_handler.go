package handler

import (
	"net/http"

	"github.com/cymonevo/go_template/internal/domain/plugin_setup/oauth_google"
	appmw "github.com/cymonevo/go_template/internal/middleware"
	"github.com/cymonevo/go_template/pkg/response"
	"github.com/go-chi/chi/v5"
)

// PluginSetupHandler exposes plugin setup endpoints.
type PluginSetupHandler struct {
	googleSvc       *oauthgoogle.Service
	successRedirect string
}

// NewPluginSetupHandler constructs a PluginSetupHandler.
func NewPluginSetupHandler(googleSvc *oauthgoogle.Service, successRedirect string) *PluginSetupHandler {
	return &PluginSetupHandler{googleSvc: googleSvc, successRedirect: successRedirect}
}

// Register mounts setup routes. The OAuth callback is public; other routes require auth.
func (h *PluginSetupHandler) Register(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Get("/api/v1/plugins/oauth/google/callback", h.GoogleCallback)

	r.Group(func(pr chi.Router) {
		pr.Use(authMiddleware)
		pr.Post("/api/v1/users/me/plugins/{pluginId}/setup", h.InitSetup)
		pr.Get("/api/v1/users/me/plugins/{pluginId}/setup/status", h.GetSetupStatus)
	})
}

// InitSetup godoc
// @Summary      Start plugin OAuth setup
// @Tags         user-plugins
// @Produce      json
// @Security     BearerAuth
// @Param        pluginId  path  string  true  "Installed plugin ID"
// @Success      200  {object}  response.Envelope{data=oauthgoogle.SetupInitResponse}
// @Failure      400  {object}  response.Envelope
// @Failure      401  {object}  response.Envelope
// @Failure      404  {object}  response.Envelope
// @Router       /api/v1/users/me/plugins/{pluginId}/setup [post]
func (h *PluginSetupHandler) InitSetup(w http.ResponseWriter, r *http.Request) {
	userID := appmw.UserIDFrom(r.Context())
	installID := chi.URLParam(r, "pluginId")

	result, err := h.googleSvc.InitSetup(r.Context(), userID, installID)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, result)
}

// GetSetupStatus godoc
// @Summary      Get plugin setup status
// @Tags         user-plugins
// @Produce      json
// @Security     BearerAuth
// @Param        pluginId  path  string  true  "Installed plugin ID"
// @Success      200  {object}  response.Envelope{data=oauthgoogle.SetupStatusResponse}
// @Failure      401  {object}  response.Envelope
// @Failure      404  {object}  response.Envelope
// @Router       /api/v1/users/me/plugins/{pluginId}/setup/status [get]
func (h *PluginSetupHandler) GetSetupStatus(w http.ResponseWriter, r *http.Request) {
	userID := appmw.UserIDFrom(r.Context())
	installID := chi.URLParam(r, "pluginId")

	result, err := h.googleSvc.GetSetupStatus(r.Context(), userID, installID)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, result)
}

// GoogleCallback godoc
// @Summary      Google OAuth callback
// @Tags         plugins
// @Param        code   query  string  true  "Authorization code"
// @Param        state  query  string  true  "OAuth state"
// @Success      302
// @Router       /api/v1/plugins/oauth/google/callback [get]
func (h *PluginSetupHandler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")

	redirectURL := h.successRedirect + "?status=error"
	if state != "" && code != "" {
		if err := h.googleSvc.HandleCallback(r.Context(), state, code); err == nil {
			redirectURL = h.successRedirect + "?status=success"
		}
	}

	http.Redirect(w, r, redirectURL, http.StatusFound)
}
