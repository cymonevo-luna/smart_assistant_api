package userplugin

import (
	"context"

	"github.com/cymonevo/go_template/pkg/store"
)

// Repository is the persistence contract for user plugin installs.
type Repository interface {
	store.Store[UserPlugin]
	FindByUserID(ctx context.Context, userID string) ([]UserPlugin, error)
	FindByUserIDAndPluginID(ctx context.Context, userID, pluginID string) (*UserPlugin, error)
}

type repository struct {
	store.Store[UserPlugin]
}

// NewRepository wraps any store.Store[UserPlugin] as a user plugin Repository.
func NewRepository(s store.Store[UserPlugin]) Repository {
	return &repository{Store: s}
}

// FindByUserID returns all plugins installed by a user, newest first.
func (r *repository) FindByUserID(ctx context.Context, userID string) ([]UserPlugin, error) {
	return r.Find(ctx, store.NewQuery().
		Eq("user_id", userID).
		OrderBy("installed_at", true))
}

// FindByUserIDAndPluginID returns a single install row for a user and catalog plugin.
func (r *repository) FindByUserIDAndPluginID(ctx context.Context, userID, pluginID string) (*UserPlugin, error) {
	q := store.NewQuery().Eq("user_id", userID).Eq("plugin_id", pluginID)
	return r.FindOne(ctx, q)
}
