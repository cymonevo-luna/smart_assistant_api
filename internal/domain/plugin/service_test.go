package plugin

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/cymonevo/go_template/pkg/cache"
	"github.com/cymonevo/go_template/pkg/logger"
	"github.com/cymonevo/go_template/pkg/response"
	"github.com/cymonevo/go_template/pkg/store"
)

type fakeRepo struct {
	mu    sync.Mutex
	items map[string]*Plugin
}

func newFakeRepo() *fakeRepo { return &fakeRepo{items: map[string]*Plugin{}} }

func (r *fakeRepo) Create(_ context.Context, p *Plugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	clone := *p
	r.items[p.ID] = &clone
	return nil
}

func (r *fakeRepo) FindByID(_ context.Context, id any) (*Plugin, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.items[id.(string)]; ok {
		clone := *p
		return &clone, nil
	}
	return nil, store.ErrNotFound
}

func (r *fakeRepo) FindOne(_ context.Context, q store.Query) (*Plugin, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range q.Conditions {
		if c.Field == "slug" {
			for _, p := range r.items {
				if p.Slug == c.Value {
					clone := *p
					return &clone, nil
				}
			}
		}
	}
	return nil, store.ErrNotFound
}

func (r *fakeRepo) Find(_ context.Context, _ store.Query) ([]Plugin, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Plugin, 0, len(r.items))
	for _, p := range r.items {
		out = append(out, *p)
	}
	return out, nil
}

func (r *fakeRepo) Update(_ context.Context, id any, p *Plugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id.(string)]; !ok {
		return store.ErrNotFound
	}
	clone := *p
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

func (r *fakeRepo) FindBySlug(ctx context.Context, slug string) (*Plugin, error) {
	return r.FindOne(ctx, store.NewQuery().Eq("slug", slug))
}

func newTestService(t *testing.T) (*Service, *fakeRepo) {
	t.Helper()
	log, _ := logger.New("error", false)
	repo := newFakeRepo()
	c := cache.NewTyped[Plugin](cache.NewMemory(), time.Minute)
	return NewService(repo, c, store.NoopTxManager{}, log), repo
}

func TestServiceRegisterRejectsInvalidManifest(t *testing.T) {
	svc, _ := newTestService(t)
	manifest := validManifest()
	manifest.Triggers = nil

	_, err := svc.Register(context.Background(), RegisterPluginInput{
		Slug:     "bad-plugin",
		Name:     "Bad Plugin",
		Version:  "1.0.0",
		Manifest: manifest,
	})
	assertValidationError(t, err, "triggers")
}

func TestServiceRegisterAndGet(t *testing.T) {
	svc, _ := newTestService(t)
	in := RegisterPluginInput{
		Slug:        "google-calendar-meet",
		Name:        "Google Meet Scheduler",
		Description: "Schedule meetings",
		Version:     "1.0.0",
		Manifest:    validManifest(),
	}

	created, err := svc.Register(context.Background(), in)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	got, err := svc.GetBySlug(context.Background(), "google-calendar-meet")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("expected same id, got %q vs %q", got.ID, created.ID)
	}
}

func TestServiceRegisterConflict(t *testing.T) {
	svc, _ := newTestService(t)
	in := RegisterPluginInput{
		Slug:     "duplicate",
		Name:     "Plugin",
		Version:  "1.0.0",
		Manifest: validManifest(),
	}
	if _, err := svc.Register(context.Background(), in); err != nil {
		t.Fatalf("first register: %v", err)
	}

	_, err := svc.Register(context.Background(), in)
	var appErr *response.AppError
	if !errors.As(err, &appErr) || appErr.Status != http.StatusConflict {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestServiceListEmpty(t *testing.T) {
	svc, _ := newTestService(t)
	plugins, meta, err := svc.List(context.Background(), ListPluginsInput{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(plugins) != 0 {
		t.Fatalf("expected empty list, got %d", len(plugins))
	}
	if meta.Total != 0 {
		t.Fatalf("expected total 0, got %d", meta.Total)
	}
}
