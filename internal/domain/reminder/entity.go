package reminder

import "time"

const (
	// TableName is the SQL table / Mongo collection name for reminders.
	TableName = "reminders"

	TriggerTypeTime     = "time"
	TriggerTypeLocation = "location"

	LocationModeExact        = "exact"
	LocationModePlaceKeyword = "place_keyword"

	StatusPending   = "pending"
	StatusNotified  = "notified"
	StatusTriggered = "triggered"
	StatusCancelled = "cancelled"
)

// Reminder is a user-scheduled notification triggered by time or location.
// All date boundaries and comparisons use UTC; there is no per-user timezone setting yet.
type Reminder struct {
	ID           string     `json:"id" db:"id" bson:"_id"`
	UserID       string     `json:"user_id" db:"user_id" bson:"user_id"`
	TriggerType  string     `json:"trigger_type" db:"trigger_type" bson:"trigger_type"`
	UserPluginID *string    `json:"user_plugin_id,omitempty" db:"user_plugin_id" bson:"user_plugin_id,omitempty"`
	Message      string     `json:"message,omitempty" db:"message" bson:"message,omitempty"`
	RemindAt     *time.Time `json:"remind_at,omitempty" db:"remind_at" bson:"remind_at,omitempty"`
	Title        string     `json:"title,omitempty" db:"title" bson:"title,omitempty"`
	LocationMode *string    `json:"location_mode,omitempty" db:"location_mode" bson:"location_mode,omitempty"`
	PlaceQuery   *string    `json:"place_query,omitempty" db:"place_query" bson:"place_query,omitempty"`
	Latitude     *float64   `json:"latitude,omitempty" db:"latitude" bson:"latitude,omitempty"`
	Longitude    *float64   `json:"longitude,omitempty" db:"longitude" bson:"longitude,omitempty"`
	PlaceKeyword *string    `json:"place_keyword,omitempty" db:"place_keyword" bson:"place_keyword,omitempty"`
	RadiusMeters int        `json:"radius_meters" db:"radius_meters" bson:"radius_meters"`
	Status       string     `json:"status" db:"status" bson:"status"`
	NotifiedAt   *time.Time `json:"notified_at,omitempty" db:"notified_at" bson:"notified_at,omitempty"`
	DeliveredAt  *time.Time `json:"delivered_at,omitempty" db:"delivered_at" bson:"delivered_at,omitempty"`
	TriggeredAt  *time.Time `json:"triggered_at,omitempty" db:"triggered_at" bson:"triggered_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at" bson:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at" bson:"updated_at"`
}
