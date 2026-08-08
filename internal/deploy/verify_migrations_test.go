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

// verifyMigrationsScript resolves scripts/verify-migrations.sh relative to this
// test file so the test works regardless of the caller's working directory.
func verifyMigrationsScript(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	script := filepath.Join(repoRoot, "scripts", "verify-migrations.sh")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("verify-migrations.sh not found: %v", err)
	}
	return script
}

// runVerify runs verify-migrations.sh with the given minimum version and env
// overrides, returning combined output and the process exit code.
func runVerify(t *testing.T, minVersion string, env map[string]string) (string, int) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	cmd := exec.Command("bash", verifyMigrationsScript(t), minVersion)
	cmd.Env = append(os.Environ(),
		// Keep the tests fast and hermetic: no real docker, instant retries.
		"VERIFY_MIGRATIONS_RETRY_DELAY=0",
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

// TestVerifyMigrations_RunsDockerAsRoot locks in the fix for the deploy-#69/#72
// failure: the deploy dir's .env is a root-owned 0600 secret and the system
// docker socket needs root, so the probe's `docker compose` MUST run as root
// (via as_root/sudo). Running it as the unprivileged deploy user fails with
// "permission denied" and a docker compose exit 14.
func TestVerifyMigrations_RunsDockerAsRoot(t *testing.T) {
	data, err := os.ReadFile(verifyMigrationsScript(t))
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "as_root env DOCKER_HOST=") {
		t.Fatalf("compose() must run docker as root (as_root env DOCKER_HOST=...); script:\n%s", content)
	}
}

func TestVerifyMigrations_PassNoColumns(t *testing.T) {
	out, code := runVerify(t, "20260712060023", map[string]string{
		"VERIFY_MIGRATIONS_CMD": `echo "version=20260712072155 dirty=false"`,
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output:\n%s", code, out)
	}
	if !strings.Contains(out, "Migration verification passed") {
		t.Fatalf("expected pass message, got:\n%s", out)
	}
}

func TestVerifyMigrations_Dirty(t *testing.T) {
	out, code := runVerify(t, "20260712060023", map[string]string{
		"VERIFY_MIGRATIONS_CMD": `echo "version=20260712072155 dirty=true"`,
	})
	if code == 0 {
		t.Fatalf("expected non-zero exit for dirty schema; output:\n%s", out)
	}
	if !strings.Contains(out, "dirty") {
		t.Fatalf("expected dirty error, got:\n%s", out)
	}
}

func TestVerifyMigrations_BelowMinimum(t *testing.T) {
	out, code := runVerify(t, "20260712060023", map[string]string{
		"VERIFY_MIGRATIONS_CMD": `echo "version=20260101000000 dirty=false"`,
	})
	if code == 0 {
		t.Fatalf("expected non-zero exit for below-min version; output:\n%s", out)
	}
	if !strings.Contains(out, "below required minimum") {
		t.Fatalf("expected below-min error, got:\n%s", out)
	}
}

func TestVerifyMigrations_UnparseableOutput(t *testing.T) {
	out, code := runVerify(t, "0", map[string]string{
		"VERIFY_MIGRATIONS_CMD": `echo "totally unexpected"`,
	})
	if code == 0 {
		t.Fatalf("expected non-zero exit for unparseable output; output:\n%s", out)
	}
	if !strings.Contains(out, "could not parse") {
		t.Fatalf("expected parse error, got:\n%s", out)
	}
}

// TestVerifyMigrations_RetriesTransientProbe covers the exact deploy-#69 shape: a
// freshly-restarted stack makes the `migrate version` one-off transiently exit
// non-zero (14). The gate must RETRY rather than roll the deploy back.
func TestVerifyMigrations_RetriesTransientProbe(t *testing.T) {
	counter := filepath.Join(t.TempDir(), "attempts")
	probe := `n=$(cat "` + counter + `" 2>/dev/null || echo 0); n=$((n+1)); echo "$n" > "` + counter + `"; ` +
		`if [ "$n" -lt 3 ]; then echo "compose create failed" >&2; exit 14; fi; ` +
		`echo "version=20260712072155 dirty=false"`
	out, code := runVerify(t, "20260712060023", map[string]string{
		"VERIFY_MIGRATIONS_CMD": probe,
	})
	if code != 0 {
		t.Fatalf("expected retry to succeed (exit 0), got %d; output:\n%s", code, out)
	}
	if !strings.Contains(out, "retrying") {
		t.Fatalf("expected retry log, got:\n%s", out)
	}
	if !strings.Contains(out, "Migration verification passed") {
		t.Fatalf("expected eventual pass, got:\n%s", out)
	}
}

// TestVerifyMigrations_SurfacesProbeOutputOnPermanentFailure ensures a probe that
// never recovers fails with a NON-zero exit AND prints the real error, instead of
// silently dying on a bare code (the original bug that made #69 opaque).
func TestVerifyMigrations_SurfacesProbeOutputOnPermanentFailure(t *testing.T) {
	out, code := runVerify(t, "20260712060023", map[string]string{
		"VERIFY_MIGRATIONS_RETRIES": "2",
		"VERIFY_MIGRATIONS_CMD":     `echo "compose exploded on the host" >&2; exit 14`,
	})
	if code == 0 {
		t.Fatalf("expected non-zero exit on permanent probe failure; output:\n%s", out)
	}
	if !strings.Contains(out, "compose exploded on the host") {
		t.Fatalf("expected the probe's error to be surfaced, got:\n%s", out)
	}
	if !strings.Contains(out, "failed after 2 attempt") {
		t.Fatalf("expected attempt-count summary, got:\n%s", out)
	}
}

func TestVerifyMigrations_ColumnsPresent(t *testing.T) {
	out, code := runVerify(t, "20260712060023", map[string]string{
		"VERIFY_MIGRATIONS_CMD":   `echo "version=20260712072155 dirty=false"`,
		"VERIFY_COLUMNS_TABLE":    "suppliers",
		"VERIFY_REQUIRED_COLUMNS": "phone_number address supports_delivery delivery_cost",
		"VERIFY_COLUMNS_CMD":      `printf "address\ndelivery_cost\nphone_number\nsupports_delivery\n"`,
	})
	if code != 0 {
		t.Fatalf("expected exit 0 when columns present, got %d; output:\n%s", code, out)
	}
	if !strings.Contains(out, "suppliers columns ok") {
		t.Fatalf("expected columns-ok message, got:\n%s", out)
	}
}

func TestVerifyMigrations_ColumnsMissing(t *testing.T) {
	out, code := runVerify(t, "20260712060023", map[string]string{
		"VERIFY_MIGRATIONS_CMD":   `echo "version=20260712072155 dirty=false"`,
		"VERIFY_COLUMNS_TABLE":    "suppliers",
		"VERIFY_REQUIRED_COLUMNS": "phone_number address supports_delivery delivery_cost",
		"VERIFY_COLUMNS_CMD":      `printf "address\nphone_number\n"`,
	})
	if code == 0 {
		t.Fatalf("expected non-zero exit when columns missing; output:\n%s", out)
	}
	if !strings.Contains(out, "missing required columns") {
		t.Fatalf("expected missing-columns error, got:\n%s", out)
	}
	if !strings.Contains(out, "supports_delivery") || !strings.Contains(out, "delivery_cost") {
		t.Fatalf("expected the missing column names to be listed, got:\n%s", out)
	}
}
