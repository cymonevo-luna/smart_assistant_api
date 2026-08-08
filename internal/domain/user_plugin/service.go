package userplugin

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/cymonevo/go_template/internal/domain/plugin"
	"github.com/cymonevo/go_template/pkg/response"
	"github.com/cymonevo/go_template/pkg/store"
	"github.com/google/uuid"
)

// CredentialsCleaner removes stored credentials when a plugin is uninstalled.
// A later sub-ticket will wire the credentials table; this hook keeps uninstall
// ready for cascade cleanup without a hard FK dependency today.
type CredentialsCleaner interface {
	DeleteForUserPlugin(ctx context.Context, userID, pluginID string) error
}

// noopCredentialsCleaner is the default until credentials persistence exists.
type noopCredentialsCleaner struct{}

func (noopCredentialsCleaner) DeleteForUserPlugin(context.Context, string, string) error {
	return nil
}

// Service holds business logic for per-user plugin installs.
type Service struct {
	repo        Repository
	pluginRepo  plugin.Repository
	credentials CredentialsCleaner
}

// NewService constructs a user plugin Service.
func NewService(repo Repository, pluginRepo plugin.Repository, credentials CredentialsCleaner) *Service {
	if credentials == nil {
		credentials = noopCredentialsCleaner{}
	}
	return &Service{repo: repo, pluginRepo: pluginRepo, credentials: credentials}
}

// ListInstalled returns the caller's installed plugins joined with catalog display fields.
func (s *Service) ListInstalled(ctx context.Context, userID string) ([]InstalledResponse, error) {
	installs, err := s.repo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, response.NewInternal("failed to list installed plugins").Wrap(err)
	}
	if len(installs) == 0 {
		return []InstalledResponse{}, nil
	}

	pluginIDs := make([]string, 0, len(installs))
	for i := range installs {
		pluginIDs = append(pluginIDs, installs[i].PluginID)
	}

	plugins, err := s.pluginRepo.Find(ctx, store.NewQuery().In("id", pluginIDs))
	if err != nil {
		return nil, response.NewInternal("failed to load plugin catalog").Wrap(err)
	}

	pluginByID := make(map[string]*plugin.Plugin, len(plugins))
	for i := range plugins {
		pluginByID[plugins[i].ID] = &plugins[i]
	}

	return ToInstalledResponses(installs, pluginByID), nil
}

// Install adds a catalog plugin to the caller's assistant. Idempotent when already installed.
func (s *Service) Install(ctx context.Context, userID, pluginSlug string) (*InstalledResponse, error) {
	slug := strings.ToLower(strings.TrimSpace(pluginSlug))
	if slug == "" {
		return nil, response.NewValidation(map[string]string{
			"plugin_slug": "must not be empty",
		})
	}

	catalog, err := s.pluginRepo.FindBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, response.NewNotFound("plugin not found")
		}
		return nil, response.NewInternal("failed to load plugin").Wrap(err)
	}

	existing, err := s.repo.FindByUserIDAndPluginID(ctx, userID, catalog.ID)
	if err == nil {
		resp := ToInstalledResponse(existing, catalog)
		return &resp, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, response.NewInternal("failed to verify install state").Wrap(err)
	}

	setupStatus := SetupStatusCompleted
	if catalog.Manifest.RequiredSetup {
		setupStatus = SetupStatusNotStarted
	}

	now := time.Now().UTC()
	install := &UserPlugin{
		ID:          uuid.NewString(),
		UserID:      userID,
		PluginID:    catalog.ID,
		Enabled:     true,
		SetupStatus: setupStatus,
		Config:      map[string]any{},
		InstalledAt: now,
		UpdatedAt:   now,
	}

	if err := s.repo.Create(ctx, install); err != nil {
		// Concurrent install: return the row that won the unique index race.
		existing, findErr := s.repo.FindByUserIDAndPluginID(ctx, userID, catalog.ID)
		if findErr == nil {
			resp := ToInstalledResponse(existing, catalog)
			return &resp, nil
		}
		return nil, response.NewInternal("failed to install plugin").Wrap(err)
	}

	resp := ToInstalledResponse(install, catalog)
	return &resp, nil
}

// Uninstall removes a plugin from the caller's assistant and cleans up credentials.
func (s *Service) Uninstall(ctx context.Context, userID, installID string) error {
	install, err := s.repo.FindByID(ctx, installID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return response.NewNotFound("installed plugin not found")
		}
		return response.NewInternal("failed to load installed plugin").Wrap(err)
	}
	if install.UserID != userID {
		return response.NewForbidden("cannot modify another user's plugin")
	}

	if err := s.credentials.DeleteForUserPlugin(ctx, userID, install.PluginID); err != nil {
		return response.NewInternal("failed to clean up plugin credentials").Wrap(err)
	}

	if err := s.repo.Delete(ctx, installID); err != nil {
		return response.NewInternal("failed to uninstall plugin").Wrap(err)
	}
	return nil
}

// SetEnabled toggles whether an installed plugin is active for the caller.
func (s *Service) SetEnabled(ctx context.Context, userID, installID string, enabled bool) (*InstalledResponse, error) {
	install, err := s.repo.FindByID(ctx, installID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, response.NewNotFound("installed plugin not found")
		}
		return nil, response.NewInternal("failed to load installed plugin").Wrap(err)
	}
	if install.UserID != userID {
		return nil, response.NewForbidden("cannot modify another user's plugin")
	}

	catalog, err := s.pluginRepo.FindByID(ctx, install.PluginID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, response.NewNotFound("plugin not found")
		}
		return nil, response.NewInternal("failed to load plugin").Wrap(err)
	}

	install.Enabled = enabled
	install.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(ctx, installID, install); err != nil {
		return nil, response.NewInternal("failed to update installed plugin").Wrap(err)
	}

	resp := ToInstalledResponse(install, catalog)
	return &resp, nil
}
