# BookMyShow

A cinema-management and ticket-booking backend written in Go — a modular
monolith building toward seat holds, bookings, payments, and reporting.
The full product and technical specification lives in
[`docs/bookmyshow_specs.md`](docs/bookmyshow_specs.md).

**Current status: early milestone.** The `identity` module (user accounts,
JWT authentication) is implemented. Booking, holds, and payments are
specified but not yet built.

## Tech stack

- **Go 1.27** (version pinned in `go.mod`)
- **PostgreSQL 17** accessed through `pgx/v5` (`database/sql`)
- **golang-migrate** SQL migrations, **JWT (HS256)** authentication
- Config from `config.yaml` via cleanenv, validation via go-playground/validator
- HTTP layer: stdlib `net/http` only — no router or web framework

## Quick start

Prerequisites: Go (matching `go.mod`), Docker, and `make`.

1. Start Postgres:

   ```bash
   docker compose up -d
   ```

2. Create `config.yaml` in the repo root (it holds secrets and is
   git-ignored — keep real values out of version control):

   ```yaml
   env: dev
   db_url: postgres://user:password@localhost:5432/bms_db?sslmode=disable
   jwt_secret: generate-a-real-secret-here
   http_server:
     address: 0.0.0.0
     port: "8080"
   log:
     level: info
     format: text
   ```

3. Apply migrations and run the API:

   ```bash
   make migrate-up   # one-time
   make sprint       # build + run
   ```

   `make run` alone executes `./bin/api` without rebuilding, so run
   `make build` first after code changes. Migrations are never run
   automatically; `make migrate-down` rolls back one migration.

## API — identity module

All routes are prefixed with `/api/v1/identity`. Responses and errors are
JSON. Login issues a 10-minute HS256 bearer token.

| Method | Path             | Auth    | Description                            |
| ------ | ---------------- | ------- | -------------------------------------- |
| POST   | `/auth/register` | none    | Create an account (password strength + phone rules apply) |
| POST   | `/auth/login`    | none    | Exchange credentials for a bearer token |
| GET    | `/{id}`          | Bearer  | Fetch a user profile (own record only)  |
| DELETE | `/{id}`          | Bearer  | Soft-delete own account                 |

## Tests

Unit tests live next to the code and run without any database. Repository
SQL is covered by build-tagged integration tests that run against a
Postgres of your choice (never the dev database) and skip when
`BMS_TEST_DB_URL` is unset.

```bash
# Unit tests — the same gate CI enforces (gofmt + vet must also pass)
gofmt -l .            # must print nothing
go vet ./...
go test -race -count=1 -timeout 60s ./...

# Integration tests
docker run --rm -p 5433:5432 \
  -e POSTGRES_USER=user -e POSTGRES_PASSWORD=password \
  -e POSTGRES_DB=bms_db_test postgres:17-alpine &
BMS_TEST_DB_URL='postgres://user:password@localhost:5433/bms_db_test?sslmode=disable' \
  go test -race -count=1 -timeout 120s -tags=integration ./modules/identity/internal
```

## CI

[`.github/workflows/ci.yml`](.github/workflows/ci.yml) runs on pushes to
`main` and on every pull request:

- **check** — gofmt, `go vet`, and race-detected unit tests
- **integration** — the repository tests against a throwaway Postgres
  service container

## Project structure

```
cmd/api/                  HTTP server wiring (no business logic)
cmd/migrate/              migration runner
modules/
  identity/
    module.go             public surface for other modules
    internal/             dto, model, errors, handler, routes, repository, service
  platform/               shared infra: config, database, httputil, logger,
                          middleware, types, validator
migrations/               golang-migrate pairs: NNNNNN_name.{up,down}.sql
docs/                     product & technical specification
```

Request flow is strictly `handler → service → repository`: handlers
parse/validate/respond, SQL lives only in repositories, and business rules
live only in services. Each module's `internal/` package is private to the
module; consumers use it through `module.go`.

See [`AGENTS.md`](AGENTS.md) for the working conventions agents follow in
this repository.
