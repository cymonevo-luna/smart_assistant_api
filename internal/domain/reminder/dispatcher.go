package reminder

import (
	"context"
	"time"

	"github.com/cymonevo/go_template/pkg/logger"
)

// Dispatch returns a scheduler task that finds due reminders and marks them notified.
// Errors for individual reminders are logged and do not block other dispatches.
func Dispatch(svc *Service, log logger.Logger) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		now := time.Now().UTC()
		due, err := svc.FindDue(ctx, now)
		if err != nil {
			return err
		}
		for _, rem := range due {
			if err := svc.MarkNotified(ctx, rem.ID); err != nil {
				log.Error("failed to mark reminder notified",
					logger.String("reminder_id", rem.ID),
					logger.Err(err))
				continue
			}
			log.Info("dispatched reminder",
				logger.String("reminder_id", rem.ID),
				logger.String("user_id", rem.UserID),
				logger.String("message", rem.Message))
		}
		return nil
	}
}
