package plugin

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/cymonevo/go_template/pkg/cache"
	"github.com/cymonevo/go_template/pkg/logger"
	"github.com/cymonevo/go_template/pkg/response"
	"github.com/cymonevo/go_template/pkg/store"
	"github.com/google/uuid"
)

// Service holds the business logic for the plugin catalog.
type Service struct {
	repo  Repository
	cache *cache.Typed[Plugin]
	tx    store.TxManager
	log   logger.Logger
}

// NewService constructs a plugin Service.
func NewService(
	repo Repository,
	c *cache.Typed[Plugin],
	tx store.TxManager,
	log logger.Logger,
) *Service {
	return &Service{repo: repo, cache: c, tx: tx, log: log}
}

func cacheKey(slug string) string { return "plugin:" + slug }

// Register creates a new catalog plugin after validating its manifest.
func (s *Service) Register(ctx context.Context, in RegisterPluginInput) (*Plugin, error) {
	if err := ValidateManifest(in.Manifest); err != nil {
		return nil, err
	}

	slug := strings.ToLower(strings.TrimSpace(in.Slug))
	if existing, err := s.repo.FindBySlug(ctx, slug); err == nil && existing != nil {
		return nil, response.NewConflict("a plugin with this slug already exists")
	} else if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, response.NewInternal("failed to verify slug").Wrap(err)
	}

	now := time.Now().UTC()
	p := &Plugin{
		ID:          uuid.NewString(),
		Slug:        slug,
		Name:        in.Name,
		Description: in.Description,
		Version:     in.Version,
		Manifest:    in.Manifest,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.tx.Do(ctx, func(ctx context.Context) error {
		return s.repo.Create(ctx, p)
	}); err != nil {
		return nil, response.NewInternal("failed to register plugin").Wrap(err)
	}

	s.log.Info("plugin registered", logger.String("slug", p.Slug))
	return p, nil
}

// Update modifies an existing catalog plugin after validating its manifest.
func (s *Service) Update(ctx context.Context, slug string, in UpdatePluginInput) (*Plugin, error) {
	if err := ValidateManifest(in.Manifest); err != nil {
		return nil, err
	}

	p, err := s.repo.FindBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, response.NewNotFound("plugin not found")
		}
		return nil, response.NewInternal("failed to load plugin").Wrap(err)
	}

	p.Name = in.Name
	p.Description = in.Description
	p.Version = in.Version
	p.Manifest = in.Manifest
	p.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(ctx, p.ID, p); err != nil {
		return nil, response.NewInternal("failed to update plugin").Wrap(err)
	}
	_ = s.cache.Delete(ctx, cacheKey(slug))
	return p, nil
}

// GetBySlug returns a plugin by slug, using the cache as a read-through layer.
func (s *Service) GetBySlug(ctx context.Context, slug string) (*Plugin, error) {
	p, err := s.cache.GetOrSet(ctx, cacheKey(slug), func(ctx context.Context) (*Plugin, error) {
		return s.repo.FindBySlug(ctx, slug)
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, response.NewNotFound("plugin not found")
		}
		return nil, response.NewInternal("failed to load plugin").Wrap(err)
	}
	return p, nil
}

// List returns a paginated set of catalog plugins.
func (s *Service) List(ctx context.Context, in ListPluginsInput) ([]Plugin, PageMeta, error) {
	if in.Page < 1 {
		in.Page = 1
	}
	if in.PerPage < 1 || in.PerPage > 100 {
		in.PerPage = 20
	}

	q := store.NewQuery().OrderBy("created_at", true).
		Paginate(in.PerPage, (in.Page-1)*in.PerPage)

	plugins, err := s.repo.Find(ctx, q)
	if err != nil {
		return nil, PageMeta{}, response.NewInternal("failed to list plugins").Wrap(err)
	}
	total, err := s.repo.Count(ctx, store.NewQuery())
	if err != nil {
		return nil, PageMeta{}, response.NewInternal("failed to count plugins").Wrap(err)
	}

	return plugins, PageMeta{Page: in.Page, PerPage: in.PerPage, Total: total}, nil
}
