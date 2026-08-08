package middleware

import (
	"context"
	"net/http"
	"time"
)

// Timeout returns middleware that bounds request handling to the given
// duration. Handlers should honour the request context to actually abort work;
// downstream calls (DB, cache, queue) already do because they receive ctx.
func Timeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
