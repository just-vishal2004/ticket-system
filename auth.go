package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// API bundles the dependencies handlers need. Methods on *API instead of
// free functions with long parameter lists, and no global state.
type API struct {
	store     *Store
	jwtSecret []byte
}

// mustJWTSecret reads JWT_SECRET from the environment. The app refuses to
// start without it instead of falling back to a default, since a known or
// guessable secret would let anyone forge valid tokens.
func mustJWTSecret() []byte {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		log.Fatal("JWT_SECRET environment variable is required")
	}
	return []byte(secret)
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResponse struct {
	Token string `json:"token"`
}

func (a *API) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not process password")
		return
	}

	user, created := a.store.CreateUser(req.Email, string(hash))
	if !created {
		writeError(w, http.StatusConflict, "email already registered")
		return
	}

	token, err := a.generateToken(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not generate token")
		return
	}

	writeJSON(w, http.StatusCreated, authResponse{Token: token})
}

func (a *API) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	user, ok := a.store.GetUserByEmail(req.Email)
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	token, err := a.generateToken(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not generate token")
		return
	}

	writeJSON(w, http.StatusOK, authResponse{Token: token})
}

// claims is deliberately minimal: just the registered "sub" (user ID) plus
// standard issued-at/expiry claims. No email, no roles — nothing handlers
// don't actually need to read back out of the token.
type claims struct {
	jwt.RegisteredClaims
}

const tokenTTL = 24 * time.Hour

func (a *API) generateToken(userID string) (string, error) {
	now := time.Now()
	c := claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(tokenTTL)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	return token.SignedString(a.jwtSecret)
}

// parseToken verifies the signature and checks expiry (the jwt library
// rejects expired tokens automatically during parsing) and returns the
// user ID from the subject claim.
func (a *API) parseToken(tokenString string) (string, error) {
	token, err := jwt.ParseWithClaims(tokenString, &claims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return a.jwtSecret, nil
	})
	if err != nil {
		return "", err
	}

	c, ok := token.Claims.(*claims)
	if !ok || !token.Valid || c.Subject == "" {
		return "", errors.New("invalid token claims")
	}
	return c.Subject, nil
}
