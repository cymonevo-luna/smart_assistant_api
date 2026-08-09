package handler

import (
	"net/http"

	"github.com/cymonevo/go_template/internal/domain/plugin"
	pluginsetup "github.com/cymonevo/go_template/internal/domain/plugin_setup"
	"github.com/cymonevo/go_template/internal/domain/plugin_setup/composio_form"
	"github.com/cymonevo/go_template/internal/domain/plugin_setup/oauth_google"
	"github.com/cymonevo/go_template/internal/domain/user_plugin"
	appmw "github.com/cymonevo/go_template/internal/middleware"
	"github.com/cymonevo/go_template/pkg/response"
	"github.com/cymonevo/go_template/pkg/validator"
	"github.com/go-chi/chi/v5"
)

// PluginSetupHandler exposes plugin setup endpoints.
type PluginSetupHandler struct {
	userPluginRepo  userplugin.Repository
	pluginRepo      plugin.Repository
	googleSvc       *oauthgoogle.Service
	composioFormSvc *composioform.Service
	validate        *validator.Validator
	successRedirect string
}

// NewPluginSetupHandler constructs a PluginSetupHandler.
func NewPluginSetupHandler(
	userPluginRepo userplugin.Repository,
	pluginRepo plugin.Repository,
	googleSvc *oauthgoogle.Service,
	composioFormSvc *composioform.Service,
	validate *validator.Validator,
	successRedirect string,
) *PluginSetupHandler {
	return &PluginSetupHandler{
		userPluginRepo:  userPluginRepo,
		pluginRepo:      pluginRepo,
		googleSvc:       googleSvc,
		composioFormSvc: composioFormSvc,
		validate:        validate,
		successRedirect: successRedirect,
	}
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

type composioSetupRequest struct {
	APIKey string `json:"api_key" validate:"required"`
}

// InitSetup godoc
// @Summary      Start or complete plugin setup
// @Tags         user-plugins
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        pluginId  path  string  true  "Installed plugin ID"
// @Param        body      body  composioSetupRequest  false  "Composio form setup payload (required for setup_type form)"
// @Success      200  {object}  response.Envelope
// @Failure      400  {object}  response.Envelope
// @Failure      401  {object}  response.Envelope
// @Failure      404  {object}  response.Envelope
// @Failure      422  {object}  response.Envelope
// @Router       /api/v1/users/me/plugins/{pluginId}/setup [post]
func (h *PluginSetupHandler) InitSetup(w http.ResponseWriter, r *http.Request) {
	userID := appmw.UserIDFrom(r.Context())
	installID := chi.URLParam(r, "pluginId")

	_, catalog, err := pluginsetup.LoadOwnedInstall(r.Context(), userID, installID, h.userPluginRepo, h.pluginRepo)
	if err != nil {
		response.Error(w, err)
		return
	}

	switch catalog.Manifest.SetupType {
	case plugin.SetupTypeOAuthGoogle:
		result, err := h.googleSvc.InitSetup(r.Context(), userID, installID)
		if err != nil {
			response.Error(w, err)
			return
		}
		response.OK(w, result)
	case plugin.SetupTypeForm:
		var in composioSetupRequest
		if err := h.validate.BindJSON(r, &in); err != nil {
			response.Error(w, err)
			return
		}
		result, err := h.composioFormSvc.SubmitSetup(r.Context(), userID, installID, in.APIKey)
		if err != nil {
			response.Error(w, err)
			return
		}
		response.OK(w, result)
	default:
		response.Error(w, response.NewBadRequest("plugin setup is not required for this plugin type"))
	}
}

// GetSetupStatus godoc
// @Summary      Get plugin setup status
// @Tags         user-plugins
// @Produce      json
// @Security     BearerAuth
// @Param        pluginId  path  string  true  "Installed plugin ID"
// @Success      200  {object}  response.Envelope
// @Failure      401  {object}  response.Envelope
// @Failure      404  {object}  response.Envelope
// @Router       /api/v1/users/me/plugins/{pluginId}/setup/status [get]
func (h *PluginSetupHandler) GetSetupStatus(w http.ResponseWriter, r *http.Request) {
	userID := appmw.UserIDFrom(r.Context())
	installID := chi.URLParam(r, "pluginId")

	_, catalog, err := pluginsetup.LoadOwnedInstall(r.Context(), userID, installID, h.userPluginRepo, h.pluginRepo)
	if err != nil {
		response.Error(w, err)
		return
	}

	switch catalog.Manifest.SetupType {
	case plugin.SetupTypeOAuthGoogle:
		result, err := h.googleSvc.GetSetupStatus(r.Context(), userID, installID)
		if err != nil {
			response.Error(w, err)
			return
		}
		response.OK(w, result)
	case plugin.SetupTypeForm:
		result, err := h.composioFormSvc.GetSetupStatus(r.Context(), userID, installID)
		if err != nil {
			response.Error(w, err)
			return
		}
		response.OK(w, result)
	default:
		response.Error(w, response.NewBadRequest("plugin setup is not required for this plugin type"))
	}
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
