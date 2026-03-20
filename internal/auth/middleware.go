package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/gorilla/mux"

	"climate-backend/internal/models"
)

type contextKey string

const claimsKey contextKey = "jwt_claims"

// Middleware validates the Bearer token, enforces tenant isolation, and
// enforces role-based access control on every protected route.
//
//   - Tenant isolation: the tenant_id claim in the token must equal the
//     {tenant_id} path variable. A token from tenant A can never access
//     tenant B's routes.
//
//   - Role enforcement: POST / PUT / PATCH / DELETE require RoleAdmin.
//     GET is permitted for both RoleAdmin and RoleUser.
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ── 1. Extract Bearer token ──────────────────────────────────────
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			http.Error(w, "unauthorized: missing Bearer token", http.StatusUnauthorized)
			return
		}
		tokenStr := strings.TrimPrefix(header, "Bearer ")

		// ── 2. Validate token ────────────────────────────────────────────
		claims, err := s.ValidateToken(tokenStr)
		if err != nil {
			http.Error(w, "unauthorized: "+err.Error(), http.StatusUnauthorized)
			return
		}

		// ── 3. Tenant isolation ──────────────────────────────────────────
		if pathTenant := mux.Vars(r)["tenant_id"]; pathTenant != "" {
			if claims.TenantID != pathTenant {
				http.Error(w, "forbidden: tenant mismatch", http.StatusForbidden)
				return
			}
		}

		// ── 4. Role-based access ─────────────────────────────────────────
		if isMutating(r.Method) && claims.Role != models.RoleAdmin {
			http.Error(w, "forbidden: admin role required", http.StatusForbidden)
			return
		}

		// ── 5. Propagate claims ──────────────────────────────────────────
		next.ServeHTTP(w, r.WithContext(
			context.WithValue(r.Context(), claimsKey, claims),
		))
	})
}

// ClaimsFromContext retrieves validated JWT claims stored by the middleware.
func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(claimsKey).(*Claims)
	return c, ok
}

// isMutating returns true for HTTP methods that modify state.
func isMutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}
