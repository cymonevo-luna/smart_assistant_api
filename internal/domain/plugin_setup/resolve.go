package pluginsetup

import (
	"context"
	"errors"

	"github.com/cymonevo/go_template/internal/domain/plugin"
	"github.com/cymonevo/go_template/internal/domain/user_plugin"
	"github.com/cymonevo/go_template/pkg/response"
	"github.com/cymonevo/go_template/pkg/store"
)

// LoadOwnedInstall returns the caller's install row and its catalog plugin.
func LoadOwnedInstall(
	ctx context.Context,
	userID, userPluginID string,
	userPluginRepo userplugin.Repository,
	pluginRepo plugin.Repository,
) (*userplugin.UserPlugin, *plugin.Plugin, error) {
	install, err := userPluginRepo.FindByID(ctx, userPluginID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil, response.NewNotFound("installed plugin not found")
		}
		return nil, nil, response.NewInternal("failed to load installed plugin").Wrap(err)
	}
	if install.UserID != userID {
		return nil, nil, response.NewForbidden("cannot access another user's plugin")
	}

	catalog, err := pluginRepo.FindByID(ctx, install.PluginID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil, response.NewNotFound("plugin not found")
		}
		return nil, nil, response.NewInternal("failed to load plugin").Wrap(err)
	}
	return install, catalog, nil
}
