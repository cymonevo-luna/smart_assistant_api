package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pgExecutor is the subset of pgx methods shared by *pgxpool.Pool and pgx.Tx.
// Routing every query through it lets a store transparently participate in a
// transaction when one is present on the context.
type pgExecutor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// PostgresStore is a generic Store[T] backed by PostgreSQL via pgx.
//
// Reads use pgx's reflection based row scanning (RowToStructByName), and writes
// build column lists from the entity's `db` tags, so a single generic
// implementation serves every entity type.
type PostgresStore[T any] struct {
	pool   *pgxpool.Pool
	schema Schema
}

// NewPostgresStore builds a PostgresStore for entities of type T.
func NewPostgresStore[T any](pool *pgxpool.Pool, schema Schema) *PostgresStore[T] {
	return &PostgresStore[T]{pool: pool, schema: schema}
}

// exec returns the active transaction from the context when present, otherwise
// the connection pool. This is what makes a single store implementation work
// both inside and outside a transaction.
func (s *PostgresStore[T]) exec(ctx context.Context) pgExecutor {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return s.pool
}

func (s *PostgresStore[T]) Create(ctx context.Context, entity *T) error {
	cols, vals := valuesOf(entity)
	placeholders := make([]string, len(cols))
	for i := range cols {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}

	sql := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		s.schema.Name,
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "),
	)
	if _, err := s.exec(ctx).Exec(ctx, sql, vals...); err != nil {
		return fmt.Errorf("postgres create: %w", err)
	}
	return nil
}

func (s *PostgresStore[T]) FindByID(ctx context.Context, id any) (*T, error) {
	sql := fmt.Sprintf("SELECT * FROM %s WHERE %s = $1 LIMIT 1", s.schema.Name, s.schema.idColumn())
	return s.queryOne(ctx, sql, id)
}

func (s *PostgresStore[T]) FindOne(ctx context.Context, q Query) (*T, error) {
	q.Limit = 1
	where, args := buildWhere(q.Conditions, 1)
	sql := fmt.Sprintf("SELECT * FROM %s%s%s LIMIT 1", s.schema.Name, where, buildOrder(q.Orders))
	return s.queryOne(ctx, sql, args...)
}

func (s *PostgresStore[T]) Find(ctx context.Context, q Query) ([]T, error) {
	where, args := buildWhere(q.Conditions, 1)
	sql := fmt.Sprintf("SELECT * FROM %s%s%s", s.schema.Name, where, buildOrder(q.Orders))
	if q.Limit > 0 {
		sql += fmt.Sprintf(" LIMIT %d", q.Limit)
	}
	if q.Offset > 0 {
		sql += fmt.Sprintf(" OFFSET %d", q.Offset)
	}

	rows, err := s.exec(ctx).Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres find: %w", err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[T])
	if err != nil {
		return nil, fmt.Errorf("postgres find scan: %w", err)
	}
	return out, nil
}

func (s *PostgresStore[T]) Update(ctx context.Context, id any, entity *T) error {
	cols, vals := valuesOf(entity)
	sets := make([]string, 0, len(cols))
	args := make([]any, 0, len(cols)+1)
	pos := 1
	for i, c := range cols {
		if c == s.schema.idColumn() {
			continue
		}
		sets = append(sets, fmt.Sprintf("%s = $%d", c, pos))
		args = append(args, vals[i])
		pos++
	}
	args = append(args, id)

	sql := fmt.Sprintf("UPDATE %s SET %s WHERE %s = $%d", s.schema.Name, strings.Join(sets, ", "), s.schema.idColumn(), pos)
	tag, err := s.exec(ctx).Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("postgres update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore[T]) Delete(ctx context.Context, id any) error {
	sql := fmt.Sprintf("DELETE FROM %s WHERE %s = $1", s.schema.Name, s.schema.idColumn())
	tag, err := s.exec(ctx).Exec(ctx, sql, id)
	if err != nil {
		return fmt.Errorf("postgres delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore[T]) Count(ctx context.Context, q Query) (int64, error) {
	where, args := buildWhere(q.Conditions, 1)
	sql := fmt.Sprintf("SELECT COUNT(*) FROM %s%s", s.schema.Name, where)
	var count int64
	if err := s.exec(ctx).QueryRow(ctx, sql, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("postgres count: %w", err)
	}
	return count, nil
}

// FindOneAndUpdate atomically claims a single row matching q and applies set,
// returning the updated row. It uses a FOR UPDATE SKIP LOCKED subquery so
// concurrent claimers never collide and never block on each other. It returns
// ErrNotFound when nothing matches. This makes PostgresStore an AtomicStore[T].
func (s *PostgresStore[T]) FindOneAndUpdate(ctx context.Context, q Query, set map[string]any) (*T, error) {
	if len(set) == 0 {
		return nil, fmt.Errorf("postgres find one and update: empty set")
	}

	setClause, args := buildSet(set, 1)
	where, whereArgs := buildWhere(q.Conditions, len(args)+1)
	args = append(args, whereArgs...)

	idCol := s.schema.idColumn()
	sub := fmt.Sprintf("SELECT %s FROM %s%s%s LIMIT 1 FOR UPDATE SKIP LOCKED",
		idCol, s.schema.Name, where, buildOrder(q.Orders))
	sql := fmt.Sprintf("UPDATE %s SET %s WHERE %s = (%s) RETURNING *",
		s.schema.Name, setClause, idCol, sub)

	return s.queryOne(ctx, sql, args...)
}

// buildSet renders a deterministic "col = $n, ..." assignment list for an UPDATE
// starting parameter numbering at startPos, returning the clause and ordered
// args.
func buildSet(set map[string]any, startPos int) (string, []any) {
	keys := sortedKeys(set)
	parts := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys))
	pos := startPos
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s = $%d", k, pos))
		args = append(args, set[k])
		pos++
	}
	return strings.Join(parts, ", "), args
}

func (s *PostgresStore[T]) queryOne(ctx context.Context, sql string, args ...any) (*T, error) {
	rows, err := s.exec(ctx).Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres query: %w", err)
	}
	entity, err := pgx.CollectOneRow(rows, pgx.RowToStructByNameLax[T])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("postgres scan: %w", err)
	}
	return &entity, nil
}

// buildWhere translates conditions into a SQL WHERE clause starting parameter
// numbering at startPos. It returns the clause (with leading space) and args.
func buildWhere(conditions []Condition, startPos int) (string, []any) {
	if len(conditions) == 0 {
		return "", nil
	}

	clauses := make([]string, 0, len(conditions))
	args := make([]any, 0, len(conditions))
	pos := startPos
	for _, c := range conditions {
		switch c.Operator {
		case OpEq:
			clauses = append(clauses, fmt.Sprintf("%s = $%d", c.Field, pos))
		case OpNe:
			clauses = append(clauses, fmt.Sprintf("%s <> $%d", c.Field, pos))
		case OpGt:
			clauses = append(clauses, fmt.Sprintf("%s > $%d", c.Field, pos))
		case OpGte:
			clauses = append(clauses, fmt.Sprintf("%s >= $%d", c.Field, pos))
		case OpLt:
			clauses = append(clauses, fmt.Sprintf("%s < $%d", c.Field, pos))
		case OpLte:
			clauses = append(clauses, fmt.Sprintf("%s <= $%d", c.Field, pos))
		case OpIn:
			clauses = append(clauses, fmt.Sprintf("%s = ANY($%d)", c.Field, pos))
		case OpLike:
			clauses = append(clauses, fmt.Sprintf("%s ILIKE $%d", c.Field, pos))
		default:
			continue
		}
		args = append(args, c.Value)
		pos++
	}

	return " WHERE " + strings.Join(clauses, " AND "), args
}

// PostgresTxManager implements TxManager using pgx transactions.
type PostgresTxManager struct {
	pool *pgxpool.Pool
}

// NewPostgresTxManager builds a TxManager bound to a connection pool.
func NewPostgresTxManager(pool *pgxpool.Pool) *PostgresTxManager {
	return &PostgresTxManager{pool: pool}
}

// Do begins a transaction, stores it on the context, and commits or rolls back
// based on the outcome of fn. Nested calls reuse the existing transaction.
func (m *PostgresTxManager) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx) // already in a transaction; join it.
	}

	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func buildOrder(orders []Order) string {
	if len(orders) == 0 {
		return ""
	}
	parts := make([]string, 0, len(orders))
	for _, o := range orders {
		dir := "ASC"
		if o.Desc {
			dir = "DESC"
		}
		parts = append(parts, fmt.Sprintf("%s %s", o.Field, dir))
	}
	return " ORDER BY " + strings.Join(parts, ", ")
}
