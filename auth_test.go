package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func newTestAPI() *API {
	return &API{store: NewStore(), jwtSecret: []byte("test-secret")}
}

func doJSON(t *testing.T, handler http.HandlerFunc, method, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

func TestRegister(t *testing.T) {
	api := newTestAPI()

	rec := doJSON(t, api.handleRegister, http.MethodPost, `{"email":"a@test.com","password":"pw12345"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("first register: got %d, want %d, body %s", rec.Code, http.StatusCreated, rec.Body)
	}
	var resp authResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.Token == "" {
		t.Fatalf("expected a token in response, got %s", rec.Body)
	}

	// duplicate email
	rec = doJSON(t, api.handleRegister, http.MethodPost, `{"email":"a@test.com","password":"other"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate register: got %d, want %d", rec.Code, http.StatusConflict)
	}

	// missing password
	rec = doJSON(t, api.handleRegister, http.MethodPost, `{"email":"b@test.com"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing password: got %d, want %d", rec.Code, http.StatusBadRequest)
	}

	// malformed JSON
	rec = doJSON(t, api.handleRegister, http.MethodPost, `{"email":`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed JSON: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestLogin(t *testing.T) {
	api := newTestAPI()
	doJSON(t, api.handleRegister, http.MethodPost, `{"email":"a@test.com","password":"pw12345"}`)

	rec := doJSON(t, api.handleLogin, http.MethodPost, `{"email":"a@test.com","password":"pw12345"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid login: got %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body)
	}

	rec = doJSON(t, api.handleLogin, http.MethodPost, `{"email":"a@test.com","password":"wrong"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	rec = doJSON(t, api.handleLogin, http.MethodPost, `{"email":"nobody@test.com","password":"pw12345"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("nonexistent email: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireAuth(t *testing.T) {
	api := newTestAPI()

	var gotUserID string
	protected := api.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		id, _ := userIDFromContext(r)
		gotUserID = id
		w.WriteHeader(http.StatusOK)
	})

	// missing header
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	protected(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing header: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	// malformed header, no "Bearer" scheme
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "sometoken")
	rec = httptest.NewRecorder()
	protected(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("malformed header: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	// garbage token
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	rec = httptest.NewRecorder()
	protected(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("garbage token: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	// token signed with the wrong secret
	badToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	badSigned, _ := badToken.SignedString([]byte("wrong-secret"))
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+badSigned)
	rec = httptest.NewRecorder()
	protected(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong signature: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	// expired token, correct secret
	expiredToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
		},
	})
	expiredSigned, _ := expiredToken.SignedString(api.jwtSecret)
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+expiredSigned)
	rec = httptest.NewRecorder()
	protected(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expired token: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	// valid token flows through and sets context
	validToken, err := api.generateToken("user-42")
	if err != nil {
		t.Fatalf("generateToken: %v", err)
	}
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+validToken)
	rec = httptest.NewRecorder()
	protected(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid token: got %d, want %d", rec.Code, http.StatusOK)
	}
	if gotUserID != "user-42" {
		t.Fatalf("context user ID = %q, want %q", gotUserID, "user-42")
	}
}
