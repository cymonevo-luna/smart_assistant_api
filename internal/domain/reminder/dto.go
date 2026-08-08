package reminder

// ListFilter scopes List results by remind_at date in UTC.
type ListFilter string

const (
	ListFilterToday    ListFilter = "today"
	ListFilterTomorrow ListFilter = "tomorrow"
	ListFilterAll      ListFilter = "all"
)

// CreateInput is the payload for creating a location reminder via the service layer.
type CreateInput struct {
	Title        string  `validate:"required"`
	TriggerType  string  `validate:"required,oneof=time location"`
	LocationMode *string `validate:"omitempty,oneof=exact place_keyword"`
	PlaceQuery   *string
	Latitude     *float64
	Longitude    *float64
	PlaceKeyword *string
	RadiusMeters int `validate:"required,min=1"`
}

// Response is the outward-facing reminder shape.
type Response struct {
	ID           string   `json:"id"`
	UserID       string   `json:"user_id,omitempty"`
	UserPluginID *string  `json:"user_plugin_id,omitempty"`
	TriggerType  string   `json:"trigger_type"`
	Title        string   `json:"title,omitempty"`
	Message      string   `json:"message,omitempty"`
	RemindAt     *string  `json:"remind_at,omitempty"`
	LocationMode *string  `json:"location_mode"`
	PlaceQuery   *string  `json:"place_query"`
	Latitude     *float64 `json:"latitude"`
	Longitude    *float64 `json:"longitude"`
	PlaceKeyword *string  `json:"place_keyword"`
	RadiusMeters int      `json:"radius_meters"`
	Status       string   `json:"status"`
	NotifiedAt   *string  `json:"notified_at,omitempty"`
	DeliveredAt  *string  `json:"delivered_at,omitempty"`
	TriggeredAt  *string  `json:"triggered_at,omitempty"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`
}

const timeLayout = "2006-01-02T15:04:05Z07:00"

// ToResponse maps a Reminder to its API representation.
func ToResponse(r *Reminder) Response {
	resp := Response{
		ID:           r.ID,
		UserID:       r.UserID,
		UserPluginID: r.UserPluginID,
		TriggerType:  r.TriggerType,
		Title:        r.Title,
		Message:      r.Message,
		LocationMode: r.LocationMode,
		PlaceQuery:   r.PlaceQuery,
		Latitude:     r.Latitude,
		Longitude:    r.Longitude,
		PlaceKeyword: r.PlaceKeyword,
		RadiusMeters: r.RadiusMeters,
		Status:       r.Status,
		CreatedAt:    r.CreatedAt.UTC().Format(timeLayout),
		UpdatedAt:    r.UpdatedAt.UTC().Format(timeLayout),
	}
	if r.RemindAt != nil {
		s := r.RemindAt.UTC().Format(timeLayout)
		resp.RemindAt = &s
	}
	if r.NotifiedAt != nil {
		s := r.NotifiedAt.UTC().Format(timeLayout)
		resp.NotifiedAt = &s
	}
	if r.DeliveredAt != nil {
		s := r.DeliveredAt.UTC().Format(timeLayout)
		resp.DeliveredAt = &s
	}
	if r.TriggeredAt != nil {
		s := r.TriggeredAt.UTC().Format(timeLayout)
		resp.TriggeredAt = &s
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
