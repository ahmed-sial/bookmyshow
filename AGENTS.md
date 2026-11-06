# bookmyshow

Go backend for a ticket-booking app. Module: `github.com/ahmed-sial/bookmyshow`.
Go version is in `go.mod`; do not bump it or add dependencies without asking.

## Layout
- `cmd/api` (HTTP server) and `cmd/migrate` (migration runner): wiring only, no business logic.
- `modules/<name>/`: one feature each. Public surface is `module.go`.
  Internals live in `modules/<name>/internal/`: `dto.go`, `model.go`, `errors.go`,
  `repository.go`, `service.go`, `handler.go`, `routes.go` (+ `password.go` in identity).
  Copy the `modules/identity` shape for any new module.
- `modules/platform/`: shared infra (`config`, `database`, `httputil`, `logger`,
  `middleware`, `types`, `validator`). Platform never imports a feature module.
- `migrations/`: golang-migrate SQL, `NNNNNN_name.up.sql` + `.down.sql`.
- Flow: handler -> service -> repository. Handlers parse/validate/respond only;
  SQL only in repositories; business rules only in services.
- A module's `internal/` cannot be imported from outside it. Other modules use it only through `module.go`.
- Only `identity` is implemented so far; the full scope (state machines, data model,
  booking-transaction design) is specified in `docs/bookmyshow_specs.md`. Read the
  relevant spec section before building a feature.

## Commands
- `make run` executes `./bin/api` without building — run `make build` first (or `make sprint` = build + run).
  It needs `config.yaml` (sets `CONFIG_PATH=config.yaml`) and Postgres via `docker compose up -d` (`bms_db`, port 5432).
- Migrations: `CONFIG_PATH=config.yaml make migrate-up` / `make migrate-down` (the Makefile
  does not set `CONFIG_PATH`; run from repo root, `migrate` uses the relative `file://migrations`).
- Check (run before saying a task is done, never claim success without running it):

  ```bash
  gofmt_files="$(gofmt -l .)"
  if [ -n "$gofmt_files" ]; then
    printf 'Files requiring gofmt:\n%s\n' "$gofmt_files"
    exit 1
  fi

  go vet ./...
  go test -race -count=1 -timeout 60s ./...
  ```

- One test: `go test -race -count=1 -timeout 60s -run '^TestName$' ./modules/identity/internal -v`
- Keep `-race` and `-count=1`. Do not drop them to go faster.

## Errors
- Wrap with context and `%w`: `fmt.Errorf("create user: %w", err)`. Never `%v`.
- For error matching, use errors.Is() and errors.As(). Never directly compare
  wrapped errors with ==.
- Comparing an error with nil using == or != is correct.
- Handle once: add context and return. Do not log and return the same error.
- Repositories map pgx errors (`pgx.ErrNoRows`, unique violation 23505) to the domain
  errors in the module's `errors.go`. Driver errors never leak above the repository.
- Use platform/httputil for all HTTP success and error responses.
- Do not write ad-hoc JSON responses, call http.Error directly, or expose
  internal error messages to clients.
- No naked returns. No `panic` outside `main()`.

## Concurrency
- Every goroutine needs a guaranteed exit path. Prefer `errgroup.WithContext` when
  `golang.org/x/sync` is available — it is in go.mod (indirect). Never promote a
  dependency to direct or add a new one without asking. Otherwise, use appropriate
  standard-library primitives.
- Functions and methods performing I/O must accept `context.Context` as their
  first parameter, unless a standard-library interface prescribes another
  signature (such as `http.Handler`).
- Always propagate the request context from handlers using `r.Context()`.
  Never accept a context and silently discard it.
- Never range over a map to produce output. Sort the keys.

## Style the linter cannot enforce
- Interfaces are declared by the CONSUMER (the service declares the small repository
  interface it needs, 1-3 methods). Return concrete types.
- HTTP uses the stdlib `net/http`; do not add a router or web framework.
- DTO validation handles input structure and format through validation tags +
  `platform/validator`.
- Business rules, domain invariants, and business validation belong in services.
- Handlers must not contain ad-hoc validation or business rules.
- Never expose `model.go` types in responses. Map model <-> DTO explicitly.
- SQL: parameterized pgx queries only. Never build SQL with string concatenation or `fmt.Sprintf`.
- Log only through `platform/logger`. No `fmt.Println` or `log.Printf` in app code.
- Prefer IDs are `uuid.UUID` when `google/uuid` is already available. Never add it as a dependency
  without asking. Otherwise, use appropriate standard-library primitives.
- Table-driven tests with `t.Run`. Test services and `password.go` first.
  Do not run tests against the dev database.
- Use standard-library APIs supported by the Go version specified in go.mod.
  Prefer os.ReadFile over ioutil.ReadFile, any over interface{}, and the
  standard-library slices/maps packages when supported.
- Prefer concrete types; generics need a reason.

## Landmines
- Auth: JWT claims live in `platform/types/claims.go` and are checked by
  `middleware/require_auth.go`. Do not change the claims shape, signing method, or
  expiry without asking. Always verify the signing algorithm when parsing.
- Passwords: hash and compare only in `identity/internal/password.go`. Never store,
  log, or return a password or hash (tag hash fields `json:"-"`).
- Migrations: write them, never run them. A human runs `make migrate-up` / `migrate-down`.
  Never edit an applied migration; add the next numbered pair, with a working `.down.sql`.
- `config.yaml` holds secrets. Never commit it, print its values, or hardcode secrets.
  Config is read via `platform/config` (cleanenv).
- Do not rename or move packages, or change the module path, without asking.

## Notes
- `skills-lock.json` pins the installed `samber/cc-skills-golang` skill collection;
  manage it through the skills tool, not by hand.
- No CI config exists; the Check block in Commands is the verification gate.

## Working with me
- I am new to Go. Prefer simple, idiomatic code over clever abstractions, and add a
  one-line explanation when you use a Go idiom that is not obvious.
- Small diffs, one concern per change. Do not refactor unrelated code.
- If the task is ambiguous, touches a landmine, or the spec is unclear, ask before editing.