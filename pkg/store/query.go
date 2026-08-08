package store

// Operator enumerates the comparison operators supported by the abstraction.
// Adapters translate these into dialect specific predicates.
type Operator string

const (
	OpEq   Operator = "eq"
	OpNe   Operator = "ne"
	OpGt   Operator = "gt"
	OpGte  Operator = "gte"
	OpLt   Operator = "lt"
	OpLte  Operator = "lte"
	OpIn   Operator = "in"
	OpLike Operator = "like"
)

// Condition is a single field predicate. Field is the storage-level field name
// (the value of the `db`/`bson` tag), which is identical across adapters by
// convention in this template.
type Condition struct {
	Field    string
	Operator Operator
	Value    any
}

// Order describes a sort directive.
type Order struct {
	Field string
	Desc  bool
}

// Query is a database-agnostic description of a read operation: filtering,
// sorting and pagination. It is intentionally a value type built via a fluent
// builder so call sites stay readable.
type Query struct {
	Conditions []Condition
	Orders     []Order
	Limit      int
	Offset     int
}

// NewQuery returns an empty query builder.
func NewQuery() Query { return Query{} }

func (q Query) where(op Operator, field string, value any) Query {
	q.Conditions = append(q.Conditions, Condition{Field: field, Operator: op, Value: value})
	return q
}

func (q Query) Eq(field string, value any) Query   { return q.where(OpEq, field, value) }
func (q Query) Ne(field string, value any) Query   { return q.where(OpNe, field, value) }
func (q Query) Gt(field string, value any) Query   { return q.where(OpGt, field, value) }
func (q Query) Gte(field string, value any) Query  { return q.where(OpGte, field, value) }
func (q Query) Lt(field string, value any) Query   { return q.where(OpLt, field, value) }
func (q Query) Lte(field string, value any) Query  { return q.where(OpLte, field, value) }
func (q Query) In(field string, value any) Query   { return q.where(OpIn, field, value) }
func (q Query) Like(field string, value any) Query { return q.where(OpLike, field, value) }

// OrderBy appends a sort directive.
func (q Query) OrderBy(field string, desc bool) Query {
	q.Orders = append(q.Orders, Order{Field: field, Desc: desc})
	return q
}

// Paginate sets limit and offset.
func (q Query) Paginate(limit, offset int) Query {
	q.Limit = limit
	q.Offset = offset
	return q
}
