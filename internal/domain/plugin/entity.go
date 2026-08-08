package plugin

import "time"

// Plugin is the domain entity for a catalog entry.
type Plugin struct {
	ID          string         `json:"id" db:"id" bson:"_id"`
	Slug        string         `json:"slug" db:"slug" bson:"slug"`
	Name        string         `json:"name" db:"name" bson:"name"`
	Description string         `json:"description" db:"description" bson:"description"`
	Version     string         `json:"version" db:"version" bson:"version"`
	Manifest    PluginManifest `json:"manifest" db:"manifest" bson:"manifest"`
	CreatedAt   time.Time      `json:"created_at" db:"created_at" bson:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at" db:"updated_at" bson:"updated_at"`
}

// TableName is the SQL table / Mongo collection name for this entity.
const TableName = "plugins"
