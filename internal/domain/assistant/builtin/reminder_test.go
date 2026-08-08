package builtin

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cymonevo/go_template/internal/domain/reminder"
	"github.com/cymonevo/go_template/pkg/response"
	"github.com/cymonevo/go_template/pkg/store"
)

func TestExecuteReminderCreateListDelete(t *testing.T) {
	svc := reminder.NewService(reminder.NewRepository(store.NewMemoryStore[reminder.Reminder]()))
	ctx := context.Background()

	createResult, err := ExecuteReminder(ctx, svc, "user-1", "install-1", map[string]any{
		"operation": "create",
		"message":   "water plants",
		"remind_at": time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !strings.Contains(createResult["reply_text"].(string), "water plants") {
		t.Fatalf("unexpected create reply: %#v", createResult)
	}

	listResult, err := ExecuteReminder(ctx, svc, "user-1", "", map[string]any{
		"operation": "list",
		"filter":    "all",
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(listResult["reply_text"].(string), "water plants") {
		t.Fatalf("unexpected list reply: %#v", listResult)
	}

	deleteResult, err := ExecuteReminder(ctx, svc, "user-1", "", map[string]any{
		"operation": "delete",
		"message":   "water plants",
	})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !strings.Contains(deleteResult["reply_text"].(string), "water plants") {
		t.Fatalf("unexpected delete reply: %#v", deleteResult)
	}
}

func TestExecuteReminderCreateRejectsMissingFields(t *testing.T) {
	svc := reminder.NewService(reminder.NewRepository(store.NewMemoryStore[reminder.Reminder]()))
	_, err := ExecuteReminder(context.Background(), svc, "user-1", "", map[string]any{
		"operation": "create",
		"remind_at": time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
	})
	if err == nil || !strings.Contains(err.Error(), "message") {
		t.Fatalf("expected message error, got %v", err)
	}
}

func TestExecuteReminderCreateRejectsPastTime(t *testing.T) {
	svc := reminder.NewService(reminder.NewRepository(store.NewMemoryStore[reminder.Reminder]()))
	_, err := ExecuteReminder(context.Background(), svc, "user-1", "", map[string]any{
		"operation": "create",
		"message":   "too late",
		"remind_at": time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	text := ExecutorErrorText(err)
	if !strings.Contains(strings.ToLower(text), "past") {
		t.Fatalf("expected past-time message, got %q", text)
	}
	appErr, ok := err.(*response.AppError)
	if !ok || appErr.Status != 422 {
		t.Fatalf("expected validation error, got %T %v", err, err)
	}
}

func TestParseDateTime(t *testing.T) {
	got, err := ParseDateTime("2026-08-09T14:00:00Z", time.UTC)
	if err != nil {
		t.Fatalf("ParseDateTime: %v", err)
	}
	if got.UTC().Hour() != 14 {
		t.Fatalf("hour = %d", got.Hour())
	}
}
