package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/cymonevo/go_template/pkg/logger"
	"github.com/cymonevo/go_template/pkg/ratelimit"
	"github.com/cymonevo/go_template/pkg/response"
)

// RateLimit returns middleware enforcing limit requests per window. The limit
// key is the authenticated user ID when present, otherwise the client IP, so
// authenticated clients get an independent budget.
func RateLimit(limiter ratelimit.Limiter, limit int, window time.Duration, log logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := rateKey(r)
			res, err := limiter.Allow(r.Context(), key, limit, window)
			if err != nil {
				// Fail open: a limiter outage should not take down the API.
				log.Warn("rate limiter error, allowing request", logger.Err(err))
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(res.Limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(res.Remaining))

			if !res.Allowed {
				w.Header().Set("Retry-After", strconv.Itoa(int(res.RetryAfter.Seconds())+1))
				response.JSON(w, http.StatusTooManyRequests, response.Envelope{
					Success: false,
					Error:   &response.ErrorBody{Code: "rate_limited", Message: "too many requests"},
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func rateKey(r *http.Request) string {
	if uid := UserIDFrom(r.Context()); uid != "" {
		return "uid:" + uid
	}
	return "ip:" + clientIP(r)
}

func clientIP(r *http.Request) string {
	if r.RemoteAddr == "" {
		return "unknown"
	}
	return r.RemoteAddr
}
