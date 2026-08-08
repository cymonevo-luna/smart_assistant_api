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

func verifyOAuthConfigScript(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	script := filepath.Join(repoRoot, "scripts", "verify-oauth-config.sh")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("verify-oauth-config.sh not found: %v", err)
	}
	return script
}

func runVerifyOAuth(t *testing.T, env map[string]string) (string, int) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	cmd := exec.Command("bash", verifyOAuthConfigScript(t))
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

func writeEnvFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	return path
}

func TestDeployCompose_StagingOAuthEnvDefaults(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	compose := filepath.Join(repoRoot, "docker-compose.deploy.yml")
	data, err := os.ReadFile(compose)
	if err != nil {
		t.Fatalf("read docker-compose.deploy.yml: %v", err)
	}
	content := string(data)
	const redirectURL = "https://jarvis-api.cymonevo.com/api/v1/plugins/oauth/google/callback"
	if !strings.Contains(content, "GOOGLE_OAUTH_REDIRECT_URL: "+redirectURL) {
		t.Fatalf("deploy compose must set staging GOOGLE_OAUTH_REDIRECT_URL; file:\n%s", content)
	}
	if !strings.Contains(content, "APP_OAUTH_SUCCESS_REDIRECT: smartassistant://plugin-setup/complete") {
		t.Fatalf("deploy compose must set APP_OAUTH_SUCCESS_REDIRECT; file:\n%s", content)
	}
}

func TestRefreshDaemon_VerifiesOAuthConfig(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	script := filepath.Join(repoRoot, "scripts", "refresh-daemon.sh")
	data, err := os.ReadFile(script)
	if err != nil {
		t.Fatalf("read refresh-daemon.sh: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "verify-oauth-config.sh") {
		t.Fatalf("refresh-daemon.sh must invoke verify-oauth-config.sh; script:\n%s", content)
	}
	if !strings.Contains(content, `ENV_FILE="$DEPLOY_DIR/.env"`) {
		t.Fatalf("refresh-daemon.sh must verify the deployed .env; script:\n%s", content)
	}
}

func TestVerifyOAuthConfig_PassLocalhost(t *testing.T) {
	envFile := writeEnvFile(t, `GOOGLE_OAUTH_CLIENT_ID=test-client
GOOGLE_OAUTH_CLIENT_SECRET=test-secret
GOOGLE_OAUTH_REDIRECT_URL=http://localhost:8080/api/v1/plugins/oauth/google/callback
`)
	out, code := runVerifyOAuth(t, map[string]string{
		"ENV_FILE": envFile,
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output:\n%s", code, out)
	}
	if !strings.Contains(out, "OAuth configuration verification passed") {
		t.Fatalf("expected pass message, got:\n%s", out)
	}
}

func TestVerifyOAuthConfig_PassStagingHTTPS(t *testing.T) {
	envFile := writeEnvFile(t, `GOOGLE_OAUTH_CLIENT_ID=staging-client
GOOGLE_OAUTH_CLIENT_SECRET=staging-secret
GOOGLE_OAUTH_REDIRECT_URL=https://jarvis-api.cymonevo.com/api/v1/plugins/oauth/google/callback
`)
	out, code := runVerifyOAuth(t, map[string]string{
		"ENV_FILE": envFile,
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output:\n%s", code, out)
	}
}

func TestVerifyOAuthConfig_MissingVars(t *testing.T) {
	envFile := writeEnvFile(t, `GOOGLE_OAUTH_CLIENT_ID=test-client
GOOGLE_OAUTH_REDIRECT_URL=https://jarvis-api.cymonevo.com/api/v1/plugins/oauth/google/callback
`)
	out, code := runVerifyOAuth(t, map[string]string{
		"ENV_FILE": envFile,
	})
	if code == 0 {
		t.Fatalf("expected non-zero exit for missing secret; output:\n%s", out)
	}
	if !strings.Contains(out, "missing required OAuth variables") {
		t.Fatalf("expected missing-vars error, got:\n%s", out)
	}
}

func TestVerifyOAuthConfig_HTTPNonLocalhostRejected(t *testing.T) {
	envFile := writeEnvFile(t, `GOOGLE_OAUTH_CLIENT_ID=test-client
GOOGLE_OAUTH_CLIENT_SECRET=test-secret
GOOGLE_OAUTH_REDIRECT_URL=http://jarvis-api.cymonevo.com/api/v1/plugins/oauth/google/callback
`)
	out, code := runVerifyOAuth(t, map[string]string{
		"ENV_FILE": envFile,
	})
	if code == 0 {
		t.Fatalf("expected non-zero exit for http non-localhost; output:\n%s", out)
	}
	if !strings.Contains(out, "must use https") {
		t.Fatalf("expected https error, got:\n%s", out)
	}
}

func TestQALocalOAuthMockCompose_HasSmokeCredentials(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	compose := filepath.Join(repoRoot, "docker-compose.qa-oauth-mock.yml")
	data, err := os.ReadFile(compose)
	if err != nil {
		t.Fatalf("read docker-compose.qa-oauth-mock.yml: %v", err)
	}
	content := string(data)
	for _, want := range []string{
		"GOOGLE_OAUTH_CLIENT_ID: qa-smoke-client-id",
		"GOOGLE_OAUTH_CLIENT_SECRET: qa-smoke-secret",
		"GOOGLE_OAUTH_REDIRECT_URL: http://localhost:8080/api/v1/plugins/oauth/google/callback",
		"GOOGLE_OAUTH_TOKEN_URL: http://oauth-mock:8080/token",
		"oauth-mock:",
		"18080:8080",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("qa oauth mock compose must contain %q; file:\n%s", want, content)
		}
	}
}

func TestQALocalCompose_HostModeTokenURL(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	compose := filepath.Join(repoRoot, "docker-compose.qa-local.yml")
	data, err := os.ReadFile(compose)
	if err != nil {
		t.Fatalf("read docker-compose.qa-local.yml: %v", err)
	}
	content := string(data)
	const want = "GOOGLE_OAUTH_TOKEN_URL: http://localhost:18080/token"
	if !strings.Contains(content, want) {
		t.Fatalf("host-network qa compose must override token URL; file:\n%s", content)
	}
}

func TestVerifyOAuthConfig_PassQASmokeCredentials(t *testing.T) {
	envFile := writeEnvFile(t, `GOOGLE_OAUTH_CLIENT_ID=qa-smoke-client-id
GOOGLE_OAUTH_CLIENT_SECRET=qa-smoke-secret
GOOGLE_OAUTH_REDIRECT_URL=http://localhost:8080/api/v1/plugins/oauth/google/callback
`)
	out, code := runVerifyOAuth(t, map[string]string{
		"ENV_FILE": envFile,
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output:\n%s", code, out)
	}
}

func TestVerifyOAuthConfig_EnvOverridesFile(t *testing.T) {
	envFile := writeEnvFile(t, `GOOGLE_OAUTH_CLIENT_ID=file-client
GOOGLE_OAUTH_CLIENT_SECRET=file-secret
GOOGLE_OAUTH_REDIRECT_URL=http://localhost:8080/callback
`)
	out, code := runVerifyOAuth(t, map[string]string{
		"ENV_FILE":                   envFile,
		"GOOGLE_OAUTH_CLIENT_ID":     "env-client",
		"GOOGLE_OAUTH_CLIENT_SECRET": "env-secret",
		"GOOGLE_OAUTH_REDIRECT_URL":  "http://localhost:8080/callback",
	})
	if code != 0 {
		t.Fatalf("expected exit 0 when env vars set, got %d; output:\n%s", code, out)
	}
	if strings.Contains(out, "missing required OAuth variables") {
		t.Fatalf("env vars should override file, got:\n%s", out)
	}
}
