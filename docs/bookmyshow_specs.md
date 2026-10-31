# BookMyShow — Product Requirements & Technical Specification

**Document version:** 1.0  
**Status:** Initial implementation specification  
**Product:** Cinema operations and online ticket-booking backend  
**Primary implementation language:** Go  
**Primary database:** PostgreSQL  
**Architecture:** Modular monolith  
**Audience:** Product owner, backend developer, tester, and future maintainers

---

## 1. Product overview

BookMyShow is a complete cinema-management and ticket-booking system. It supports the cinema's internal operations—movies, screens, seats, showtimes, pricing, staff, concessions, and reporting—as well as a customer-facing booking API.

The defining technical requirement is **correctness under concurrency**: multiple customers may attempt to reserve the same seat at the same time, but the system must never create overlapping active reservations for that seat and showtime.

BookMyShow is initially designed for one cinema business that may operate multiple branches. Its data model should not prevent adding more cinema businesses later, but multi-tenant SaaS administration is not an MVP requirement.

### Product goals

1. Manage the cinema's catalogue, locations, screens, seat layouts, showtimes, prices, and operational status.
2. Let customers discover showtimes, choose seats, hold seats temporarily, pay through a payment-provider adapter, and receive tickets.
3. Prevent double booking and recover safely from timeouts, retries, process crashes, and duplicate payment callbacks.
4. Support front-desk staff and managers with booking lookup, ticket validation, refunds, concessions, and operational reporting.
5. Provide auditable administrative actions, secure role-based access, and useful observability.
6. Demonstrate production-grade Go backend engineering without unnecessary microservices or infrastructure.

### Non-goals for the first release

- Building a complete frontend, mobile app, or point-of-sale hardware integration.
- Real payment processing with live funds. Use a deterministic payment simulator initially.
- Streaming movies or managing digital rights.
- AI recommendations, loyalty gamification, or social features.
- Multi-region active-active deployment.
- Microservices, Kubernetes, or Kafka before a measured requirement exists.

---

## 2. Users and roles

### Customer
- Browse movies, cinema branches, dates, and showtimes.
- View available seats and prices.
- Create and manage a booking.
- Pay through the configured payment adapter.
- Retrieve ticket QR codes and booking details.
- Cancel or request a refund when policy permits.
- View booking history.

### Box-office staff
- Search bookings by booking reference, customer contact, or ticket code.
- Create walk-in bookings using the same inventory rules as online bookings.
- Confirm payment through an approved method.
- Reprint or resend tickets.
- Validate tickets at entry.
- Process permitted cancellations or refunds.

### Cinema manager
- Manage movies, branches, screens, seat maps, showtimes, pricing, promotions, and booking policies.
- View occupancy, sales, refunds, and concession reports.
- Configure operational settings and manage staff access.

### Administrator
- Manage staff accounts, roles, permissions, audit logs, and system-wide settings.
- Perform controlled operational interventions.
- Cannot silently alter financial records; corrections must be auditable.

### Concession staff
- View and fulfil concession orders.
- Update order status.
- Cannot access unrelated customer data or modify showtimes and seat inventory.

### System worker
- Expires seat holds, reconciles pending payments, sends notifications, and processes durable background jobs.
- Uses internal service permissions, not a human login.

---

## 3. Core business rules

These rules are mandatory and must be enforced by the backend/database, not just by a client.

### 3.1 Showtime and inventory
1. A showtime belongs to one movie, one screen, and one scheduled start/end time.
2. A screen belongs to a branch and has a configured seat layout.
3. A seat's identity is stable within a screen, such as `A-01`. A showtime's inventory is separate from the screen's physical seat definition.
4. A showtime cannot be published if it overlaps another showtime on the same screen. Include a configurable cleanup/turnaround buffer.
5. Only active, sellable seats can be booked. Broken, blocked, or maintenance seats cannot be sold.
6. A showtime can be cancelled by an authorized manager. Customers with affected bookings must be eligible for notification and refund processing.
7. Times are stored as `timestamptz` in PostgreSQL and returned as ISO 8601 timestamps. The cinema's configured time zone is used for customer-facing schedules and business reports.

### 3.2 Seat holds and bookings
1. A seat hold temporarily reserves one or more seats for a particular showtime and customer/session.
2. A hold has an expiry timestamp, configurable by policy; default 5 minutes.
3. A hold request for multiple seats succeeds entirely or fails entirely. Partial holds are not allowed.
4. The database must be the final authority on inventory. Application-level mutexes alone are insufficient.
5. Two active holds/bookings cannot own the same seat for the same showtime.
6. Expired holds are treated as unavailable only until their expiry is reclaimed/recognized; all reservation operations must correctly handle expiry even if the cleanup worker is delayed.
7. A customer cannot extend a hold indefinitely. Default: one extension at most, adding 2 minutes, only if the hold has not expired and no payment attempt has started. This is configurable.
8. Booking creation and payment initiation must be idempotent using an `Idempotency-Key` and a persisted request/result record.
9. A booking must not be confirmed based only on a browser redirect. Confirmation comes from a verified provider result or a verified simulator callback.
10. Repeated provider callbacks must not duplicate payment records, tickets, emails, or state transitions.
11. If payment succeeds after a hold expires or inventory is no longer available, the system must enter a recoverable `payment_review`/compensation state and initiate a refund or manual review. It must not silently oversell.
12. A confirmed booking is not automatically released by the hold-expiry worker.
13. Seat release and booking cancellation must be transactional and auditable.
14. A ticket is valid only for its intended showtime and only once for admission, unless a manager-authorized re-entry policy is later implemented.

### 3.3 Pricing and fees
1. Prices are stored as integer minor units (for example, paisa/cents), never floating-point values.
2. Each booking stores a price snapshot for each ticket so future price changes do not alter past bookings.
3. Price may depend on branch, screen/class, showtime, day, seat type, promotion, and booking channel.
4. A quote includes ticket subtotal, discount, fees, taxes if configured, concession subtotal, and final total.
5. Promotions must have validity windows, usage limits, eligibility rules, and a clear stacking policy.
6. All monetary operations and refunds are auditable.

### 3.4 Cancellation and refunds
1. Cancellation eligibility depends on showtime start time, booking state, and cinema policy.
2. The initial default policy is configurable; suggested default: customer cancellation allowed until 2 hours before showtime, subject to manager settings.
3. Refund status is separate from booking status.
4. Refund requests are idempotent and may remain pending while the provider processes them.
5. Staff overrides require a reason and an audit record.
6. A cancelled showtime triggers an operational workflow for affected bookings, customer notification, and refund/credit handling.

### 3.5 Access and audit
1. All management endpoints require authentication and role/permission authorization.
2. Customers can only view or modify their own bookings, except staff with explicit permissions.
3. Staff must not be able to grant themselves roles.
4. Sensitive actions—including refunds, price changes, schedule changes, seat blocking, and role changes—must create audit events.
5. Secrets, payment credentials, access tokens, and full payment details must never be logged.
6. Never store raw card numbers or CVVs.

---

## 4. Functional requirements

Priority definitions: **P0** = required for first usable release; **P1** = important next milestone; **P2** = later enhancement.

### 4.1 Identity and access

- **AUTH-01 (P0):** Staff can log in and receive short-lived access tokens and refresh credentials, or use a secure server-side session strategy.
- **AUTH-02 (P0):** Support customer registration and login by email/password.
- **AUTH-03 (P0):** Store passwords using a modern password hash such as Argon2id or bcrypt with appropriate parameters; never store plaintext.
- **AUTH-04 (P0):** Implement role-based access control for administrator, manager, box-office staff, concession staff, and customer.
- **AUTH-05 (P0):** Support logout/revocation of refresh sessions.
- **AUTH-06 (P0):** Apply login rate limits and generic authentication error responses.
- **AUTH-07 (P1):** Email verification and password-reset flows using expiring, single-use tokens.
- **AUTH-08 (P1):** Optional staff MFA.
- **AUTH-09 (P0):** Record security-relevant audit events without logging credentials.

### 4.2 Cinema branches and screens

- **CIN-01 (P0):** Create and edit cinema branches with name, address, contact details, time zone, and status.
- **CIN-02 (P0):** Create screens within a branch with name/number, screen type, capacity, accessibility metadata, and status.
- **CIN-03 (P0):** Define screen seat layouts with row/seat labels, seat class, accessibility designation, and coordinates for seat-map rendering.
- **CIN-04 (P0):** Mark seats active, blocked, or under maintenance.
- **CIN-05 (P0):** Prevent layout changes that invalidate seats in already-published or booked showtimes. Use versioned layouts or require a controlled migration.
- **CIN-06 (P1):** Record screen equipment and maintenance notes.

### 4.3 Movie catalogue

- **MOV-01 (P0):** Create, edit, publish, archive, and search movies.
- **MOV-02 (P0):** Store title, synopsis, runtime, release date, language, age rating, genres, poster URL, trailer URL, and publication status.
- **MOV-03 (P0):** Support cast/crew credits and multiple languages/subtitles.
- **MOV-04 (P0):** Support movie status such as draft, published, archived; only published movies appear in public listings.
- **MOV-05 (P1):** Store external catalogue identifiers and metadata source.
- **MOV-06 (P1):** Maintain movie images/media metadata as URLs or object-storage references; do not store large binaries in PostgreSQL.

### 4.4 Showtimes and scheduling

- **SHOW-01 (P0):** Create a showtime for a movie, branch, screen, start/end time, language, format, and pricing plan.
- **SHOW-02 (P0):** Calculate end time from runtime plus configured trailers/turnaround where appropriate, while allowing authorized override.
- **SHOW-03 (P0):** Reject overlapping showtimes on a screen, including configured cleanup buffers.
- **SHOW-04 (P0):** Publish, unpublish, reschedule, and cancel showtimes with audit records.
- **SHOW-05 (P0):** Expose public showtime search by movie, branch, date, language, and format.
- **SHOW-06 (P0):** Freeze or version booking-relevant showtime data after sales begin; changes that affect customers require an explicit reschedule/cancellation workflow.
- **SHOW-07 (P1):** Support recurring schedule templates and bulk schedule creation.
- **SHOW-08 (P1):** Prevent sales after a configured cutoff before showtime.

### 4.5 Discovery and seat map

- **DISC-01 (P0):** Public APIs list published movies and their details.
- **DISC-02 (P0):** Public APIs list showtimes with availability summaries.
- **DISC-03 (P0):** Return a seat map showing available, held, sold, blocked, and inaccessible states without exposing another customer's identity.
- **DISC-04 (P0):** Availability is advisory only; a seat becomes owned only after a successful transactional hold.
- **DISC-05 (P1):** Add search, pagination, sorting, and filtering for movie catalogue and showtimes.

### 4.6 Seat hold, checkout, booking, and tickets

- **BOOK-01 (P0):** Hold one or more seats atomically for a showtime.
- **BOOK-02 (P0):** Return hold ID, expiry timestamp, selected seats, quote, and server time.
- **BOOK-03 (P0):** Release a hold explicitly when the customer abandons checkout.
- **BOOK-04 (P0):** Expire abandoned holds through both request-time correctness checks and a background worker.
- **BOOK-05 (P0):** Create a booking from a valid hold and preserve price snapshots.
- **BOOK-06 (P0):** Use idempotency keys for hold/booking/payment operations where retries could cause duplicate effects.
- **BOOK-07 (P0):** Track booking states with validated transitions.
- **BOOK-08 (P0):** Generate a unique, unguessable ticket token or QR payload for each ticket.
- **BOOK-09 (P0):** Allow customer and authorized staff to retrieve booking details.
- **BOOK-10 (P0):** Support ticket validation/check-in and reject repeated check-ins.
- **BOOK-11 (P0):** Support booking lookup for box-office staff.
- **BOOK-12 (P1):** Resend booking confirmation and tickets.
- **BOOK-13 (P1):** Support walk-in bookings through the same inventory and pricing domain services.

### 4.7 Payments and refunds

- **PAY-01 (P0):** Define a payment-provider interface independent of any specific vendor.
- **PAY-02 (P0):** Implement a local deterministic simulator for success, failure, delayed result, timeout, and duplicate callback scenarios.
- **PAY-03 (P0):** Persist payment attempts and provider references.
- **PAY-04 (P0):** Verify callback signatures in production adapters and make callback handling idempotent.
- **PAY-05 (P0):** Reconcile payment attempts stuck in pending state.
- **PAY-06 (P0):** Keep booking, payment, and refund states separate and validate transitions.
- **PAY-07 (P0):** If payment succeeds but the booking cannot be confirmed, record a compensation/refund action and alert staff.
- **PAY-08 (P1):** Add a real payment adapter after the simulator and failure tests are reliable.
- **PAY-09 (P1):** Support partial/full refunds when provider and policy allow; all requests must be idempotent and auditable.

### 4.8 Promotions and pricing

- **PRICE-01 (P0):** Configure base ticket prices by branch, screen/seat class, and showtime where necessary.
- **PRICE-02 (P0):** Calculate quotes server-side and store price snapshots.
- **PRICE-03 (P1):** Create coupon codes with validity dates, usage limits, minimum spend, and eligible showtimes/movies.
- **PRICE-04 (P1):** Support configurable child/student/senior categories without assuming eligibility rules.
- **PRICE-05 (P1):** Define whether discounts stack and enforce the policy deterministically.

### 4.9 Concessions

- **CON-01 (P1):** Maintain concession products, categories, prices, stock status, and availability by branch.
- **CON-02 (P1):** Let customers add concession items to a booking or separate order.
- **CON-03 (P1):** Track order status: placed, accepted, preparing, ready, fulfilled, cancelled.
- **CON-04 (P1):** Prevent invalid quantity and stock transitions.
- **CON-05 (P1):** Allow concession staff to fulfil orders and view only operationally necessary details.
- **CON-06 (P2):** Add ingredient/allergen metadata and inventory replenishment records.

### 4.10 Notifications

- **NOT-01 (P0):** Record notification intents for booking confirmation, cancellation, schedule change, and refund updates.
- **NOT-02 (P0):** Use a transactional outbox so a committed booking and its notification intent cannot be separated by a process crash.
- **NOT-03 (P0):** Provide a local email/log adapter for development and tests.
- **NOT-04 (P1):** Add production email/SMS adapters, delivery attempts, retry policy, and dead-letter handling.
- **NOT-05 (P1):** Avoid sending duplicate notifications when jobs retry.

### 4.11 Reports and operations

- **OPS-01 (P0):** Provide manager reports for bookings, gross sales, refunds, and occupancy by date, branch, movie, and showtime.
- **OPS-02 (P0):** Define metrics precisely: sold seats, held seats, confirmed tickets, gross amount, refunded amount, and net amount must not be conflated.
- **OPS-03 (P0):** Provide a health/readiness endpoint and structured application logs.
- **OPS-04 (P0):** Record administrative audit events.
- **OPS-05 (P1):** Add CSV exports for reports and bookings with permission checks.
- **OPS-06 (P1):** Track failed background jobs, expired holds, pending payments, notification failures, and compensation actions.
- **OPS-07 (P1):** Add dashboard metrics and alert thresholds.

---

## 5. State machines

Implement transitions in domain services and test every allowed and forbidden transition.

### 5.1 Hold state
`active -> converted`  
`active -> released`  
`active -> expired`

No transition out of `converted`, `released`, or `expired`. A hold cannot be converted after its expiry. State changes must be transactional.

### 5.2 Booking state
Suggested states:
- `pending_payment`
- `confirmed`
- `cancelled`
- `completed`
- `payment_review`

Typical transitions:
- `pending_payment -> confirmed` after verified payment success and inventory validation.
- `pending_payment -> cancelled` after payment failure/timeout and policy-driven release.
- `pending_payment -> payment_review` if payment succeeds but confirmation is no longer safe.
- `confirmed -> cancelled` through an authorized cancellation workflow.
- `confirmed -> completed` after showtime ends or an operational completion process.
- `payment_review -> confirmed` only after explicit reconciliation proves it is safe.
- `payment_review -> cancelled` after refund/compensation is recorded.

Do not use one status field to represent booking, payment, refund, and ticket admission simultaneously.

### 5.3 Payment state
`created -> pending -> succeeded | failed | cancelled | review`

Provider callbacks may arrive more than once or out of order. Store provider event IDs and enforce idempotency. Never regress a terminal state because a stale callback arrived.

### 5.4 Refund state
`requested -> pending -> succeeded | failed | review`

Refund amount must not exceed the refundable amount. Duplicate requests with the same idempotency key return the same result.

### 5.5 Ticket state
`valid -> checked_in`  
A ticket may become `void` if its booking is cancelled/refunded according to policy. A checked-in ticket cannot be checked in again.

---

## 6. Architecture and implementation constraints

### 6.1 Architecture

Use a **modular monolith**: one deployable Go application, one PostgreSQL database, clear internal module boundaries. Avoid premature microservices.

Suggested modules:
- `identity`
- `cinema`
- `catalog`
- `scheduling`
- `pricing`
- `inventory`
- `booking`
- `payment`
- `ticketing`
- `concessions`
- `notification`
- `reporting`
- `audit`
- `platform` (configuration, logging, database, HTTP, clock, IDs)

Modules should expose narrow service interfaces. Avoid circular dependencies. Keep HTTP handlers thin; business rules belong in domain/application services, and persistence belongs in repositories or focused SQL query packages.

### 6.2 Suggested repository layout

```text
seatlock/
├── cmd/
│   └── seatlock/
│       └── main.go
├── internal/
│   ├── identity/
│   ├── cinema/
│   ├── catalog/
│   ├── scheduling/
│   ├── pricing/
│   ├── inventory/
│   ├── booking/
│   ├── payment/
│   ├── ticketing/
│   ├── concessions/
│   ├── notification/
│   ├── reporting/
│   ├── audit/
│   └── platform/
│       ├── config/
│       ├── database/
│       ├── httpserver/
│       ├── logging/
│       └── clock/
├── migrations/
├── api/
│   └── openapi.yaml
├── docs/
│   ├── architecture.md
│   ├── decisions/
│   └── runbooks/
├── test/
│   ├── integration/
│   └── load/
├── scripts/
├── Dockerfile
├── compose.yaml
├── Makefile
├── go.mod
└── README.md
```

This is a proposed layout, not a requirement to create every directory on day one. Start with the modules needed for the current milestone.

### 6.3 Go standards
- Use a current stable Go release and pin the project's `go` directive accordingly.
- Use `net/http` and `http.ServeMux` method/path patterns for routing unless a concrete need justifies a third-party router.
- Use `context.Context` for request-scoped work and cancellation; do not store request contexts in long-lived structs.
- Use `log/slog` or an equivalent structured logger.
- Configure server read, write, idle, and header timeouts.
- Handle graceful shutdown on SIGINT/SIGTERM.
- Use `errors.Is`/`errors.As` and typed/domain errors where useful.
- Avoid global mutable state and goroutine leaks.
- Use a database connection pool with explicit limits and sensible timeouts.
- Do not hold database transactions open while calling a payment provider, sending email, or performing other network I/O.
- Use migrations tracked in source control.
- Prefer explicit SQL for critical inventory/financial operations.
- Add dependencies only when they materially improve correctness or maintainability.

### 6.4 External dependencies
Initial:
- PostgreSQL
- `pgx` or `database/sql` driver
- Migration tool selected by the developer
- Optional OpenAPI tooling
- Docker Compose for local PostgreSQL

Later if justified:
- Redis for cache/rate-limit coordination, not as the sole source of booking truth
- A durable broker only when database-backed workers/outbox are insufficient
- Object storage for media assets
- Email/SMS provider
- Real payment gateway
- OpenTelemetry-compatible instrumentation

---

## 7. Data model

Use UUIDs or another consistently generated opaque identifier. Use foreign keys, check constraints, unique constraints, and indexes. Add `created_at` and `updated_at` to mutable records; use `deleted_at` only where soft deletion has clear semantics. Financial and audit records should not be silently deleted.

The following is a logical model; exact columns and normalization are to be refined during implementation.

### 7.1 Identity
- `users`: id, email, password_hash, display_name, phone, status, email_verified_at, created_at, updated_at
- `roles`: id, name
- `user_roles`: user_id, role_id
- `refresh_sessions`: id, user_id, token_hash, expires_at, revoked_at, created_at
- `password_reset_tokens`: id, user_id, token_hash, expires_at, consumed_at

### 7.2 Cinema and seating
- `branches`: id, name, address, timezone, contact_details, status
- `screens`: id, branch_id, name, screen_type, status
- `screen_layout_versions`: id, screen_id, version, effective_at, status
- `seats`: id, screen_id, stable_label, row_label, seat_number, seat_class, is_accessible, status, x_position, y_position
- `showtime_seats`: id, showtime_id, seat_id, sale_status, version
  - Unique constraint: `(showtime_id, seat_id)`.
  - Sale status should be derived or updated transactionally; do not rely on an in-memory map.

### 7.3 Catalogue and scheduling
- `movies`: id, title, synopsis, runtime_minutes, release_date, age_rating, language, poster_url, trailer_url, status
- `genres`: id, name
- `movie_genres`: movie_id, genre_id
- `movie_credits`: id, movie_id, person_name, role, character_name, display_order
- `showtimes`: id, movie_id, screen_id, starts_at, ends_at, language, format, status, pricing_plan_id, sales_cutoff_at, created_by
- `pricing_plans`: id, name, branch_id, status
- `price_rules`: id, pricing_plan_id, seat_class, amount_minor, currency, conditions_json, priority, valid_from, valid_until

Prevent overlapping showtimes for the same screen at the database level where practical, for example with a PostgreSQL exclusion constraint over a timestamp range. Include the configured turnaround buffer in the range being checked. Application validation should produce a useful error, while the database protects against concurrent schedule writes.

### 7.4 Holds and booking
- `seat_holds`: id, showtime_id, user_id nullable for guest checkout, session_token_hash nullable, status, expires_at, extension_count, idempotency_key, created_at
- `seat_hold_items`: hold_id, showtime_seat_id, quoted_amount_minor
- `bookings`: id, public_reference, user_id nullable for guest checkout, showtime_id, status, currency, ticket_subtotal_minor, discount_minor, fee_minor, tax_minor, concession_subtotal_minor, total_minor, idempotency_key, confirmed_at, cancelled_at, created_at
- `booking_items`: id, booking_id, showtime_seat_id, seat_label_snapshot, seat_class_snapshot, unit_price_minor, discount_minor, tax_minor, status
- `tickets`: id, booking_item_id, token_hash or securely generated token, status, checked_in_at, checked_in_by, issued_at
- `idempotency_records`: id, scope, key, request_hash, response_status, response_body, resource_id, created_at, expires_at
- `booking_status_history`: id, booking_id, from_status, to_status, reason, actor_id nullable, created_at

Critical uniqueness/consistency rules:
- One active inventory owner per showtime seat.
- One ticket per confirmed booking item.
- Unique public booking reference.
- Unique idempotency key within its defined scope.
- Use a database-enforced strategy for active inventory ownership; partial unique indexes or a dedicated seat-claim table may be appropriate. Document and test the selected approach.
- Use row locks and transactional checks; do not rely on “check availability, then insert” outside a transaction.

### 7.5 Payments and refunds
- `payments`: id, booking_id, provider, provider_payment_id, status, amount_minor, currency, idempotency_key, created_at, updated_at
- `payment_events`: id, provider, provider_event_id, payment_id, event_type, payload_digest, received_at, processed_at, processing_result
- `refunds`: id, payment_id, amount_minor, currency, status, reason, idempotency_key, provider_refund_id, requested_by, created_at, updated_at
- `financial_adjustments`: id, booking_id, type, amount_minor, currency, reason, actor_id, created_at

Unique provider event IDs per provider. Store only the minimum provider payload needed for reconciliation, redact sensitive data, and define retention.

### 7.6 Concessions
- `concession_categories`: id, branch_id, name, status
- `concession_products`: id, branch_id, category_id, name, description, price_minor, currency, stock_mode, status
- `concession_orders`: id, booking_id nullable, branch_id, status, subtotal_minor, created_at, updated_at
- `concession_order_items`: id, order_id, product_id, product_name_snapshot, quantity, unit_price_minor, line_total_minor
- `concession_stock_movements`: id, product_id, quantity_delta, reason, actor_id, created_at

### 7.7 Notifications, jobs, audit
- `outbox_events`: id, aggregate_type, aggregate_id, event_type, payload, occurred_at, published_at, attempts, next_attempt_at
- `background_jobs`: id, job_type, payload, status, attempts, available_at, locked_until, locked_by, last_error, created_at, updated_at
- `notification_deliveries`: id, outbox_event_id, channel, recipient_hash or protected recipient, status, attempts, next_attempt_at, sent_at, last_error
- `audit_logs`: id, actor_id nullable, action, resource_type, resource_id, reason, request_id, safe_metadata, created_at

Use JSONB only where flexible payloads are genuinely useful; keep core business relationships and queryable fields relational.

---

## 8. API conventions

### 8.1 General
- Base path: `/api/v1`
- JSON request/response bodies unless documented otherwise.
- Use standard HTTP methods and status codes.
- Use RFC 3339 timestamps.
- Use integer minor currency units plus ISO currency code.
- Paginate list endpoints with a consistent cursor or limit/offset scheme.
- Add a request/correlation ID to responses and logs.
- Validate request bodies, path parameters, and query parameters.
- Set body-size limits and reject unknown fields where practical.
- Do not expose internal database errors to clients.
- Provide consistent error responses, e.g. `{ "error": { "code": "SEAT_UNAVAILABLE", "message": "One or more selected seats are unavailable", "request_id": "..." } }`.
- Document APIs in `api/openapi.yaml`.

### 8.2 Public/customer endpoints

| Method | Endpoint | Purpose |
|---|---|---|
| POST | `/auth/register` | Register customer |
| POST | `/auth/login` | Authenticate |
| POST | `/auth/refresh` | Refresh credentials/session |
| POST | `/auth/logout` | Revoke session |
| GET | `/movies` | Browse published movies |
| GET | `/movies/{movieID}` | Movie details |
| GET | `/branches` | List public branches |
| GET | `/branches/{branchID}/screens` | Public screen metadata if needed |
| GET | `/showtimes` | Search showtimes |
| GET | `/showtimes/{showtimeID}/seats` | Seat map and availability |
| POST | `/showtimes/{showtimeID}/holds` | Atomically hold seats |
| GET | `/holds/{holdID}` | Get own hold state |
| DELETE | `/holds/{holdID}` | Release own active hold |
| POST | `/holds/{holdID}/extend` | Attempt permitted hold extension |
| POST | `/holds/{holdID}/checkout` | Create booking/payment attempt |
| GET | `/bookings/{bookingID}` | Get own booking |
| GET | `/me/bookings` | Customer booking history |
| POST | `/bookings/{bookingID}/cancel` | Request cancellation |
| GET | `/tickets/{ticketID}` | Retrieve own ticket |
| GET | `/concessions/products` | List available products |
| POST | `/concessions/orders` | Create concession order |

For guest checkout, use a high-entropy guest access token and store only its hash. Do not expose bookings based on sequential IDs or public references alone.

### 8.3 Payment callbacks and internal operations

| Method | Endpoint | Purpose |
|---|---|---|
| POST | `/webhooks/payments/{provider}` | Receive verified provider events |
| GET | `/health/live` | Process liveness |
| GET | `/health/ready` | Readiness including database dependency |
| GET | `/metrics` | Metrics endpoint restricted to trusted environments |

### 8.4 Management endpoints

| Method | Endpoint | Purpose |
|---|---|---|
| CRUD | `/admin/branches` | Manage branches |
| CRUD | `/admin/screens` | Manage screens |
| CRUD | `/admin/seats` | Manage seat definitions/layouts |
| CRUD | `/admin/movies` | Manage movie catalogue |
| CRUD | `/admin/genres` | Manage genres |
| CRUD | `/admin/showtimes` | Manage schedules |
| POST | `/admin/showtimes/{id}/publish` | Publish a showtime |
| POST | `/admin/showtimes/{id}/cancel` | Cancel a showtime |
| CRUD | `/admin/pricing-plans` | Manage pricing |
| CRUD | `/admin/promotions` | Manage promotions |
| GET | `/admin/bookings` | Search bookings |
| POST | `/admin/bookings/walk-in` | Create walk-in booking |
| POST | `/admin/tickets/{id}/check-in` | Validate and admit ticket |
| POST | `/admin/bookings/{id}/refunds` | Request refund |
| CRUD | `/admin/concessions/products` | Manage concession products |
| GET | `/admin/reports/sales` | Sales report |
| GET | `/admin/reports/occupancy` | Occupancy report |
| GET | `/admin/audit-logs` | Search audit records |
| CRUD | `/admin/staff` | Manage staff accounts and roles |

“CRUD” means the relevant create/list/read/update operations; deletion should generally be archival or status change where records are referenced by transactions.

---

## 9. Critical booking transaction design

The exact SQL depends on the selected inventory model, but the invariants and transaction boundaries are mandatory.

### 9.1 Holding seats
1. Validate showtime exists, is published, and sales are open.
2. Validate requested seats belong to the showtime's screen and are sellable.
3. Start a database transaction.
4. Lock the relevant showtime-seat rows in deterministic order to reduce deadlocks.
5. Reclaim or ignore expired claims using database time and the chosen inventory model.
6. Check every requested seat is available. If any seat is unavailable, roll back the entire request.
7. Create the hold and hold items with one expiry timestamp.
8. Commit, then return the hold and quote.
9. If a concurrent transaction wins, return a stable `409 Conflict` response such as `SEAT_UNAVAILABLE`.

### 9.2 Checkout and payment
1. Validate hold ownership, status, and expiry inside a transaction.
2. Create or retrieve an idempotent booking/payment attempt and persist the required inventory claim.
3. Commit before making a network call to the payment adapter.
4. Call the payment adapter outside the database transaction.
5. Process the result or callback in a new transaction; deduplicate provider events.
6. Confirm booking and issue tickets only when payment is verified and the inventory claim is valid.
7. If outcome is uncertain, keep a pending/review state and reconcile. Do not release seats merely because the HTTP request timed out.
8. If a successful payment cannot be fulfilled, create a durable compensation/refund task and surface it to operations.

The implementation must document the chosen policy for whether seats remain claimed during a pending payment attempt and how long. Never hold a database transaction open while waiting for the payment provider.

### 9.3 Expiration worker
- Claim batches of expired holds using a safe concurrency pattern, such as `FOR UPDATE SKIP LOCKED`.
- Use database time for expiry comparisons.
- Never release seats belonging to confirmed bookings.
- Be safe to run multiple worker instances.
- Make repeated execution harmless.
- Record metrics for holds expired, retries, failures, and backlog age.
- Add integration tests for worker crashes and duplicate processing.

---

## 10. Security and privacy

- Enforce TLS in deployed environments.
- Store secrets in environment/configuration secret stores, never in source control.
- Use secure password hashing, session/token expiry, revocation, and rotation.
- Authorize every object-level operation to prevent insecure direct object reference (IDOR) vulnerabilities.
- Rate-limit login, registration, hold creation, checkout, ticket validation, and public search as appropriate.
- Validate webhook signatures against the raw request body and protect against replay using provider event IDs/timestamps where supported.
- Use CSRF protection if cookie-based authentication is chosen.
- Use `HttpOnly`, `Secure`, and appropriate `SameSite` cookie settings for browser sessions.
- Redact email, phone, tokens, provider payloads, and other personal information from logs.
- Encrypt sensitive stored values when risk assessment requires it.
- Restrict metrics and administrative endpoints.
- Define data retention and deletion rules for customer accounts and operational records.
- Do not store payment card details.
- Add security tests for unauthorized access, cross-customer booking access, role escalation, replayed callbacks, and malformed input.

---

## 11. Reliability, performance, and observability

### Reliability
- Graceful shutdown with bounded timeout.
- Database transactions and constraints protect inventory/financial invariants.
- Idempotent writes for retried operations.
- Durable outbox/jobs for asynchronous effects.
- Retry only transient failures; use bounded exponential backoff with jitter.
- Dead-letter or review states for exhausted jobs.
- Startup validates required configuration and dependencies.
- Readiness becomes unhealthy when critical dependencies are unavailable.

### Observability
- Structured logs with request ID, route, status, duration, and safe actor/resource identifiers.
- Metrics for request rate, error rate, latency, DB pool usage, active holds, expired holds, booking conversion, payment outcomes, refund backlog, job retries, and notification delivery.
- Distributed tracing can be added when there is enough cross-operation complexity to justify it.
- Never log secrets or raw payment credentials.

### Initial performance targets
These are initial engineering targets to validate with a documented local test environment, not guarantees:
- Public read API: p95 under 250 ms at 100 requests/second on a documented developer/staging machine, excluding external providers.
- Hold endpoint: p95 under 400 ms at 50 requests/second with realistic contention, excluding external providers.
- No duplicate active seat claims in concurrency tests.
- No lost committed booking/payment event in crash/retry integration tests.
- Load tests report throughput, p50/p95/p99 latency, error rate, and resource usage.
- Performance targets can be revised after measuring baseline hardware and data volume.

---

## 12. Testing strategy and acceptance criteria

### Unit tests
- Pricing, discounts, fees, and rounding.
- Hold/booking/payment/refund state transitions.
- Cancellation policy and sales cutoff.
- Authorization policies.
- Ticket-token validation logic.
- Retry delay and job scheduling logic.

### PostgreSQL integration tests
- Unique constraints and foreign keys.
- Concurrent attempts to hold the same seat.
- Atomic multi-seat holds.
- Expired hold reclamation.
- Showtime overlap prevention.
- Duplicate idempotency key with same payload returns same result.
- Reusing an idempotency key with a different payload returns conflict.
- Duplicate/out-of-order payment events.
- Booking cancellation and inventory release.
- Worker processing with multiple concurrent workers.

### Race and failure tests
- Run `go test -race ./...`.
- Inject payment success after client timeout.
- Deliver the same payment callback multiple times.
- Crash a worker after claiming a job and before marking it complete.
- Restart the application while holds and payments are pending.
- Make the database unavailable during shutdown/startup.
- Test cancellation and hold expiry racing with checkout.
- Test two managers editing overlapping showtimes simultaneously.

### Acceptance criteria for first production-grade milestone
1. A customer can browse a published movie and available showtimes.
2. A manager can configure a branch, screen, seat layout, movie, pricing, and showtime.
3. A customer can view the seat map and place an atomic temporary hold.
4. Concurrent requests cannot create more than one active claim for a seat/showtime.
5. A customer can complete a booking using the payment simulator.
6. Repeated booking requests and payment callbacks do not duplicate bookings, payments, or tickets.
7. Expired holds are reclaimed safely even after worker restart.
8. Ticket check-in rejects invalid and previously checked-in tickets.
9. Staff permissions prevent unauthorized schedule, pricing, refund, and role changes.
10. Sales and occupancy reports reconcile against bookings, tickets, and refunds.
11. Critical operations are logged and auditable.
12. Integration tests, race tests, migrations, and local setup instructions run reproducibly.

---

## 13. Implementation roadmap

Build in vertical slices. Each milestone should produce working software and tests.

### Milestone 0 — Foundation
- Create Go module, configuration, HTTP server, structured logging, graceful shutdown.
- Add PostgreSQL, migrations, health/readiness endpoints.
- Establish error response format, request IDs, linting, and CI checks.
- Write an architecture decision record (ADR) for inventory ownership and transaction strategy.

**Exit criteria:** Application starts locally, migrates schema, serves health checks, and runs tests reliably.

### Milestone 1 — Cinema catalogue and scheduling
- Branches, screens, versioned seat layouts, movies, genres, showtimes, and pricing.
- Management authorization and audit events.
- Prevent overlapping showtimes.

**Exit criteria:** Manager can configure a realistic schedule; invalid schedules are rejected under concurrent writes.

### Milestone 2 — Inventory and seat holds
- Showtime inventory, seat map, atomic multi-seat holds, expiry, release, and bounded extension.
- Implement database-enforced uniqueness and transactional locking.
- Add contention tests and load-test scripts.

**Exit criteria:** Concurrent test proves no seat is held by multiple active owners.

### Milestone 3 — Booking and payment simulation
- Booking state machine, idempotency store, payment adapter interface, deterministic simulator.
- Callback deduplication, pending-payment reconciliation, compensation flow.
- Ticket issuance and QR/token payload.

**Exit criteria:** Success, failure, timeout, duplicate callback, late success, and process restart scenarios behave predictably.

### Milestone 4 — Customer and box-office workflows
- Customer auth and booking history.
- Staff booking lookup, walk-in booking, ticket check-in, cancellation/refund workflows.
- Role checks and audit coverage.

**Exit criteria:** Online and walk-in sales share the same inventory and pricing rules.

### Milestone 5 — Notifications and operations
- Transactional outbox, background jobs, local email/log adapter.
- Sales/occupancy/refund reports, metrics, structured logs, operational queries.
- Retry/dead-letter/review workflows.

**Exit criteria:** A committed booking can be recovered and its notification retried after a process crash.

### Milestone 6 — Concessions and polish
- Concession products, orders, fulfilment, and stock movements.
- Promotions, CSV exports, operational dashboards or documented metric endpoints.
- Security review, API documentation, backup/restore rehearsal, and performance baseline.

**Exit criteria:** End-to-end cinema workflows are testable, documented, and demonstrably reliable.

### Milestone 7 — Optional production integrations
- Real payment provider, production email/SMS, object storage for posters, OpenTelemetry.
- Deploy only after local reliability and recovery criteria pass.
- Document secrets, backups, migrations, rollback, and incident runbooks.

Do not move to the next milestone just because endpoints exist. Move when the exit criteria and tests pass.

---

## 14. Definition of done

A feature is not complete merely because its happy path works. It is done when:
- Business rules and failure cases are specified.
- Authorization and validation are implemented.
- Database constraints and migration are included where needed.
- Unit and relevant integration tests pass.
- Retry/idempotency behavior is defined for mutating endpoints.
- Logs and metrics are sufficient to diagnose failures.
- API documentation is updated.
- No secrets or sensitive personal/payment data leak into logs.
- The feature can be exercised through documented local instructions.
- Relevant race/failure tests pass.

---

## 15. Required project documentation

Maintain these files as the project grows:
- `README.md` — prerequisites, local setup, commands, environment variables, test commands.
- `docs/architecture.md` — module boundaries, request flow, transaction boundaries, background workers.
- `docs/decisions/ADR-001-inventory-ownership.md` — chosen concurrency/seat-claim model and alternatives considered.
- `docs/decisions/ADR-002-payment-consistency.md` — pending payment, idempotency, reconciliation, and compensation policy.
- `docs/decisions/ADR-003-authentication.md` — session/token model and security rationale.
- `docs/runbooks/payment-reconciliation.md` — inspect and resolve stuck payments.
- `docs/runbooks/booking-recovery.md` — recover booking/payment inconsistencies.
- `api/openapi.yaml` — API contract.
- `test/load/README.md` — reproducible load-test setup and interpretation.

---

## 16. Initial environment configuration

Use environment variables or a local ignored `.env` file; commit only `.env.example`.

Suggested keys:
- `APP_ENV`
- `HTTP_ADDRESS`
- `HTTP_PORT`
- `DATABASE_URL`
- `DATABASE_MAX_CONNS`
- `DATABASE_MIN_CONNS`
- `DATABASE_TIMEOUT`
- `LOG_LEVEL`
- `AUTH_ACCESS_TOKEN_TTL`
- `AUTH_REFRESH_TOKEN_TTL`
- `HOLD_TTL`
- `HOLD_MAX_EXTENSIONS`
- `HOLD_EXTENSION_TTL`
- `BOOKING_SALES_CUTOFF`
- `PAYMENT_PROVIDER` (initially `simulator`)
- `PAYMENT_WEBHOOK_SECRET` (development-only simulator secret)
- `WORKER_POLL_INTERVAL`
- `WORKER_BATCH_SIZE`
- `METRICS_ENABLED`

Configuration must be validated at startup. Do not commit real secrets. Document safe development defaults.

---

## 17. Important design decisions to make before coding

Resolve these in short ADRs; do not leave them implicit.

1. **Inventory model:** Should availability be represented by a dedicated seat-claim table, or by explicit status/claim columns on `showtime_seats`? Choose the model that makes active-claim uniqueness enforceable in PostgreSQL.
2. **Payment hold policy:** How long do seats remain claimed while payment is pending, and what happens on an uncertain provider outcome?
3. **Guest checkout:** Is guest checkout supported at launch, and how are guest booking credentials delivered and recovered?
4. **Currency and tax:** Choose the launch currency and configure tax/fee rules without hardcoding assumptions into business logic.
5. **Cancellation policy:** Define the default customer cutoff, staff override permissions, and refund calculation.
6. **Seat layout changes:** Define whether published showtimes use immutable layout versions and how maintenance blocks affect existing bookings.
7. **Authentication:** Choose secure server-side sessions or access/refresh tokens based on the intended clients.
8. **Data retention:** Set retention periods for audit events, payment events, notification payloads, and customer personal data.
9. **Worker durability:** Decide whether PostgreSQL-backed jobs are sufficient for the first release.
10. **Reporting definitions:** Document exactly how sales, occupancy, refunds, and checked-in attendance are calculated.

Recommended defaults are provided throughout this document, but these decisions should be explicit and covered by tests.

---

## 18. Final project principle

BookMyShow's quality will be judged by correctness under stress and failure, not by the number of endpoints or technologies used. Prioritize transactional invariants, explicit state machines, idempotency, recovery, observability, and reproducible tests. A smaller system that proves it cannot double-book seats is more impressive than a sprawling cinema CRUD API that only works on the happy path.
