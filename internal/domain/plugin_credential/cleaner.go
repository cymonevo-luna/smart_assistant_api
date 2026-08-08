package plugincredential

import (
	"context"
	"errors"

	"github.com/cymonevo/go_template/internal/domain/user_plugin"
	"github.com/cymonevo/go_template/pkg/store"
)

// Cleaner adapts credential deletion to userplugin.CredentialsCleaner.
type Cleaner struct {
	credSvc        *Service
	userPluginRepo userplugin.Repository
}

// NewCleaner builds a CredentialsCleaner backed by plugin credentials.
func NewCleaner(credSvc *Service, userPluginRepo userplugin.Repository) *Cleaner {
	return &Cleaner{credSvc: credSvc, userPluginRepo: userPluginRepo}
}

// DeleteForUserPlugin removes credentials for a user's catalog plugin install.
func (c *Cleaner) DeleteForUserPlugin(ctx context.Context, userID, pluginID string) error {
	install, err := c.userPluginRepo.FindByUserIDAndPluginID(ctx, userID, pluginID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return err
	}
	return c.credSvc.DeleteByUserPluginID(ctx, install.ID)
}
