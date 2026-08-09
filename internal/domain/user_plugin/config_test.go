package userplugin

import (
	"testing"
)

func TestInstallConfig_ToMap_ParseInstallConfig(t *testing.T) {
	cfg := InstallConfig{
		ConnectedToolkits:      []string{"github", "slack"},
		ConnectedAccountsCount: 2,
	}

	m, err := cfg.ToMap()
	if err != nil {
		t.Fatalf("ToMap: %v", err)
	}
	if m["connected_accounts_count"] != float64(2) {
		t.Fatalf("expected connected_accounts_count 2, got %v", m["connected_accounts_count"])
	}

	parsed, err := ParseInstallConfig(m)
	if err != nil {
		t.Fatalf("ParseInstallConfig: %v", err)
	}
	if len(parsed.ConnectedToolkits) != 2 {
		t.Fatalf("expected 2 toolkits, got %d", len(parsed.ConnectedToolkits))
	}
	if parsed.ConnectedAccountsCount != 2 {
		t.Fatalf("expected connected_accounts_count 2, got %d", parsed.ConnectedAccountsCount)
	}
}
