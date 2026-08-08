package store

import (
	"reflect"
	"strings"
	"sync"
)

// fieldMeta describes a single struct field's mapping to a SQL column.
type fieldMeta struct {
	column string
	index  int
}

// structMeta caches the reflected column layout for an entity type so we only
// pay the reflection cost once per type.
type structMeta struct {
	fields []fieldMeta
}

var metaCache sync.Map // reflect.Type -> *structMeta

// columnsFor extracts the SQL column mapping for type T using `db` struct tags.
// Fields tagged `db:"-"` or without a `db` tag are ignored.
func columnsFor[T any]() *structMeta {
	var zero T
	t := reflect.TypeOf(zero)
	if cached, ok := metaCache.Load(t); ok {
		return cached.(*structMeta)
	}

	meta := &structMeta{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("db")
		if tag == "" || tag == "-" {
			continue
		}
		name := strings.SplitN(tag, ",", 2)[0]
		meta.fields = append(meta.fields, fieldMeta{column: name, index: i})
	}

	metaCache.Store(t, meta)
	return meta
}

// valuesOf returns column names and the corresponding values pulled from entity
// via reflection, honouring the same `db` tag mapping as columnsFor.
func valuesOf[T any](entity *T) ([]string, []any) {
	meta := columnsFor[T]()
	v := reflect.ValueOf(entity).Elem()

	cols := make([]string, 0, len(meta.fields))
	vals := make([]any, 0, len(meta.fields))
	for _, fm := range meta.fields {
		cols = append(cols, fm.column)
		vals = append(vals, v.Field(fm.index).Interface())
	}
	return cols, vals
}
