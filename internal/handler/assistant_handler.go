package handler

import (
	"net/http"

	"github.com/cymonevo/go_template/internal/domain/assistant"
	appmw "github.com/cymonevo/go_template/internal/middleware"
	"github.com/cymonevo/go_template/pkg/response"
	"github.com/cymonevo/go_template/pkg/validator"
	"github.com/go-chi/chi/v5"
)

// AssistantHandler exposes assistant session and message endpoints.
type AssistantHandler struct {
	svc      *assistant.Service
	validate *validator.Validator
}

// NewAssistantHandler constructs an AssistantHandler.
func NewAssistantHandler(svc *assistant.Service, validate *validator.Validator) *AssistantHandler {
	return &AssistantHandler{svc: svc, validate: validate}
}

// Register mounts authenticated assistant routes.
func (h *AssistantHandler) Register(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Group(func(pr chi.Router) {
		pr.Use(authMiddleware)
		pr.Post("/api/v1/assistant/sessions", h.CreateSession)
		pr.Post("/api/v1/assistant/sessions/{sessionId}/messages", h.ProcessMessage)
		pr.Get("/api/v1/assistant/sessions/{sessionId}/messages", h.ListMessages)
	})
}

// CreateSession godoc
// @Summary      Create assistant session
// @Tags         assistant
// @Produce      json
// @Security     BearerAuth
// @Success      201  {object}  response.Envelope{data=assistant.CreateSessionResponse}
// @Failure      401  {object}  response.Envelope
// @Router       /api/v1/assistant/sessions [post]
func (h *AssistantHandler) CreateSession(w http.ResponseWriter, r *http.Request) {
	userID := appmw.UserIDFrom(r.Context())
	out, err := h.svc.CreateSession(r.Context(), userID)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.Created(w, out)
}

// ProcessMessage godoc
// @Summary      Send a message to the assistant
// @Tags         assistant
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        sessionId  path  string  true  "Session ID"
// @Param        body       body  assistant.ProcessMessageInput  true  "Message payload"
// @Success      200  {object}  response.Envelope{data=assistant.ProcessMessageResponse}
// @Failure      401  {object}  response.Envelope
// @Failure      403  {object}  response.Envelope
// @Failure      404  {object}  response.Envelope
// @Failure      422  {object}  response.Envelope
// @Router       /api/v1/assistant/sessions/{sessionId}/messages [post]
func (h *AssistantHandler) ProcessMessage(w http.ResponseWriter, r *http.Request) {
	var in assistant.ProcessMessageInput
	if err := h.validate.BindJSON(r, &in); err != nil {
		response.Error(w, err)
		return
	}

	userID := appmw.UserIDFrom(r.Context())
	sessionID := chi.URLParam(r, "sessionId")
	out, err := h.svc.ProcessMessage(r.Context(), userID, sessionID, in)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, out)
}

// ListMessages godoc
// @Summary      List session messages
// @Tags         assistant
// @Produce      json
// @Security     BearerAuth
// @Param        sessionId  path  string  true  "Session ID"
// @Success      200  {object}  response.Envelope{data=assistant.MessageHistoryResponse}
// @Failure      401  {object}  response.Envelope
// @Failure      403  {object}  response.Envelope
// @Failure      404  {object}  response.Envelope
// @Router       /api/v1/assistant/sessions/{sessionId}/messages [get]
func (h *AssistantHandler) ListMessages(w http.ResponseWriter, r *http.Request) {
	userID := appmw.UserIDFrom(r.Context())
	sessionID := chi.URLParam(r, "sessionId")
	out, err := h.svc.ListMessages(r.Context(), userID, sessionID)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, out)
}
