package store

import "testing"

// These benchmarks track the cost of the reflection-based metadata extraction
// and the query-to-SQL translation, which run on every write and read
// respectively and are therefore on the hot path.

func BenchmarkValuesOf(b *testing.B) {
	e := &sampleEntity{ID: "1", Email: "a@b.com"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = valuesOf(e)
	}
}

func BenchmarkColumnsFor(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = columnsFor[sampleEntity]()
	}
}

func BenchmarkBuildWhere(b *testing.B) {
	q := NewQuery().Eq("email", "a@b.com").Gte("age", 18).In("role", []string{"admin", "user"})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = buildWhere(q.Conditions, 1)
	}
}

func BenchmarkToBSONFilter(b *testing.B) {
	q := NewQuery().Eq("id", "1").Gte("age", 18).Like("name", "jo")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = toBSONFilter(q.Conditions)
	}
}
