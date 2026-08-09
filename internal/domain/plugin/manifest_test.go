package plugin

import (
	"errors"
	"strings"
	"testing"

	"github.com/cymonevo/go_template/pkg/response"
)

func validManifest() PluginManifest {
	return PluginManifest{
		Triggers:      []string{"schedule a meeting", "book a call"},
		RequiredSetup: true,
		SetupType:     SetupTypeOAuthGoogle,
		Arguments: []ManifestArgument{
			{
				Name:        "title",
				Type:        "string",
				Required:    true,
				Description: "Meeting title",
				Prompt:      "What should the meeting be called?",
			},
		},
		ConfirmationRequired: true,
		Executor: Executor{
			Type: ExecutorTypeComposio,
			Config: map[string]any{
				"action": "google_calendar_create_event",
			},
		},
	}
}

func TestManifestValidation(t *testing.T) {
	t.Run("valid manifest passes", func(t *testing.T) {
		if err := ValidateManifest(validManifest()); err != nil {
			t.Fatalf("expected valid manifest, got %v", err)
		}
	})

	t.Run("missing triggers rejected", func(t *testing.T) {
		m := validManifest()
		m.Triggers = nil
		err := ValidateManifest(m)
		assertValidationError(t, err, "triggers")
	})

	t.Run("empty triggers rejected", func(t *testing.T) {
		m := validManifest()
		m.Triggers = []string{}
		err := ValidateManifest(m)
		assertValidationError(t, err, "triggers")
	})

	t.Run("bad setup_type rejected", func(t *testing.T) {
		m := validManifest()
		m.SetupType = SetupType("oauth_microsoft")
		err := ValidateManifest(m)
		assertValidationError(t, err, "setup_type")
	})

	t.Run("missing executor rejected", func(t *testing.T) {
		m := validManifest()
		m.Executor = Executor{}
		err := ValidateManifest(m)
		assertValidationErrorPrefix(t, err, "executor")
	})

	t.Run("bad executor type rejected", func(t *testing.T) {
		m := validManifest()
		m.Executor.Type = ExecutorType("lambda")
		err := ValidateManifest(m)
		assertValidationError(t, err, "executor.type")
	})

	t.Run("composio_mcp executor type accepted", func(t *testing.T) {
		m := validManifest()
		m.Executor.Type = ExecutorTypeComposioMCP
		m.Executor.Config = map[string]any{
			"tool_slug": "GITHUB_CREATE_ISSUE",
		}
		if err := ValidateManifest(m); err != nil {
			t.Fatalf("expected composio_mcp executor to be valid, got %v", err)
		}
	})
}

func TestRedactExecutorConfig(t *testing.T) {
	m := validManifest()
	m.Executor.Config = map[string]any{
		"action":        "google_calendar_create_event",
		"api_key":       "secret-value",
		"client_secret": "also-secret",
	}

	redacted := RedactExecutorConfig(m)
	if redacted.Executor.Config["api_key"] != "[REDACTED]" {
		t.Fatalf("expected api_key to be redacted, got %v", redacted.Executor.Config["api_key"])
	}
	if redacted.Executor.Config["client_secret"] != "[REDACTED]" {
		t.Fatalf("expected client_secret to be redacted, got %v", redacted.Executor.Config["client_secret"])
	}
	if redacted.Executor.Config["action"] != "google_calendar_create_event" {
		t.Fatalf("expected action to remain visible, got %v", redacted.Executor.Config["action"])
	}
}

func assertValidationErrorPrefix(t *testing.T, err error, fieldPrefix string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected validation error for field prefix %q", fieldPrefix)
	}

	var appErr *response.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T: %v", err, err)
	}
	if appErr.Status != 422 {
		t.Fatalf("expected status 422, got %d", appErr.Status)
	}
	if appErr.Fields == nil {
		t.Fatalf("expected field-level errors, got none")
	}
	for field := range appErr.Fields {
		if field == fieldPrefix || strings.HasPrefix(field, fieldPrefix+".") {
			return
		}
	}
	t.Fatalf("expected field prefix %q in %v", fieldPrefix, appErr.Fields)
}

func assertValidationError(t *testing.T, err error, field string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected validation error for field %q", field)
	}

	var appErr *response.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T: %v", err, err)
	}
	if appErr.Status != 422 {
		t.Fatalf("expected status 422, got %d", appErr.Status)
	}
	if appErr.Fields == nil {
		t.Fatalf("expected field-level errors, got none")
	}
	if _, ok := appErr.Fields[field]; !ok {
		t.Fatalf("expected field %q in %v", field, appErr.Fields)
	}
}
