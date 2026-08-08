package reminder

import "time"

const (
	// TableName is the SQL table / Mongo collection name for reminders.
	TableName = "reminders"

	StatusPending   = "pending"
	StatusNotified  = "notified"
	StatusCancelled = "cancelled"
)

// Reminder is a user-scheduled notification. All date boundaries and comparisons
// use UTC; there is no per-user timezone setting yet.
type Reminder struct {
	ID           string     `json:"id" db:"id" bson:"_id"`
	UserID       string     `json:"user_id" db:"user_id" bson:"user_id"`
	UserPluginID *string    `json:"user_plugin_id,omitempty" db:"user_plugin_id" bson:"user_plugin_id,omitempty"`
	Message      string     `json:"message" db:"message" bson:"message"`
	RemindAt     time.Time  `json:"remind_at" db:"remind_at" bson:"remind_at"`
	Status       string     `json:"status" db:"status" bson:"status"`
	NotifiedAt   *time.Time `json:"notified_at,omitempty" db:"notified_at" bson:"notified_at,omitempty"`
	DeliveredAt  *time.Time `json:"delivered_at,omitempty" db:"delivered_at" bson:"delivered_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at" bson:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at" bson:"updated_at"`
}
