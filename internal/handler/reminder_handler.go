package handler

import (
	"errors"
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

// Register mounts authenticated reminder routes.
func (h *ReminderHandler) Register(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Group(func(pr chi.Router) {
		pr.Use(authMiddleware)
		pr.Get("/api/v1/reminders", h.ListLocation)
		pr.Get("/api/v1/reminders/{id}", h.Get)
		pr.Delete("/api/v1/reminders/{id}", h.Cancel)
		pr.Patch("/api/v1/reminders/{id}/triggered", h.MarkTriggered)
		pr.Get("/api/v1/users/me/reminders", h.List)
		pr.Get("/api/v1/users/me/reminders/notifications/pending", h.ListPendingNotifications)
		pr.Post("/api/v1/users/me/reminders/{id}/delivered", h.MarkDelivered)
	})
}

// ListLocation godoc
// @Summary      List location reminders
// @Tags         reminders
// @Produce      json
// @Security     BearerAuth
// @Param        status  query  string  false  "Filter by status (default: pending)"
// @Success      200     {object}  response.Envelope{data=[]reminder.Response}
// @Failure      401     {object}  response.Envelope
// @Router       /api/v1/reminders [get]
func (h *ReminderHandler) ListLocation(w http.ResponseWriter, r *http.Request) {
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

// List godoc
// @Summary      List reminders
// @Description  Returns active time reminders for the authenticated user. Filter by UTC date: today, tomorrow, or all (default).
// @Tags         reminders
// @Produce      json
// @Security     BearerAuth
// @Param        filter  query  string  false  "Date filter: today, tomorrow, or all"  default(all)
// @Success      200  {object}  response.Envelope{data=[]reminder.Response}
// @Failure      401  {object}  response.Envelope
// @Router       /api/v1/users/me/reminders [get]
func (h *ReminderHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := appmw.UserIDFrom(r.Context())
	filter := parseListFilter(r.URL.Query().Get("filter"))

	items, err := h.svc.List(r.Context(), userID, filter)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, reminder.ToResponses(items))
}

// ListPendingNotifications godoc
// @Summary      List pending notification deliveries
// @Description  Returns notified reminders awaiting client delivery acknowledgement.
// @Tags         reminders
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Envelope{data=[]reminder.Response}
// @Failure      401  {object}  response.Envelope
// @Router       /api/v1/users/me/reminders/notifications/pending [get]
func (h *ReminderHandler) ListPendingNotifications(w http.ResponseWriter, r *http.Request) {
	userID := appmw.UserIDFrom(r.Context())

	items, err := h.svc.ListPendingDelivery(r.Context(), userID)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, reminder.ToResponses(items))
}

// MarkDelivered godoc
// @Summary      Acknowledge reminder delivery
// @Description  Marks a notified reminder as delivered by the client.
// @Tags         reminders
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "Reminder ID"
// @Success      204
// @Failure      401  {object}  response.Envelope
// @Failure      404  {object}  response.Envelope
// @Router       /api/v1/users/me/reminders/{id}/delivered [post]
func (h *ReminderHandler) MarkDelivered(w http.ResponseWriter, r *http.Request) {
	userID := appmw.UserIDFrom(r.Context())
	id := chi.URLParam(r, "id")

	if err := h.svc.MarkDelivered(r.Context(), userID, id); err != nil {
		response.Error(w, mapMarkDeliveredError(err))
		return
	}
	response.NoContent(w)
}

func parseListFilter(raw string) reminder.ListFilter {
	switch raw {
	case "today":
		return reminder.ListFilterToday
	case "tomorrow":
		return reminder.ListFilterTomorrow
	default:
		return reminder.ListFilterAll
	}
}

func mapMarkDeliveredError(err error) error {
	var appErr *response.AppError
	if errors.As(err, &appErr) {
		if appErr.Status == http.StatusForbidden || appErr.Status == http.StatusConflict {
			return response.NewNotFound("reminder not found")
		}
	}
	return err
}
