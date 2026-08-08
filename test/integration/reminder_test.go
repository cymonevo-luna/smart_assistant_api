//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/cymonevo/go_template/internal/domain/reminder"
	"github.com/cymonevo/go_template/pkg/logger"
	"github.com/google/uuid"
)

type authedUser struct {
	client *client
	userID string
}

func registerReminderUser(t *testing.T) authedUser {
	t.Helper()
	email := fmt.Sprintf("reminder-%s@integration.test", uuid.NewString())
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
		Tokens struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	login.decode(t, &loginData)
	token := loginData.Tokens.AccessToken
	if token == "" {
		t.Fatal("expected an access token after login")
	}

	claims, err := application.Container().Tokens.Parse(token)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}

	return authedUser{
		client: pub.authed(token),
		userID: claims.UserID,
	}
}

func seedReminder(t *testing.T, userID, message string, remindAt time.Time, status string) *reminder.Reminder {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	rem := &reminder.Reminder{
		ID:        uuid.NewString(),
		UserID:    userID,
		Message:   message,
		RemindAt:  remindAt.UTC(),
		Status:    status,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := application.Container().ReminderRepo.Create(ctx, rem); err != nil {
		t.Fatalf("seed reminder: %v", err)
	}
	return rem
}

func runReminderDispatch(t *testing.T) {
	t.Helper()
	log, err := logger.New("error", false)
	if err != nil {
		t.Fatalf("logger.New: %v", err)
	}
	dispatch := reminder.Dispatch(application.Container().ReminderService, log)
	if err := dispatch(context.Background()); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
}

func TestSchedulerMarksDueReminderNotified(t *testing.T) {
	user := registerReminderUser(t)
	now := time.Now().UTC()

	seed := seedReminder(t, user.userID, "call mom", now.Add(-time.Minute), reminder.StatusPending)
	runReminderDispatch(t)

	ctx := context.Background()
	got, err := application.Container().ReminderRepo.FindByID(ctx, seed.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Status != reminder.StatusNotified {
		t.Fatalf("expected status notified, got %q", got.Status)
	}
	if got.NotifiedAt == nil {
		t.Fatal("expected notified_at to be set")
	}
}

func TestGetPendingNotifications(t *testing.T) {
	owner := registerReminderUser(t)
	other := registerReminderUser(t)
	now := time.Now().UTC()
	notifiedAt := now.Add(-time.Minute)

	seed := seedReminder(t, owner.userID, "pending notify", now.Add(-2*time.Minute), reminder.StatusNotified)
	seed.NotifiedAt = &notifiedAt
	ctx := context.Background()
	if err := application.Container().ReminderRepo.Update(ctx, seed.ID, seed); err != nil {
		t.Fatalf("update seeded reminder: %v", err)
	}

	// Another user's notified reminder should not appear.
	otherSeed := seedReminder(t, other.userID, "other user", now.Add(-time.Minute), reminder.StatusNotified)
	otherSeed.NotifiedAt = &notifiedAt
	if err := application.Container().ReminderRepo.Update(ctx, otherSeed.ID, otherSeed); err != nil {
		t.Fatalf("update other reminder: %v", err)
	}

	res := owner.client.get("/api/v1/users/me/reminders/notifications/pending")
	res.requireStatus(t, http.StatusOK)

	var items []reminder.Response
	res.decode(t, &items)
	if len(items) != 1 {
		t.Fatalf("expected 1 pending notification, got %d", len(items))
	}
	if items[0].ID != seed.ID {
		t.Fatalf("expected reminder %q, got %q", seed.ID, items[0].ID)
	}
	if items[0].Status != reminder.StatusNotified {
		t.Fatalf("expected status notified, got %q", items[0].Status)
	}

	otherRes := other.client.get("/api/v1/users/me/reminders/notifications/pending")
	otherRes.requireStatus(t, http.StatusOK)
	var otherItems []reminder.Response
	otherRes.decode(t, &otherItems)
	if len(otherItems) != 1 || otherItems[0].ID != otherSeed.ID {
		t.Fatalf("expected other user to see only their reminder, got %+v", otherItems)
	}
}

func TestAcknowledgeDelivery(t *testing.T) {
	user := registerReminderUser(t)
	now := time.Now().UTC()
	notifiedAt := now.Add(-time.Minute)

	seed := seedReminder(t, user.userID, "deliver me", now.Add(-2*time.Minute), reminder.StatusNotified)
	seed.NotifiedAt = &notifiedAt
	ctx := context.Background()
	if err := application.Container().ReminderRepo.Update(ctx, seed.ID, seed); err != nil {
		t.Fatalf("update seeded reminder: %v", err)
	}

	delivered := user.client.post(fmt.Sprintf("/api/v1/users/me/reminders/%s/delivered", seed.ID), nil)
	delivered.requireStatus(t, http.StatusNoContent)

	pending := user.client.get("/api/v1/users/me/reminders/notifications/pending")
	pending.requireStatus(t, http.StatusOK)
	var items []reminder.Response
	pending.decode(t, &items)
	if len(items) != 0 {
		t.Fatalf("expected empty pending list after delivery ack, got %d", len(items))
	}
}

func TestGetRemindersWithTodayFilter(t *testing.T) {
	user := registerReminderUser(t)
	now := time.Now().UTC()
	todayStart, todayEnd := utcDayBounds(now)
	tomorrowStart := todayEnd

	seedReminder(t, user.userID, "today reminder", todayStart.Add(2*time.Hour), reminder.StatusPending)
	seedReminder(t, user.userID, "tomorrow reminder", tomorrowStart.Add(3*time.Hour), reminder.StatusPending)

	res := user.client.get("/api/v1/users/me/reminders?filter=today")
	res.requireStatus(t, http.StatusOK)

	var items []reminder.Response
	res.decode(t, &items)
	if len(items) != 1 {
		t.Fatalf("expected 1 today reminder, got %d", len(items))
	}
	if items[0].Message != "today reminder" {
		t.Fatalf("expected today reminder, got %q", items[0].Message)
	}
}

func TestFutureReminderNotDispatched(t *testing.T) {
	user := registerReminderUser(t)
	now := time.Now().UTC()

	seed := seedReminder(t, user.userID, "future", now.Add(time.Hour), reminder.StatusPending)
	runReminderDispatch(t)

	ctx := context.Background()
	got, err := application.Container().ReminderRepo.FindByID(ctx, seed.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Status != reminder.StatusPending {
		t.Fatalf("expected status pending, got %q", got.Status)
	}
	if got.NotifiedAt != nil {
		t.Fatal("expected notified_at to remain unset")
	}
}

func utcDayBounds(day time.Time) (start, end time.Time) {
	y, m, d := day.UTC().Date()
	start = time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	end = start.Add(24 * time.Hour)
	return start, end
}
