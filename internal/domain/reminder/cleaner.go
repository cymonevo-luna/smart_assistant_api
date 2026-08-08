package reminder

import "context"

// Cleaner adapts reminder cancellation to userplugin.ReminderCleaner.
type Cleaner struct {
	svc *Service
}

// NewCleaner builds a ReminderCleaner backed by the reminder service.
func NewCleaner(svc *Service) *Cleaner {
	return &Cleaner{svc: svc}
}

// CancelAllForUserPlugin cancels pending reminders for a user's plugin install.
func (c *Cleaner) CancelAllForUserPlugin(ctx context.Context, userID, userPluginID string) error {
	return c.svc.CancelAllForUserPlugin(ctx, userID, userPluginID)
}
