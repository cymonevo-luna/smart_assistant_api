package plugincredential

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/cymonevo/go_template/pkg/crypto"
	"github.com/cymonevo/go_template/pkg/response"
	"github.com/cymonevo/go_template/pkg/store"
	"github.com/google/uuid"
)

// TokenPayload is the decrypted credential payload for OAuth providers.
type TokenPayload struct {
	AccessToken  string     `json:"access_token,omitempty"`
	RefreshToken string     `json:"refresh_token,omitempty"`
	TokenType    string     `json:"token_type,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
}

// Service manages encrypted plugin credentials.
type Service struct {
	repo      Repository
	encryptor *crypto.Encryptor
}

// NewService constructs a plugin credential Service.
func NewService(repo Repository, encryptor *crypto.Encryptor) *Service {
	return &Service{repo: repo, encryptor: encryptor}
}

// Upsert stores encrypted OAuth tokens for a user plugin install.
func (s *Service) Upsert(ctx context.Context, userPluginID, provider string, payload TokenPayload) error {
	plain, err := json.Marshal(payload)
	if err != nil {
		return response.NewInternal("failed to encode credentials").Wrap(err)
	}

	encrypted, err := s.encryptor.Encrypt(string(plain))
	if err != nil {
		return response.NewInternal("failed to encrypt credentials").Wrap(err)
	}

	now := time.Now().UTC()
	existing, err := s.repo.FindByUserPluginID(ctx, userPluginID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return response.NewInternal("failed to load credentials").Wrap(err)
	}

	if existing != nil {
		existing.EncryptedPayload = encrypted
		existing.ExpiresAt = payload.ExpiresAt
		existing.UpdatedAt = now
		if err := s.repo.Update(ctx, existing.ID, existing); err != nil {
			return response.NewInternal("failed to update credentials").Wrap(err)
		}
		return nil
	}

	cred := &Credential{
		ID:               uuid.NewString(),
		UserPluginID:     userPluginID,
		Provider:         provider,
		EncryptedPayload: encrypted,
		ExpiresAt:        payload.ExpiresAt,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.repo.Create(ctx, cred); err != nil {
		return response.NewInternal("failed to store credentials").Wrap(err)
	}
	return nil
}

// DeleteByUserPluginID removes credentials for a specific install row.
func (s *Service) DeleteByUserPluginID(ctx context.Context, userPluginID string) error {
	if err := s.repo.DeleteByUserPluginID(ctx, userPluginID); err != nil {
		return response.NewInternal("failed to delete credentials").Wrap(err)
	}
	return nil
}
