package store

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoStore is a generic Store[T] backed by MongoDB.
//
// It relies on the driver's native BSON (de)serialisation, so entities map to
// documents through their `bson` struct tags without any per-type code.
type MongoStore[T any] struct {
	coll *mongo.Collection
}

// NewMongoStore builds a MongoStore for entities of type T. The Schema's Name is
// used as the collection name.
func NewMongoStore[T any](db *mongo.Database, schema Schema) *MongoStore[T] {
	return &MongoStore[T]{coll: db.Collection(schema.Name)}
}

// MongoTxManager implements TxManager using MongoDB multi-document transactions.
// It requires the server to be a replica set or sharded cluster.
//
// MongoDB threads the active session through the context, and the mongo driver
// automatically enlists operations whose context carries a session. Because
// MongoStore already passes the caller's context to every operation, no store
// code is transaction-aware.
type MongoTxManager struct {
	client *mongo.Client
}

// NewMongoTxManager builds a TxManager bound to a Mongo client.
func NewMongoTxManager(client *mongo.Client) *MongoTxManager {
	return &MongoTxManager{client: client}
}

// Do runs fn inside a transaction, retrying on transient errors per the driver.
func (m *MongoTxManager) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	return m.client.UseSession(ctx, func(sc mongo.SessionContext) error {
		_, err := sc.WithTransaction(sc, func(txCtx mongo.SessionContext) (interface{}, error) {
			return nil, fn(txCtx)
		})
		return err
	})
}

func (s *MongoStore[T]) Create(ctx context.Context, entity *T) error {
	if _, err := s.coll.InsertOne(ctx, entity); err != nil {
		return fmt.Errorf("mongo create: %w", err)
	}
	return nil
}

func (s *MongoStore[T]) FindByID(ctx context.Context, id any) (*T, error) {
	return s.findOne(ctx, bson.M{"_id": id}, nil)
}

func (s *MongoStore[T]) FindOne(ctx context.Context, q Query) (*T, error) {
	return s.findOne(ctx, toBSONFilter(q.Conditions), toFindOneOptions(q))
}

func (s *MongoStore[T]) Find(ctx context.Context, q Query) ([]T, error) {
	cur, err := s.coll.Find(ctx, toBSONFilter(q.Conditions), toFindOptions(q))
	if err != nil {
		return nil, fmt.Errorf("mongo find: %w", err)
	}
	out := make([]T, 0)
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("mongo find decode: %w", err)
	}
	return out, nil
}

func (s *MongoStore[T]) Update(ctx context.Context, id any, entity *T) error {
	res, err := s.coll.ReplaceOne(ctx, bson.M{"_id": id}, entity)
	if err != nil {
		return fmt.Errorf("mongo update: %w", err)
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *MongoStore[T]) Delete(ctx context.Context, id any) error {
	res, err := s.coll.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return fmt.Errorf("mongo delete: %w", err)
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *MongoStore[T]) Count(ctx context.Context, q Query) (int64, error) {
	count, err := s.coll.CountDocuments(ctx, toBSONFilter(q.Conditions))
	if err != nil {
		return 0, fmt.Errorf("mongo count: %w", err)
	}
	return count, nil
}

// FindOneAndUpdate atomically matches a single document, applies set via $set,
// and returns the document after modification. It honours q.Orders so callers
// can claim, for example, the oldest matching document. It returns ErrNotFound
// when nothing matches. This makes MongoStore an AtomicStore[T].
func (s *MongoStore[T]) FindOneAndUpdate(ctx context.Context, q Query, set map[string]any) (*T, error) {
	update := bson.M{"$set": toBSONSet(set)}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	if sort := toSort(q.Orders); sort != nil {
		opts.SetSort(sort)
	}

	var entity T
	res := s.coll.FindOneAndUpdate(ctx, toBSONFilter(q.Conditions), update, opts)
	if err := res.Decode(&entity); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("mongo find one and update: %w", err)
	}
	return &entity, nil
}

// toBSONSet converts a storage-level set map into a BSON document, mapping the
// logical "id" key to Mongo's "_id".
func toBSONSet(set map[string]any) bson.M {
	out := bson.M{}
	for _, k := range sortedKeys(set) {
		out[storageField(k, true)] = set[k]
	}
	return out
}

func (s *MongoStore[T]) findOne(ctx context.Context, filter bson.M, opts *options.FindOneOptions) (*T, error) {
	var entity T
	res := s.coll.FindOne(ctx, filter, optsSlice(opts)...)
	if err := res.Decode(&entity); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("mongo find one: %w", err)
	}
	return &entity, nil
}

func optsSlice(o *options.FindOneOptions) []*options.FindOneOptions {
	if o == nil {
		return nil
	}
	return []*options.FindOneOptions{o}
}

// toBSONFilter converts abstract conditions into a MongoDB filter document.
func toBSONFilter(conditions []Condition) bson.M {
	filter := bson.M{}
	for _, c := range conditions {
		field := c.Field
		if field == "id" {
			field = "_id"
		}
		switch c.Operator {
		case OpEq:
			filter[field] = c.Value
		case OpNe:
			filter[field] = bson.M{"$ne": c.Value}
		case OpGt:
			filter[field] = bson.M{"$gt": c.Value}
		case OpGte:
			filter[field] = bson.M{"$gte": c.Value}
		case OpLt:
			filter[field] = bson.M{"$lt": c.Value}
		case OpLte:
			filter[field] = bson.M{"$lte": c.Value}
		case OpIn:
			filter[field] = bson.M{"$in": c.Value}
		case OpLike:
			filter[field] = bson.M{"$regex": c.Value, "$options": "i"}
		}
	}
	return filter
}

func toFindOptions(q Query) *options.FindOptions {
	opts := options.Find()
	if sort := toSort(q.Orders); sort != nil {
		opts.SetSort(sort)
	}
	if q.Limit > 0 {
		opts.SetLimit(int64(q.Limit))
	}
	if q.Offset > 0 {
		opts.SetSkip(int64(q.Offset))
	}
	return opts
}

func toFindOneOptions(q Query) *options.FindOneOptions {
	if len(q.Orders) == 0 {
		return nil
	}
	return options.FindOne().SetSort(toSort(q.Orders))
}

func toSort(orders []Order) bson.D {
	if len(orders) == 0 {
		return nil
	}
	sort := make(bson.D, 0, len(orders))
	for _, o := range orders {
		dir := 1
		if o.Desc {
			dir = -1
		}
		field := o.Field
		if field == "id" {
			field = "_id"
		}
		sort = append(sort, bson.E{Key: field, Value: dir})
	}
	return sort
}
