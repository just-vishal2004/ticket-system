# Ticket System

Backend take-home for EVA Bharat: a small REST service where a user can register, log in,
create tickets, and manage the status of their own tickets. Built in Go, in-memory storage,
JWT authentication.

## Architecture

Plain `net/http`, no framework. One package (`main`), six source files:

- `models.go` - `User`/`Ticket` types and the status transition rule
- `store.go` - in-memory storage (two maps, one mutex), no persistence
- `auth.go` - register/login handlers, password hashing, JWT issue/verify
- `middleware.go` - JWT auth middleware, puts the authenticated user ID on the request context
- `tickets.go` - the four ticket handlers, ownership enforced via `lookupOwnTicket`
- `main.go` - route wiring and server startup
- `httpjson.go` - shared JSON response helpers

No layered architecture (no repository/service interfaces) since there is exactly one storage
implementation and adding indirection for a single in-memory store didn't seem worth it for a
two-day assignment.

## Setup

Requires Go 1.22+.

```bash
go mod tidy
JWT_SECRET=some-local-secret go run .
```

The app refuses to start if `JWT_SECRET` isn't set.

## Docker

```bash
docker build -t ticket-system .
docker run -p 8080:8080 -e JWT_SECRET=some-local-secret ticket-system
curl http://localhost:8080/health
```

## Environment variables

| Variable | Required | Purpose |
|---|---|---|
| `JWT_SECRET` | yes | Signing key for issued JWTs. No default; the app exits if it's unset. |

See `.env.example`.

## API

| Method | Path | Auth | Purpose |
|---|---|---|---|
| GET | `/health` | no | Health check |
| POST | `/auth/register` | no | Register a user |
| POST | `/auth/login` | no | Log in, returns a JWT |
| POST | `/tickets` | yes | Create a ticket |
| GET | `/tickets` | yes | List your own tickets |
| GET | `/tickets/{id}` | yes | Get one of your own tickets |
| PATCH | `/tickets/{id}/status` | yes | Update your own ticket's status |

Protected routes require `Authorization: Bearer <token>`.

**Register / login**

```json
{ "email": "a@test.com", "password": "pw12345" }
```

Response: `{ "token": "<jwt>" }`

**Create ticket**

```json
{ "title": "fix login bug", "description": "optional" }
```

New tickets start as `open`.

**Update status**

```json
{ "status": "in_progress" }
```

Status flow: `open -> in_progress -> closed`, one step at a time. No skipping straight from
`open` to `closed`, no moving backward, and a `closed` ticket can't be reopened.

## Ownership

A user only sees and can act on tickets they created. Requesting another user's ticket by ID
returns `404`, the same response as a ticket that doesn't exist at all, so a caller can't tell
the difference between "not yours" and "doesn't exist."

## Testing

```bash
go test ./...
```

Covers registration, login, JWT middleware (missing/malformed header, invalid signature,
expired token), the full ticket lifecycle across two separate users, and every status
transition (valid, backward, skip, reopen-after-close).

## Deployment

- GitHub repository: https://github.com/just-vishal2004/ticket-system
- Deployed URL: https://ticket-system-2399.onrender.com
- Public health check: https://ticket-system-2399.onrender.com/health

## Assumptions

- Storage is in-memory only; data does not survive a restart. The assignment explicitly allows
  this ("No complex database schema is required. Use in-memory storage...").
- Register/login field names (`email`, `password`) and ticket field names (`title`,
  `description`, `status`) aren't specified in the assignment brief, so I used the most common
  convention.
- A ticket owned by someone else and a ticket that doesn't exist both return `404` on
  `GET /tickets/{id}` and `PATCH /tickets/{id}/status`, rather than `403` for the former. This
  avoids confirming to a caller that a ticket ID they don't own actually exists.
- Status transitions are strictly one step at a time: `open -> in_progress -> closed` only,
  `open -> closed` directly is rejected. The brief's diagram shows this exact sequence and
  explicitly bans moving backward, but doesn't explicitly say whether skipping a step forward
  is allowed. We read the diagram as the literal required path and went with the strict,
  one-step-at-a-time interpretation.
