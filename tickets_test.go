package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestServer wires the same routes main.go registers, so tests exercise
// real path-value extraction (r.PathValue) instead of calling handlers directly.
func newTestServer(a *API) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /tickets", a.requireAuth(a.handleCreateTicket))
	mux.HandleFunc("GET /tickets", a.requireAuth(a.handleListTickets))
	mux.HandleFunc("GET /tickets/{id}", a.requireAuth(a.handleGetTicket))
	mux.HandleFunc("PATCH /tickets/{id}/status", a.requireAuth(a.handleUpdateTicketStatus))
	return mux
}

// registerAndLogin is a test helper: creates a user and returns their token.
func registerAndLogin(t *testing.T, api *API, email string) string {
	t.Helper()
	rec := doJSON(t, api.handleRegister, http.MethodPost, `{"email":"`+email+`","password":"pw12345"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup: register %s failed: %d %s", email, rec.Code, rec.Body)
	}
	var resp authResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	return resp.Token
}

func authedRequest(method, path, body, token string) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func TestTicketFlow_TwoUsersOwnershipAndTransitions(t *testing.T) {
	api := newTestAPI()
	server := newTestServer(api)

	aliceToken := registerAndLogin(t, api, "alice@test.com")
	bobToken := registerAndLogin(t, api, "bob@test.com")

	// Alice creates a ticket.
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, authedRequest(http.MethodPost, "/tickets", `{"title":"fix login bug","description":"users can't log in"}`, aliceToken))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create ticket: got %d, want %d, body %s", rec.Code, http.StatusCreated, rec.Body)
	}
	var ticket Ticket
	json.Unmarshal(rec.Body.Bytes(), &ticket)
	if ticket.Status != StatusOpen {
		t.Fatalf("new ticket status = %q, want %q", ticket.Status, StatusOpen)
	}
	if ticket.ID == "" {
		t.Fatal("expected a generated ticket ID")
	}

	// Missing title.
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, authedRequest(http.MethodPost, "/tickets", `{"description":"no title"}`, aliceToken))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing title: got %d, want %d", rec.Code, http.StatusBadRequest)
	}

	// Malformed JSON.
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, authedRequest(http.MethodPost, "/tickets", `{"title":`, aliceToken))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed JSON: got %d, want %d", rec.Code, http.StatusBadRequest)
	}

	// No token at all.
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tickets", bytes.NewBufferString(`{"title":"x"}`))
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	// Alice lists her tickets: should see exactly the one she made.
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, authedRequest(http.MethodGet, "/tickets", "", aliceToken))
	var aliceTickets []Ticket
	json.Unmarshal(rec.Body.Bytes(), &aliceTickets)
	if len(aliceTickets) != 1 || aliceTickets[0].ID != ticket.ID {
		t.Fatalf("alice's ticket list = %+v, want exactly her one ticket", aliceTickets)
	}

	// Bob lists tickets: should see none of Alice's.
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, authedRequest(http.MethodGet, "/tickets", "", bobToken))
	var bobTickets []Ticket
	json.Unmarshal(rec.Body.Bytes(), &bobTickets)
	if len(bobTickets) != 0 {
		t.Fatalf("bob's ticket list = %+v, want empty", bobTickets)
	}

	// Alice can fetch her own ticket by ID.
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, authedRequest(http.MethodGet, "/tickets/"+ticket.ID, "", aliceToken))
	if rec.Code != http.StatusOK {
		t.Fatalf("alice get own ticket: got %d, want %d", rec.Code, http.StatusOK)
	}

	// Bob cannot fetch Alice's ticket by ID -> 404, not 403 (existence isn't leaked).
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, authedRequest(http.MethodGet, "/tickets/"+ticket.ID, "", bobToken))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("bob get alice's ticket: got %d, want %d", rec.Code, http.StatusNotFound)
	}

	// Malformed / nonexistent ticket ID.
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, authedRequest(http.MethodGet, "/tickets/not-a-real-id", "", aliceToken))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("bad ticket id: got %d, want %d", rec.Code, http.StatusNotFound)
	}

	// Bob cannot update Alice's ticket status.
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, authedRequest(http.MethodPatch, "/tickets/"+ticket.ID+"/status", `{"status":"in_progress"}`, bobToken))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("bob update alice's ticket: got %d, want %d", rec.Code, http.StatusNotFound)
	}

	// Invalid status value.
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, authedRequest(http.MethodPatch, "/tickets/"+ticket.ID+"/status", `{"status":"done"}`, aliceToken))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid status value: got %d, want %d", rec.Code, http.StatusBadRequest)
	}

	// Skipping straight to closed is rejected under the strict rule.
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, authedRequest(http.MethodPatch, "/tickets/"+ticket.ID+"/status", `{"status":"closed"}`, aliceToken))
	if rec.Code != http.StatusConflict {
		t.Fatalf("skip open->closed: got %d, want %d", rec.Code, http.StatusConflict)
	}

	// Valid transition: open -> in_progress.
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, authedRequest(http.MethodPatch, "/tickets/"+ticket.ID+"/status", `{"status":"in_progress"}`, aliceToken))
	if rec.Code != http.StatusOK {
		t.Fatalf("open->in_progress: got %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body)
	}

	// Backward transition is rejected.
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, authedRequest(http.MethodPatch, "/tickets/"+ticket.ID+"/status", `{"status":"open"}`, aliceToken))
	if rec.Code != http.StatusConflict {
		t.Fatalf("in_progress->open: got %d, want %d", rec.Code, http.StatusConflict)
	}

	// Valid transition: in_progress -> closed.
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, authedRequest(http.MethodPatch, "/tickets/"+ticket.ID+"/status", `{"status":"closed"}`, aliceToken))
	if rec.Code != http.StatusOK {
		t.Fatalf("in_progress->closed: got %d, want %d", rec.Code, http.StatusOK)
	}

	// Closed ticket cannot be reopened.
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, authedRequest(http.MethodPatch, "/tickets/"+ticket.ID+"/status", `{"status":"open"}`, aliceToken))
	if rec.Code != http.StatusConflict {
		t.Fatalf("reopen closed: got %d, want %d", rec.Code, http.StatusConflict)
	}
}
