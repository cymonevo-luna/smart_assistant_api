//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"

	"github.com/cymonevo/go_template/internal/domain/plugin_setup/composio_form"
	"github.com/cymonevo/go_template/internal/domain/user_plugin"
)

func TestComposioFormSetupSucceedsWithValidKey(t *testing.T) {
	authed := registerAndLogin(t)
	installed := installPlugin(t, authed, composioAIPluginSlug)

	setup := authed.post("/api/v1/users/me/plugins/"+installed.ID+"/setup", map[string]any{
		"api_key": mockValidComposioAPIKey,
	})
	setup.requireStatus(t, http.StatusOK)

	var result composioform.SubmitResponse
	setup.decode(t, &result)
	if result.SetupStatus != userplugin.SetupStatusCompleted {
		t.Fatalf("expected completed, got %q", result.SetupStatus)
	}
	if len(result.ConnectedToolkits) == 0 {
		t.Fatalf("expected non-empty connected_toolkits, got %v", result.ConnectedToolkits)
	}
	if result.ConnectedAccountsCount < 1 {
		t.Fatalf("expected connected_accounts_count >= 1, got %d", result.ConnectedAccountsCount)
	}

	status := authed.get("/api/v1/users/me/plugins/" + installed.ID + "/setup/status")
	status.requireStatus(t, http.StatusOK)

	var statusResp composioform.SetupStatusResponse
	status.decode(t, &statusResp)
	if statusResp.SetupStatus != userplugin.SetupStatusCompleted {
		t.Fatalf("expected completed status, got %q", statusResp.SetupStatus)
	}
	if len(statusResp.ConnectedToolkits) == 0 {
		t.Fatalf("expected connected_toolkits in status, got %v", statusResp.ConnectedToolkits)
	}
}

func TestComposioFormSetupInvalidKeyFails(t *testing.T) {
	authed := registerAndLogin(t)
	installed := installPlugin(t, authed, composioAIPluginSlug)

	setup := authed.post("/api/v1/users/me/plugins/"+installed.ID+"/setup", map[string]any{
		"api_key": mockInvalidComposioAPIKey,
	})
	if setup.Status != http.StatusBadRequest && setup.Status != http.StatusUnprocessableEntity {
		t.Fatalf("expected 400 or 422, got %d body %s", setup.Status, string(setup.Body))
	}

	status := authed.get("/api/v1/users/me/plugins/" + installed.ID + "/setup/status")
	status.requireStatus(t, http.StatusOK)

	var statusResp composioform.SetupStatusResponse
	status.decode(t, &statusResp)
	if statusResp.SetupStatus != userplugin.SetupStatusFailed {
		t.Fatalf("expected failed, got %q", statusResp.SetupStatus)
	}
	if statusResp.SetupError == nil || *statusResp.SetupError == "" {
		t.Fatal("expected setup_error to be set")
	}
}

func TestComposioFormSetupEmptyKeyRejected(t *testing.T) {
	authed := registerAndLogin(t)
	installed := installPlugin(t, authed, composioAIPluginSlug)

	setup := authed.post("/api/v1/users/me/plugins/"+installed.ID+"/setup", map[string]any{
		"api_key": "",
	})
	setup.requireStatus(t, http.StatusUnprocessableEntity)

	status := authed.get("/api/v1/users/me/plugins/" + installed.ID + "/setup/status")
	status.requireStatus(t, http.StatusOK)

	var statusResp composioform.SetupStatusResponse
	status.decode(t, &statusResp)
	if statusResp.SetupStatus != userplugin.SetupStatusNotStarted {
		t.Fatalf("expected not_started without Composio call, got %q", statusResp.SetupStatus)
	}
}

func TestComposioFormSetupCredentialsNotExposedInList(t *testing.T) {
	authed := registerAndLogin(t)
	installed := installPlugin(t, authed, composioAIPluginSlug)

	setup := authed.post("/api/v1/users/me/plugins/"+installed.ID+"/setup", map[string]any{
		"api_key": mockValidComposioAPIKey,
	})
	setup.requireStatus(t, http.StatusOK)

	list := authed.get("/api/v1/users/me/plugins")
	list.requireStatus(t, http.StatusOK)

	raw := string(list.Body)
	for _, forbidden := range []string{"api_key", "encrypted_payload", mockValidComposioAPIKey} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("response must not contain %q: %s", forbidden, raw)
		}
	}
}
