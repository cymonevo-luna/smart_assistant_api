// Package server runs the HTTP server with graceful shutdown.
package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/cymonevo/go_template/internal/config"
	"github.com/cymonevo/go_template/pkg/logger"
)

// Server wraps http.Server with lifecycle helpers.
type Server struct {
	httpServer *http.Server
	cfg        config.HTTPConfig
	log        logger.Logger
}

// New builds a Server bound to the configured address using the given handler.
func New(cfg config.HTTPConfig, handler http.Handler, log logger.Logger) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:         cfg.Addr(),
			Handler:      handler,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			IdleTimeout:  cfg.IdleTimeout,
		},
		cfg: cfg,
		log: log,
	}
}

// Start blocks serving requests until the context is cancelled, then performs a
// graceful shutdown bounded by the configured timeout.
func (s *Server) Start(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.log.Info("http server listening", logger.String("addr", s.httpServer.Addr))
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		s.log.Info("shutting down http server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
		defer cancel()
		return s.httpServer.Shutdown(shutdownCtx)
	}
}
