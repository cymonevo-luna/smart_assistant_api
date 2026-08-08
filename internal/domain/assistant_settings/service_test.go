package assistantsettings

import (
	"context"
	"sync"
	"testing"

	"github.com/cymonevo/go_template/pkg/response"
	"github.com/cymonevo/go_template/pkg/store"
)

type fakeRepo struct {
	mu    sync.Mutex
	items map[string]*Settings
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{items: map[string]*Settings{}}
}

func (r *fakeRepo) Create(_ context.Context, s *Settings) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	clone := *s
	r.items[s.UserID] = &clone
	return nil
}

func (r *fakeRepo) FindByID(_ context.Context, id any) (*Settings, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.items[id.(string)]; ok {
		clone := *s
		return &clone, nil
	}
	return nil, store.ErrNotFound
}

func (r *fakeRepo) FindOne(_ context.Context, _ store.Query) (*Settings, error) {
	return nil, store.ErrNotFound
}

func (r *fakeRepo) Find(_ context.Context, _ store.Query) ([]Settings, error) {
	return nil, nil
}

func (r *fakeRepo) Update(_ context.Context, id any, s *Settings) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id.(string)]; !ok {
		return store.ErrNotFound
	}
	clone := *s
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

func (r *fakeRepo) Count(_ context.Context, _ store.Query) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return int64(len(r.items)), nil
}

func newTestService(t *testing.T) (*Service, *fakeRepo) {
	t.Helper()
	repo := newFakeRepo()
	return NewService(repo), repo
}

func TestService_GetOrCreateDefaults(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()

	settings, err := svc.GetOrCreate(ctx, "user-1")
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	if settings.WakeWord != DefaultWakeWord {
		t.Fatalf("expected wake word %q, got %q", DefaultWakeWord, settings.WakeWord)
	}
	if settings.ActiveListeningEnabled {
		t.Fatal("expected active listening disabled by default")
	}
	if len(repo.items) != 1 {
		t.Fatalf("expected one persisted row, got %d", len(repo.items))
	}

	// Second call returns the same row without creating another.
	again, err := svc.GetOrCreate(ctx, "user-1")
	if err != nil {
		t.Fatalf("GetOrCreate again: %v", err)
	}
	if again.WakeWord != DefaultWakeWord {
		t.Fatalf("expected wake word %q, got %q", DefaultWakeWord, again.WakeWord)
	}
	if len(repo.items) != 1 {
		t.Fatalf("expected still one row, got %d", len(repo.items))
	}
}

func TestService_UpdatePersistsChanges(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	updated, err := svc.Update(ctx, "user-1", UpdateInput{
		WakeWord:               "Friday",
		ActiveListeningEnabled: true,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.WakeWord != "Friday" {
		t.Fatalf("expected wake word Friday, got %q", updated.WakeWord)
	}
	if !updated.ActiveListeningEnabled {
		t.Fatal("expected active listening enabled")
	}

	got, err := svc.GetOrCreate(ctx, "user-1")
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	if got.WakeWord != "Friday" || !got.ActiveListeningEnabled {
		t.Fatalf("unexpected persisted settings: %+v", got)
	}
}

func TestService_UpdateTrimsWakeWord(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	updated, err := svc.Update(ctx, "user-1", UpdateInput{
		WakeWord:               "  Friday  ",
		ActiveListeningEnabled: false,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.WakeWord != "Friday" {
		t.Fatalf("expected trimmed wake word Friday, got %q", updated.WakeWord)
	}
}

func TestService_UpdateRejectsEmptyWakeWord(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.Update(ctx, "user-1", UpdateInput{WakeWord: "   "})
	if err == nil {
		t.Fatal("expected validation error for empty wake word")
	}
	appErr, ok := err.(*response.AppError)
	if !ok {
		t.Fatalf("expected *response.AppError, got %T", err)
	}
	if appErr.Status != 422 {
		t.Fatalf("expected status 422, got %d", appErr.Status)
	}
	if appErr.Fields["wake_word"] == "" {
		t.Fatal("expected field error on wake_word")
	}
}

func TestService_UpdateRejectsLongWakeWord(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	longWord := string(make([]byte, 33))
	_, err := svc.Update(ctx, "user-1", UpdateInput{WakeWord: longWord})
	if err == nil {
		t.Fatal("expected validation error for long wake word")
	}
	appErr, ok := err.(*response.AppError)
	if !ok {
		t.Fatalf("expected *response.AppError, got %T", err)
	}
	if appErr.Fields["wake_word"] == "" {
		t.Fatal("expected field error on wake_word")
	}
}
