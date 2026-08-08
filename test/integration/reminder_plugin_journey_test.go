//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/cymonevo/go_template/internal/domain/plugin"
	"github.com/cymonevo/go_template/internal/domain/user_plugin"
)

func TestReminderPluginInCatalog(t *testing.T) {
	authed := registerAndLogin(t)

	list := authed.get("/api/v1/plugins")
	list.requireStatus(t, http.StatusOK)

	var plugins []plugin.CatalogSummary
	list.decode(t, &plugins)

	found := false
	for _, p := range plugins {
		if p.Slug == "reminder" {
			found = true
			if p.Name != "Reminder" {
				t.Fatalf("expected name %q, got %q", "Reminder", p.Name)
			}
		}
	}
	if !found {
		t.Fatalf("expected reminder plugin in catalog, got %+v", plugins)
	}
}

func TestInstallReminderPluginWithoutSetup(t *testing.T) {
	authed := registerAndLogin(t)

	install := authed.post("/api/v1/users/me/plugins", map[string]any{
		"plugin_slug": "reminder",
	})
	install.requireStatus(t, http.StatusCreated)

	var installed userplugin.InstalledResponse
	install.decode(t, &installed)
	if installed.SetupStatus != userplugin.SetupStatusCompleted {
		t.Fatalf("expected setup_status completed, got %q", installed.SetupStatus)
	}
	if !installed.Enabled {
		t.Fatal("expected enabled true")
	}
	if installed.Plugin.Slug != "reminder" {
		t.Fatalf("expected slug reminder, got %q", installed.Plugin.Slug)
	}
}

func TestUninstallReminderPlugin(t *testing.T) {
	authed := registerAndLogin(t)

	install := authed.post("/api/v1/users/me/plugins", map[string]any{
		"plugin_slug": "reminder",
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
	for _, item := range items {
		if item.Plugin.Slug == "reminder" {
			t.Fatalf("expected reminder plugin omitted after uninstall, got %+v", items)
		}
	}
}
