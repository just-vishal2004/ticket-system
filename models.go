package main

import "time"

// User is a registered account. Password is never stored or serialized in
// plain text; PasswordHash is the bcrypt output.
type User struct {
	ID           string
	Email        string
	PasswordHash string
}

// Ticket belongs to exactly one user (UserID). Status is one of the
// TicketStatus* constants below.
type Ticket struct {
	ID          string    `json:"id"`
	UserID      string    `json:"-"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

const (
	StatusOpen       = "open"
	StatusInProgress = "in_progress"
	StatusClosed     = "closed"
)

// validStatuses lets us check "is this a real status" without hardcoding
// the check in multiple places.
var validStatuses = map[string]bool{
	StatusOpen:       true,
	StatusInProgress: true,
	StatusClosed:     true,
}

// isValidTransition enforces the assignment's exact rule: strictly forward,
// one step at a time. open -> in_progress -> closed. Nothing moves once closed.
func isValidTransition(from, to string) bool {
	switch from {
	case StatusOpen:
		return to == StatusInProgress
	case StatusInProgress:
		return to == StatusClosed
	default: // closed, or anything unexpected
		return false
	}
}
