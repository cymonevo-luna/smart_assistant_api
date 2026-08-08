//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/cymonevo/go_template/internal/domain/plugin"
	"github.com/google/uuid"
)

func TestPluginCatalogEmptyList(t *testing.T) {
	email := fmt.Sprintf("plugins-%s@integration.test", uuid.NewString())
	const password = "supersecret123"

	pub := newClient(t)

	reg := pub.post("/api/v1/auth/register", map[string]any{
		"email":    email,
		"name":     "Plugin Tester",
		"password": password,
	})
	reg.requireStatus(t, http.StatusCreated)

	login := pub.post("/api/v1/auth/login", map[string]any{
		"email":    email,
		"password": password,
	})
	login.requireStatus(t, http.StatusOK)

	var loginData struct {
		Tokens struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	login.decode(t, &loginData)

	list := pub.authed(loginData.Tokens.AccessToken).get("/api/v1/plugins")
	list.requireStatus(t, http.StatusOK)
	if !list.Envelope.Success {
		t.Fatalf("expected success envelope, got %+v", list.Envelope)
	}

	var plugins []plugin.CatalogSummary
	list.decode(t, &plugins)
	if len(plugins) == 0 {
		t.Fatal("expected at least the seeded google-calendar-meet plugin")
	}
	found := false
	for _, p := range plugins {
		if p.Slug == "google-calendar-meet" {
			found = true
			if !p.RequiredSetup {
				t.Fatalf("expected google-calendar-meet required_setup true, got false")
			}
		}
	}
	if !found {
		t.Fatalf("expected google-calendar-meet in catalog, got %+v", plugins)
	}
}

func TestPluginAdminRegisterInvalidManifest(t *testing.T) {
	admin := newClient(t).authed(adminAccessToken(t))
	resp := admin.post("/api/admin/plugins", map[string]any{
		"slug":    "invalid-plugin",
		"name":    "Invalid Plugin",
		"version": "1.0.0",
		"manifest": map[string]any{
			"required_setup":        true,
			"setup_type":            "oauth_google",
			"arguments":             []any{},
			"confirmation_required": false,
			"executor": map[string]any{
				"type":   "composio",
				"config": map[string]any{},
			},
		},
	})
	resp.requireStatus(t, http.StatusUnprocessableEntity)
	if resp.Envelope.Error == nil || resp.Envelope.Error.Fields == nil {
		t.Fatalf("expected field-level validation errors, got %+v", resp.Envelope.Error)
	}
}

func TestPluginAdminRegisterAndGetDetail(t *testing.T) {
	admin := newClient(t).authed(adminAccessToken(t))
	slug := fmt.Sprintf("test-plugin-%s", uuid.NewString())

	reg := admin.post("/api/admin/plugins", map[string]any{
		"slug":        slug,
		"name":        "Test Plugin",
		"description": "Integration test plugin",
		"version":     "1.0.0",
		"manifest": map[string]any{
			"triggers":              []string{"run test"},
			"required_setup":        false,
			"setup_type":            "none",
			"arguments":             []any{},
			"confirmation_required": false,
			"executor": map[string]any{
				"type":   "builtin",
				"config": map[string]any{"handler": "test"},
			},
		},
	})
	reg.requireStatus(t, http.StatusCreated)

	pub := newClient(t)
	email := fmt.Sprintf("plugin-detail-%s@integration.test", uuid.NewString())
	pub.post("/api/v1/auth/register", map[string]any{
		"email":    email,
		"name":     "Reader",
		"password": "supersecret123",
	}).requireStatus(t, http.StatusCreated)
	login := pub.post("/api/v1/auth/login", map[string]any{
		"email":    email,
		"password": "supersecret123",
	})
	login.requireStatus(t, http.StatusOK)

	var loginData struct {
		Tokens struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	login.decode(t, &loginData)

	detail := pub.authed(loginData.Tokens.AccessToken).get("/api/v1/plugins/" + slug)
	detail.requireStatus(t, http.StatusOK)

	var got plugin.DetailResponse
	detail.decode(t, &got)
	if got.Slug != slug {
		t.Fatalf("expected slug %q, got %q", slug, got.Slug)
	}
	if got.Manifest.Executor.Config["handler"] != "test" {
		t.Fatalf("expected executor config to be present")
	}
}
