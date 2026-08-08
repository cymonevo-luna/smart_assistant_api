package handler

import (
	"net/http"
	"strconv"

	"github.com/cymonevo/go_template/internal/domain/user"
	"github.com/cymonevo/go_template/pkg/response"
	"github.com/go-chi/chi/v5"
)

// AdminUserHandler exposes the admin-only user endpoints. Every route it mounts
// lives under the "/api/admin" prefix and requires both a valid access token
// and the admin role. Keeping admin endpoints in their own handler keeps the
// authorization surface explicit and separate from the public/self-service API.
type AdminUserHandler struct {
	svc *user.Service
}

// NewAdminUserHandler constructs an AdminUserHandler.
func NewAdminUserHandler(svc *user.Service) *AdminUserHandler {
	return &AdminUserHandler{svc: svc}
}

// Register mounts the admin routes under "/api/admin". authMiddleware enforces a
// valid access token and adminMiddleware additionally restricts access to the
// admin role; both run on every route in this group.
func (h *AdminUserHandler) Register(r chi.Router, authMiddleware, adminMiddleware func(http.Handler) http.Handler) {
	r.Group(func(ar chi.Router) {
		ar.Use(authMiddleware)
		ar.Use(adminMiddleware)

		ar.Get("/api/admin/users", h.List)
		ar.Get("/api/admin/users/{id}", h.Get)
		ar.Delete("/api/admin/users/{id}", h.Delete)
	})
}

// List godoc
// @Summary      List users (admin only)
// @Tags         admin
// @Produce      json
// @Security     BearerAuth
// @Param        page      query  int     false  "Page number"
// @Param        per_page  query  int     false  "Items per page"
// @Param        search    query  string  false  "Name search"
// @Success      200  {object}  response.Envelope{data=[]user.Response,meta=user.PageMeta}
// @Failure      403  {object}  response.Envelope
// @Router       /api/admin/users [get]
func (h *AdminUserHandler) List(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))

	in := user.ListUsersInput{
		Page:    page,
		PerPage: perPage,
		Search:  r.URL.Query().Get("search"),
	}

	users, meta, err := h.svc.List(r.Context(), in)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.Paginated(w, user.ToResponses(users), meta)
}

// Get godoc
// @Summary      Get any user by ID (admin only)
// @Tags         admin
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      string  true  "User ID"
// @Success      200  {object}  response.Envelope{data=user.Response}
// @Failure      403  {object}  response.Envelope
// @Failure      404  {object}  response.Envelope
// @Router       /api/admin/users/{id} [get]
func (h *AdminUserHandler) Get(w http.ResponseWriter, r *http.Request) {
	u, err := h.svc.GetByID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, user.ToResponse(u))
}

// Delete godoc
// @Summary      Delete a user (admin only)
// @Tags         admin
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  string  true  "User ID"
// @Success      204  "No Content"
// @Failure      403  {object}  response.Envelope
// @Router       /api/admin/users/{id} [delete]
func (h *AdminUserHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		response.Error(w, err)
		return
	}
	response.NoContent(w)
}
