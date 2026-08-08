package user

import (
	"time"

	"github.com/cymonevo/go_template/pkg/auth"
)

// User is the domain entity. The struct tags make a single definition usable by
// every store adapter:
//   - `db`   maps fields to PostgreSQL columns
//   - `bson` maps fields to MongoDB document fields
//   - `json` controls the API representation
type User struct {
	ID        string    `json:"id" db:"id" bson:"_id"`
	Email     string    `json:"email" db:"email" bson:"email"`
	Name      string    `json:"name" db:"name" bson:"name"`
	Password  string    `json:"-" db:"password" bson:"password"`
	Role      auth.Role `json:"role" db:"role" bson:"role"`
	CreatedAt time.Time `json:"created_at" db:"created_at" bson:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at" bson:"updated_at"`
}

// TableName is the SQL table / Mongo collection name for this entity.
const TableName = "users"
