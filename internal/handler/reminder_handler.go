package handler

import (
	"net/http"

	"github.com/cymonevo/go_template/internal/domain/reminder"
	appmw "github.com/cymonevo/go_template/internal/middleware"
	"github.com/cymonevo/go_template/pkg/response"
	"github.com/go-chi/chi/v5"
)

// ReminderHandler exposes authenticated reminder endpoints.
type ReminderHandler struct {
	svc *reminder.Service
}

// NewReminderHandler constructs a ReminderHandler.
func NewReminderHandler(svc *reminder.Service) *ReminderHandler {
	return &ReminderHandler{svc: svc}
}

// Register mounts authenticated reminder routes under /api/v1/reminders.
func (h *ReminderHandler) Register(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Group(func(pr chi.Router) {
		pr.Use(authMiddleware)
		pr.Get("/api/v1/reminders", h.List)
		pr.Get("/api/v1/reminders/{id}", h.Get)
		pr.Delete("/api/v1/reminders/{id}", h.Cancel)
		pr.Patch("/api/v1/reminders/{id}/triggered", h.MarkTriggered)
	})
}

// List godoc
// @Summary      List reminders
// @Tags         reminders
// @Produce      json
// @Security     BearerAuth
// @Param        status  query  string  false  "Filter by status (default: pending)"
// @Success      200     {object}  response.Envelope{data=[]reminder.Response}
// @Failure      401     {object}  response.Envelope
// @Router       /api/v1/reminders [get]
func (h *ReminderHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := appmw.UserIDFrom(r.Context())
	status := r.URL.Query().Get("status")

	items, err := h.svc.ListByUser(r.Context(), userID, status)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, reminder.ToResponses(items))
}

// Get godoc
// @Summary      Get a reminder
// @Tags         reminders
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "Reminder ID"
// @Success      200  {object}  response.Envelope{data=reminder.Response}
// @Failure      401  {object}  response.Envelope
// @Failure      404  {object}  response.Envelope
// @Router       /api/v1/reminders/{id} [get]
func (h *ReminderHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID := appmw.UserIDFrom(r.Context())
	id := chi.URLParam(r, "id")

	item, err := h.svc.GetByID(r.Context(), userID, id)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, reminder.ToResponse(item))
}

// Cancel godoc
// @Summary      Cancel a reminder
// @Tags         reminders
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "Reminder ID"
// @Success      200  {object}  response.Envelope{data=reminder.Response}
// @Failure      401  {object}  response.Envelope
// @Failure      404  {object}  response.Envelope
// @Router       /api/v1/reminders/{id} [delete]
func (h *ReminderHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	userID := appmw.UserIDFrom(r.Context())
	id := chi.URLParam(r, "id")

	item, err := h.svc.Cancel(r.Context(), userID, id)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, reminder.ToResponse(item))
}

// MarkTriggered godoc
// @Summary      Mark a reminder as triggered
// @Tags         reminders
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "Reminder ID"
// @Success      200  {object}  response.Envelope{data=reminder.Response}
// @Failure      401  {object}  response.Envelope
// @Failure      404  {object}  response.Envelope
// @Router       /api/v1/reminders/{id}/triggered [patch]
func (h *ReminderHandler) MarkTriggered(w http.ResponseWriter, r *http.Request) {
	userID := appmw.UserIDFrom(r.Context())
	id := chi.URLParam(r, "id")

	item, err := h.svc.MarkTriggered(r.Context(), userID, id)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, reminder.ToResponse(item))
}
