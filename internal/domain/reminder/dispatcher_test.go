package reminder

import (
	"context"
	"testing"
	"time"

	"github.com/cymonevo/go_template/pkg/logger"
	"github.com/cymonevo/go_template/pkg/response"
)

func TestDispatchMarksDueRemindersNotified(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	pastID := "past-1"
	futureID := "future-1"
	repo.items[pastID] = &Reminder{
		ID:       pastID,
		UserID:   "user-1",
		Message:  "call mom",
		RemindAt: now.Add(-time.Minute),
		Status:   StatusPending,
	}
	repo.items[futureID] = &Reminder{
		ID:       futureID,
		UserID:   "user-1",
		Message:  "later",
		RemindAt: now.Add(time.Hour),
		Status:   StatusPending,
	}

	log, err := logger.New("error", false)
	if err != nil {
		t.Fatalf("logger.New: %v", err)
	}
	dispatch := Dispatch(svc, log)
	if err := dispatch(ctx); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	got := repo.items[pastID]
	if got.Status != StatusNotified {
		t.Fatalf("expected past reminder notified, got %q", got.Status)
	}
	if got.NotifiedAt == nil {
		t.Fatal("expected notified_at to be set")
	}

	future := repo.items[futureID]
	if future.Status != StatusPending {
		t.Fatalf("expected future reminder pending, got %q", future.Status)
	}
}

func TestDispatchContinuesOnIndividualErrors(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	repo.items["due-1"] = &Reminder{
		ID:       "due-1",
		UserID:   "user-1",
		Message:  "one",
		RemindAt: now.Add(-time.Minute),
		Status:   StatusPending,
	}
	repo.items["already-notified"] = &Reminder{
		ID:       "already-notified",
		UserID:   "user-1",
		Message:  "two",
		RemindAt: now.Add(-2 * time.Minute),
		Status:   StatusNotified,
	}
	repo.items["due-2"] = &Reminder{
		ID:       "due-2",
		UserID:   "user-1",
		Message:  "three",
		RemindAt: now.Add(-30 * time.Second),
		Status:   StatusPending,
	}

	log, err := logger.New("error", false)
	if err != nil {
		t.Fatalf("logger.New: %v", err)
	}
	if err := Dispatch(svc, log)(ctx); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if repo.items["due-1"].Status != StatusNotified {
		t.Fatal("expected due-1 notified")
	}
	if repo.items["due-2"].Status != StatusNotified {
		t.Fatal("expected due-2 notified despite earlier failure")
	}
}

func TestService_FindDueAndMarkNotified(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	repo.items["due"] = &Reminder{
		ID:       "due",
		UserID:   "user-1",
		Message:  "due",
		RemindAt: now.Add(-time.Minute),
		Status:   StatusPending,
	}

	due, err := svc.FindDue(ctx, now)
	if err != nil {
		t.Fatalf("FindDue: %v", err)
	}
	if len(due) != 1 || due[0].ID != "due" {
		t.Fatalf("expected one due reminder, got %+v", due)
	}

	if err := svc.MarkNotified(ctx, "due"); err != nil {
		t.Fatalf("MarkNotified: %v", err)
	}
	if repo.items["due"].Status != StatusNotified || repo.items["due"].NotifiedAt == nil {
		t.Fatal("expected reminder marked notified")
	}

	err = svc.MarkNotified(ctx, "due")
	if err == nil {
		t.Fatal("expected conflict when marking non-pending reminder")
	}
	if appErr, ok := err.(*response.AppError); !ok || appErr.Status != 409 {
		t.Fatalf("expected 409, got %v", err)
	}
}

func TestService_ListPendingDeliveryAndMarkDelivered(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	now := time.Now().UTC()
	notifiedAt := now.Add(-time.Minute)

	repo.items["pending-delivery"] = &Reminder{
		ID:         "pending-delivery",
		UserID:     "user-1",
		Message:    "notify me",
		RemindAt:   now.Add(-2 * time.Minute),
		Status:     StatusNotified,
		NotifiedAt: &notifiedAt,
	}
	repo.items["delivered"] = &Reminder{
		ID:          "delivered",
		UserID:      "user-1",
		Message:     "done",
		RemindAt:    now.Add(-3 * time.Minute),
		Status:      StatusNotified,
		NotifiedAt:  &notifiedAt,
		DeliveredAt: &now,
	}
	repo.items["other-user"] = &Reminder{
		ID:         "other-user",
		UserID:     "user-2",
		Message:    "not mine",
		RemindAt:   now.Add(-time.Minute),
		Status:     StatusNotified,
		NotifiedAt: &notifiedAt,
	}

	items, err := svc.ListPendingDelivery(ctx, "user-1")
	if err != nil {
		t.Fatalf("ListPendingDelivery: %v", err)
	}
	if len(items) != 1 || items[0].ID != "pending-delivery" {
		t.Fatalf("expected one pending delivery, got %+v", items)
	}

	if err := svc.MarkDelivered(ctx, "user-1", "pending-delivery"); err != nil {
		t.Fatalf("MarkDelivered: %v", err)
	}
	if repo.items["pending-delivery"].DeliveredAt == nil {
		t.Fatal("expected delivered_at set")
	}

	err = svc.MarkDelivered(ctx, "user-2", "pending-delivery")
	if err == nil {
		t.Fatal("expected forbidden for other user")
	}
	if appErr, ok := err.(*response.AppError); !ok || appErr.Status != 403 {
		t.Fatalf("expected 403, got %v", err)
	}
}
