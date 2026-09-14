package main

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const userIDContextKey contextKey = "userID"

// requireAuth validates the Authorization header and, on success, stores
// the authenticated user's ID in the request context. Handlers wrapped by
// this never parse or verify the token themselves — they just call
// userIDFromContext.
func (a *API) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if header == "" {
			writeError(w, http.StatusUnauthorized, "missing authorization header")
			return
		}

		scheme, token, found := strings.Cut(header, " ")
		if !found || scheme != "Bearer" || token == "" {
			writeError(w, http.StatusUnauthorized, "malformed authorization header")
			return
		}

		userID, err := a.parseToken(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}

		ctx := context.WithValue(r.Context(), userIDContextKey, userID)
		next(w, r.WithContext(ctx))
	}
}

func userIDFromContext(r *http.Request) (string, bool) {
	id, ok := r.Context().Value(userIDContextKey).(string)
	return id, ok
}
