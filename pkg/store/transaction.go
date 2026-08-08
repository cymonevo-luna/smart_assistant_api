package store

import "context"

// TxManager runs a function inside a database transaction. The function
// receives a derived context that carries the active transaction; any Store
// operation performed with that context participates in the same transaction
// and is committed or rolled back atomically.
//
// Like Store, this is a database-agnostic abstraction: the Postgres and Mongo
// implementations live in their respective files but callers depend only on
// this interface.
type TxManager interface {
	// Do executes fn within a transaction. If fn returns an error the
	// transaction is rolled back, otherwise it is committed.
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

// txKey is the private context key under which adapters stash their native
// transaction handle.
type txKey struct{}

// NoopTxManager runs fn without any transaction. Useful in tests or for
// backends without transaction support.
type NoopTxManager struct{}

func (NoopTxManager) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}
