package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

type createTicketRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type updateStatusRequest struct {
	Status string `json:"status"`
}

func (a *API) handleCreateTicket(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r) // guaranteed set: this handler only runs behind requireAuth

	var req createTicketRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	ticket := a.store.CreateTicket(Ticket{
		UserID:      userID,
		Title:       req.Title,
		Description: req.Description,
		Status:      StatusOpen,
		CreatedAt:   time.Now(),
	})

	writeJSON(w, http.StatusCreated, ticket)
}

func (a *API) handleListTickets(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r)
	writeJSON(w, http.StatusOK, a.store.ListTicketsByUser(userID))
}

func (a *API) handleGetTicket(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r)

	ticket, ok := a.lookupOwnTicket(r.PathValue("id"), userID)
	if !ok {
		writeError(w, http.StatusNotFound, "ticket not found")
		return
	}

	writeJSON(w, http.StatusOK, ticket)
}

func (a *API) handleUpdateTicketStatus(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r)
	id := r.PathValue("id")

	// Fast-path existence/ownership check so a request against a ticket that
	// isn't found or isn't yours gets 404 before we even look at the body.
	// Tickets are never deleted, so this can't go stale between here and the
	// write below - only Status can change concurrently, and that's re-checked
	// atomically inside UpdateStatusIfValid.
	if _, ok := a.lookupOwnTicket(id, userID); !ok {
		writeError(w, http.StatusNotFound, "ticket not found")
		return
	}

	var req updateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if !validStatuses[req.Status] {
		writeError(w, http.StatusBadRequest, "invalid status value")
		return
	}

	updated, err := a.store.UpdateStatusIfValid(id, userID, req.Status)
	switch {
	case errors.Is(err, ErrTicketNotFound):
		writeError(w, http.StatusNotFound, "ticket not found")
	case errors.Is(err, ErrInvalidTransition):
		writeError(w, http.StatusConflict, "cannot move to "+req.Status)
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not update ticket")
	default:
		writeJSON(w, http.StatusOK, updated)
	}
}

// lookupOwnTicket fetches a ticket and confirms the given user owns it. A
// ticket that doesn't exist and a ticket that exists but belongs to someone
// else are treated identically by callers (both become 404) so a user can't
// probe for other people's ticket IDs by watching for a 403 vs 404 split.
func (a *API) lookupOwnTicket(id, userID string) (Ticket, bool) {
	ticket, found := a.store.GetTicket(id)
	if !found || ticket.UserID != userID {
		return Ticket{}, false
	}
	return ticket, true
}
