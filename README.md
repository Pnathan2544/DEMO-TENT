# SaaS Template

A production-ready, multi-tenant SaaS starter kit with a Go API backend and Next.js frontend.

## Tech Stack

**Backend** — Go 1.24, Fiber v2, pgx/v5 (no ORM), PostgreSQL 16  
**Frontend** — Next.js 16, React 19, Tailwind CSS v4, shadcn/ui, Zustand 5  
**Infrastructure** — Docker Compose, multi-stage Dockerfile

## Features

- **Multi-tenancy** — Users belong to multiple tenants via memberships. PostgreSQL Row-Level Security (RLS) enforces data isolation per tenant.
- **Authentication** — Register, login, token refresh, logout. JWTs stored in HttpOnly cookies (access: 15min, refresh: 7 days). Bcrypt password hashing.
- **RBAC** — Four roles with numeric weight: `owner` > `admin` > `member` > `viewer`. Enforced at the middleware level.
- **Tenant switching** — Users can switch between tenants they belong to.
- **Invite system** — Admins invite users by email; invitees accept via token link.
- **Member management** — List, update roles, remove members (with self-leave support).
- **Projects CRUD** — Full create/read/update/delete, isolated per tenant via RLS.
- **Audit log** — Tracks actions per tenant (admin-visible).
- **Stripe integration** — Webhook handler for subscription events, billing portal, per-tenant plan tracking.
- **Profile management** — View/update profile, change password.

## Project Structure

```
apps/
  api/                    # Go backend
    cmd/api/main.go       # Entrypoint
    internal/
      config/             # Environment config
      db/                 # pgxpool connection
      http/               # Fiber server, routes, handlers, middleware
      mail/               # Email sending
      security/           # JWT, bcrypt, refresh tokens
      types/              # Context keys, role helpers
    Dockerfile            # Multi-stage build
  web/                    # Next.js frontend
    app/
      (public)/           # Login, register, invite accept
      (app)/              # Dashboard, members, audit, billing
    lib/
      api.ts              # Server-side API fetch helper
      store.ts            # Zustand auth store
    proxy.ts              # Auth-aware proxy for Next.js 16
db/
  migrations/
    0001_init.sql         # Core schema + RLS policies
    0002_milestone_b.sql  # Invites, audit log, member mgmt
infra/
  docker-compose.yml      # Postgres, Adminer, API
```

## Getting Started

### Prerequisites

- Go 1.24+
- Node.js 20+ and pnpm
- Docker and Docker Compose
- PostgreSQL client (`psql`) for migrations

### 1. Start the database

```bash
make docker-up
```

This starts PostgreSQL 16 on port `5432` and Adminer (DB UI) on port `8081`.

### 2. Run migrations

```bash
make migrate
```

Runs SQL migrations as the `postgres` superuser to create tables, RLS policies, and the `saas_app` database role.

### 3. Configure environment

Create a `.env` file in the project root:

```env
DATABASE_URL=postgres://saas_app:saas_secret@localhost:5432/saas_db
JWT_SECRET=your-secret-key-here
STRIPE_SECRET_KEY=sk_test_...
STRIPE_WEBHOOK_SECRET=whsec_...
APP_ENV=development
FRONTEND_URL=http://localhost:3000
```

### 4. Run the API

```bash
make dev
```

The API starts on `http://localhost:8080`.

### 5. Run the frontend

```bash
make web-dev
```

The frontend starts on `http://localhost:3000`. API requests are proxied to the Go backend.

## API Routes

| Method | Path | Auth | Role | Description |
|--------|------|------|------|-------------|
| POST | `/api/v1/auth/register` | - | - | Register |
| POST | `/api/v1/auth/login` | - | - | Login |
| POST | `/api/v1/auth/refresh` | - | - | Refresh tokens |
| POST | `/api/v1/auth/logout` | - | - | Logout |
| POST | `/api/v1/auth/switch` | Yes | - | Switch tenant |
| GET | `/api/v1/auth/tenants` | Yes | - | List user's tenants |
| GET | `/api/v1/me` | Yes | - | Get profile |
| PUT | `/api/v1/me` | Yes | - | Update profile |
| PUT | `/api/v1/me/password` | Yes | - | Change password |
| GET | `/api/v1/me/invites` | Yes | - | List pending invites |
| GET | `/api/v1/me/billing` | Yes | - | Get billing info |
| POST | `/api/v1/me/billing/portal` | Yes | - | Stripe billing portal |
| GET | `/api/v1/members` | Yes | member | List members |
| PATCH | `/api/v1/members/:uid` | Yes | admin | Update member role |
| DELETE | `/api/v1/members/:uid` | Yes | * | Remove member |
| POST | `/api/v1/invites` | Yes | admin | Create invite |
| GET | `/api/v1/invites` | Yes | admin | List invites |
| DELETE | `/api/v1/invites/:id` | Yes | admin | Revoke invite |
| POST | `/api/v1/invites/accept` | Yes | - | Accept invite |
| GET | `/api/v1/projects` | Yes | member | List projects |
| POST | `/api/v1/projects` | Yes | admin | Create project |
| GET | `/api/v1/projects/:id` | Yes | member | Get project |
| PUT | `/api/v1/projects/:id` | Yes | admin | Update project |
| DELETE | `/api/v1/projects/:id` | Yes | admin | Delete project |
| GET | `/api/v1/audit` | Yes | admin | Audit log |
| POST | `/api/v1/webhooks/stripe` | - | - | Stripe webhook |

## Architecture

- **Tenant isolation** is enforced at the database level using PostgreSQL RLS. Each request begins a transaction with `SET LOCAL app.tenant_id = $tid`, and RLS policies filter rows automatically.
- **Auth cookies** — Access and refresh tokens are HttpOnly cookies (not Authorization headers), eliminating XSS token theft.
- **Middleware chain** — `RequestID` -> `Logger` -> `Recover` -> `AuthRequired` -> `TenantTx` -> `RequireRole` -> Handler.
- The frontend uses Next.js App Router with route groups: `(public)` for unauthenticated pages and `(app)` for the authenticated dashboard. Server Components fetch the Go API directly; browser fetches are proxied through `next.config.ts` rewrites.

## Make Commands

| Command | Description |
|---------|-------------|
| `make dev` | Run API in development mode |
| `make build` | Build API binary |
| `make test` | Run Go tests |
| `make tidy` | Run `go mod tidy` |
| `make migrate` | Run database migrations |
| `make docker-up` | Start Docker services |
| `make docker-down` | Stop Docker services |
| `make web-dev` | Start frontend dev server |

## License

MIT
