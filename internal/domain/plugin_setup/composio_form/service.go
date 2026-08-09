package composioform

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/cymonevo/go_template/internal/domain/plugin"
	"github.com/cymonevo/go_template/internal/domain/plugin_credential"
	"github.com/cymonevo/go_template/internal/domain/user_plugin"
	"github.com/cymonevo/go_template/pkg/composio"
	"github.com/cymonevo/go_template/pkg/response"
	"github.com/cymonevo/go_template/pkg/store"
)

const defaultClientTimeout = 15 * time.Second

// ComposioAPI is the subset of the Composio client used during form setup.
type ComposioAPI interface {
	ValidateAPIKey(ctx context.Context) error
	ListConnectedAccounts(ctx context.Context, opts composio.ListConnectedAccountsOpts) ([]composio.ConnectedAccount, error)
}

// ClientFactory builds a Composio client for a user-supplied API key.
type ClientFactory func(apiKey string) ComposioAPI

// Config holds Composio API settings for form setup validation.
type Config struct {
	BaseURL string
	Timeout time.Duration
}

// SubmitResponse is returned after a successful form setup submission.
type SubmitResponse struct {
	SetupStatus            userplugin.SetupStatus `json:"setup_status"`
	ConnectedToolkits      []string               `json:"connected_toolkits"`
	ConnectedAccountsCount int                    `json:"connected_accounts_count"`
}

// SetupStatusResponse reports form setup progress and a non-secret summary.
type SetupStatusResponse struct {
	SetupStatus            userplugin.SetupStatus `json:"setup_status"`
	SetupError             *string                `json:"setup_error"`
	ConnectedToolkits      []string               `json:"connected_toolkits"`
	ConnectedAccountsCount int                    `json:"connected_accounts_count"`
}

// Service orchestrates Composio API-key form plugin setup.
type Service struct {
	cfg            Config
	userPluginRepo userplugin.Repository
	pluginRepo     plugin.Repository
	credentialSvc  *plugincredential.Service
	newClient      ClientFactory
}

// NewService constructs a Composio form setup Service.
func NewService(
	cfg Config,
	userPluginRepo userplugin.Repository,
	pluginRepo plugin.Repository,
	credentialSvc *plugincredential.Service,
	newClient ClientFactory,
) *Service {
	if newClient == nil {
		newClient = defaultClientFactory(cfg)
	}
	return &Service{
		cfg:            cfg,
		userPluginRepo: userPluginRepo,
		pluginRepo:     pluginRepo,
		credentialSvc:  credentialSvc,
		newClient:      newClient,
	}
}

func defaultClientFactory(cfg Config) ClientFactory {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultClientTimeout
	}
	baseURL := cfg.BaseURL
	return func(apiKey string) ComposioAPI {
		return composio.New(composio.Config{
			APIKey:     apiKey,
			BaseURL:    baseURL,
			HTTPClient: &http.Client{Timeout: timeout},
		})
	}
}

// SubmitSetup validates a Composio API key and stores encrypted credentials.
func (s *Service) SubmitSetup(ctx context.Context, userID, userPluginID, apiKey string) (*SubmitResponse, error) {
	install, catalog, err := s.loadOwnedInstall(ctx, userID, userPluginID)
	if err != nil {
		return nil, err
	}

	if err := s.requireFormSetup(catalog); err != nil {
		return nil, err
	}

	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, response.NewValidation(map[string]string{
			"api_key": "must not be empty",
		})
	}

	install.SetupStatus = userplugin.SetupStatusInProgress
	install.SetupError = nil
	install.UpdatedAt = time.Now().UTC()
	if err := s.userPluginRepo.Update(ctx, install.ID, install); err != nil {
		return nil, response.NewInternal("failed to update setup status").Wrap(err)
	}

	client := s.newClient(apiKey)
	if err := client.ValidateAPIKey(ctx); err != nil {
		msg := "invalid Composio API key"
		s.markFailed(ctx, install, msg)
		return nil, response.NewBadRequest(msg).Wrap(err)
	}

	accounts, err := client.ListConnectedAccounts(ctx, composio.ListConnectedAccountsOpts{
		Statuses: []string{"ACTIVE"},
	})
	if err != nil {
		msg := "failed to list connected accounts"
		s.markFailed(ctx, install, msg)
		return nil, response.NewBadRequest(msg).Wrap(err)
	}

	toolkits := uniqueToolkitSlugs(accounts)
	snapshot := toCredentialAccounts(accounts)

	payload := plugincredential.ComposioPayload{
		APIKey:            apiKey,
		BaseURL:           s.cfg.BaseURL,
		ConnectedAccounts: snapshot,
	}
	if err := s.credentialSvc.UpsertComposio(ctx, install.ID, payload); err != nil {
		s.markFailed(ctx, install, "failed to store credentials")
		return nil, err
	}

	installCfg := userplugin.InstallConfig{
		ConnectedToolkits:      toolkits,
		ConnectedAccountsCount: len(accounts),
	}
	configMap, err := installCfg.ToMap()
	if err != nil {
		return nil, response.NewInternal("failed to encode install config").Wrap(err)
	}

	install.Config = configMap
	install.SetupStatus = userplugin.SetupStatusCompleted
	install.SetupError = nil
	install.UpdatedAt = time.Now().UTC()
	if err := s.userPluginRepo.Update(ctx, install.ID, install); err != nil {
		return nil, response.NewInternal("failed to update setup status").Wrap(err)
	}

	return &SubmitResponse{
		SetupStatus:            userplugin.SetupStatusCompleted,
		ConnectedToolkits:      toolkits,
		ConnectedAccountsCount: len(accounts),
	}, nil
}

// GetSetupStatus returns setup progress and a non-secret Composio summary.
func (s *Service) GetSetupStatus(ctx context.Context, userID, userPluginID string) (*SetupStatusResponse, error) {
	install, catalog, err := s.loadOwnedInstall(ctx, userID, userPluginID)
	if err != nil {
		return nil, err
	}

	if err := s.requireFormSetup(catalog); err != nil {
		return nil, err
	}

	cfg, err := userplugin.ParseInstallConfig(install.Config)
	if err != nil {
		return nil, response.NewInternal("failed to parse install config").Wrap(err)
	}

	toolkits := cfg.ConnectedToolkits
	if toolkits == nil {
		toolkits = []string{}
	}

	return &SetupStatusResponse{
		SetupStatus:            install.SetupStatus,
		SetupError:             install.SetupError,
		ConnectedToolkits:      toolkits,
		ConnectedAccountsCount: cfg.ConnectedAccountsCount,
	}, nil
}

func (s *Service) requireFormSetup(catalog *plugin.Plugin) error {
	if catalog.Manifest.SetupType != plugin.SetupTypeForm {
		return response.NewBadRequest("plugin setup is not required for this plugin type")
	}
	return nil
}

func (s *Service) loadOwnedInstall(ctx context.Context, userID, userPluginID string) (*userplugin.UserPlugin, *plugin.Plugin, error) {
	install, err := s.userPluginRepo.FindByID(ctx, userPluginID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil, response.NewNotFound("installed plugin not found")
		}
		return nil, nil, response.NewInternal("failed to load installed plugin").Wrap(err)
	}
	if install.UserID != userID {
		return nil, nil, response.NewForbidden("cannot access another user's plugin")
	}

	catalog, err := s.pluginRepo.FindByID(ctx, install.PluginID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil, response.NewNotFound("plugin not found")
		}
		return nil, nil, response.NewInternal("failed to load plugin").Wrap(err)
	}
	return install, catalog, nil
}

func (s *Service) markFailed(ctx context.Context, install *userplugin.UserPlugin, msg string) {
	install.SetupStatus = userplugin.SetupStatusFailed
	install.SetupError = &msg
	install.UpdatedAt = time.Now().UTC()
	_ = s.userPluginRepo.Update(ctx, install.ID, install)
}

func uniqueToolkitSlugs(accounts []composio.ConnectedAccount) []string {
	seen := make(map[string]struct{}, len(accounts))
	out := make([]string, 0, len(accounts))
	for _, acct := range accounts {
		slug := strings.TrimSpace(acct.ToolkitSlug)
		if slug == "" {
			continue
		}
		if _, ok := seen[slug]; ok {
			continue
		}
		seen[slug] = struct{}{}
		out = append(out, slug)
	}
	return out
}

func toCredentialAccounts(accounts []composio.ConnectedAccount) []plugincredential.ComposioConnectedAccount {
	out := make([]plugincredential.ComposioConnectedAccount, 0, len(accounts))
	for _, acct := range accounts {
		out = append(out, plugincredential.ComposioConnectedAccount{
			ID:          acct.ID,
			ToolkitSlug: acct.ToolkitSlug,
			Status:      acct.Status,
			Alias:       acct.Alias,
		})
	}
	return out
}
