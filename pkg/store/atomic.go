package store

import (
	"context"
	"sort"
)

// AtomicStore augments Store[T] with a compare-and-set style operation. It is an
// optional capability: adapters that support an atomic "find one matching the
// query and update it in a single round trip" implement it. Both the Postgres
// and Mongo adapters in this package do.
//
// The canonical use case is race-free work claiming: multiple workers race to
// claim the next available row/document, and exactly one wins because the
// matching-and-mutation happens atomically in the database.
type AtomicStore[T any] interface {
	Store[T]
	// FindOneAndUpdate atomically finds a single record matching q, applies the
	// fields in set, and returns the updated record. The keys of set are
	// storage-level field names (the `db`/`bson` tag value). It returns
	// ErrNotFound when no record matches.
	FindOneAndUpdate(ctx context.Context, q Query, set map[string]any) (*T, error)
}

// sortedKeys returns the keys of set in deterministic (sorted) order. Adapters
// rely on this so generated SQL and BSON are stable and testable.
func sortedKeys(set map[string]any) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// storageField maps the logical "id" field to the storage primary key. SQL
// adapters keep "id"; Mongo maps it to "_id" when mongo is true.
func storageField(field string, mongo bool) string {
	if field == "id" && mongo {
		return "_id"
	}
	return field
}
