# Docflow — Backend

The Go REST API and WebSocket server for [Docflow](https://docflow.asia) — a real-time collaborative workspace. Handles authentication, workspace/board/document management, real-time event broadcasting, and transactional email.

**Live API:** [api.docflow.asia](https://api.docflow.asia) · **Frontend:** [docflow-web](https://github.com/saqlainsyb/docflow-web)

---

## Features

- **JWT authentication** — access + refresh token pair with configurable expiry. Separate short-lived document tokens for WebSocket handshakes.
- **Real-time WebSocket hub** — per-board WebSocket rooms broadcast Kanban events (card/column CRUD, member changes) to all connected clients.
- **Collaborative document support** — document sessions scoped by JWT claims, compatible with Yjs sync on the frontend.
- **Workspace & board management** — full CRUD for workspaces, boards, columns, cards, and members with role-based access.
- **Email invitations** — transactional invite emails via Resend with DMARC/SPF/DKIM configured on a custom domain.
- **Rate limiting** — per-IP request limiting with a configurable window and request count.
- **Database migrations** — SQL migration files managed independently for reproducible schema evolution.

---

## Tech Stack

| Concern | Choice |
|---|---|
| Language | Go 1.22 |
| HTTP framework | Gin |
| Database | PostgreSQL (via `pgx`) |
| Cache / pub-sub | Redis |
| Auth | JWT (`golang-jwt/jwt`) |
| Email | Resend API |
| Hot reload (dev) | Air |
| Containerisation | Docker Compose |
| Deployment | Render |

---

## Architecture

```
cmd/
└── server/         # Entrypoint — wires config, DB, Redis, router
internal/
├── config/         # Env-driven config struct (godotenv)
├── database/       # pgx connection pool
├── middleware/     # CORS, JWT auth, rate limiting
├── handler/        # Gin route handlers (auth, board, card, document, …)
├── service/        # Business logic layer
├── repository/     # SQL queries via pgx
├── websocket/      # Hub + per-board room broadcaster
└── email/          # Resend client wrapper
migrations/         # Numbered SQL migration files
```

The server follows a clean layered architecture: handlers parse HTTP, delegate to services for business logic, which call repositories for data access. The WebSocket hub runs as a long-lived goroutine, routing board-scoped events to all subscribers in a room.

---

## API Overview

| Group | Endpoints |
|---|---|
| Auth | `POST /auth/register`, `POST /auth/login`, `POST /auth/refresh`, `POST /auth/logout` |
| Workspaces | `GET/POST /workspaces`, `GET/PATCH/DELETE /workspaces/:id` |
| Members | `GET/POST /workspaces/:id/members`, `PATCH/DELETE /workspaces/:id/members/:userId` |
| Boards | `GET/POST /workspaces/:id/boards`, `GET/PATCH/DELETE /boards/:id` |
| Columns | `GET/POST /boards/:id/columns`, `PATCH/DELETE /columns/:id` |
| Cards | `GET/POST /columns/:id/cards`, `PATCH/DELETE /cards/:id`, `PATCH /cards/:id/move` |
| Documents | `GET/PUT /boards/:id/document` |
| Invitations | `POST /invitations`, `GET /invitations/:token`, `POST /invitations/:token/accept` |
| WebSocket | `GET /ws/board/:id` |

All routes except auth and invitation acceptance require a valid `Authorization: Bearer <token>` header.

---

## Local Development

**Prerequisites:** Go 1.22+, Docker (for Postgres + Redis)

```bash
# Start Postgres and Redis
docker compose up -d

# Copy env template and fill in values
cp .env.example .env

# Run migrations
make migrate-up

# Start server with hot reload
make dev
# or without Air:
make run
```

### Makefile targets

| Target | Description |
|---|---|
| `make dev` | Hot reload via Air |
| `make run` | `go run ./cmd/server` |
| `make build` | Compile binary to `./bin/server` |
| `make migrate-up` | Apply all pending migrations |
| `make migrate-down` | Roll back last migration |

---

## Environment Variables

| Variable | Required | Description |
|---|---|---|
| `DATABASE_URL` | ✅ | PostgreSQL connection string |
| `REDIS_URL` | | Redis connection string |
| `JWT_ACCESS_SECRET` | ✅ | HMAC secret for access tokens |
| `JWT_REFRESH_SECRET` | ✅ | HMAC secret for refresh tokens |
| `JWT_DOCUMENT_SECRET` | ✅ | HMAC secret for document tokens |
| `JWT_ACCESS_EXPIRY` | | Default `15m` |
| `JWT_REFRESH_EXPIRY` | | Default `168h` |
| `JWT_DOCUMENT_EXPIRY` | | Default `1h` |
| `CORS_ALLOWED_ORIGIN` | | Frontend origin, default `http://localhost:5173` |
| `RESEND_API_KEY` | | Resend API key (invites silently skipped if unset) |
| `RESEND_FROM_ADDR` | | From address for invitation emails |
| `APP_ENV` | | `development` or `production` |
| `APP_PORT` | | Default `8080` |
| `RATE_LIMIT_REQUESTS` | | Default `60` |
| `RATE_LIMIT_WINDOW` | | Default `1m` |