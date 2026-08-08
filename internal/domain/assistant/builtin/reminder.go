package builtin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cymonevo/go_template/internal/domain/reminder"
	"github.com/cymonevo/go_template/pkg/response"
)

// ReminderSlug is the catalog slug for the reminder plugin.
const ReminderSlug = "reminder"

// AdapterReminder is the executor config key for the reminder builtin adapter.
const AdapterReminder = "reminder"

// ExecuteReminder runs create, list, or delete reminder operations.
func ExecuteReminder(ctx context.Context, svc *reminder.Service, userID, installID string, args map[string]any) (map[string]any, error) {
	operation := stringArg(args, "operation")
	if operation == "" {
		operation = "create"
	}

	switch operation {
	case "create":
		return executeReminderCreate(ctx, svc, userID, installID, args)
	case "list":
		return executeReminderList(ctx, svc, userID, args)
	case "delete":
		return executeReminderDelete(ctx, svc, userID, args)
	default:
		return nil, fmt.Errorf("unsupported reminder operation %q", operation)
	}
}

func executeReminderCreate(ctx context.Context, svc *reminder.Service, userID, installID string, args map[string]any) (map[string]any, error) {
	message := stringArg(args, "message")
	if message == "" {
		return nil, fmt.Errorf("message is required")
	}

	rawAt := stringArg(args, "remind_at")
	if rawAt == "" {
		return nil, fmt.Errorf("remind_at is required")
	}

	remindAt, err := ParseDateTime(rawAt, time.UTC)
	if err != nil {
		return nil, err
	}

	var userPluginID *string
	if installID != "" {
		userPluginID = &installID
	}

	created, err := svc.Create(ctx, userID, userPluginID, message, remindAt)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"reply_text":  fmt.Sprintf("Reminder set for %s: %s", formatReminderTime(created.RemindAt), message),
		"reminder_id": created.ID,
	}, nil
}

func executeReminderList(ctx context.Context, svc *reminder.Service, userID string, args map[string]any) (map[string]any, error) {
	filter := reminder.ListFilter(stringArg(args, "filter"))
	if filter == "" {
		filter = reminder.ListFilterToday
	}

	items, err := svc.List(ctx, userID, filter)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"reply_text": formatReminderList(items, filter),
	}, nil
}

func executeReminderDelete(ctx context.Context, svc *reminder.Service, userID string, args map[string]any) (map[string]any, error) {
	message := stringArg(args, "message")
	if message == "" {
		return nil, fmt.Errorf("message is required")
	}

	if err := svc.DeleteByMessage(ctx, userID, message); err != nil {
		return nil, err
	}

	return map[string]any{
		"reply_text": fmt.Sprintf("Deleted reminder: %s", message),
	}, nil
}

func formatReminderList(items []reminder.Reminder, filter reminder.ListFilter) string {
	label := reminderFilterLabel(filter)
	if len(items) == 0 {
		return fmt.Sprintf("You have no reminders for %s.", label)
	}

	var b strings.Builder
	for i := range items {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%d. %s at %s", i+1, items[i].Message, formatReminderTime(items[i].RemindAt))
	}
	return b.String()
}

func reminderFilterLabel(filter reminder.ListFilter) string {
	switch filter {
	case reminder.ListFilterTomorrow:
		return "tomorrow"
	case reminder.ListFilterAll:
		return "all dates"
	default:
		return "today"
	}
}

func formatReminderTime(t time.Time) string {
	return t.UTC().Format("3:04 PM") + " UTC"
}

// FormatRemindAtForConfirmation formats a remind_at value for confirmation prompts.
func FormatRemindAtForConfirmation(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "the requested time"
	}

	t, err := ParseDateTime(raw, time.UTC)
	if err != nil {
		return raw
	}

	now := time.Now().UTC()
	day := t.UTC().Format("January 2, 2006")
	today := now.Format("January 2, 2006")
	tomorrow := now.Add(24 * time.Hour).Format("January 2, 2006")

	clock := t.UTC().Format("3:04 PM")
	switch day {
	case today:
		return clock + " today"
	case tomorrow:
		return clock + " tomorrow"
	default:
		return clock + " on " + day
	}
}

// ExecutorErrorText returns a user-facing message for executor failures.
func ExecutorErrorText(err error) string {
	var appErr *response.AppError
	if !errors.As(err, &appErr) {
		return "I tried to complete that action, but something went wrong. Please try again later."
	}

	if len(appErr.Fields) > 0 {
		parts := make([]string, 0, len(appErr.Fields))
		for field, msg := range appErr.Fields {
			switch field {
			case "remind_at":
				if msg == "must be in the future" {
					parts = append(parts, "That reminder time is in the past. Please choose a future time.")
					continue
				}
			case "message":
				if msg == "must not be empty" {
					parts = append(parts, "Please tell me what to remind you about.")
					continue
				}
			}
			parts = append(parts, msg)
		}
		if len(parts) == 1 {
			return parts[0]
		}
		if len(parts) > 0 {
			return strings.Join(parts, " ")
		}
	}

	switch appErr.Code {
	case "not_found":
		return "I couldn't find a matching reminder."
	case "conflict":
		return appErr.Message
	default:
		if appErr.Message != "" && appErr.Message != "validation failed" {
			return appErr.Message
		}
	}

	return "I tried to complete that action, but something went wrong. Please try again later."
}
