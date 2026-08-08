// Package handler adapts HTTP requests to the domain services.
package handler

import (
	"net/http"

	"github.com/cymonevo/go_template/internal/domain/user"
	"github.com/cymonevo/go_template/pkg/response"
	"github.com/cymonevo/go_template/pkg/validator"
	"github.com/go-chi/chi/v5"
)

// UserHandler exposes the public and self-service user endpoints under the
// "/api/v1" prefix. Admin-only operations live in AdminUserHandler. It is
// intentionally thin: parse, delegate to the service, render the response.
type UserHandler struct {
	svc      *user.Service
	validate *validator.Validator
}

// NewUserHandler constructs a UserHandler.
func NewUserHandler(svc *user.Service, validate *validator.Validator) *UserHandler {
	return &UserHandler{svc: svc, validate: validate}
}

// Register mounts the public auth routes and the authenticated self-service
// routes. authMiddleware protects the routes that require a valid access token.
func (h *UserHandler) Register(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Post("/api/v1/auth/register", h.Create)
	r.Post("/api/v1/auth/login", h.Login)
	r.Post("/api/v1/auth/refresh", h.Refresh)

	r.Group(func(pr chi.Router) {
		pr.Use(authMiddleware)
		pr.Get("/api/v1/users/{id}", h.Get)
		pr.Put("/api/v1/users/{id}", h.Update)
	})
}

// Create godoc
// @Summary      Register a new user
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      user.CreateUserInput  true  "Registration payload"
// @Success      201   {object}  response.Envelope{data=user.Response}
// @Failure      409   {object}  response.Envelope
// @Failure      422   {object}  response.Envelope
// @Router       /api/v1/auth/register [post]
func (h *UserHandler) Create(w http.ResponseWriter, r *http.Request) {
	var in user.CreateUserInput
	if err := h.validate.BindJSON(r, &in); err != nil {
		response.Error(w, err)
		return
	}

	u, err := h.svc.Create(r.Context(), in)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.Created(w, user.ToResponse(u))
}

// Login godoc
// @Summary      Authenticate and obtain tokens
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      user.LoginInput  true  "Credentials"
// @Success      200   {object}  response.Envelope
// @Failure      401   {object}  response.Envelope
// @Router       /api/v1/auth/login [post]
func (h *UserHandler) Login(w http.ResponseWriter, r *http.Request) {
	var in user.LoginInput
	if err := h.validate.BindJSON(r, &in); err != nil {
		response.Error(w, err)
		return
	}

	pair, u, err := h.svc.Authenticate(r.Context(), in.Email, in.Password)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, map[string]any{"tokens": pair, "user": user.ToResponse(u)})
}

// Refresh godoc
// @Summary      Refresh an access token
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      user.RefreshInput  true  "Refresh token"
// @Success      200   {object}  response.Envelope
// @Failure      401   {object}  response.Envelope
// @Router       /api/v1/auth/refresh [post]
func (h *UserHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var in user.RefreshInput
	if err := h.validate.BindJSON(r, &in); err != nil {
		response.Error(w, err)
		return
	}

	pair, err := h.svc.Refresh(r.Context(), in.RefreshToken)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, map[string]any{"tokens": pair})
}

// Get godoc
// @Summary      Get a user by ID
// @Tags         users
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      string  true  "User ID"
// @Success      200  {object}  response.Envelope{data=user.Response}
// @Failure      404  {object}  response.Envelope
// @Router       /api/v1/users/{id} [get]
func (h *UserHandler) Get(w http.ResponseWriter, r *http.Request) {
	u, err := h.svc.GetByID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, user.ToResponse(u))
}

// Update godoc
// @Summary      Update a user
// @Tags         users
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      string                true  "User ID"
// @Param        body  body      user.UpdateUserInput  true  "Update payload"
// @Success      200   {object}  response.Envelope{data=user.Response}
// @Failure      404   {object}  response.Envelope
// @Router       /api/v1/users/{id} [put]
func (h *UserHandler) Update(w http.ResponseWriter, r *http.Request) {
	var in user.UpdateUserInput
	if err := h.validate.BindJSON(r, &in); err != nil {
		response.Error(w, err)
		return
	}

	u, err := h.svc.Update(r.Context(), chi.URLParam(r, "id"), in)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, user.ToResponse(u))
}
