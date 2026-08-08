//go:build integration

package integration

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/cymonevo/go_template/internal/domain/plugin_setup/oauth_google"
	"github.com/cymonevo/go_template/internal/domain/user_plugin"
	"github.com/google/uuid"
)

func seedCatalogPluginWithSetup(t *testing.T, slug string, requiredSetup bool, setupType string) {
	t.Helper()
	admin := newClient(t).authed(adminAccessToken(t))
	resp := admin.post("/api/admin/plugins", map[string]any{
		"slug":        slug,
		"name":        "Integration test plugin",
		"description": "Integration test plugin",
		"version":     "1.0.0",
		"manifest": map[string]any{
			"triggers":              []string{"schedule meeting"},
			"required_setup":        requiredSetup,
			"setup_type":            setupType,
			"arguments":             []any{},
			"confirmation_required": false,
			"executor": map[string]any{
				"type":   "composio",
				"config": map[string]any{},
			},
		},
	})
	resp.requireStatus(t, http.StatusCreated)
}

func installPlugin(t *testing.T, authed *client, slug string) userplugin.InstalledResponse {
	t.Helper()
	install := authed.post("/api/v1/users/me/plugins", map[string]any{
		"plugin_slug": slug,
	})
	install.requireStatus(t, http.StatusCreated)

	var installed userplugin.InstalledResponse
	install.decode(t, &installed)
	return installed
}

func httpGetNoRedirect(rawURL string) (*http.Response, error) {
	client := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return client.Get(rawURL)
}

func TestPluginSetupInitReturnsAuthorizationURL(t *testing.T) {
	slug := fmt.Sprintf("oauth-google-%s", uuid.NewString())
	seedCatalogPluginWithSetup(t, slug, true, "oauth_google")

	authed := registerAndLogin(t)
	installed := installPlugin(t, authed, slug)

	setup := authed.post("/api/v1/users/me/plugins/"+installed.ID+"/setup", nil)
	setup.requireStatus(t, http.StatusOK)

	var initResp oauthgoogle.SetupInitResponse
	setup.decode(t, &initResp)
	if initResp.AuthorizationURL == "" || initResp.State == "" {
		t.Fatalf("expected non-empty authorization_url and state, got %+v", initResp)
	}
	if !strings.Contains(initResp.AuthorizationURL, "accounts.google.com") {
		t.Fatalf("expected google auth url, got %s", initResp.AuthorizationURL)
	}
	if !strings.Contains(initResp.AuthorizationURL, "client_id=integration-client-id") {
		t.Fatalf("expected client_id in url: %s", initResp.AuthorizationURL)
	}

	status := authed.get("/api/v1/users/me/plugins/" + installed.ID + "/setup/status")
	status.requireStatus(t, http.StatusOK)

	var statusResp oauthgoogle.SetupStatusResponse
	status.decode(t, &statusResp)
	if statusResp.SetupStatus != userplugin.SetupStatusInProgress {
		t.Fatalf("expected in_progress, got %q", statusResp.SetupStatus)
	}
}

func TestPluginSetupOAuthCallbackCompletesSetup(t *testing.T) {
	slug := fmt.Sprintf("oauth-callback-%s", uuid.NewString())
	seedCatalogPluginWithSetup(t, slug, true, "oauth_google")

	authed := registerAndLogin(t)
	installed := installPlugin(t, authed, slug)

	setup := authed.post("/api/v1/users/me/plugins/"+installed.ID+"/setup", nil)
	setup.requireStatus(t, http.StatusOK)

	var initResp oauthgoogle.SetupInitResponse
	setup.decode(t, &initResp)

	callbackURL := server.URL + "/api/v1/plugins/oauth/google/callback?code=test-code&state=" + url.QueryEscape(initResp.State)
	resp, err := httpGetNoRedirect(callbackURL)
	if err != nil {
		t.Fatalf("callback request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 302, got %d body %s", resp.StatusCode, body)
	}
	if !strings.Contains(resp.Header.Get("Location"), "status=success") {
		t.Fatalf("expected success redirect, got %s", resp.Header.Get("Location"))
	}

	status := authed.get("/api/v1/users/me/plugins/" + installed.ID + "/setup/status")
	status.requireStatus(t, http.StatusOK)

	var statusResp oauthgoogle.SetupStatusResponse
	status.decode(t, &statusResp)
	if statusResp.SetupStatus != userplugin.SetupStatusCompleted {
		t.Fatalf("expected completed, got %q", statusResp.SetupStatus)
	}
}

func TestPluginSetupBlockedForWrongSetupType(t *testing.T) {
	slug := fmt.Sprintf("no-setup-%s", uuid.NewString())
	seedCatalogPluginWithSetup(t, slug, false, "none")

	authed := registerAndLogin(t)
	installed := installPlugin(t, authed, slug)

	setup := authed.post("/api/v1/users/me/plugins/"+installed.ID+"/setup", nil)
	if setup.Status != http.StatusBadRequest && setup.Status != http.StatusUnprocessableEntity {
		t.Fatalf("expected 400 or 422, got %d body %s", setup.Status, string(setup.Body))
	}
}

func TestPluginSetupCredentialsNotExposedInList(t *testing.T) {
	slug := fmt.Sprintf("cred-hidden-%s", uuid.NewString())
	seedCatalogPluginWithSetup(t, slug, true, "oauth_google")

	authed := registerAndLogin(t)
	installed := installPlugin(t, authed, slug)

	setup := authed.post("/api/v1/users/me/plugins/"+installed.ID+"/setup", nil)
	setup.requireStatus(t, http.StatusOK)

	var initResp oauthgoogle.SetupInitResponse
	setup.decode(t, &initResp)

	callbackURL := server.URL + "/api/v1/plugins/oauth/google/callback?code=test-code&state=" + url.QueryEscape(initResp.State)
	resp, err := httpGetNoRedirect(callbackURL)
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	_ = resp.Body.Close()

	list := authed.get("/api/v1/users/me/plugins")
	list.requireStatus(t, http.StatusOK)

	raw := string(list.Body)
	for _, forbidden := range []string{"refresh_token", "access_token", "encrypted_payload", "mock-refresh", "mock-access"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("response must not contain %q: %s", forbidden, raw)
		}
	}
}
