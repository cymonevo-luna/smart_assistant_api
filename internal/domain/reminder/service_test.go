package reminder

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cymonevo/go_template/pkg/response"
	"github.com/cymonevo/go_template/pkg/store"
)

type fakeRepo struct {
	mu    sync.Mutex
	items map[string]*Reminder
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{items: map[string]*Reminder{}}
}

func (r *fakeRepo) Create(_ context.Context, rem *Reminder) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	clone := *rem
	r.items[rem.ID] = &clone
	return nil
}

func (r *fakeRepo) FindByID(_ context.Context, id any) (*Reminder, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if rem, ok := r.items[id.(string)]; ok {
		clone := *rem
		return &clone, nil
	}
	return nil, store.ErrNotFound
}

func (r *fakeRepo) FindOne(_ context.Context, q store.Query) (*Reminder, error) {
	items, err := r.Find(context.Background(), q)
	if err != nil || len(items) == 0 {
		return nil, store.ErrNotFound
	}
	return &items[0], nil
}

func (r *fakeRepo) Find(_ context.Context, q store.Query) ([]Reminder, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]Reminder, 0)
	for _, rem := range r.items {
		if matchesQuery(rem, q) {
			out = append(out, *rem)
		}
	}
	return out, nil
}

func (r *fakeRepo) Update(_ context.Context, id any, rem *Reminder) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id.(string)]; !ok {
		return store.ErrNotFound
	}
	clone := *rem
	r.items[id.(string)] = &clone
	return nil
}

func (r *fakeRepo) Delete(_ context.Context, id any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id.(string)]; !ok {
		return store.ErrNotFound
	}
	delete(r.items, id.(string))
	return nil
}

func (r *fakeRepo) Count(_ context.Context, q store.Query) (int64, error) {
	items, err := r.Find(context.Background(), q)
	return int64(len(items)), err
}

func (r *fakeRepo) FindByUserAndStatus(ctx context.Context, userID, status string) ([]Reminder, error) {
	return r.Find(ctx, store.NewQuery().
		Eq("user_id", userID).
		Eq("status", status))
}

func matchesQuery(rem *Reminder, q store.Query) bool {
	for _, c := range q.Conditions {
		switch c.Operator {
		case store.OpEq:
			if !eqField(rem, c.Field, c.Value) {
				return false
			}
		}
	}
	return true
}

func eqField(rem *Reminder, field string, value any) bool {
	switch field {
	case "user_id":
		return rem.UserID == value
	case "status":
		return rem.Status == value
	default:
		return false
	}
}

func newTestService(t *testing.T) (*Service, *fakeRepo) {
	t.Helper()
	repo := newFakeRepo()
	return NewService(repo), repo
}

func locationExactInput() CreateInput {
	mode := LocationModeExact
	query := "Cempaka Putih Tengah 20"
	lat := -6.1751
	lng := 106.8650
	return CreateInput{
		Title:        "pick my printer",
		TriggerType:  TriggerTypeLocation,
		LocationMode: &mode,
		PlaceQuery:   &query,
		Latitude:     &lat,
		Longitude:    &lng,
		RadiusMeters: 100,
	}
}

func TestService_CreateRejectsEmptyTitle(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	in := locationExactInput()
	in.Title = "   "
	_, err := svc.Create(ctx, "user-1", in)
	if err == nil {
		t.Fatal("expected validation error for empty title")
	}
	appErr, ok := err.(*response.AppError)
	if !ok || appErr.Status != 422 {
		t.Fatalf("expected 422 validation error, got %v", err)
	}
}

func TestService_CreateRequiresLocationModeForLocationTrigger(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	in := locationExactInput()
	in.LocationMode = nil
	_, err := svc.Create(ctx, "user-1", in)
	if err == nil {
		t.Fatal("expected validation error for missing location_mode")
	}
}

func TestService_CreatePersistsPendingReminder(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()

	got, err := svc.Create(ctx, "user-1", locationExactInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.Status != StatusPending {
		t.Fatalf("expected status pending, got %q", got.Status)
	}
	if len(repo.items) != 1 {
		t.Fatalf("expected one reminder, got %d", len(repo.items))
	}
}

func TestService_ListByUserFiltersStatus(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seed := []Reminder{
		{ID: "1", UserID: "user-1", Title: "pending", TriggerType: TriggerTypeLocation, RadiusMeters: 100, Status: StatusPending, CreatedAt: now, UpdatedAt: now},
		{ID: "2", UserID: "user-1", Title: "triggered", TriggerType: TriggerTypeLocation, RadiusMeters: 100, Status: StatusTriggered, CreatedAt: now, UpdatedAt: now},
		{ID: "3", UserID: "user-1", Title: "cancelled", TriggerType: TriggerTypeLocation, RadiusMeters: 100, Status: StatusCancelled, CreatedAt: now, UpdatedAt: now},
		{ID: "4", UserID: "user-2", Title: "other user", TriggerType: TriggerTypeLocation, RadiusMeters: 100, Status: StatusPending, CreatedAt: now, UpdatedAt: now},
	}
	for i := range seed {
		repo.items[seed[i].ID] = &seed[i]
	}

	pending, err := svc.ListByUser(ctx, "user-1", StatusPending)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != "1" {
		t.Fatalf("expected one pending reminder, got %+v", pending)
	}
}

func TestService_GetByIDRejectsCrossUser(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	repo.items["a"] = &Reminder{ID: "a", UserID: "user-1", Title: "mine", TriggerType: TriggerTypeLocation, RadiusMeters: 100, Status: StatusPending, CreatedAt: now, UpdatedAt: now}

	_, err := svc.GetByID(ctx, "user-2", "a")
	if err == nil {
		t.Fatal("expected not found for cross-user access")
	}
	appErr, ok := err.(*response.AppError)
	if !ok || appErr.Status != 404 {
		t.Fatalf("expected 404, got %v", err)
	}
}

func TestService_CancelIsIdempotent(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	repo.items["a"] = &Reminder{ID: "a", UserID: "user-1", Title: "mine", TriggerType: TriggerTypeLocation, RadiusMeters: 100, Status: StatusCancelled, CreatedAt: now, UpdatedAt: now}

	got, err := svc.Cancel(ctx, "user-1", "a")
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if got.Status != StatusCancelled {
		t.Fatalf("expected cancelled status, got %q", got.Status)
	}
}

func TestService_MarkTriggeredIsIdempotent(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	now := time.Now().UTC()
	triggered := now.Add(-time.Hour)

	repo.items["a"] = &Reminder{
		ID: "a", UserID: "user-1", Title: "mine", TriggerType: TriggerTypeLocation,
		RadiusMeters: 100, Status: StatusTriggered, TriggeredAt: &triggered,
		CreatedAt: now, UpdatedAt: now,
	}

	got, err := svc.MarkTriggered(ctx, "user-1", "a")
	if err != nil {
		t.Fatalf("MarkTriggered: %v", err)
	}
	if got.Status != StatusTriggered {
		t.Fatalf("expected triggered status, got %q", got.Status)
	}
	if got.TriggeredAt == nil || !got.TriggeredAt.Equal(triggered) {
		t.Fatal("expected triggered_at unchanged on idempotent call")
	}
}

func TestService_CancelAllForUserPluginIsNoOp(t *testing.T) {
	svc, _ := newTestService(t)
	if err := svc.CancelAllForUserPlugin(context.Background(), "user-1", "plugin-1"); err != nil {
		t.Fatalf("CancelAllForUserPlugin: %v", err)
	}
}
