package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// RequireInternalAuth middleware ensures the request has the correct system secret.
func RequireInternalAuth(systemSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Missing authorization header", http.StatusUnauthorized)
				return
			}

			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				http.Error(w, "Invalid authorization header", http.StatusUnauthorized)
				return
			}

			token := parts[1]

			if subtle.ConstantTimeCompare([]byte(token), []byte(systemSecret)) != 1 {
				http.Error(w, "Invalid authorization token", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
