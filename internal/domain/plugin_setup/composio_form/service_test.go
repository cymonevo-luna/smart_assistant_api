package composioform

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cymonevo/go_template/internal/domain/plugin"
	"github.com/cymonevo/go_template/internal/domain/plugin_credential"
	"github.com/cymonevo/go_template/internal/domain/user_plugin"
	"github.com/cymonevo/go_template/pkg/composio"
	"github.com/cymonevo/go_template/pkg/crypto"
	"github.com/cymonevo/go_template/pkg/response"
	"github.com/cymonevo/go_template/pkg/store"
)

type stubComposioClient struct {
	validateErr error
	accounts    []composio.ConnectedAccount
	listErr     error
}

func (s *stubComposioClient) ValidateAPIKey(context.Context) error {
	return s.validateErr
}

func (s *stubComposioClient) ListConnectedAccounts(context.Context, composio.ListConnectedAccountsOpts) ([]composio.ConnectedAccount, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.accounts, nil
}

type memCredentialRepo struct {
	mu    sync.Mutex
	items map[string]*plugincredential.Credential
}

func newMemCredentialRepo() *memCredentialRepo {
	return &memCredentialRepo{items: map[string]*plugincredential.Credential{}}
}

func (r *memCredentialRepo) Create(_ context.Context, c *plugincredential.Credential) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	clone := *c
	r.items[c.ID] = &clone
	return nil
}

func (r *memCredentialRepo) FindByID(_ context.Context, id any) (*plugincredential.Credential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.items[id.(string)]; ok {
		clone := *c
		return &clone, nil
	}
	return nil, store.ErrNotFound
}

func (r *memCredentialRepo) FindOne(_ context.Context, q store.Query) (*plugincredential.Credential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.items {
		match := true
		for _, cond := range q.Conditions {
			if cond.Field == "user_plugin_id" && c.UserPluginID != cond.Value {
				match = false
				break
			}
		}
		if match {
			clone := *c
			return &clone, nil
		}
	}
	return nil, store.ErrNotFound
}

func (r *memCredentialRepo) Find(_ context.Context, _ store.Query) ([]plugincredential.Credential, error) {
	return nil, nil
}

func (r *memCredentialRepo) Update(_ context.Context, id any, c *plugincredential.Credential) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id.(string)]; !ok {
		return store.ErrNotFound
	}
	clone := *c
	r.items[id.(string)] = &clone
	return nil
}

func (r *memCredentialRepo) Delete(_ context.Context, id any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.items, id.(string))
	return nil
}

func (r *memCredentialRepo) Count(_ context.Context, _ store.Query) (int64, error) {
	return 0, nil
}

func (r *memCredentialRepo) FindByUserPluginID(_ context.Context, userPluginID string) (*plugincredential.Credential, error) {
	return r.FindOne(context.Background(), store.NewQuery().Eq("user_plugin_id", userPluginID))
}

func (r *memCredentialRepo) DeleteByUserPluginID(_ context.Context, userPluginID string) error {
	c, err := r.FindByUserPluginID(context.Background(), userPluginID)
	if err != nil {
		if err == store.ErrNotFound {
			return nil
		}
		return err
	}
	return r.Delete(context.Background(), c.ID)
}

type memUserPluginRepo struct {
	mu    sync.Mutex
	items map[string]*userplugin.UserPlugin
}

func newMemUserPluginRepo() *memUserPluginRepo {
	return &memUserPluginRepo{items: map[string]*userplugin.UserPlugin{}}
}

func (r *memUserPluginRepo) Create(_ context.Context, up *userplugin.UserPlugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	clone := *up
	r.items[up.ID] = &clone
	return nil
}

func (r *memUserPluginRepo) FindByID(_ context.Context, id any) (*userplugin.UserPlugin, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if up, ok := r.items[id.(string)]; ok {
		clone := *up
		return &clone, nil
	}
	return nil, store.ErrNotFound
}

func (r *memUserPluginRepo) FindOne(_ context.Context, q store.Query) (*userplugin.UserPlugin, error) {
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
			}
		}
		if match {
			clone := *up
			return &clone, nil
		}
	}
	return nil, store.ErrNotFound
}

func (r *memUserPluginRepo) Find(_ context.Context, q store.Query) ([]userplugin.UserPlugin, error) {
	up, err := r.FindOne(context.Background(), q)
	if err != nil {
		return nil, err
	}
	return []userplugin.UserPlugin{*up}, nil
}

func (r *memUserPluginRepo) Update(_ context.Context, id any, up *userplugin.UserPlugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id.(string)]; !ok {
		return store.ErrNotFound
	}
	clone := *up
	r.items[id.(string)] = &clone
	return nil
}

func (r *memUserPluginRepo) Delete(_ context.Context, id any) error {
	delete(r.items, id.(string))
	return nil
}

func (r *memUserPluginRepo) Count(_ context.Context, _ store.Query) (int64, error) {
	return 0, nil
}

func (r *memUserPluginRepo) FindByUserID(_ context.Context, _ string) ([]userplugin.UserPlugin, error) {
	return nil, nil
}

func (r *memUserPluginRepo) FindByUserIDAndPluginID(_ context.Context, userID, pluginID string) (*userplugin.UserPlugin, error) {
	return r.FindOne(context.Background(), store.NewQuery().Eq("user_id", userID).Eq("plugin_id", pluginID))
}

type memPluginRepo struct {
	items map[string]*plugin.Plugin
}

func newMemPluginRepo(p *plugin.Plugin) *memPluginRepo {
	return &memPluginRepo{items: map[string]*plugin.Plugin{p.ID: p}}
}

func (r *memPluginRepo) Create(_ context.Context, _ *plugin.Plugin) error { return nil }
func (r *memPluginRepo) FindByID(_ context.Context, id any) (*plugin.Plugin, error) {
	if p, ok := r.items[id.(string)]; ok {
		clone := *p
		return &clone, nil
	}
	return nil, store.ErrNotFound
}
func (r *memPluginRepo) FindOne(_ context.Context, _ store.Query) (*plugin.Plugin, error) {
	return nil, store.ErrNotFound
}
func (r *memPluginRepo) Find(_ context.Context, _ store.Query) ([]plugin.Plugin, error) {
	return nil, nil
}
func (r *memPluginRepo) Update(_ context.Context, _ any, _ *plugin.Plugin) error { return nil }
func (r *memPluginRepo) Delete(_ context.Context, _ any) error                   { return nil }
func (r *memPluginRepo) Count(_ context.Context, _ store.Query) (int64, error)   { return 0, nil }
func (r *memPluginRepo) FindBySlug(_ context.Context, slug string) (*plugin.Plugin, error) {
	for _, p := range r.items {
		if p.Slug == slug {
			clone := *p
			return &clone, nil
		}
	}
	return nil, store.ErrNotFound
}

func newTestService(setupType plugin.SetupType, client *stubComposioClient) (*Service, *memUserPluginRepo, *memCredentialRepo) {
	catalog := &plugin.Plugin{
		ID:   "catalog-1",
		Slug: "composio-ai",
		Manifest: plugin.PluginManifest{
			RequiredSetup: true,
			SetupType:     setupType,
		},
	}
	userRepo := newMemUserPluginRepo()
	userRepo.items["install-1"] = &userplugin.UserPlugin{
		ID:          "install-1",
		UserID:      "user-1",
		PluginID:    catalog.ID,
		SetupStatus: userplugin.SetupStatusNotStarted,
		Config:      map[string]any{},
		UpdatedAt:   time.Now(),
	}

	credRepo := newMemCredentialRepo()
	enc, err := crypto.NewEncryptor("test-encryption-key")
	if err != nil {
		panic(err)
	}
	credSvc := plugincredential.NewService(credRepo, enc)

	factory := func(_ string) ComposioAPI { return client }
	svc := NewService(Config{BaseURL: "http://composio.test"}, userRepo, newMemPluginRepo(catalog), credSvc, factory)
	return svc, userRepo, credRepo
}

func TestSubmitSetupValidKey(t *testing.T) {
	client := &stubComposioClient{
		accounts: []composio.ConnectedAccount{
			{ID: "ca1", ToolkitSlug: "github", Status: "ACTIVE"},
			{ID: "ca2", ToolkitSlug: "gmail", Status: "ACTIVE"},
		},
	}
	svc, userRepo, credRepo := newTestService(plugin.SetupTypeForm, client)
	ctx := context.Background()

	got, err := svc.SubmitSetup(ctx, "user-1", "install-1", " valid-key ")
	if err != nil {
		t.Fatalf("SubmitSetup: %v", err)
	}
	if got.SetupStatus != userplugin.SetupStatusCompleted {
		t.Fatalf("expected completed, got %q", got.SetupStatus)
	}
	if len(got.ConnectedToolkits) != 2 {
		t.Fatalf("expected 2 toolkits, got %v", got.ConnectedToolkits)
	}
	if got.ConnectedAccountsCount != 2 {
		t.Fatalf("expected 2 accounts, got %d", got.ConnectedAccountsCount)
	}

	install := userRepo.items["install-1"]
	if install.SetupStatus != userplugin.SetupStatusCompleted {
		t.Fatalf("install status = %q", install.SetupStatus)
	}
	cfg, err := userplugin.ParseInstallConfig(install.Config)
	if err != nil {
		t.Fatalf("ParseInstallConfig: %v", err)
	}
	if len(cfg.ConnectedToolkits) != 2 || cfg.ConnectedAccountsCount != 2 {
		t.Fatalf("unexpected config: %+v", cfg)
	}

	if len(credRepo.items) != 1 {
		t.Fatalf("expected one credential, got %d", len(credRepo.items))
	}
	for _, c := range credRepo.items {
		if c.EncryptedPayload == "" || c.EncryptedPayload == "valid-key" {
			t.Fatal("expected encrypted payload")
		}
	}
}

func TestSubmitSetupInvalidKey(t *testing.T) {
	client := &stubComposioClient{
		validateErr: errors.Join(composio.ErrUnauthorized, errors.New("status 401")),
	}
	svc, userRepo, _ := newTestService(plugin.SetupTypeForm, client)
	ctx := context.Background()

	_, err := svc.SubmitSetup(ctx, "user-1", "install-1", "bad-key")
	if err == nil {
		t.Fatal("expected error")
	}
	appErr, ok := err.(*response.AppError)
	if !ok || appErr.Status != 400 {
		t.Fatalf("expected 400, got %v", err)
	}

	install := userRepo.items["install-1"]
	if install.SetupStatus != userplugin.SetupStatusFailed {
		t.Fatalf("expected failed, got %q", install.SetupStatus)
	}
	if install.SetupError == nil || *install.SetupError == "" {
		t.Fatal("expected setup_error")
	}
}

func TestSubmitSetupEmptyKey(t *testing.T) {
	client := &stubComposioClient{}
	svc, userRepo, _ := newTestService(plugin.SetupTypeForm, client)
	ctx := context.Background()

	_, err := svc.SubmitSetup(ctx, "user-1", "install-1", "  ")
	if err == nil {
		t.Fatal("expected error")
	}
	appErr, ok := err.(*response.AppError)
	if !ok || appErr.Status != 422 {
		t.Fatalf("expected 422, got %v", err)
	}

	install := userRepo.items["install-1"]
	if install.SetupStatus != userplugin.SetupStatusNotStarted {
		t.Fatalf("expected not_started unchanged, got %q", install.SetupStatus)
	}
}

func TestSubmitSetupInstallNotOwned(t *testing.T) {
	client := &stubComposioClient{}
	svc, _, _ := newTestService(plugin.SetupTypeForm, client)
	ctx := context.Background()

	_, err := svc.SubmitSetup(ctx, "other-user", "install-1", "key")
	if err == nil {
		t.Fatal("expected error")
	}
	appErr, ok := err.(*response.AppError)
	if !ok || appErr.Status != 403 {
		t.Fatalf("expected 403, got %v", err)
	}
}

func TestSubmitSetupWrongSetupType(t *testing.T) {
	client := &stubComposioClient{}
	svc, _, _ := newTestService(plugin.SetupTypeOAuthGoogle, client)
	ctx := context.Background()

	_, err := svc.SubmitSetup(ctx, "user-1", "install-1", "key")
	if err == nil {
		t.Fatal("expected error")
	}
	appErr, ok := err.(*response.AppError)
	if !ok || appErr.Status != 400 {
		t.Fatalf("expected 400, got %v", err)
	}
}

func TestGetSetupStatusReturnsSummary(t *testing.T) {
	client := &stubComposioClient{
		accounts: []composio.ConnectedAccount{
			{ID: "ca1", ToolkitSlug: "github", Status: "ACTIVE"},
		},
	}
	svc, _, _ := newTestService(plugin.SetupTypeForm, client)
	ctx := context.Background()

	if _, err := svc.SubmitSetup(ctx, "user-1", "install-1", "key"); err != nil {
		t.Fatalf("SubmitSetup: %v", err)
	}

	status, err := svc.GetSetupStatus(ctx, "user-1", "install-1")
	if err != nil {
		t.Fatalf("GetSetupStatus: %v", err)
	}
	if status.SetupStatus != userplugin.SetupStatusCompleted {
		t.Fatalf("expected completed, got %q", status.SetupStatus)
	}
	if len(status.ConnectedToolkits) != 1 || status.ConnectedToolkits[0] != "github" {
		t.Fatalf("unexpected toolkits: %v", status.ConnectedToolkits)
	}
	if status.ConnectedAccountsCount != 1 {
		t.Fatalf("expected 1 account, got %d", status.ConnectedAccountsCount)
	}
}
