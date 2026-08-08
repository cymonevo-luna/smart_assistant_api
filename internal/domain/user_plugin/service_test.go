package userplugin

import (
	"context"
	"sync"
	"testing"

	"github.com/cymonevo/go_template/internal/domain/plugin"
	"github.com/cymonevo/go_template/pkg/response"
	"github.com/cymonevo/go_template/pkg/store"
)

type fakeUserPluginRepo struct {
	mu    sync.Mutex
	items map[string]*UserPlugin
}

func newFakeUserPluginRepo() *fakeUserPluginRepo {
	return &fakeUserPluginRepo{items: map[string]*UserPlugin{}}
}

func (r *fakeUserPluginRepo) Create(_ context.Context, up *UserPlugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	clone := *up
	r.items[up.ID] = &clone
	return nil
}

func (r *fakeUserPluginRepo) FindByID(_ context.Context, id any) (*UserPlugin, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if up, ok := r.items[id.(string)]; ok {
		clone := *up
		return &clone, nil
	}
	return nil, store.ErrNotFound
}

func (r *fakeUserPluginRepo) FindOne(_ context.Context, q store.Query) (*UserPlugin, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, up := range r.items {
		match := true
		for _, c := range q.Conditions {
			switch c.Field {
			case "user_id":
				if up.UserID != c.Value {
					match = false
				}
			case "plugin_id":
				if up.PluginID != c.Value {
					match = false
				}
			default:
				match = false
			}
		}
		if match {
			clone := *up
			return &clone, nil
		}
	}
	return nil, store.ErrNotFound
}

func (r *fakeUserPluginRepo) Find(_ context.Context, q store.Query) ([]UserPlugin, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]UserPlugin, 0)
	for _, up := range r.items {
		match := true
		for _, c := range q.Conditions {
			if c.Field == "user_id" && up.UserID != c.Value {
				match = false
			}
		}
		if match {
			out = append(out, *up)
		}
	}
	return out, nil
}

func (r *fakeUserPluginRepo) Update(_ context.Context, id any, up *UserPlugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id.(string)]; !ok {
		return store.ErrNotFound
	}
	clone := *up
	r.items[id.(string)] = &clone
	return nil
}

func (r *fakeUserPluginRepo) Delete(_ context.Context, id any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id.(string)]; !ok {
		return store.ErrNotFound
	}
	delete(r.items, id.(string))
	return nil
}

func (r *fakeUserPluginRepo) Count(_ context.Context, _ store.Query) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return int64(len(r.items)), nil
}

func (r *fakeUserPluginRepo) FindByUserID(_ context.Context, userID string) ([]UserPlugin, error) {
	return r.Find(context.Background(), store.NewQuery().Eq("user_id", userID))
}

func (r *fakeUserPluginRepo) FindByUserIDAndPluginID(_ context.Context, userID, pluginID string) (*UserPlugin, error) {
	return r.FindOne(context.Background(), store.NewQuery().Eq("user_id", userID).Eq("plugin_id", pluginID))
}

type fakePluginRepo struct {
	mu    sync.Mutex
	items map[string]*plugin.Plugin
}

func newFakePluginRepo() *fakePluginRepo {
	return &fakePluginRepo{items: map[string]*plugin.Plugin{}}
}

func (r *fakePluginRepo) Create(_ context.Context, p *plugin.Plugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	clone := *p
	r.items[p.ID] = &clone
	return nil
}

func (r *fakePluginRepo) FindByID(_ context.Context, id any) (*plugin.Plugin, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.items[id.(string)]; ok {
		clone := *p
		return &clone, nil
	}
	return nil, store.ErrNotFound
}

func (r *fakePluginRepo) FindOne(_ context.Context, q store.Query) (*plugin.Plugin, error) {
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

func (r *fakePluginRepo) Find(_ context.Context, q store.Query) ([]plugin.Plugin, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]plugin.Plugin, 0)
	for _, c := range q.Conditions {
		if c.Operator == store.OpIn && c.Field == "id" {
			ids, ok := c.Value.([]string)
			if !ok {
				continue
			}
			for _, id := range ids {
				if p, ok := r.items[id]; ok {
					out = append(out, *p)
				}
			}
		}
	}
	return out, nil
}

func (r *fakePluginRepo) Update(_ context.Context, id any, p *plugin.Plugin) error {
	return store.ErrNotFound
}

func (r *fakePluginRepo) Delete(_ context.Context, id any) error {
	return store.ErrNotFound
}

func (r *fakePluginRepo) Count(_ context.Context, _ store.Query) (int64, error) {
	return 0, nil
}

func (r *fakePluginRepo) FindBySlug(_ context.Context, slug string) (*plugin.Plugin, error) {
	return r.FindOne(context.Background(), store.NewQuery().Eq("slug", slug))
}

func seedCatalogPlugin(repo *fakePluginRepo, id, slug string, requiredSetup bool) *plugin.Plugin {
	p := &plugin.Plugin{
		ID:   id,
		Slug: slug,
		Name: "Test Plugin",
		Manifest: plugin.PluginManifest{
			RequiredSetup: requiredSetup,
			SetupType:     plugin.SetupTypeOAuthGoogle,
		},
	}
	repo.items[id] = p
	return p
}

func TestService_InstallCreatesWithSetupStatus(t *testing.T) {
	userRepo := newFakeUserPluginRepo()
	pluginRepo := newFakePluginRepo()
	seedCatalogPlugin(pluginRepo, "plugin-1", "google-calendar-meet", true)

	svc := NewService(userRepo, pluginRepo, nil, nil)
	ctx := context.Background()

	got, err := svc.Install(ctx, "user-1", "google-calendar-meet")
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if got.SetupStatus != SetupStatusNotStarted {
		t.Fatalf("expected setup_status not_started, got %q", got.SetupStatus)
	}
	if !got.Enabled {
		t.Fatal("expected enabled true by default")
	}
	if got.Plugin.Slug != "google-calendar-meet" {
		t.Fatalf("expected slug google-calendar-meet, got %q", got.Plugin.Slug)
	}
}

func TestService_InstallNoSetupRequiredCompleted(t *testing.T) {
	userRepo := newFakeUserPluginRepo()
	pluginRepo := newFakePluginRepo()
	seedCatalogPlugin(pluginRepo, "plugin-2", "builtin-tool", false)

	svc := NewService(userRepo, pluginRepo, nil, nil)
	ctx := context.Background()

	got, err := svc.Install(ctx, "user-1", "builtin-tool")
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if got.SetupStatus != SetupStatusCompleted {
		t.Fatalf("expected setup_status completed, got %q", got.SetupStatus)
	}
}

func TestService_InstallUnknownSlugNotFound(t *testing.T) {
	svc := NewService(newFakeUserPluginRepo(), newFakePluginRepo(), nil, nil)
	ctx := context.Background()

	_, err := svc.Install(ctx, "user-1", "does-not-exist")
	if err == nil {
		t.Fatal("expected error for unknown slug")
	}
	appErr, ok := err.(*response.AppError)
	if !ok {
		t.Fatalf("expected *response.AppError, got %T", err)
	}
	if appErr.Status != 404 {
		t.Fatalf("expected status 404, got %d", appErr.Status)
	}
}

func TestService_InstallIdempotent(t *testing.T) {
	userRepo := newFakeUserPluginRepo()
	pluginRepo := newFakePluginRepo()
	seedCatalogPlugin(pluginRepo, "plugin-1", "google-calendar-meet", true)

	svc := NewService(userRepo, pluginRepo, nil, nil)
	ctx := context.Background()

	first, err := svc.Install(ctx, "user-1", "google-calendar-meet")
	if err != nil {
		t.Fatalf("first Install: %v", err)
	}
	second, err := svc.Install(ctx, "user-1", "google-calendar-meet")
	if err != nil {
		t.Fatalf("second Install: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected same install id, got %q and %q", first.ID, second.ID)
	}
	if len(userRepo.items) != 1 {
		t.Fatalf("expected one row, got %d", len(userRepo.items))
	}
}

func TestService_UninstallRemovesRow(t *testing.T) {
	userRepo := newFakeUserPluginRepo()
	pluginRepo := newFakePluginRepo()
	seedCatalogPlugin(pluginRepo, "plugin-1", "google-calendar-meet", true)

	svc := NewService(userRepo, pluginRepo, nil, nil)
	ctx := context.Background()

	installed, err := svc.Install(ctx, "user-1", "google-calendar-meet")
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if err := svc.Uninstall(ctx, "user-1", installed.ID); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if len(userRepo.items) != 0 {
		t.Fatalf("expected empty repo, got %d items", len(userRepo.items))
	}
}

func TestService_SetEnabledToggle(t *testing.T) {
	userRepo := newFakeUserPluginRepo()
	pluginRepo := newFakePluginRepo()
	seedCatalogPlugin(pluginRepo, "plugin-1", "google-calendar-meet", true)

	svc := NewService(userRepo, pluginRepo, nil, nil)
	ctx := context.Background()

	installed, err := svc.Install(ctx, "user-1", "google-calendar-meet")
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	disabled, err := svc.SetEnabled(ctx, "user-1", installed.ID, false)
	if err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if disabled.Enabled {
		t.Fatal("expected enabled false")
	}

	list, err := svc.ListInstalled(ctx, "user-1")
	if err != nil {
		t.Fatalf("ListInstalled: %v", err)
	}
	if len(list) != 1 || list[0].Enabled {
		t.Fatalf("expected one disabled plugin, got %+v", list)
	}
}

func TestService_UninstallForbiddenForOtherUser(t *testing.T) {
	userRepo := newFakeUserPluginRepo()
	pluginRepo := newFakePluginRepo()
	seedCatalogPlugin(pluginRepo, "plugin-1", "google-calendar-meet", true)

	svc := NewService(userRepo, pluginRepo, nil, nil)
	ctx := context.Background()

	installed, err := svc.Install(ctx, "user-1", "google-calendar-meet")
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	err = svc.Uninstall(ctx, "user-2", installed.ID)
	if err == nil {
		t.Fatal("expected forbidden error")
	}
	appErr, ok := err.(*response.AppError)
	if !ok || appErr.Status != 403 {
		t.Fatalf("expected 403, got %v", err)
	}
}

type trackingReminderCleaner struct {
	calls []struct {
		userID       string
		userPluginID string
	}
}

func (c *trackingReminderCleaner) CancelAllForUserPlugin(_ context.Context, userID, userPluginID string) error {
	c.calls = append(c.calls, struct {
		userID       string
		userPluginID string
	}{userID, userPluginID})
	return nil
}

func TestService_UninstallCancelsRemindersForReminderPlugin(t *testing.T) {
	userRepo := newFakeUserPluginRepo()
	pluginRepo := newFakePluginRepo()
	seedCatalogPlugin(pluginRepo, "plugin-reminder", "reminder", false)
	reminderCleaner := &trackingReminderCleaner{}

	svc := NewService(userRepo, pluginRepo, nil, reminderCleaner)
	ctx := context.Background()

	installed, err := svc.Install(ctx, "user-1", "reminder")
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if err := svc.Uninstall(ctx, "user-1", installed.ID); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if len(reminderCleaner.calls) != 1 {
		t.Fatalf("expected one reminder cancel call, got %d", len(reminderCleaner.calls))
	}
	if reminderCleaner.calls[0].userID != "user-1" || reminderCleaner.calls[0].userPluginID != installed.ID {
		t.Fatalf("unexpected cancel call: %+v", reminderCleaner.calls[0])
	}
}

func TestService_UninstallDoesNotCancelRemindersForOtherPlugins(t *testing.T) {
	userRepo := newFakeUserPluginRepo()
	pluginRepo := newFakePluginRepo()
	seedCatalogPlugin(pluginRepo, "plugin-1", "google-calendar-meet", true)
	reminderCleaner := &trackingReminderCleaner{}

	svc := NewService(userRepo, pluginRepo, nil, reminderCleaner)
	ctx := context.Background()

	installed, err := svc.Install(ctx, "user-1", "google-calendar-meet")
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if err := svc.Uninstall(ctx, "user-1", installed.ID); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if len(reminderCleaner.calls) != 0 {
		t.Fatalf("expected no reminder cancel calls, got %d", len(reminderCleaner.calls))
	}
}
