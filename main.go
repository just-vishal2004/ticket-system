package main

import (
	"log"
	"net/http"
)

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func main() {
	api := &API{
		store:     NewStore(),
		jwtSecret: mustJWTSecret(),
	}

	mux := http.NewServeMux()

	// Public routes.
	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("POST /auth/register", api.handleRegister)
	mux.HandleFunc("POST /auth/login", api.handleLogin)

	// Protected routes: each wrapped individually in requireAuth so the
	// list of what needs a token is visible right here, not hidden behind
	// a route-group helper.
	mux.HandleFunc("POST /tickets", api.requireAuth(api.handleCreateTicket))
	mux.HandleFunc("GET /tickets", api.requireAuth(api.handleListTickets))
	mux.HandleFunc("GET /tickets/{id}", api.requireAuth(api.handleGetTicket))
	mux.HandleFunc("PATCH /tickets/{id}/status", api.requireAuth(api.handleUpdateTicketStatus))

	const addr = ":8080"
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
