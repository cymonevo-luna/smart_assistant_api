package reminder

import (
	"context"
	"strings"
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

func ptrTime(t time.Time) *time.Time {
	utc := t.UTC()
	return &utc
}

func timeReminder(id, userID, message string, remindAt time.Time, status string) *Reminder {
	now := time.Now().UTC()
	return &Reminder{
		ID:          id,
		UserID:      userID,
		TriggerType: TriggerTypeTime,
		Message:     message,
		RemindAt:    ptrTime(remindAt),
		Status:      status,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
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

	if len(q.Orders) > 0 {
		sortReminders(out, q.Orders[0].Field, q.Orders[0].Desc)
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

func (r *fakeRepo) FindDue(ctx context.Context, before time.Time) ([]Reminder, error) {
	return r.Find(ctx, store.NewQuery().
		Eq("trigger_type", TriggerTypeTime).
		Eq("status", StatusPending).
		Lte("remind_at", before.UTC()).
		OrderBy("remind_at", false))
}

func (r *fakeRepo) FindActiveByUser(ctx context.Context, userID string, filter ListFilter) ([]Reminder, error) {
	q := store.NewQuery().
		Eq("user_id", userID).
		Eq("trigger_type", TriggerTypeTime).
		In("status", []string{StatusPending, StatusNotified}).
		OrderBy("remind_at", false)

	switch filter {
	case ListFilterToday:
		start, end := utcDayBounds(time.Now().UTC())
		q = q.Gte("remind_at", start).Lt("remind_at", end)
	case ListFilterTomorrow:
		start, end := utcDayBounds(time.Now().UTC().Add(24 * time.Hour))
		q = q.Gte("remind_at", start).Lt("remind_at", end)
	}
	return r.Find(ctx, q)
}

func (r *fakeRepo) FindPendingByUserAndMessage(ctx context.Context, userID, messageQuery string) ([]Reminder, error) {
	pattern := "%" + strings.TrimSpace(messageQuery) + "%"
	return r.Find(ctx, store.NewQuery().
		Eq("user_id", userID).
		Eq("trigger_type", TriggerTypeTime).
		Eq("status", StatusPending).
		Like("message", pattern).
		OrderBy("remind_at", false))
}

func (r *fakeRepo) FindPendingDeliveryByUser(ctx context.Context, userID string) ([]Reminder, error) {
	items, err := r.Find(ctx, store.NewQuery().
		Eq("user_id", userID).
		Eq("trigger_type", TriggerTypeTime).
		Eq("status", StatusNotified).
		OrderBy("remind_at", false))
	if err != nil {
		return nil, err
	}
	out := make([]Reminder, 0, len(items))
	for i := range items {
		if items[i].DeliveredAt == nil {
			out = append(out, items[i])
		}
	}
	return out, nil
}

func (r *fakeRepo) CancelPendingForUserPlugin(ctx context.Context, userID, userPluginID string) error {
	items, err := r.Find(ctx, store.NewQuery().
		Eq("user_id", userID).
		Eq("trigger_type", TriggerTypeTime).
		Eq("user_plugin_id", userPluginID).
		Eq("status", StatusPending))
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for i := range items {
		items[i].Status = StatusCancelled
		items[i].UpdatedAt = now
		if err := r.Update(ctx, items[i].ID, &items[i]); err != nil {
			return err
		}
	}
	return nil
}

func (r *fakeRepo) FindByUserAndStatus(ctx context.Context, userID, status string) ([]Reminder, error) {
	return r.Find(ctx, store.NewQuery().
		Eq("user_id", userID).
		Eq("trigger_type", TriggerTypeLocation).
		Eq("status", status).
		OrderBy("created_at", true))
}

func matchesQuery(rem *Reminder, q store.Query) bool {
	for _, c := range q.Conditions {
		switch c.Operator {
		case store.OpEq:
			if !eqField(rem, c.Field, c.Value) {
				return false
			}
		case store.OpIn:
			if !inField(rem, c.Field, c.Value) {
				return false
			}
		case store.OpGte:
			if !gteField(rem, c.Field, c.Value) {
				return false
			}
		case store.OpLt:
			if !ltField(rem, c.Field, c.Value) {
				return false
			}
		case store.OpLte:
			if !lteField(rem, c.Field, c.Value) {
				return false
			}
		case store.OpLike:
			if !likeField(rem, c.Field, c.Value) {
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
	case "trigger_type":
		return rem.TriggerType == value
	case "user_plugin_id":
		if rem.UserPluginID == nil {
			return value == nil
		}
		return *rem.UserPluginID == value
	default:
		return false
	}
}

func inField(rem *Reminder, field string, value any) bool {
	if field != "status" {
		return false
	}
	statuses, ok := value.([]string)
	if !ok {
		return false
	}
	for _, s := range statuses {
		if rem.Status == s {
			return true
		}
	}
	return false
}

func gteField(rem *Reminder, field string, value any) bool {
	if field != "remind_at" || rem.RemindAt == nil {
		return false
	}
	t, ok := value.(time.Time)
	return ok && !rem.RemindAt.Before(t)
}

func ltField(rem *Reminder, field string, value any) bool {
	if field != "remind_at" || rem.RemindAt == nil {
		return false
	}
	t, ok := value.(time.Time)
	return ok && rem.RemindAt.Before(t)
}

func lteField(rem *Reminder, field string, value any) bool {
	if field != "remind_at" || rem.RemindAt == nil {
		return false
	}
	t, ok := value.(time.Time)
	return ok && !rem.RemindAt.After(t)
}

func likeField(rem *Reminder, field string, value any) bool {
	if field != "message" {
		return false
	}
	pattern, ok := value.(string)
	if !ok {
		return false
	}
	pattern = strings.Trim(pattern, "%")
	return strings.Contains(strings.ToLower(rem.Message), strings.ToLower(pattern))
}

func sortReminders(items []Reminder, field string, desc bool) {
	if field != "remind_at" || desc {
		return
	}
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[i].RemindAt == nil || items[j].RemindAt == nil {
				continue
			}
			if items[j].RemindAt.Before(*items[i].RemindAt) {
				items[i], items[j] = items[j], items[i]
			}
		}
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

func TestService_CreateRejectsPastRemindAt(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, "user-1", nil, "call mom", time.Now().UTC().Add(-time.Hour))
	if err == nil {
		t.Fatal("expected validation error for past remind_at")
	}
	appErr, ok := err.(*response.AppError)
	if !ok || appErr.Status != 422 {
		t.Fatalf("expected 422 validation error, got %v", err)
	}
}

func TestService_CreatePersistsPendingReminder(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	remindAt := time.Now().UTC().Add(2 * time.Hour)

	got, err := svc.Create(ctx, "user-1", nil, "call mom", remindAt)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.Status != StatusPending {
		t.Fatalf("expected status pending, got %q", got.Status)
	}
	if got.TriggerType != TriggerTypeTime {
		t.Fatalf("expected trigger_type time, got %q", got.TriggerType)
	}
	if len(repo.items) != 1 {
		t.Fatalf("expected one reminder, got %d", len(repo.items))
	}
}

func TestService_ListFiltersTodayTomorrowAndAll(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	now := time.Now().UTC()
	todayStart, _ := utcDayBounds(now)
	tomorrowStart, _ := utcDayBounds(now.Add(24 * time.Hour))

	seed := []*Reminder{
		timeReminder("1", "user-1", "today", todayStart.Add(2*time.Hour), StatusPending),
		timeReminder("2", "user-1", "tomorrow", tomorrowStart.Add(3*time.Hour), StatusPending),
		timeReminder("3", "user-1", "cancelled", todayStart.Add(4*time.Hour), StatusCancelled),
		timeReminder("4", "user-1", "notified", todayStart.Add(5*time.Hour), StatusNotified),
	}
	for _, rem := range seed {
		repo.items[rem.ID] = rem
	}

	today, err := svc.List(ctx, "user-1", ListFilterToday)
	if err != nil {
		t.Fatalf("List today: %v", err)
	}
	if len(today) != 2 {
		t.Fatalf("expected 2 today reminders (pending+notified), got %d", len(today))
	}

	tomorrow, err := svc.List(ctx, "user-1", ListFilterTomorrow)
	if err != nil {
		t.Fatalf("List tomorrow: %v", err)
	}
	if len(tomorrow) != 1 || tomorrow[0].ID != "2" {
		t.Fatalf("expected one tomorrow reminder, got %+v", tomorrow)
	}

	all, err := svc.List(ctx, "user-1", ListFilterAll)
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 active reminders, got %d", len(all))
	}
}

func TestService_DeleteByMessageMatchNotFoundAndAmbiguous(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	remindAt := time.Now().UTC().Add(time.Hour)

	err := svc.DeleteByMessage(ctx, "user-1", "mom")
	if err == nil {
		t.Fatal("expected not found")
	}
	if appErr, ok := err.(*response.AppError); !ok || appErr.Status != 404 {
		t.Fatalf("expected 404, got %v", err)
	}

	repo.items["a"] = timeReminder("a", "user-1", "call mom", remindAt, StatusPending)
	repo.items["b"] = timeReminder("b", "user-1", "email mom", remindAt, StatusPending)
	repo.items["c"] = timeReminder("c", "user-1", "call MOM later", remindAt, StatusPending)

	err = svc.DeleteByMessage(ctx, "user-1", "mom")
	if err == nil {
		t.Fatal("expected ambiguous conflict")
	}
	if appErr, ok := err.(*response.AppError); !ok || appErr.Status != 409 {
		t.Fatalf("expected 409, got %v", err)
	}

	repo.items = map[string]*Reminder{
		"a": timeReminder("a", "user-1", "call mom", remindAt, StatusPending),
	}
	if err := svc.DeleteByMessage(ctx, "user-1", "mom"); err != nil {
		t.Fatalf("DeleteByMessage: %v", err)
	}
	if repo.items["a"].Status != StatusCancelled {
		t.Fatalf("expected reminder cancelled, got %q", repo.items["a"].Status)
	}
}

func TestService_CancelAllForUserPlugin(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	pluginID := "install-1"
	remindAt := time.Now().UTC().Add(time.Hour)

	repo.items["a"] = timeReminder("a", "user-1", "one", remindAt, StatusPending)
	repo.items["a"].UserPluginID = &pluginID
	repo.items["b"] = timeReminder("b", "user-1", "two", remindAt, StatusNotified)
	repo.items["b"].UserPluginID = &pluginID
	repo.items["c"] = timeReminder("c", "user-1", "three", remindAt, StatusPending)
	repo.items["c"].UserPluginID = &pluginID

	if err := svc.CancelAllForUserPlugin(ctx, "user-1", pluginID); err != nil {
		t.Fatalf("CancelAllForUserPlugin: %v", err)
	}
	if repo.items["a"].Status != StatusCancelled {
		t.Fatal("expected pending reminder cancelled")
	}
	if repo.items["c"].Status != StatusCancelled {
		t.Fatal("expected second pending reminder cancelled")
	}
	if repo.items["b"].Status != StatusNotified {
		t.Fatal("expected notified reminder unchanged")
	}
}

func TestService_CreateLocationRejectsEmptyTitle(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	in := locationExactInput()
	in.Title = "   "
	_, err := svc.CreateLocation(ctx, "user-1", in)
	if err == nil {
		t.Fatal("expected validation error for empty title")
	}
	appErr, ok := err.(*response.AppError)
	if !ok || appErr.Status != 422 {
		t.Fatalf("expected 422 validation error, got %v", err)
	}
}

func TestService_CreateLocationRequiresLocationMode(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	in := locationExactInput()
	in.LocationMode = nil
	_, err := svc.CreateLocation(ctx, "user-1", in)
	if err == nil {
		t.Fatal("expected validation error for missing location_mode")
	}
}

func TestService_CreateLocationPersistsPendingReminder(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()

	got, err := svc.CreateLocation(ctx, "user-1", locationExactInput())
	if err != nil {
		t.Fatalf("CreateLocation: %v", err)
	}
	if got.Status != StatusPending {
		t.Fatalf("expected status pending, got %q", got.Status)
	}
	if got.TriggerType != TriggerTypeLocation {
		t.Fatalf("expected trigger_type location, got %q", got.TriggerType)
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
		{ID: "1", UserID: "user-1", TriggerType: TriggerTypeLocation, Title: "pending", RadiusMeters: 100, Status: StatusPending, CreatedAt: now, UpdatedAt: now},
		{ID: "2", UserID: "user-1", TriggerType: TriggerTypeLocation, Title: "triggered", RadiusMeters: 100, Status: StatusTriggered, CreatedAt: now, UpdatedAt: now},
		{ID: "3", UserID: "user-1", TriggerType: TriggerTypeLocation, Title: "cancelled", RadiusMeters: 100, Status: StatusCancelled, CreatedAt: now, UpdatedAt: now},
		{ID: "4", UserID: "user-2", TriggerType: TriggerTypeLocation, Title: "other user", RadiusMeters: 100, Status: StatusPending, CreatedAt: now, UpdatedAt: now},
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

	repo.items["a"] = &Reminder{
		ID: "a", UserID: "user-1", TriggerType: TriggerTypeLocation,
		Title: "mine", RadiusMeters: 100, Status: StatusPending,
		CreatedAt: now, UpdatedAt: now,
	}

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

	repo.items["a"] = &Reminder{
		ID: "a", UserID: "user-1", TriggerType: TriggerTypeLocation,
		Title: "mine", RadiusMeters: 100, Status: StatusCancelled,
		CreatedAt: now, UpdatedAt: now,
	}

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
		ID: "a", UserID: "user-1", TriggerType: TriggerTypeLocation,
		Title: "mine", RadiusMeters: 100, Status: StatusTriggered, TriggeredAt: &triggered,
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
