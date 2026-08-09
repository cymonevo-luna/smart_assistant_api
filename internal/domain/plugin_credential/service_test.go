package plugincredential

import (
	"context"
	"testing"

	"github.com/cymonevo/go_template/pkg/crypto"
	"github.com/cymonevo/go_template/pkg/store"
)

func TestService_UpsertComposio_GetComposio_RoundTrip(t *testing.T) {
	repo := NewRepository(store.NewMemoryStore[Credential]())
	encryptor, err := crypto.NewEncryptor("test-secret-key")
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	svc := NewService(repo, encryptor)
	ctx := context.Background()

	payload := ComposioPayload{
		APIKey:  "composio-api-key-secret",
		BaseURL: "https://api.composio.dev",
		ConnectedAccounts: []ComposioConnectedAccount{
			{
				ID:          "acc-1",
				ToolkitSlug: "github",
				Status:      "active",
				Alias:       "work",
			},
		},
	}

	if err := svc.UpsertComposio(ctx, "install-1", payload); err != nil {
		t.Fatalf("UpsertComposio: %v", err)
	}

	got, err := svc.GetComposio(ctx, "install-1")
	if err != nil {
		t.Fatalf("GetComposio: %v", err)
	}
	if got.APIKey != payload.APIKey {
		t.Fatalf("expected api_key %q, got %q", payload.APIKey, got.APIKey)
	}
	if got.BaseURL != payload.BaseURL {
		t.Fatalf("expected base_url %q, got %q", payload.BaseURL, got.BaseURL)
	}
	if len(got.ConnectedAccounts) != 1 {
		t.Fatalf("expected 1 connected account, got %d", len(got.ConnectedAccounts))
	}
	if got.ConnectedAccounts[0].ID != "acc-1" {
		t.Fatalf("expected connected account id acc-1, got %q", got.ConnectedAccounts[0].ID)
	}

	updated := ComposioPayload{
		APIKey:  "composio-api-key-updated",
		BaseURL: "https://api.composio.dev/v2",
	}
	if err := svc.UpsertComposio(ctx, "install-1", updated); err != nil {
		t.Fatalf("UpsertComposio update: %v", err)
	}

	got, err = svc.GetComposio(ctx, "install-1")
	if err != nil {
		t.Fatalf("GetComposio after update: %v", err)
	}
	if got.APIKey != updated.APIKey {
		t.Fatalf("expected updated api_key %q, got %q", updated.APIKey, got.APIKey)
	}
	if got.BaseURL != updated.BaseURL {
		t.Fatalf("expected updated base_url %q, got %q", updated.BaseURL, got.BaseURL)
	}
}

func TestService_GetComposio_NotFound(t *testing.T) {
	repo := NewRepository(store.NewMemoryStore[Credential]())
	encryptor, err := crypto.NewEncryptor("test-secret-key")
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	svc := NewService(repo, encryptor)

	_, err = svc.GetComposio(context.Background(), "missing-install")
	if err == nil {
		t.Fatal("expected error for missing credentials")
	}
}

func TestService_UpsertComposio_EncryptedAtRest(t *testing.T) {
	mem := store.NewMemoryStore[Credential]()
	repo := NewRepository(mem)
	encryptor, err := crypto.NewEncryptor("test-secret-key")
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	svc := NewService(repo, encryptor)
	ctx := context.Background()

	apiKey := "raw-key-never-stored"
	if err := svc.UpsertComposio(ctx, "install-1", ComposioPayload{APIKey: apiKey}); err != nil {
		t.Fatalf("UpsertComposio: %v", err)
	}

	stored, err := mem.FindOne(ctx, store.NewQuery().Eq("user_plugin_id", "install-1"))
	if err != nil {
		t.Fatalf("FindOne: %v", err)
	}
	if stored.EncryptedPayload == apiKey {
		t.Fatal("encrypted payload must not equal raw api key")
	}
	if stored.Provider != ProviderComposio {
		t.Fatalf("expected provider %q, got %q", ProviderComposio, stored.Provider)
	}
}

func TestService_DeleteByUserPluginID_RemovesComposio(t *testing.T) {
	repo := NewRepository(store.NewMemoryStore[Credential]())
	encryptor, err := crypto.NewEncryptor("test-secret-key")
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	svc := NewService(repo, encryptor)
	ctx := context.Background()

	if err := svc.UpsertComposio(ctx, "install-1", ComposioPayload{APIKey: "key"}); err != nil {
		t.Fatalf("UpsertComposio: %v", err)
	}
	if err := svc.DeleteByUserPluginID(ctx, "install-1"); err != nil {
		t.Fatalf("DeleteByUserPluginID: %v", err)
	}
	_, err = svc.GetComposio(ctx, "install-1")
	if err == nil {
		t.Fatal("expected credentials to be deleted")
	}
}
