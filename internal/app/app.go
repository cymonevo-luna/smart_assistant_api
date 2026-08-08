// Package app wires the application together and owns its lifecycle.
package app

import (
	"context"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/cymonevo/go_template/docs" // generated swagger docs
	"github.com/cymonevo/go_template/internal/config"
	"github.com/cymonevo/go_template/internal/handler"
	appmw "github.com/cymonevo/go_template/internal/middleware"
	"github.com/cymonevo/go_template/internal/server"
	"github.com/cymonevo/go_template/pkg/auth"
	"github.com/cymonevo/go_template/pkg/logger"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	httpswagger "github.com/swaggo/http-swagger/v2"
)

// App is the fully assembled application.
type App struct {
	cfg       *config.Config
	log       logger.Logger
	container *Container
	server    *server.Server
	handler   http.Handler
}

// New loads configuration, builds the logger and the dependency container, and
// assembles the HTTP router.
func New(ctx context.Context) (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	log, err := logger.New(cfg.App.LogLevel, cfg.App.IsProduction())
	if err != nil {
		return nil, err
	}

	container, err := BuildContainer(ctx, cfg, log)
	if err != nil {
		return nil, err
	}

	router := buildRouter(cfg, log, container)
	srv := server.New(cfg.HTTP, router, log)

	return &App{cfg: cfg, log: log, container: container, server: srv, handler: router}, nil
}

// Handler returns the fully assembled HTTP handler. It is primarily intended
// for integration tests that drive the API through an httptest.Server without
// binding a real network listener.
func (a *App) Handler() http.Handler { return a.handler }

// Container exposes the dependency container so tests can reach shared
// collaborators (for example the token manager, to mint privileged tokens) and
// control background workers.
func (a *App) Container() *Container { return a.container }

// Run starts background workers and the HTTP server, blocking until an
// interrupt signal triggers a graceful shutdown.
func (a *App) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := a.container.StartBackground(ctx); err != nil {
		return err
	}

	err := a.server.Start(ctx)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a.container.Close(shutdownCtx)
	_ = a.log.Sync()

	return err
}

// @title           go_template API
// @version         1.0
// @description     A scalable, database-agnostic Go backend template.
// @BasePath        /
// @securityDefinitions.apikey  BearerAuth
// @in              header
// @name            Authorization
func buildRouter(cfg *config.Config, log logger.Logger, c *Container) http.Handler {
	r := chi.NewRouter()

	r.Use(appmw.RequestID)
	r.Use(appmw.Recover(log))
	r.Use(appmw.Logger(log))
	r.Use(middleware.Compress(5))
	r.Use(appmw.Timeout(cfg.HTTP.RequestTimeout))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		ExposedHeaders:   []string{"X-Request-ID"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	if cfg.RateLimit.Enabled {
		r.Use(appmw.RateLimit(c.RateLimiter, cfg.RateLimit.Requests, cfg.RateLimit.Window, log))
	}

	health := handler.NewHealthHandler(c.pingers)
	health.Register(r)

	r.Get("/swagger/*", httpswagger.Handler(httpswagger.URL("/swagger/doc.json")))

	// Public and self-service routes live under /api/v1.
	userHandler := handler.NewUserHandler(c.UserService, c.Validator)
	userHandler.Register(r, appmw.Auth(c.Tokens))

	// Admin-only routes live under /api/admin and require the admin role.
	adminHandler := handler.NewAdminUserHandler(c.UserService)
	adminHandler.Register(r, appmw.Auth(c.Tokens), appmw.RequireRole(auth.RoleAdmin))

	return r
}
