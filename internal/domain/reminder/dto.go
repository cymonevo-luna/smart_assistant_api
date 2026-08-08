package reminder

// ListFilter scopes List results by remind_at date in UTC.
type ListFilter string

const (
	ListFilterToday    ListFilter = "today"
	ListFilterTomorrow ListFilter = "tomorrow"
	ListFilterAll      ListFilter = "all"
)

// Response is the outward-facing reminder shape.
type Response struct {
	ID           string  `json:"id"`
	UserID       string  `json:"user_id"`
	UserPluginID *string `json:"user_plugin_id,omitempty"`
	Message      string  `json:"message"`
	RemindAt     string  `json:"remind_at"`
	Status       string  `json:"status"`
	NotifiedAt   *string `json:"notified_at,omitempty"`
	DeliveredAt  *string `json:"delivered_at,omitempty"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

// ToResponse maps a Reminder to its API representation.
func ToResponse(r *Reminder) Response {
	resp := Response{
		ID:           r.ID,
		UserID:       r.UserID,
		UserPluginID: r.UserPluginID,
		Message:      r.Message,
		RemindAt:     r.RemindAt.UTC().Format(timeLayout),
		Status:       r.Status,
		CreatedAt:    r.CreatedAt.UTC().Format(timeLayout),
		UpdatedAt:    r.UpdatedAt.UTC().Format(timeLayout),
	}
	if r.NotifiedAt != nil {
		s := r.NotifiedAt.UTC().Format(timeLayout)
		resp.NotifiedAt = &s
	}
	if r.DeliveredAt != nil {
		s := r.DeliveredAt.UTC().Format(timeLayout)
		resp.DeliveredAt = &s
	}
	return resp
}

// ToResponses maps a slice of reminders to responses.
func ToResponses(items []Reminder) []Response {
	out := make([]Response, 0, len(items))
	for i := range items {
		out = append(out, ToResponse(&items[i]))
	}
	return out
}

const timeLayout = "2006-01-02T15:04:05Z07:00"
