package main

import (
	"errors"
	"sync"

	"github.com/google/uuid"
)

// Store holds all application state in memory. net/http handles each
// request on its own goroutine, so every map access goes through mu.
type Store struct {
	mu      sync.RWMutex
	users   map[string]User   // keyed by user ID
	byEmail map[string]string // email -> user ID, for login and duplicate checks
	tickets map[string]Ticket // keyed by ticket ID
}

func NewStore() *Store {
	return &Store{
		users:   make(map[string]User),
		byEmail: make(map[string]string),
		tickets: make(map[string]Ticket),
	}
}

// CreateUser fails if the email is already registered.
func (s *Store) CreateUser(email, passwordHash string) (User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.byEmail[email]; exists {
		return User{}, false
	}

	u := User{
		ID:           uuid.NewString(),
		Email:        email,
		PasswordHash: passwordHash,
	}
	s.users[u.ID] = u
	s.byEmail[email] = u.ID
	return u, true
}

func (s *Store) GetUserByEmail(email string) (User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.byEmail[email]
	if !ok {
		return User{}, false
	}
	return s.users[id], true
}

func (s *Store) CreateTicket(t Ticket) Ticket {
	s.mu.Lock()
	defer s.mu.Unlock()

	t.ID = uuid.NewString()
	s.tickets[t.ID] = t
	return t
}

// ListTicketsByUser returns only tickets owned by userID.
func (s *Store) ListTicketsByUser(userID string) []Ticket {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := []Ticket{}
	for _, t := range s.tickets {
		if t.UserID == userID {
			result = append(result, t)
		}
	}
	return result
}

// GetTicket returns the ticket and whether it exists at all. Ownership is
// checked by the caller (handlers), not here, so this stays a plain lookup.
func (s *Store) GetTicket(id string) (Ticket, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	t, ok := s.tickets[id]
	return t, ok
}

var (
	ErrTicketNotFound    = errors.New("ticket not found")
	ErrInvalidTransition = errors.New("invalid status transition")
)

// UpdateStatusIfValid checks ownership and the transition rule and applies
// the new status as one atomic operation under a single lock. This matters:
// if ownership/transition were checked against a separately-fetched snapshot
// and the write happened afterward, a concurrent request could change the
// ticket's status in between, and this write would land based on stale data
// - possibly moving a ticket backward or past "closed" after another request
// already closed it. Holding the lock across check-and-set closes that gap.
func (s *Store) UpdateStatusIfValid(id, userID, newStatus string) (Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.tickets[id]
	if !ok || t.UserID != userID {
		return Ticket{}, ErrTicketNotFound
	}
	if !isValidTransition(t.Status, newStatus) {
		return Ticket{}, ErrInvalidTransition
	}

	t.Status = newStatus
	s.tickets[id] = t
	return t, nil
}
