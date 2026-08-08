// Package middleware contains reusable net/http middleware used by the router.
package middleware

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/cymonevo/go_template/pkg/auth"
	"github.com/cymonevo/go_template/pkg/logger"
	"github.com/cymonevo/go_template/pkg/response"
	"github.com/google/uuid"
)

type ctxKey string

const (
	requestIDKey ctxKey = "request_id"
	claimsKey    ctxKey = "claims"
)

// RequestID assigns a unique ID to each request, propagating an incoming
// X-Request-ID header when present.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFrom extracts the request ID from a context.
func RequestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

// ClaimsFrom extracts the authenticated user's JWT claims from a context.
func ClaimsFrom(ctx context.Context) (*auth.Claims, bool) {
	v, ok := ctx.Value(claimsKey).(*auth.Claims)
	return v, ok
}

// UserIDFrom extracts the authenticated user ID from a context.
func UserIDFrom(ctx context.Context) string {
	if c, ok := ClaimsFrom(ctx); ok {
		return c.UserID
	}
	return ""
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Hijack lets the wrapped writer be used for protocol upgrades (e.g. WebSocket).
// Embedding the http.ResponseWriter interface does not promote Hijack, so it
// must be forwarded explicitly or upgrades fail with "http.Hijacker is
// unavailable".
func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("middleware: underlying ResponseWriter does not implement http.Hijacker")
	}
	return hijacker.Hijack()
}

// Flush forwards to the wrapped writer so streaming responses (e.g. SSE) keep
// working through the logging middleware.
func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Unwrap exposes the wrapped writer for http.ResponseController and other
// middleware that need direct access to the underlying writer.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// Logger logs one structured line per request including latency and status.
func Logger(log logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)

			log.Info("request",
				logger.String("method", r.Method),
				logger.String("path", r.URL.Path),
				logger.Int("status", rec.status),
				logger.Duration("latency", time.Since(start)),
				logger.String("request_id", RequestIDFrom(r.Context())),
			)
		})
	}
}

// Recover converts panics into a 500 response so a single bad request cannot
// take down the server.
func Recover(log logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("panic recovered", logger.Any("panic", rec))
					response.Error(w, response.NewInternal("internal server error"))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// Auth enforces a valid bearer token and injects the user ID into the context.
func Auth(tokens *auth.TokenManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				response.Error(w, response.NewUnauthorized("missing bearer token"))
				return
			}

			claims, err := tokens.Parse(parts[1])
			if err != nil {
				response.Error(w, response.NewUnauthorized("invalid or expired token"))
				return
			}
			if claims.Type != auth.AccessToken {
				response.Error(w, response.NewUnauthorized("token is not an access token"))
				return
			}

			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
