//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/cymonevo/go_template/internal/domain/reminder"
	"github.com/google/uuid"
)

func registerAndLoginReminderUser(t *testing.T, prefix string) *client {
	t.Helper()
	email := fmt.Sprintf("%s-%s@integration.test", prefix, uuid.NewString())
	const password = "supersecret123"

	pub := newClient(t)
	reg := pub.post("/api/v1/auth/register", map[string]any{
		"email":    email,
		"name":     "Reminder User",
		"password": password,
	})
	reg.requireStatus(t, http.StatusCreated)

	login := pub.post("/api/v1/auth/login", map[string]any{
		"email":    email,
		"password": password,
	})
	login.requireStatus(t, http.StatusOK)

	var loginData struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
		Tokens struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	login.decode(t, &loginData)
	if loginData.Tokens.AccessToken == "" {
		t.Fatal("expected an access token after login")
	}
	if loginData.User.ID == "" {
		t.Fatal("expected user id after login")
	}

	c := pub.authed(loginData.Tokens.AccessToken)
	c.userID = loginData.User.ID
	return c
}

func seedReminder(t *testing.T, userID string, in reminder.CreateInput) *reminder.Reminder {
	t.Helper()
	ctx := context.Background()
	rem, err := application.Container().ReminderService.Create(ctx, userID, in)
	if err != nil {
		t.Fatalf("seed reminder: %v", err)
	}
	return rem
}

func locationReminderInput() reminder.CreateInput {
	mode := reminder.LocationModeExact
	query := "Cempaka Putih Tengah 20"
	lat := -6.1751
	lng := 106.8650
	return reminder.CreateInput{
		Title:        "pick my printer",
		TriggerType:  reminder.TriggerTypeLocation,
		LocationMode: &mode,
		PlaceQuery:   &query,
		Latitude:     &lat,
		Longitude:    &lng,
		RadiusMeters: 100,
	}
}

func TestReminderListReturnsSeededReminder(t *testing.T) {
	authed := registerAndLoginReminderUser(t, "reminder-list")
	seeded := seedReminder(t, authed.userID, locationReminderInput())

	res := authed.get("/api/v1/reminders?status=pending")
	res.requireStatus(t, http.StatusOK)

	var items []reminder.Response
	res.decode(t, &items)
	if len(items) != 1 {
		t.Fatalf("expected 1 reminder, got %d", len(items))
	}
	if items[0].ID != seeded.ID {
		t.Fatalf("expected reminder id %q, got %q", seeded.ID, items[0].ID)
	}
	if items[0].Title != "pick my printer" {
		t.Fatalf("unexpected title %q", items[0].Title)
	}
	if items[0].TriggerType != reminder.TriggerTypeLocation {
		t.Fatalf("unexpected trigger_type %q", items[0].TriggerType)
	}
	if items[0].RadiusMeters != 100 {
		t.Fatalf("unexpected radius_meters %d", items[0].RadiusMeters)
	}
}

func TestReminderCrossUserAccessDenied(t *testing.T) {
	userA := registerAndLoginReminderUser(t, "reminder-a")
	userB := registerAndLoginReminderUser(t, "reminder-b")
	seeded := seedReminder(t, userA.userID, locationReminderInput())

	res := userB.get("/api/v1/reminders/" + seeded.ID)
	if res.Status != http.StatusNotFound && res.Status != http.StatusForbidden {
		t.Fatalf("expected 404 or 403 for cross-user access, got %d (body: %s)", res.Status, string(res.Body))
	}
}

func TestReminderMarkTriggered(t *testing.T) {
	authed := registerAndLoginReminderUser(t, "reminder-trigger")
	seeded := seedReminder(t, authed.userID, locationReminderInput())

	patch := authed.patch("/api/v1/reminders/"+seeded.ID+"/triggered", nil)
	patch.requireStatus(t, http.StatusOK)

	var triggered reminder.Response
	patch.decode(t, &triggered)
	if triggered.Status != reminder.StatusTriggered {
		t.Fatalf("expected status triggered, got %q", triggered.Status)
	}
	if triggered.TriggeredAt == nil || *triggered.TriggeredAt == "" {
		t.Fatal("expected triggered_at to be set")
	}

	get := authed.get("/api/v1/reminders/" + seeded.ID)
	get.requireStatus(t, http.StatusOK)

	var fetched reminder.Response
	get.decode(t, &fetched)
	if fetched.Status != reminder.StatusTriggered {
		t.Fatalf("expected persisted status triggered, got %q", fetched.Status)
	}
	if fetched.TriggeredAt == nil {
		t.Fatal("expected triggered_at on GET after patch")
	}

	again := authed.patch("/api/v1/reminders/"+seeded.ID+"/triggered", nil)
	again.requireStatus(t, http.StatusOK)
}

func TestReminderCancel(t *testing.T) {
	authed := registerAndLoginReminderUser(t, "reminder-cancel")
	seeded := seedReminder(t, authed.userID, locationReminderInput())

	del := authed.delete("/api/v1/reminders/" + seeded.ID)
	if del.Status != http.StatusOK && del.Status != http.StatusNoContent {
		t.Fatalf("expected 200 or 204 on cancel, got %d (body: %s)", del.Status, string(del.Body))
	}

	list := authed.get("/api/v1/reminders?status=pending")
	list.requireStatus(t, http.StatusOK)

	var pending []reminder.Response
	list.decode(t, &pending)
	for _, item := range pending {
		if item.ID == seeded.ID {
			t.Fatal("cancelled reminder should not appear in pending list")
		}
	}

	get := authed.get("/api/v1/reminders/" + seeded.ID)
	get.requireStatus(t, http.StatusOK)

	var cancelled reminder.Response
	get.decode(t, &cancelled)
	if cancelled.Status != reminder.StatusCancelled {
		t.Fatalf("expected status cancelled, got %q", cancelled.Status)
	}
}

func TestReminderRequireAuthentication(t *testing.T) {
	newClient(t).get("/api/v1/reminders").requireStatus(t, http.StatusUnauthorized)
}
