package handler

import (
	"net/http"
	"strconv"

	"github.com/cymonevo/go_template/internal/domain/plugin"
	"github.com/cymonevo/go_template/pkg/response"
	"github.com/go-chi/chi/v5"
)

// PluginHandler exposes the authenticated plugin catalog endpoints.
type PluginHandler struct {
	svc *plugin.Service
}

// NewPluginHandler constructs a PluginHandler.
func NewPluginHandler(svc *plugin.Service) *PluginHandler {
	return &PluginHandler{svc: svc}
}

// Register mounts the plugin catalog routes under "/api/v1/plugins".
func (h *PluginHandler) Register(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Group(func(pr chi.Router) {
		pr.Use(authMiddleware)
		pr.Get("/api/v1/plugins", h.List)
		pr.Get("/api/v1/plugins/{slug}", h.GetBySlug)
	})
}

// List godoc
// @Summary      List plugins in the catalog
// @Tags         plugins
// @Produce      json
// @Security     BearerAuth
// @Param        page      query  int  false  "Page number"
// @Param        per_page  query  int  false  "Items per page"
// @Success      200  {object}  response.Envelope{data=[]plugin.CatalogSummary,meta=plugin.PageMeta}
// @Failure      401  {object}  response.Envelope
// @Router       /api/v1/plugins [get]
func (h *PluginHandler) List(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))

	plugins, meta, err := h.svc.List(r.Context(), plugin.ListPluginsInput{
		Page:    page,
		PerPage: perPage,
	})
	if err != nil {
		response.Error(w, err)
		return
	}
	response.Paginated(w, plugin.ToCatalogSummaries(plugins), meta)
}

// GetBySlug godoc
// @Summary      Get a plugin by slug
// @Tags         plugins
// @Produce      json
// @Security     BearerAuth
// @Param        slug  path      string  true  "Plugin slug"
// @Success      200   {object}  response.Envelope{data=plugin.DetailResponse}
// @Failure      401   {object}  response.Envelope
// @Failure      404   {object}  response.Envelope
// @Router       /api/v1/plugins/{slug} [get]
func (h *PluginHandler) GetBySlug(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.GetBySlug(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, plugin.ToDetailResponse(p))
}
