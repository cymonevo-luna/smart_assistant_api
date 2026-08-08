package middleware

import (
	"net/http"

	"github.com/cymonevo/go_template/pkg/auth"
	"github.com/cymonevo/go_template/pkg/response"
)

// RequireRole returns middleware that allows the request only if the
// authenticated user holds one of the given roles. It must run after Auth.
func RequireRole(roles ...auth.Role) func(http.Handler) http.Handler {
	allowed := make(map[auth.Role]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFrom(r.Context())
			if !ok {
				response.Error(w, response.NewUnauthorized("authentication required"))
				return
			}
			if _, permitted := allowed[claims.Role]; !permitted {
				response.Error(w, response.NewForbidden("insufficient privileges"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAdmin is a convenience wrapper for admin-only routes.
func RequireAdmin() func(http.Handler) http.Handler {
	return RequireRole(auth.RoleAdmin)
}
