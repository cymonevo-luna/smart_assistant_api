//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/cymonevo/go_template/internal/domain/user"
	"github.com/google/uuid"
)

// TestUserJourney is the reference end-to-end flow: register -> login ->
// read self -> update self -> verify admin authorization -> admin list ->
// admin delete -> confirm gone. Every step is a real HTTP call against the
// fully wired router.
//
// Use this as a template: copy it and reshape the steps to script whatever
// user journey you need to cover.
func TestUserJourney(t *testing.T) {
	email := fmt.Sprintf("journey-%s@integration.test", uuid.NewString())
	const password = "supersecret123"

	pub := newClient(t)

	// 1. Register a brand new user.
	reg := pub.post("/api/v1/auth/register", map[string]any{
		"email":    email,
		"name":     "Journey User",
		"password": password,
	})
	reg.requireStatus(t, http.StatusCreated)

	var created user.Response
	reg.decode(t, &created)
	if created.ID == "" {
		t.Fatal("expected a user id after registration")
	}

	// 2. Log in and capture the access token.
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
	authed := pub.authed(loginData.Tokens.AccessToken)

	// 3. Read self.
	authed.get("/api/v1/users/"+created.ID).requireStatus(t, http.StatusOK)

	// 4. Update self.
	upd := authed.put("/api/v1/users/"+created.ID, map[string]any{"name": "Renamed User"})
	upd.requireStatus(t, http.StatusOK)

	var updated user.Response
	upd.decode(t, &updated)
	if updated.Name != "Renamed User" {
		t.Fatalf("expected name to be updated, got %q", updated.Name)
	}

	// 5. A regular user must not reach the admin API.
	authed.get("/api/admin/users").requireStatus(t, http.StatusForbidden)

	// 6. An admin token unlocks the admin API.
	admin := pub.authed(adminAccessToken(t))
	admin.get("/api/admin/users").requireStatus(t, http.StatusOK)

	// 7. Admin deletes the user.
	admin.delete("/api/admin/users/"+created.ID).requireStatus(t, http.StatusNoContent)

	// 8. The user is gone.
	admin.get("/api/admin/users/"+created.ID).requireStatus(t, http.StatusNotFound)
}

// TestAuthRejectsAnonymous is a second, smaller example showing how to assert
// negative cases against protected routes.
func TestAuthRejectsAnonymous(t *testing.T) {
	newClient(t).get("/api/v1/users/"+uuid.NewString()).requireStatus(t, http.StatusUnauthorized)
}
