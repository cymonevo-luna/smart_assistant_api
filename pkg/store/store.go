// Package store defines a database-agnostic, generic persistence abstraction.
//
// The central idea: business code depends only on the Store[T] interface and on
// the Query/Filter value types defined here. Concrete adapters (PostgresStore,
// MongoStore) translate those into driver specific operations. Swapping the
// database therefore means swapping which adapter is wired in at startup — no
// repository, service, or handler code needs to change.
package store

import (
	"context"
	"errors"
)

// ErrNotFound is returned by adapters when a record does not exist. Callers can
// rely on it via errors.Is regardless of the underlying driver.
var ErrNotFound = errors.New("store: record not found")

// Store is the generic CRUD contract for an entity of type T.
//
// T must be a plain struct whose fields carry both `db` (SQL column) and `bson`
// (Mongo field) tags so a single definition works across adapters.
type Store[T any] interface {
	Create(ctx context.Context, entity *T) error
	FindByID(ctx context.Context, id any) (*T, error)
	FindOne(ctx context.Context, q Query) (*T, error)
	Find(ctx context.Context, q Query) ([]T, error)
	Update(ctx context.Context, id any, entity *T) error
	Delete(ctx context.Context, id any) error
	Count(ctx context.Context, q Query) (int64, error)
}

// Schema carries the per-entity metadata an adapter needs. It is supplied once
// at wiring time so the rest of the code remains free of table/collection names.
type Schema struct {
	// Name is the SQL table name or the Mongo collection name.
	Name string
	// IDColumn is the SQL primary key column. Defaults to "id". Mongo always
	// uses "_id" and ignores this field.
	IDColumn string
}

func (s Schema) idColumn() string {
	if s.IDColumn == "" {
		return "id"
	}
	return s.IDColumn
}
