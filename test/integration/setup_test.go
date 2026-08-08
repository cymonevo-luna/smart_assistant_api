//go:build integration

// Package integration holds black-box, end-to-end tests that drive the fully
// wired application over HTTP. They are guarded by the `integration` build tag
// so they never run during the normal unit-test pass; run them with:
//
//	make test-integration
//
// The suite boots the real router (the same one cmd/api serves) against the
// database configured via DB_DRIVER/DB_URI, exposes it through an
// httptest.Server, and exercises it exactly as an external client would.
package integration

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cymonevo/go_template/internal/app"
	"github.com/cymonevo/go_template/pkg/auth"
	"github.com/google/uuid"
)

// Shared across all tests in the suite. Set up once in TestMain.
var (
	application *app.App
	server      *httptest.Server
)

// TestMain builds the application, starts the in-process HTTP server, runs the
// suite, and tears everything down. Setup failures abort the suite with a clear
// message rather than producing confusing per-test errors.
func TestMain(m *testing.M) {
	os.Exit(runSuite(m))
}

func runSuite(m *testing.M) int {
	ctx := context.Background()

	a, err := app.New(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration setup: build app: %v\n", err)
		fmt.Fprintln(os.Stderr, "hint: ensure the configured database (DB_DRIVER/DB_URI) is reachable and migrated.")
		return 1
	}

	if err := a.Container().StartBackground(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "integration setup: start background workers: %v\n", err)
		return 1
	}

	application = a
	server = httptest.NewServer(a.Handler())

	code := m.Run()

	server.Close()
	a.Container().Close(ctx)
	return code
}

// adminAccessToken mints a valid admin access token directly from the running
// app's token manager. The subject user need not exist in the database: admin
// authorization is decided purely from the token claims, which is enough to
// exercise the /api/admin routes.
func adminAccessToken(t *testing.T) string {
	t.Helper()
	pair, err := application.Container().Tokens.GeneratePair(uuid.NewString(), "admin@integration.test", auth.RoleAdmin)
	if err != nil {
		t.Fatalf("mint admin token: %v", err)
	}
	return pair.AccessToken
}
