# go_template

A reusable Go base for building production HTTP APIs that can run against
**Postgres or MongoDB** with a single config switch. It ships with a complete
foundation: layered architecture, configuration, structured logging, dependency
injection, a database-agnostic store, caching, async queues, rate limiting, JWT
auth + RBAC, request validation, a standard JSON envelope, graceful shutdown,
migrations, OpenAPI/Swagger docs, and optional automation clients.

---

## Table of contents

- [Quick start](#quick-start)
- [Architecture overview](#architecture-overview)
- [Project structure](#project-structure)
- [Startup flow](#startup-flow)
- [Configuration (env)](#configuration-env)
- [Database & storage (Store[T])](#database--storage-storet)
- [Domain layer (entity / service / repository)](#domain-layer-entity--service--repository)
- [HTTP routing & handlers](#http-routing--handlers)
- [Middleware](#middleware)
- [Auth & RBAC (JWT)](#auth--rbac-jwt)
- [Responses & error handling](#responses--error-handling)
- [Caching](#caching)
- [Queue & workers](#queue--workers)
- [Rate limiting](#rate-limiting)
- [Logging](#logging)
- [Migrations](#migrations)
- [API docs (Swagger)](#api-docs-swagger)
- [Automation clients (composio / cursoragent / github)](#automation-clients-composio--cursoragent--github)
- [Slack notifications (slack)](#slack-notifications-slack)
- [Testing](#testing)
- [Docker](#docker)
- [Creating a new service from this template](#creating-a-new-service-from-this-template)
- [Dependencies](#dependencies)

---

## Quick start

```bash
cp .env.example .env          # then edit values
make migrate-up               # apply DB migrations (Postgres by default)
make run                      # go run ./cmd/api  ->  listens on :8080
```

Defaults work out of the box: Postgres at `localhost:5432`, in-memory cache and
queue (no Redis required). The bundled `user` domain (auth, CRUD, admin) is a
reference implementation — keep it as a guide or replace it with your own
domains.

Smoke-test it:

```bash
curl localhost:8080/healthz
curl localhost:8080/swagger/index.html   # interactive API docs
```

---

## Architecture overview

| Concern | Tool | Where |
|---------|------|-------|
| HTTP router | `go-chi/chi` | `internal/app/app.go` |
| Configuration | `caarlos0/env` + `joho/godotenv` | `internal/config/` + `.env` |
| Dependency injection | manual container | `internal/app/container.go` |
| Persistence (DB-agnostic) | generic `Store[T]` | `pkg/store/` |
| PostgreSQL driver | `jackc/pgx` | `internal/infra/postgres/` |
| MongoDB driver | `mongo-driver` | `internal/infra/mongo/` |
| Caching | memory / `go-redis` | `pkg/cache/` |
| Async jobs | memory / `go-redis` | `pkg/queue/` + `pkg/worker/` |
| Rate limiting | memory / `go-redis` | `pkg/ratelimit/` |
| Auth | `golang-jwt` + bcrypt | `pkg/auth/` |
| Validation | `go-playground/validator` | `pkg/validator/` |
| Logging | `go.uber.org/zap` | `pkg/logger/` |
| Migrations | `golang-migrate` | `cmd/migrate` + `migrations/` |
| API docs | `swaggo/swag` | `make swagger` -> `docs/` |

Rule of thumb: the **domain layer depends on abstractions** (`store.Store[T]`,
cache, queue), never on a concrete database. Swapping Postgres ↔ Mongo is a
single env change; the only place that branches on driver is `container.go`.

---

## Project structure

```
.
├── cmd/
│   ├── api/main.go               # HTTP API entry point (thin)
│   └── migrate/main.go           # Migration CLI (golang-migrate)
├── internal/
│   ├── app/
│   │   ├── app.go                # Lifecycle, router assembly, Swagger meta
│   │   └── container.go          # DI container (DB/cache/queue/auth wiring)
│   ├── config/config.go          # Env-backed configuration
│   ├── domain/user/              # Example domain
│   │   ├── entity.go             # Entity (db/bson/json tags)
│   │   ├── dto.go                # Request/response DTOs
│   │   ├── events.go             # Queue topics + payloads
│   │   ├── repository.go         # DB-agnostic repository
│   │   └── service.go            # Business logic
│   ├── handler/                  # HTTP handlers (user/admin/health)
│   ├── infra/                    # Postgres & Mongo connection setup
│   ├── middleware/               # RequestID, Logger, Recover, Auth, RBAC, ...
│   └── server/server.go          # HTTP server + graceful shutdown
├── pkg/                          # Reusable, app-agnostic building blocks
│   ├── auth/   cache/   logger/  queue/   ratelimit/
│   ├── response/   store/   validator/   worker/
│   ├── composio/  cursoragent/  github/  slack/   # Automation & notification clients
├── migrations/                   # *.sql (Postgres) + mongo/*.json
├── docs/                         # Generated OpenAPI/Swagger
├── test/integration/            # Integration tests (build tag: integration)
├── Dockerfile  docker-compose.yml  Makefile  .env.example
└── go.mod
```

---

## Startup flow

`cmd/api/main.go` is intentionally thin — it calls `app.New(ctx)` then
`app.Run()`. `app.New` runs in order:

1. `config.Load()` — read `.env`, then env vars; validate drivers + defaults.
2. `logger.New(level, isProduction)` — structured zap logger.
3. `BuildContainer(ctx, cfg, log)` — wire Redis (if needed), cache, queue, rate
   limiter, DB store, and the user service.
4. `buildRouter(cfg, log, container)` — chi router + middleware + handlers.
5. `server.New(cfg.HTTP, router, log)` — configured HTTP server.

`app.Run` traps `SIGINT`/`SIGTERM`, starts background workers, serves HTTP, then
gracefully shuts down the server and container.

---

## Configuration (env)

Config is loaded from `.env` (git-ignored; `.env.example` is the committed
template) and overridden by real env vars, parsed into a typed struct in
`internal/config`.

| Group | Variables |
|-------|-----------|
| App | `APP_NAME`, `APP_ENV`, `LOG_LEVEL` |
| HTTP | `HTTP_HOST`, `HTTP_PORT`, `HTTP_*_TIMEOUT` |
| Database | `DB_DRIVER` (`postgres`\|`mongo`), `DB_URI`, `MONGO_DATABASE`, pool settings |
| Cache | `CACHE_DRIVER` (`memory`\|`redis`), `CACHE_TTL`, `REDIS_*` |
| Queue | `QUEUE_DRIVER` (`memory`\|`redis`), `QUEUE_MAX_ATTEMPTS`, `QUEUE_BACKOFF` |
| Rate limit | `RATE_LIMIT_ENABLED`, `RATE_LIMIT_REQUESTS`, `RATE_LIMIT_WINDOW` |
| Auth | `JWT_SECRET`, `JWT_TTL`, `JWT_REFRESH_TTL`, `JWT_ISSUER` |

**Single-switch infrastructure:** `DB_DRIVER`, `CACHE_DRIVER`, and `QUEUE_DRIVER`
each select an implementation without touching application code.

---

## Database & storage (`Store[T]`)

Persistence goes through a generic, DB-agnostic interface in `pkg/store`:

```go
type Store[T any] interface {
    Create(ctx context.Context, entity *T) error
    GetByID(ctx context.Context, id string) (*T, error)
    Update(ctx context.Context, id string, entity *T) error
    Delete(ctx context.Context, id string) error
    List(ctx context.Context, q Query) ([]T, error)
}
```

Adapters (`pkg/store/postgres.go`, `pkg/store/mongo.go`) implement it for each
engine; a query builder (`query.go`) and transactions (`transaction.go`,
`TxManager`) round it out. The **only** place that picks an adapter is
`buildUserStore()` in `internal/app/container.go`.

---

## Domain layer (entity / service / repository)

Each domain is a self-contained package (see `internal/domain/user`):

- `entity.go` — the model, tagged for all backends (`db`, `bson`, `json`).
- `dto.go` — request/response shapes (decoupled from the entity).
- `repository.go` — domain-specific data access built on `Store[T]`.
- `service.go` — business logic; returns `*response.AppError` on failure.
- `events.go` — queue topics + payloads (e.g. `user.created`).

```go
repo := user.NewRepository(store)
svc  := user.NewService(repo, cache, queue, tokens, log)
created, err := svc.Register(ctx, user.RegisterInput{ /* ... */ })
```

---

## HTTP routing & handlers

Routes are assembled in `internal/app/app.go`. Handlers are thin: bind/validate
JSON, call the domain service, write a `pkg/response` envelope.

| Method | Path | Auth |
|--------|------|------|
| GET | `/healthz`, `/readyz` | none |
| GET | `/swagger/*` | none |
| POST | `/api/v1/auth/register` | none |
| POST | `/api/v1/auth/login` | none |
| POST | `/api/v1/auth/refresh` | none |
| GET / PUT | `/api/v1/users/{id}` | Bearer |
| GET | `/api/admin/users`, `/api/admin/users/{id}` | Bearer + admin |
| DELETE | `/api/admin/users/{id}` | Bearer + admin |

---

## Middleware

Applied in order in `buildRouter`:

1. `RequestID` — generate/propagate `X-Request-ID`.
2. `Recover` — turn panics into a 500.
3. `Logger` — structured request log (method, path, status, latency, request_id).
4. `Compress` — gzip responses.
5. `Timeout` — per-request context deadline (`HTTP_REQUEST_TIMEOUT`).
6. `CORS` — configurable cross-origin policy.
7. `RateLimit` — optional; per user ID (authenticated) or IP, fail-open.

`Logger` wraps the `http.ResponseWriter` to record the status code. The wrapper
forwards `Hijack` (and `Flush`/`Unwrap`) to the underlying writer so connection
upgrades keep working — without this, WebSocket/SSE handlers mounted under the
logging middleware fail with `http.Hijacker is unavailable`. Register long-lived
streaming routes (e.g. WebSockets) **outside** the `Timeout` middleware so their
context is not cancelled mid-connection.

---

## Auth & RBAC (JWT)

`pkg/auth` issues and verifies JWTs with separate **access** and **refresh**
token types and roles (`user`, `admin`); passwords are bcrypt-hashed.

```go
mux.With(middleware.Auth(tokens)).Get("/api/v1/users/{id}", h.Get)
mux.With(middleware.Auth(tokens), middleware.RequireRole(auth.RoleAdmin)).
    Get("/api/admin/users", h.List)
```

`Auth` injects parsed `Claims` into the request context; `RequireRole` /
`RequireAdmin` enforce RBAC after authentication.

---

## Responses & error handling

Every response uses one JSON envelope (`pkg/response`):

```json
{ "success": true, "data": { }, "error": null, "meta": { } }
```

Services return typed `*response.AppError` (HTTP status + stable `code`);
handlers call `response.Error(w, err)` to render them. Validation failures map to
`422` with a per-field error map **and** a single self-explanatory `message` that
summarises every field error using JSON paths (e.g.
`validation failed: board_url is required; repos[0].git_url must be a valid URL`),
so clients that only surface the top-level message still tell the user what to
fix. Unknown errors become `500`.

```go
if err != nil {
    response.Error(w, err)       // AppError -> proper status + code
    return
}
response.OK(w, dto)              // 200 with envelope
```

---

## Caching

`pkg/cache` offers memory and Redis backends behind one interface, plus a typed
helper:

```go
users := cache.NewTyped[User](backend, "user", cfg.CacheTTL)
u, err := users.Get(ctx, id)            // read-through
_ = users.Set(ctx, id, value)
```

The user service caches `GetByID` reads automatically.

---

## Queue & workers

`pkg/queue` (memory/Redis) plus `pkg/worker` provide typed enqueue/handle and an
interval scheduler. Jobs run in the background started by `container.StartBackground`.

```go
worker.Register(reg, func(ctx context.Context, e user.CreatedEvent) error {
    // send welcome email, etc.
    return nil
})
worker.Enqueue(ctx, queue, user.TopicCreated, event)
```

Retries honor `QUEUE_MAX_ATTEMPTS` / `QUEUE_BACKOFF`.

The `worker.Scheduler` runs registered tasks on fixed intervals and shuts down
**gracefully**: on `Stop` (wired into `container.Close`, triggered by
`SIGINT`/`SIGTERM`) it stops scheduling new runs but lets a task that is already
executing finish. The in-flight task's context is cancelled only if the shutdown
grace period expires, so a periodic job is never torn down mid-operation by a
restart/deploy. `Stop(ctx)` is bounded by `ctx`, so shutdown still cannot hang
forever.

---

## Rate limiting

`pkg/ratelimit` (memory/Redis) is wired as middleware. Enable and tune via
`RATE_LIMIT_ENABLED`, `RATE_LIMIT_REQUESTS`, and `RATE_LIMIT_WINDOW`. Limits key
on user ID when authenticated, otherwise client IP.

---

## Logging

`pkg/logger` wraps zap behind a small interface so call sites stay backend-free:

```go
log.Info("user created", "user_id", id, "request_id", reqID)
log.Error("db failure", "error", err)
```

Production mode emits JSON; development mode is human-readable.

---

## Migrations

Migrations live in `migrations/` (`*.sql` for Postgres, `mongo/*.json` for
Mongo). The CLI auto-selects the right set based on `DB_DRIVER`.

```bash
make migrate-up        # apply
make migrate-down      # roll back last
```

`docker-compose.yml` also seeds the first Postgres migration on initial boot.

### Deploy-time schema handling (`scripts/refresh-daemon.sh` + `scripts/verify-migrations.sh`)

`scripts/refresh-daemon.sh` (invoked by the CI/CD deploy job) is migrate-aware:
when the deployed compose file defines a `migrate` service it runs `migrate up`
as an `ExecStartPre` on every start (so a completed one-shot from an older image
can never skip pending changes), stops the existing stack before restart, and —
once the stack is up — runs `scripts/verify-migrations.sh` as a deploy gate.
Services with no `migrate` service are unaffected.

`scripts/verify-migrations.sh` verifies the live schema and is hardened for use
as a gate that runs seconds after a restart, while containers are still settling:

- probes (`migrate version`, optional column check) are **retried**, so a
  one-off `docker compose run` that transiently fails to create its container
  does not roll a good deploy back;
- on a genuine failure the probe's output is **printed, never swallowed**, so a
  rollback always logs the real reason instead of a bare exit code;
- only infrastructure/transient probe errors retry — a real verification failure
  (dirty schema, version below the required minimum, missing column) fails fast;
- probes run docker **as root** (like `refresh-daemon.sh`), since the deploy
  dir's `.env` is a root-owned `0600` secret and the system docker socket needs
  root — running as the unprivileged deploy user fails with `permission denied`.

It is service-agnostic. Configure the gate from the calling service via env:

```bash
# required minimum applied version (default: 0 = any version)
VERIFY_MIGRATIONS_MIN_VERSION=20260712060023
# optional: assert specific columns exist on a table after migrating
VERIFY_COLUMNS_TABLE=suppliers
VERIFY_REQUIRED_COLUMNS="phone_number address supports_delivery delivery_cost"
```

`VERIFY_MIGRATIONS_CMD` / `VERIFY_COLUMNS_CMD` override the probes for testing
(see `internal/deploy/verify_migrations_test.go`).

---

## API docs (Swagger)

Handlers carry `swaggo` annotations. Regenerate the spec after changing them:

```bash
make swagger           # writes docs/{docs.go,swagger.json,swagger.yaml}
```

The UI is served at `/swagger/index.html`.

---

## Automation clients (composio / cursoragent / github)

`pkg/composio`, `pkg/cursoragent`, and `pkg/github` are standalone, well-tested
clients for orchestration workflows (e.g. launch a Cursor Cloud Agent, track a
GitHub PR to merge, drive Trello via Composio). They are **not** imported by the
HTTP app — keep them if useful, delete them if not.

| Package | Purpose |
|---------|---------|
| `composio` | Composio tool-execution API + typed Trello and GitHub helpers |
| `cursoragent` | Cursor Cloud Agents REST API (launch, status, MCP config, auto-PR) |
| `github` | Minimal GitHub REST client (PR state, URL parsing, merge checks) |

`composio` brokers third-party access (Trello, GitHub, ...) behind a single
execute endpoint, so callers never handle upstream credentials. Highlights:

- **Per-service connected accounts.** `Config.ConnectedAccountIDs` is an ordered
  fallback list; `Execute` tries each account in turn and returns the first
  success (or the last error if all fail). Use `Client.ForAccounts(ids...)` to
  derive a scoped client so each toolkit helper targets its own account(s) -
  e.g. one set of accounts for Trello and a different set for GitHub, including
  multiple identities within a single toolkit.
- **Per-service entity (user_id).** Composio resolves a connected account within
  its owning entity (`user_id`). When toolkits are connected under different
  entities (e.g. Trello under one username, GitHub under another),
  `Client.ForEntity(userID, ids...)` pins the matching `user_id` alongside the
  connected accounts so each helper talks to the right entity.
- **Trello helper** (`NewTrello`): list/read/move/update cards and comment, plus
  `ParseBoardID` to extract a board id/shortLink from a Trello board link
  (`https://trello.com/b/abc123/my-board`) so users can paste the browser URL.
  `GetCard` requests an explicit `fields` set and sub-resource includes (labels,
  members, attachments, checklists, due date, recent comments) because Trello's
  GET card endpoint otherwise returns only `id` and `badges`, leaving orchestrator
  agents with an empty title/description. `Card.Comments()` flattens
  `commentCard` actions with author fallback when display names are absent.
- **GitHub helper** (`NewGitHub`): open a pull request via Composio
  (`CreatePullRequest`, `GITHUB_CREATE_A_PULL_REQUEST`) so the PR is authored by
  the GitHub account connected in Composio — not by an ambient credential — and
  read a pull request (`GetPullRequest`, `GITHUB_GET_A_PULL_REQUEST`) to detect
  merges, plus `ParsePullRequestURL` and `ParseRepoSlug` (derive an `owner/repo`
  slug from a repository URL). Prefer this over the standalone `github` REST
  client when GitHub access should be brokered through Composio rather than a
  personal access token. When pairing with `cursoragent`, launch the run with
  auto-PR disabled and open the PR yourself via `CreatePullRequest` from the
  branch the agent pushed, otherwise Cursor opens the PR as its own linked GitHub
  identity instead of the Composio-connected account.

`cursoragent` highlights:

- **Reading a run's result** (`ResultText`): concatenates the agent's assistant
  output from the conversation. The Cursor API tags conversation messages with
  types `user_message` and `assistant_message`, so `ResultText` keeps any
  assistant-typed message (matching both `assistant` and `assistant_message`,
  plus empty/legacy types) and always drops user-authored messages. This is what
  callers parse for a final JSON block the agent was instructed to emit; matching
  the wrong type yields empty output and a spurious "no JSON found" failure
  downstream.

---

## Slack notifications (slack)

`pkg/slack` is a small, standalone client for posting messages to Slack via
Incoming Webhooks. It is **not** imported by the HTTP app — wire it in your
service's container when you need operational alerts.

| Type | Purpose |
|------|---------|
| `Client` | Posts `Message` or plain text to a webhook URL |
| `Notifier` | Binds a `Client` to one webhook URL for dependency injection |

Highlights:

- **Safe no-op when unconfigured.** An empty webhook URL skips the HTTP call, so
  services start cleanly without Slack credentials.
- **URL validation.** Only `https://hooks.slack.com/services/...` URLs are
  accepted; errors and logs use `RedactWebhookURL` so secrets never appear in
  full.
- **Per-use-case webhooks.** Product services typically define env-backed config
  (e.g. `SLACK_WEBHOOK_URL`, `SLACK_WEBHOOK_ORCHESTRATOR`) and construct a
  `Notifier` per use case via `NewNotifier(client, webhookURL)`.

Example:

```go
client := slack.New(slack.Config{})
notifier := slack.NewNotifier(client, cfg.Slack.WebhookFor("orchestrator"))
if err := notifier.Notify(ctx, "Ticket picked up: LUNA-15"); err != nil {
    log.Warn("slack notify failed", zap.Error(err))
}
```

---

## Testing

```bash
make test               # unit tests (race + coverage)
make test-integration   # integration tests (-tags=integration)
make bench              # benchmarks
```

Integration tests (`test/integration/`) spin up the real router via
`App.Handler()` against a migrated database; run `make migrate-up` first.

---

## Docker

```bash
make docker-up          # api + postgres:16 + redis:7
make docker-down
```

The compose stack runs the API against Postgres with Redis-backed cache/queue
and rate limiting enabled.

**Secret injection:** the `api` service loads your `.env` via `env_file`, so every
credential/setting defined there (e.g. `JWT_SECRET`, API keys) is injected into
the container on startup — no secrets are hardcoded in `docker-compose.yml`. The
file is optional (`required: false`), so the stack still boots from the built-in
defaults when no `.env` exists. The explicit `environment` entries (DB/Redis
hosts, drivers) target the compose network and take precedence over `.env`.

**MongoDB requires a replica set.** The store uses multi-document transactions,
which MongoDB only permits on a replica set (a standalone node returns
`Transaction numbers are only allowed on a replica set member or mongos`). The
commented `mongo` service therefore runs as a single-node replica set
(`--replSet rs0`) and self-initiates via its healthcheck; point the api at it
with `DB_URI=mongodb://mongo:27017/?replicaSet=rs0`.

---

## Creating a new service from this template

1. Copy the repo (or use it as a starter).
2. Rename the module: `module` in `go.mod` + update
   `github.com/cymonevo/go_template/...` imports.
3. `cp .env.example .env` and set `DB_DRIVER`/`DB_URI`, `JWT_SECRET`, etc.
4. Model your first domain under `internal/domain/<name>/` following the `user`
   package; register it in `container.go` and add routes in `app.go`.
5. Add migrations under `migrations/` (and `migrations/mongo/` if needed).
6. Regenerate docs (`make swagger`) and run (`make run`).
7. Delete the example `user` domain and any unused `pkg/` clients.

---

## Dependencies

| Package | Use |
|---------|-----|
| `go-chi/chi` + `go-chi/cors` | HTTP router + CORS |
| `caarlos0/env` + `joho/godotenv` | Env-based configuration |
| `jackc/pgx` | PostgreSQL driver/pool |
| `go.mongodb.org/mongo-driver` | MongoDB driver |
| `redis/go-redis` | Redis (cache, queue, rate limit) |
| `golang-migrate/migrate` | Schema migrations |
| `golang-jwt/jwt` | JWT auth |
| `golang.org/x/crypto` | bcrypt password hashing |
| `go-playground/validator` | Request validation |
| `go.uber.org/zap` | Structured logging |
| `google/uuid` | Request/entity IDs |
| `swaggo/swag` + `http-swagger` | OpenAPI generation + UI |
