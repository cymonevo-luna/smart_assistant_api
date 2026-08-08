package store

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
)

// MemoryStore is an in-memory AtomicStore[T] intended for tests and local
// development. It uses the entity's `bson` tags to resolve query field names, so
// the same Query values used against the real adapters work unchanged.
//
// It is deliberately simple: it supports the operators and ordering the domain
// layer relies on (equality, IN, ordering, pagination, atomic find-and-update)
// and is not optimised for large datasets.
type MemoryStore[T any] struct {
	mu    sync.RWMutex
	items map[string]*T
}

// NewMemoryStore builds an empty in-memory store.
func NewMemoryStore[T any]() *MemoryStore[T] {
	return &MemoryStore[T]{items: map[string]*T{}}
}

func (s *MemoryStore[T]) Create(_ context.Context, entity *T) error {
	id := storageValue(entity, "_id")
	if id == "" {
		return fmt.Errorf("memory store: entity has empty id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	clone := *entity
	s.items[id] = &clone
	return nil
}

func (s *MemoryStore[T]) FindByID(_ context.Context, id any) (*T, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if e, ok := s.items[fmt.Sprintf("%v", id)]; ok {
		clone := *e
		return &clone, nil
	}
	return nil, ErrNotFound
}

func (s *MemoryStore[T]) FindOne(_ context.Context, q Query) (*T, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	matches := s.match(q.Conditions)
	sortEntities(matches, q.Orders)
	if len(matches) == 0 {
		return nil, ErrNotFound
	}
	clone := *matches[0]
	return &clone, nil
}

func (s *MemoryStore[T]) Find(_ context.Context, q Query) ([]T, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	matches := s.match(q.Conditions)
	sortEntities(matches, q.Orders)
	matches = paginate(matches, q.Limit, q.Offset)
	out := make([]T, 0, len(matches))
	for _, m := range matches {
		out = append(out, *m)
	}
	return out, nil
}

func (s *MemoryStore[T]) Update(_ context.Context, id any, entity *T) error {
	key := fmt.Sprintf("%v", id)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[key]; !ok {
		return ErrNotFound
	}
	clone := *entity
	s.items[key] = &clone
	return nil
}

func (s *MemoryStore[T]) Delete(_ context.Context, id any) error {
	key := fmt.Sprintf("%v", id)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[key]; !ok {
		return ErrNotFound
	}
	delete(s.items, key)
	return nil
}

func (s *MemoryStore[T]) Count(_ context.Context, q Query) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return int64(len(s.match(q.Conditions))), nil
}

// FindOneAndUpdate atomically finds the first matching entity (honouring order)
// and applies set, returning the updated copy. This provides race-free claiming
// in tests, mirroring the database adapters.
func (s *MemoryStore[T]) FindOneAndUpdate(_ context.Context, q Query, set map[string]any) (*T, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	matches := s.match(q.Conditions)
	sortEntities(matches, q.Orders)
	if len(matches) == 0 {
		return nil, ErrNotFound
	}
	target := matches[0]
	for _, k := range sortedKeys(set) {
		applyField(target, storageField(k, true), set[k])
	}
	clone := *target
	return &clone, nil
}

func (s *MemoryStore[T]) match(conditions []Condition) []*T {
	out := make([]*T, 0, len(s.items))
	for _, e := range s.items {
		if entityMatches(e, conditions) {
			out = append(out, e)
		}
	}
	return out
}

// --- reflection helpers ---

func fieldByStorage(v reflect.Value, storage string) (reflect.Value, bool) {
	v = reflect.Indirect(v)
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		tag := strings.SplitN(t.Field(i).Tag.Get("bson"), ",", 2)[0]
		if tag == "" || tag == "-" {
			continue
		}
		if tag == storage {
			return v.Field(i), true
		}
	}
	return reflect.Value{}, false
}

func storageValue[T any](entity *T, storage string) string {
	f, ok := fieldByStorage(reflect.ValueOf(entity), storage)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%v", f.Interface())
}

func entityMatches[T any](entity *T, conditions []Condition) bool {
	v := reflect.ValueOf(entity)
	for _, c := range conditions {
		storage := storageField(c.Field, true)
		f, ok := fieldByStorage(v, storage)
		if !ok {
			return false
		}
		if !conditionMatches(f, c) {
			return false
		}
	}
	return true
}

func conditionMatches(f reflect.Value, c Condition) bool {
	switch c.Operator {
	case OpEq:
		return valuesEqual(f, c.Value)
	case OpNe:
		return !valuesEqual(f, c.Value)
	case OpIn:
		rv := reflect.ValueOf(c.Value)
		if rv.Kind() != reflect.Slice {
			return false
		}
		for i := 0; i < rv.Len(); i++ {
			if scalarEqual(f.Interface(), rv.Index(i).Interface()) {
				return true
			}
		}
		return false
	case OpLike:
		return strings.Contains(
			strings.ToLower(fmt.Sprintf("%v", f.Interface())),
			strings.ToLower(strings.Trim(fmt.Sprintf("%v", c.Value), "%")),
		)
	default:
		return false
	}
}

func valuesEqual(f reflect.Value, want any) bool {
	return scalarEqual(f.Interface(), want)
}

func scalarEqual(a, b any) bool {
	av, bv := reflect.ValueOf(a), reflect.ValueOf(b)
	if av.Kind() == reflect.String || bv.Kind() == reflect.String {
		return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
	}
	if av.Kind() == reflect.Bool && bv.Kind() == reflect.Bool {
		return av.Bool() == bv.Bool()
	}
	return reflect.DeepEqual(a, b)
}

func applyField[T any](entity *T, storage string, val any) {
	f, ok := fieldByStorage(reflect.ValueOf(entity), storage)
	if !ok || !f.CanSet() {
		return
	}
	rv := reflect.ValueOf(val)
	switch {
	case rv.Type().AssignableTo(f.Type()):
		f.Set(rv)
	case rv.Type().ConvertibleTo(f.Type()):
		f.Set(rv.Convert(f.Type()))
	}
}

func sortEntities[T any](items []*T, orders []Order) {
	if len(orders) == 0 {
		return
	}
	sort.SliceStable(items, func(i, j int) bool {
		for _, o := range orders {
			storage := storageField(o.Field, true)
			fi, oki := fieldByStorage(reflect.ValueOf(items[i]), storage)
			fj, okj := fieldByStorage(reflect.ValueOf(items[j]), storage)
			if !oki || !okj {
				continue
			}
			c := compareValues(fi, fj)
			if c == 0 {
				continue
			}
			if o.Desc {
				return c > 0
			}
			return c < 0
		}
		return false
	})
}

func compareValues(a, b reflect.Value) int {
	if ta, ok := a.Interface().(time.Time); ok {
		if tb, ok := b.Interface().(time.Time); ok {
			switch {
			case ta.Before(tb):
				return -1
			case ta.After(tb):
				return 1
			default:
				return 0
			}
		}
	}
	sa, sb := fmt.Sprintf("%v", a.Interface()), fmt.Sprintf("%v", b.Interface())
	return strings.Compare(sa, sb)
}

func paginate[T any](items []*T, limit, offset int) []*T {
	if offset > 0 {
		if offset >= len(items) {
			return nil
		}
		items = items[offset:]
	}
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}
	return items
}
