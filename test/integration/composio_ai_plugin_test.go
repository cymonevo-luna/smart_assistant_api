//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/cymonevo/go_template/internal/domain/plugin"
)

const composioAIPluginSlug = "composio-ai"

func TestComposioAIPluginVisibleInCatalog(t *testing.T) {
	authed := registerAndLogin(t)
	list := authed.get("/api/v1/plugins")
	list.requireStatus(t, http.StatusOK)

	var plugins []plugin.CatalogSummary
	list.decode(t, &plugins)

	var found *plugin.CatalogSummary
	for i := range plugins {
		if plugins[i].Slug == composioAIPluginSlug {
			found = &plugins[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected %q plugin in catalog, got slugs: %+v", composioAIPluginSlug, plugins)
	}
	if !found.RequiredSetup {
		t.Fatal("expected required_setup true")
	}
	if found.SetupType != plugin.SetupTypeForm {
		t.Fatalf("expected setup_type form, got %q", found.SetupType)
	}

	detail := authed.get("/api/v1/plugins/" + composioAIPluginSlug)
	detail.requireStatus(t, http.StatusOK)

	var p plugin.Plugin
	detail.decode(t, &p)

	if p.Manifest.Executor.Type != plugin.ExecutorTypeComposioMCP {
		t.Fatalf("expected composio_mcp executor, got %q", p.Manifest.Executor.Type)
	}
	if len(p.Manifest.Triggers) < 10 {
		t.Fatalf("expected at least 10 triggers, got %d", len(p.Manifest.Triggers))
	}
	if len(p.Manifest.Arguments) != 1 || p.Manifest.Arguments[0].Name != "task" {
		t.Fatalf("expected single task argument, got %+v", p.Manifest.Arguments)
	}
}
