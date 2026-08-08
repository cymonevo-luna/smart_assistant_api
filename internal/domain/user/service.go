package user

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/cymonevo/go_template/pkg/auth"
	"github.com/cymonevo/go_template/pkg/cache"
	"github.com/cymonevo/go_template/pkg/logger"
	"github.com/cymonevo/go_template/pkg/queue"
	"github.com/cymonevo/go_template/pkg/response"
	"github.com/cymonevo/go_template/pkg/store"
	"github.com/cymonevo/go_template/pkg/worker"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// Service holds the business logic for users. It depends only on abstractions
// (Repository, cache.Typed, TxManager, Queue, TokenManager, Logger), keeping it
// decoupled from any concrete database, cache, broker, or transport.
type Service struct {
	repo   Repository
	cache  *cache.Typed[User]
	tx     store.TxManager
	queue  queue.Queue
	tokens *auth.TokenManager
	log    logger.Logger
}

// NewService constructs a user Service.
func NewService(
	repo Repository,
	c *cache.Typed[User],
	tx store.TxManager,
	q queue.Queue,
	tokens *auth.TokenManager,
	log logger.Logger,
) *Service {
	return &Service{repo: repo, cache: c, tx: tx, queue: q, tokens: tokens, log: log}
}

func cacheKey(id string) string { return "user:" + id }

// Create registers a new user. The insert runs inside a transaction to
// demonstrate the unit-of-work pattern; the domain event is published only
// after the transaction commits successfully.
func (s *Service) Create(ctx context.Context, in CreateUserInput) (*User, error) {
	if existing, err := s.repo.FindByEmail(ctx, in.Email); err == nil && existing != nil {
		return nil, response.NewConflict("a user with this email already exists")
	} else if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, response.NewInternal("failed to verify email").Wrap(err)
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, response.NewInternal("failed to hash password").Wrap(err)
	}

	now := time.Now().UTC()
	u := &User{
		ID:        uuid.NewString(),
		Email:     strings.ToLower(strings.TrimSpace(in.Email)),
		Name:      in.Name,
		Password:  string(hashed),
		Role:      auth.RoleUser,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.tx.Do(ctx, func(ctx context.Context) error {
		return s.repo.Create(ctx, u)
	}); err != nil {
		return nil, response.NewInternal("failed to create user").Wrap(err)
	}

	s.publishCreated(ctx, u)
	s.log.Info("user created", logger.String("user_id", u.ID))
	return u, nil
}

func (s *Service) publishCreated(ctx context.Context, u *User) {
	evt := UserCreatedEvent{UserID: u.ID, Email: u.Email, Name: u.Name, CreatedAt: u.CreatedAt}
	if err := worker.Enqueue(ctx, s.queue, TopicUserCreated, evt); err != nil {
		// Event publication is best-effort; log rather than fail the request.
		s.log.Warn("failed to publish user.created", logger.String("user_id", u.ID), logger.Err(err))
	}
}

// GetByID returns a user, using the cache as a read-through layer.
func (s *Service) GetByID(ctx context.Context, id string) (*User, error) {
	u, err := s.cache.GetOrSet(ctx, cacheKey(id), func(ctx context.Context) (*User, error) {
		return s.repo.FindByID(ctx, id)
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, response.NewNotFound("user not found")
		}
		return nil, response.NewInternal("failed to load user").Wrap(err)
	}
	return u, nil
}

// Update modifies a user's mutable fields and invalidates its cache entry.
func (s *Service) Update(ctx context.Context, id string, in UpdateUserInput) (*User, error) {
	u, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, response.NewNotFound("user not found")
		}
		return nil, response.NewInternal("failed to load user").Wrap(err)
	}

	u.Name = in.Name
	u.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(ctx, id, u); err != nil {
		return nil, response.NewInternal("failed to update user").Wrap(err)
	}
	_ = s.cache.Delete(ctx, cacheKey(id))
	return u, nil
}

// Delete removes a user and invalidates its cache entry.
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return response.NewNotFound("user not found")
		}
		return response.NewInternal("failed to delete user").Wrap(err)
	}
	_ = s.cache.Delete(ctx, cacheKey(id))
	return nil
}

// List returns a paginated set of users plus pagination metadata.
func (s *Service) List(ctx context.Context, in ListUsersInput) ([]User, PageMeta, error) {
	if in.Page < 1 {
		in.Page = 1
	}
	if in.PerPage < 1 || in.PerPage > 100 {
		in.PerPage = 20
	}

	q := store.NewQuery().OrderBy("created_at", true).
		Paginate(in.PerPage, (in.Page-1)*in.PerPage)
	countQ := store.NewQuery()
	if in.Search != "" {
		pattern := "%" + in.Search + "%"
		q = q.Like("name", pattern)
		countQ = countQ.Like("name", pattern)
	}

	users, err := s.repo.Find(ctx, q)
	if err != nil {
		return nil, PageMeta{}, response.NewInternal("failed to list users").Wrap(err)
	}
	total, err := s.repo.Count(ctx, countQ)
	if err != nil {
		return nil, PageMeta{}, response.NewInternal("failed to count users").Wrap(err)
	}

	return users, PageMeta{Page: in.Page, PerPage: in.PerPage, Total: total}, nil
}

// Authenticate verifies credentials and returns a signed access/refresh pair.
func (s *Service) Authenticate(ctx context.Context, email, password string) (auth.TokenPair, *User, error) {
	u, err := s.repo.FindByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return auth.TokenPair{}, nil, response.NewUnauthorized("invalid credentials")
		}
		return auth.TokenPair{}, nil, response.NewInternal("failed to load user").Wrap(err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)); err != nil {
		return auth.TokenPair{}, nil, response.NewUnauthorized("invalid credentials")
	}

	pair, err := s.tokens.GeneratePair(u.ID, u.Email, u.Role)
	if err != nil {
		return auth.TokenPair{}, nil, response.NewInternal("failed to issue token").Wrap(err)
	}
	return pair, u, nil
}

// Refresh exchanges a valid refresh token for a new token pair.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (auth.TokenPair, error) {
	claims, err := s.tokens.ParseRefresh(refreshToken)
	if err != nil {
		return auth.TokenPair{}, response.NewUnauthorized("invalid refresh token")
	}

	u, err := s.repo.FindByID(ctx, claims.UserID)
	if err != nil {
		return auth.TokenPair{}, response.NewUnauthorized("user no longer exists")
	}

	pair, err := s.tokens.GeneratePair(u.ID, u.Email, u.Role)
	if err != nil {
		return auth.TokenPair{}, response.NewInternal("failed to issue token").Wrap(err)
	}
	return pair, nil
}
