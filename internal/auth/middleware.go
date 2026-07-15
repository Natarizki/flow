package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/Natarizki/flow/pkg/utils"
)

type contextKey string

const ClaimsContextKey contextKey = "claims"

// Middleware validasi Bearer token dari header Authorization, inject
// Claims ke request context kalau valid.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeAuthError(w, "missing authorization header")
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			writeAuthError(w, "invalid authorization header format")
			return
		}

		claims, err := ParseToken(parts[1])
		if err != nil {
			writeAuthError(w, "invalid or expired token")
			return
		}

		ctx := context.WithValue(r.Context(), ClaimsContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func writeAuthError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`{"error":"` + message + `"}`))
	utils.LogWarn("auth failed: %s", message)
}

func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(ClaimsContextKey).(*Claims)
	return claims, ok
}
