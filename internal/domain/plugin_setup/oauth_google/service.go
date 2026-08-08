package oauthgoogle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cymonevo/go_template/internal/domain/plugin"
	"github.com/cymonevo/go_template/internal/domain/plugin_credential"
	"github.com/cymonevo/go_template/internal/domain/user_plugin"
	"github.com/cymonevo/go_template/pkg/response"
	"github.com/cymonevo/go_template/pkg/store"
	"github.com/golang-jwt/jwt/v5"
)

const (
	googleAuthURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL = "https://oauth2.googleapis.com/token"
	statePurpose   = "plugin_oauth_google"
	stateTTL       = 10 * time.Minute
	googleScopes   = "openid email profile https://www.googleapis.com/auth/calendar"
)

// Config holds Google OAuth client settings.
type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	TokenURL     string
}

// TokenResponse is the Google OAuth token endpoint response.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

// TokenExchanger exchanges an authorization code for tokens.
type TokenExchanger interface {
	Exchange(ctx context.Context, code string) (*TokenResponse, error)
}

// HTTPClient is the default TokenExchanger using Google's token endpoint.
type HTTPClient struct {
	cfg    Config
	client *http.Client
}

// NewHTTPClient builds a TokenExchanger backed by net/http.
func NewHTTPClient(cfg Config, client *http.Client) *HTTPClient {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPClient{cfg: cfg, client: client}
}

// Exchange posts the authorization code to Google and returns tokens.
func (c *HTTPClient) Exchange(ctx context.Context, code string) (*TokenResponse, error) {
	tokenURL := c.cfg.TokenURL
	if tokenURL == "" {
		tokenURL = googleTokenURL
	}
	body := url.Values{
		"client_id":     {c.cfg.ClientID},
		"client_secret": {c.cfg.ClientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {c.cfg.RedirectURL},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(body.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google token exchange failed: status %d", resp.StatusCode)
	}

	var tok TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return nil, err
	}
	if tok.RefreshToken == "" {
		return nil, errors.New("google token response missing refresh_token")
	}
	return &tok, nil
}

// SetupInitResponse is returned when OAuth setup is started.
type SetupInitResponse struct {
	AuthorizationURL string `json:"authorization_url"`
	State            string `json:"state"`
}

// SetupStatusResponse reports plugin setup progress.
type SetupStatusResponse struct {
	SetupStatus userplugin.SetupStatus `json:"setup_status"`
	SetupError  *string                `json:"setup_error"`
}

type stateClaims struct {
	UserID       string `json:"uid"`
	UserPluginID string `json:"upid"`
	Purpose      string `json:"pur"`
	jwt.RegisteredClaims
}

// Service orchestrates Google OAuth plugin setup.
type Service struct {
	cfg            Config
	stateSecret    []byte
	userPluginRepo userplugin.Repository
	pluginRepo     plugin.Repository
	credentialSvc  *plugincredential.Service
	exchanger      TokenExchanger
}

// NewService constructs a Google OAuth setup Service.
func NewService(
	cfg Config,
	stateSecret string,
	userPluginRepo userplugin.Repository,
	pluginRepo plugin.Repository,
	credentialSvc *plugincredential.Service,
	exchanger TokenExchanger,
) *Service {
	return &Service{
		cfg:            cfg,
		stateSecret:    []byte(stateSecret),
		userPluginRepo: userPluginRepo,
		pluginRepo:     pluginRepo,
		credentialSvc:  credentialSvc,
		exchanger:      exchanger,
	}
}

// InitSetup starts Google OAuth for an installed plugin.
func (s *Service) InitSetup(ctx context.Context, userID, userPluginID string) (*SetupInitResponse, error) {
	install, catalog, err := s.loadOwnedInstall(ctx, userID, userPluginID)
	if err != nil {
		return nil, err
	}

	if catalog.Manifest.SetupType != plugin.SetupTypeOAuthGoogle {
		return nil, response.NewBadRequest("plugin setup is not required for this plugin type")
	}

	state, err := s.signState(userID, userPluginID)
	if err != nil {
		return nil, response.NewInternal("failed to generate oauth state").Wrap(err)
	}

	install.SetupStatus = userplugin.SetupStatusInProgress
	install.SetupError = nil
	install.UpdatedAt = time.Now().UTC()
	if err := s.userPluginRepo.Update(ctx, install.ID, install); err != nil {
		return nil, response.NewInternal("failed to update setup status").Wrap(err)
	}

	authURL, err := s.buildAuthURL(state)
	if err != nil {
		return nil, response.NewInternal("failed to build authorization url").Wrap(err)
	}

	return &SetupInitResponse{AuthorizationURL: authURL, State: state}, nil
}

// HandleCallback completes OAuth after Google redirects back.
func (s *Service) HandleCallback(ctx context.Context, state, code string) error {
	claims, err := s.parseState(state)
	if err != nil {
		return response.NewBadRequest("invalid or expired oauth state").Wrap(err)
	}

	install, catalog, err := s.loadOwnedInstall(ctx, claims.UserID, claims.UserPluginID)
	if err != nil {
		return err
	}

	if catalog.Manifest.SetupType != plugin.SetupTypeOAuthGoogle {
		return response.NewBadRequest("plugin setup is not required for this plugin type")
	}

	tok, err := s.exchanger.Exchange(ctx, code)
	if err != nil {
		s.markFailed(ctx, install, err.Error())
		return response.NewBadRequest("failed to exchange authorization code").Wrap(err)
	}

	var expiresAt *time.Time
	if tok.ExpiresIn > 0 {
		t := time.Now().UTC().Add(time.Duration(tok.ExpiresIn) * time.Second)
		expiresAt = &t
	}

	payload := plugincredential.TokenPayload{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		TokenType:    tok.TokenType,
		ExpiresAt:    expiresAt,
	}
	if err := s.credentialSvc.Upsert(ctx, install.ID, plugincredential.ProviderGoogle, payload); err != nil {
		s.markFailed(ctx, install, "failed to store credentials")
		return err
	}

	install.SetupStatus = userplugin.SetupStatusCompleted
	install.SetupError = nil
	install.UpdatedAt = time.Now().UTC()
	if err := s.userPluginRepo.Update(ctx, install.ID, install); err != nil {
		return response.NewInternal("failed to update setup status").Wrap(err)
	}
	return nil
}

// GetSetupStatus returns setup progress for an installed plugin.
func (s *Service) GetSetupStatus(ctx context.Context, userID, userPluginID string) (*SetupStatusResponse, error) {
	install, _, err := s.loadOwnedInstall(ctx, userID, userPluginID)
	if err != nil {
		return nil, err
	}
	return &SetupStatusResponse{
		SetupStatus: install.SetupStatus,
		SetupError:  install.SetupError,
	}, nil
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

func (s *Service) buildAuthURL(state string) (string, error) {
	u, err := url.Parse(googleAuthURL)
	if err != nil {
		return "", err
	}
	q := url.Values{
		"client_id":     {s.cfg.ClientID},
		"redirect_uri":  {s.cfg.RedirectURL},
		"response_type": {"code"},
		"scope":         {googleScopes},
		"access_type":   {"offline"},
		"prompt":        {"consent"},
		"state":         {state},
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (s *Service) signState(userID, userPluginID string) (string, error) {
	now := time.Now()
	claims := stateClaims{
		UserID:       userID,
		UserPluginID: userPluginID,
		Purpose:      statePurpose,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(stateTTL)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.stateSecret)
}

func (s *Service) parseState(token string) (*stateClaims, error) {
	claims := &stateClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.stateSecret, nil
	})
	if err != nil {
		return nil, err
	}
	if !parsed.Valid || claims.Purpose != statePurpose {
		return nil, errors.New("invalid state token")
	}
	return claims, nil
}
