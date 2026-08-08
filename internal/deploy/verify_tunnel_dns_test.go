package deploy

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func configureCloudflareTunnelScript(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	script := filepath.Join(repoRoot, "scripts", "configure-cloudflare-tunnel.sh")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("configure-cloudflare-tunnel.sh not found: %v", err)
	}
	return script
}

func verifyTunnelDNSScript(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	script := filepath.Join(repoRoot, "scripts", "verify-tunnel-dns.sh")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("verify-tunnel-dns.sh not found: %v", err)
	}
	return script
}

func runConfigureTunnel(t *testing.T, env map[string]string) (string, int) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	cmd := exec.Command("bash", configureCloudflareTunnelScript(t))
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("running script: %v (output: %s)", err, out)
		}
	}
	return string(out), code
}

func runVerifyTunnel(t *testing.T, env map[string]string) (string, int) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	cmd := exec.Command("bash", verifyTunnelDNSScript(t))
	cmd.Env = append(os.Environ(),
		"VERIFY_TUNNEL_RETRIES=1",
		"VERIFY_TUNNEL_RETRY_DELAY=0",
	)
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("running script: %v (output: %s)", err, out)
		}
	}
	return string(out), code
}

func TestConfigureCloudflareTunnel_MissingVars(t *testing.T) {
	out, code := runConfigureTunnel(t, map[string]string{
		"ENV_FILE": "/dev/null",
	})
	if code == 0 {
		t.Fatalf("expected non-zero exit for missing Cloudflare vars; output:\n%s", out)
	}
	if !strings.Contains(out, "missing required Cloudflare variable") {
		t.Fatalf("expected missing-vars error, got:\n%s", out)
	}
}

func TestConfigureCloudflareTunnel_IdempotentFlow(t *testing.T) {
	cfCmd := `
case "$CONFIGURE_TUNNEL_CF_METHOD" in
GET)
  case "$CONFIGURE_TUNNEL_CF_PATH" in
  */cfd_tunnel?name=*)
    echo '{"success":true,"result":[{"id":"tunnel-123","name":"smart_assistant_api-staging"}]}'
    ;;
  */cfd_tunnel/tunnel-123/token)
    echo '{"success":true,"result":"connector-token"}'
    ;;
  */dns_records?*)
    echo '{"success":true,"result":[]}'
    ;;
  esac
  ;;
PUT|POST)
  echo '{"success":true,"result":{"id":"dns-1"}}'
  ;;
esac
`
	out, code := runConfigureTunnel(t, map[string]string{
		"CLOUDFLARE_API_TOKEN":          "test-token",
		"CLOUDFLARE_ACCOUNT_ID":         "acct-1",
		"CLOUDFLARE_ZONE_ID":            "zone-1",
		"CONFIGURE_TUNNEL_SKIP_INSTALL": "1",
		"CONFIGURE_TUNNEL_CF_CMD":       cfCmd,
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output:\n%s", code, out)
	}
	if !strings.Contains(out, "Tunnel ingress configured") {
		t.Fatalf("expected ingress configuration log, got:\n%s", out)
	}
	if !strings.Contains(out, "Creating DNS record") {
		t.Fatalf("expected DNS creation log, got:\n%s", out)
	}
}

func TestVerifyTunnelDNS_Pass(t *testing.T) {
	out, code := runVerifyTunnel(t, map[string]string{
		"VERIFY_TUNNEL_DNS_CMD":    `echo "jarvis-api.cymonevo.com.cdn.cloudflare.net."`,
		"VERIFY_TUNNEL_HEALTH_CMD": `echo '{"success":true,"data":{"status":"ok"}}'`,
		"VERIFY_TUNNEL_READY_CMD":  `echo '{"success":true,"data":{"postgres":"up","redis":"up"}}'`,
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output:\n%s", code, out)
	}
	if !strings.Contains(out, "Tunnel/DNS verification passed") {
		t.Fatalf("expected pass message, got:\n%s", out)
	}
}

func TestVerifyTunnelDNS_EmptyDNS(t *testing.T) {
	out, code := runVerifyTunnel(t, map[string]string{
		"VERIFY_TUNNEL_DNS_CMD": `true`,
	})
	if code == 0 {
		t.Fatalf("expected non-zero exit for empty DNS; output:\n%s", out)
	}
	if !strings.Contains(out, "returned empty output") && !strings.Contains(out, "returned no records") {
		t.Fatalf("expected DNS failure, got:\n%s", out)
	}
}

func TestVerifyTunnelDNS_HealthMissingOK(t *testing.T) {
	out, code := runVerifyTunnel(t, map[string]string{
		"VERIFY_TUNNEL_DNS_CMD":    `echo "104.21.39.58"`,
		"VERIFY_TUNNEL_HEALTH_CMD": `echo '{"success":true,"data":{"status":"degraded"}}'`,
	})
	if code == 0 {
		t.Fatalf("expected non-zero exit for bad health payload; output:\n%s", out)
	}
	if !strings.Contains(out, "status ok") {
		t.Fatalf("expected health failure, got:\n%s", out)
	}
}

func TestVerifyTunnelDNS_ReadyMissingDependency(t *testing.T) {
	out, code := runVerifyTunnel(t, map[string]string{
		"VERIFY_TUNNEL_DNS_CMD":    `echo "104.21.39.58"`,
		"VERIFY_TUNNEL_HEALTH_CMD": `echo '{"success":true,"data":{"status":"ok"}}'`,
		"VERIFY_TUNNEL_READY_CMD":  `echo '{"success":true,"data":{"postgres":"up","redis":"down"}}'`,
	})
	if code == 0 {
		t.Fatalf("expected non-zero exit for redis down; output:\n%s", out)
	}
	if !strings.Contains(out, "redis up") {
		t.Fatalf("expected redis failure, got:\n%s", out)
	}
}
