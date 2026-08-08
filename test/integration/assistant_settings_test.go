//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/cymonevo/go_template/internal/domain/assistant_settings"
	"github.com/google/uuid"
)

func registerAndLogin(t *testing.T) *client {
	t.Helper()
	email := fmt.Sprintf("assistant-%s@integration.test", uuid.NewString())
	const password = "supersecret123"

	pub := newClient(t)
	reg := pub.post("/api/v1/auth/register", map[string]any{
		"email":    email,
		"name":     "Assistant User",
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
	if loginData.Tokens.AccessToken == "" {
		t.Fatal("expected an access token after login")
	}
	return pub.authed(loginData.Tokens.AccessToken)
}

func TestAssistantSettingsDefaultOnFirstGet(t *testing.T) {
	authed := registerAndLogin(t)

	res := authed.get("/api/v1/assistant/settings")
	res.requireStatus(t, http.StatusOK)

	var settings assistantsettings.Response
	res.decode(t, &settings)
	if settings.WakeWord != assistantsettings.DefaultWakeWord {
		t.Fatalf("expected wake_word %q, got %q", assistantsettings.DefaultWakeWord, settings.WakeWord)
	}
	if settings.ActiveListeningEnabled {
		t.Fatal("expected active_listening_enabled false by default")
	}
	if settings.UpdatedAt.IsZero() {
		t.Fatal("expected updated_at to be set")
	}
}

func TestAssistantSettingsUpdateAndPersist(t *testing.T) {
	authed := registerAndLogin(t)

	put := authed.put("/api/v1/assistant/settings", map[string]any{
		"wake_word":                "Friday",
		"active_listening_enabled": true,
	})
	put.requireStatus(t, http.StatusOK)

	var updated assistantsettings.Response
	put.decode(t, &updated)
	if updated.WakeWord != "Friday" {
		t.Fatalf("expected wake_word Friday, got %q", updated.WakeWord)
	}
	if !updated.ActiveListeningEnabled {
		t.Fatal("expected active_listening_enabled true")
	}

	get := authed.get("/api/v1/assistant/settings")
	get.requireStatus(t, http.StatusOK)

	var persisted assistantsettings.Response
	get.decode(t, &persisted)
	if persisted.WakeWord != "Friday" || !persisted.ActiveListeningEnabled {
		t.Fatalf("unexpected persisted settings: %+v", persisted)
	}
}

func TestAssistantSettingsRejectEmptyWakeWord(t *testing.T) {
	authed := registerAndLogin(t)

	res := authed.put("/api/v1/assistant/settings", map[string]any{
		"wake_word": "   ",
	})
	res.requireStatus(t, http.StatusUnprocessableEntity)

	if res.Envelope.Error == nil {
		t.Fatal("expected error body")
	}
	if res.Envelope.Error.Fields["wake_word"] == "" {
		t.Fatalf("expected field error on wake_word, got %+v", res.Envelope.Error.Fields)
	}
}

func TestAssistantSettingsRequireAuthentication(t *testing.T) {
	newClient(t).get("/api/v1/assistant/settings").requireStatus(t, http.StatusUnauthorized)
}
