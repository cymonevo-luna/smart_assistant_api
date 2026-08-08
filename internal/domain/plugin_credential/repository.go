package plugincredential

import (
	"context"
	"errors"

	"github.com/cymonevo/go_template/pkg/store"
)

// Repository is the persistence contract for plugin credentials.
type Repository interface {
	store.Store[Credential]
	FindByUserPluginID(ctx context.Context, userPluginID string) (*Credential, error)
	DeleteByUserPluginID(ctx context.Context, userPluginID string) error
}

type repository struct {
	store.Store[Credential]
}

// NewRepository wraps any store.Store[Credential] as a plugin credential Repository.
func NewRepository(s store.Store[Credential]) Repository {
	return &repository{Store: s}
}

// FindByUserPluginID returns the credential row for a user plugin install.
func (r *repository) FindByUserPluginID(ctx context.Context, userPluginID string) (*Credential, error) {
	return r.FindOne(ctx, store.NewQuery().Eq("user_plugin_id", userPluginID))
}

// DeleteByUserPluginID removes credentials for a user plugin install.
func (r *repository) DeleteByUserPluginID(ctx context.Context, userPluginID string) error {
	cred, err := r.FindByUserPluginID(ctx, userPluginID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return err
	}
	return r.Delete(ctx, cred.ID)
}
