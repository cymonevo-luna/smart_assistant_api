package store

import (
	"testing"
	"time"
)

type sampleEntity struct {
	ID        string    `db:"id" bson:"_id"`
	Email     string    `db:"email" bson:"email"`
	Ignored   string    `db:"-"`
	NoTag     string    // no db tag -> skipped
	CreatedAt time.Time `db:"created_at" bson:"created_at"`
}

func TestValuesOf_RespectsDBTags(t *testing.T) {
	e := &sampleEntity{ID: "1", Email: "a@b.com", Ignored: "x", NoTag: "y"}
	cols, vals := valuesOf(e)

	wantCols := []string{"id", "email", "created_at"}
	if len(cols) != len(wantCols) {
		t.Fatalf("got %d columns %v, want %d", len(cols), cols, len(wantCols))
	}
	for i, c := range wantCols {
		if cols[i] != c {
			t.Errorf("column %d = %q, want %q", i, cols[i], c)
		}
	}
	if vals[0] != "1" || vals[1] != "a@b.com" {
		t.Errorf("unexpected values: %v", vals)
	}
}

func TestBuildWhere_TranslatesOperators(t *testing.T) {
	q := NewQuery().Eq("email", "a@b.com").Gte("age", 18).In("role", []string{"admin"})
	clause, args := buildWhere(q.Conditions, 1)

	want := " WHERE email = $1 AND age >= $2 AND role = ANY($3)"
	if clause != want {
		t.Errorf("clause = %q, want %q", clause, want)
	}
	if len(args) != 3 {
		t.Errorf("got %d args, want 3", len(args))
	}
}

func TestBuildOrder(t *testing.T) {
	q := NewQuery().OrderBy("created_at", true).OrderBy("email", false)
	if got := buildOrder(q.Orders); got != " ORDER BY created_at DESC, email ASC" {
		t.Errorf("order = %q", got)
	}
}

func TestToBSONFilter_MapsIDAndOperators(t *testing.T) {
	q := NewQuery().Eq("id", "1").Gt("age", 18)
	filter := toBSONFilter(q.Conditions)

	if got := filter["_id"]; got != "1" {
		t.Errorf("expected id to map to _id with value 1, got %v", filter)
	}
	if _, ok := filter["age"]; !ok {
		t.Errorf("expected age condition, got %v", filter)
	}
}

func TestBuildSet_DeterministicOrderAndNumbering(t *testing.T) {
	clause, args := buildSet(map[string]any{"status": "busy", "agent_id": "a1"}, 3)

	// Keys are sorted, so agent_id ($3) precedes status ($4).
	want := "agent_id = $3, status = $4"
	if clause != want {
		t.Errorf("clause = %q, want %q", clause, want)
	}
	if len(args) != 2 || args[0] != "a1" || args[1] != "busy" {
		t.Errorf("unexpected args %v", args)
	}
}

func TestToBSONSet_MapsIDAndSortsKeys(t *testing.T) {
	set := toBSONSet(map[string]any{"id": "1", "status": "idle"})
	if got := set["_id"]; got != "1" {
		t.Errorf("expected id to map to _id, got %v", set)
	}
	if got := set["status"]; got != "idle" {
		t.Errorf("expected status idle, got %v", set)
	}
}

func TestStorageField(t *testing.T) {
	if got := storageField("id", true); got != "_id" {
		t.Errorf("mongo id should map to _id, got %q", got)
	}
	if got := storageField("id", false); got != "id" {
		t.Errorf("sql id should stay id, got %q", got)
	}
	if got := storageField("status", true); got != "status" {
		t.Errorf("non-id field should be unchanged, got %q", got)
	}
}
