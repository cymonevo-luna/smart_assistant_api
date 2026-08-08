//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/cymonevo/go_template/internal/domain/user_plugin"
	"github.com/google/uuid"
)

func seedCatalogPlugin(t *testing.T, slug string, requiredSetup bool) {
	t.Helper()
	admin := newClient(t).authed(adminAccessToken(t))
	resp := admin.post("/api/admin/plugins", map[string]any{
		"slug":        slug,
		"name":        "Google Meet Scheduler",
		"description": "Integration test plugin",
		"version":     "1.0.0",
		"manifest": map[string]any{
			"triggers":              []string{"schedule meeting"},
			"required_setup":        requiredSetup,
			"setup_type":            "oauth_google",
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

func TestUserPluginInstallFromCatalog(t *testing.T) {
	slug := fmt.Sprintf("google-calendar-meet-%s", uuid.NewString())
	seedCatalogPlugin(t, slug, true)

	authed := registerAndLogin(t)

	install := authed.post("/api/v1/users/me/plugins", map[string]any{
		"plugin_slug": slug,
	})
	install.requireStatus(t, http.StatusCreated)

	var installed userplugin.InstalledResponse
	install.decode(t, &installed)
	if installed.SetupStatus != userplugin.SetupStatusNotStarted {
		t.Fatalf("expected setup_status not_started, got %q", installed.SetupStatus)
	}
	if installed.Plugin.Slug != slug {
		t.Fatalf("expected slug %q, got %q", slug, installed.Plugin.Slug)
	}

	list := authed.get("/api/v1/users/me/plugins")
	list.requireStatus(t, http.StatusOK)

	var items []userplugin.InstalledResponse
	list.decode(t, &items)
	if len(items) != 1 {
		t.Fatalf("expected one installed plugin, got %d", len(items))
	}
	if items[0].SetupStatus != userplugin.SetupStatusNotStarted {
		t.Fatalf("expected list setup_status not_started, got %q", items[0].SetupStatus)
	}
}

func TestUserPluginInstallUnknownSlug404(t *testing.T) {
	authed := registerAndLogin(t)

	resp := authed.post("/api/v1/users/me/plugins", map[string]any{
		"plugin_slug": "does-not-exist",
	})
	resp.requireStatus(t, http.StatusNotFound)
}

func TestUserPluginUninstallRemovesPlugin(t *testing.T) {
	slug := fmt.Sprintf("uninstall-plugin-%s", uuid.NewString())
	seedCatalogPlugin(t, slug, true)

	authed := registerAndLogin(t)

	install := authed.post("/api/v1/users/me/plugins", map[string]any{
		"plugin_slug": slug,
	})
	install.requireStatus(t, http.StatusCreated)

	var installed userplugin.InstalledResponse
	install.decode(t, &installed)

	del := authed.delete("/api/v1/users/me/plugins/" + installed.ID)
	del.requireStatus(t, http.StatusNoContent)

	list := authed.get("/api/v1/users/me/plugins")
	list.requireStatus(t, http.StatusOK)

	var items []userplugin.InstalledResponse
	list.decode(t, &items)
	if len(items) != 0 {
		t.Fatalf("expected empty list after uninstall, got %d", len(items))
	}
}

func TestUserPluginDisableViaPatch(t *testing.T) {
	slug := fmt.Sprintf("disable-plugin-%s", uuid.NewString())
	seedCatalogPlugin(t, slug, true)

	authed := registerAndLogin(t)

	install := authed.post("/api/v1/users/me/plugins", map[string]any{
		"plugin_slug": slug,
	})
	install.requireStatus(t, http.StatusCreated)

	var installed userplugin.InstalledResponse
	install.decode(t, &installed)

	patch := authed.patch("/api/v1/users/me/plugins/"+installed.ID, map[string]any{
		"enabled": false,
	})
	patch.requireStatus(t, http.StatusOK)

	list := authed.get("/api/v1/users/me/plugins")
	list.requireStatus(t, http.StatusOK)

	var items []userplugin.InstalledResponse
	list.decode(t, &items)
	if len(items) != 1 {
		t.Fatalf("expected one plugin, got %d", len(items))
	}
	if items[0].Enabled {
		t.Fatal("expected enabled false after PATCH")
	}
}

func TestUserPluginNewUserHasNoPlugins(t *testing.T) {
	authed := registerAndLogin(t)

	list := authed.get("/api/v1/users/me/plugins")
	list.requireStatus(t, http.StatusOK)

	var items []userplugin.InstalledResponse
	list.decode(t, &items)
	if len(items) != 0 {
		t.Fatalf("expected zero installed plugins after registration, got %d", len(items))
	}
}
