# Webhooks — Design

**Status:** Approved (brainstorming)
**Date:** 2026-05-14
**Branch:** `webhook-feature`

## Summary

Add CRUD management of outbound webhooks and dispatch a `form.submission` event to every active webhook whenever a new submission is created. Delivery is async, signed with HMAC-SHA256, retried up to 3 times in-process, and recorded in a delivery-history table that is queryable via the API.

The feature is gated by a single static `ADMIN_API_KEY` — the management endpoints are tenant-wide settings, not per-submission like the existing `admin_token` URLs.

## Goals

- Let an operator register, list, update, disable, rotate-secret, and delete webhook subscriptions.
- Fire `form.submission` to all active subscribers when `POST /api/submissions` succeeds.
- Survive transient receiver failures via short bounded retries with backoff (up to 4 total attempts over ~2.5 min).
- Give the operator enough observability (delivery list + detail) to debug a misbehaving receiver.
- Make it trivial for receivers to verify the request came from us (HMAC-SHA256 signature over the body, plus a timestamp).

## Non-goals

- Multi-tenant auth, user accounts, or sessions. One static admin key is sufficient for the current single-tenant deployment.
- A separate worker process or external queue. Retries are in-process and bounded; surviving server restart mid-retry is explicitly out of scope.
- Manual redelivery of a past attempt.
- Events other than `form.submission`. The schema supports more, but only this one is implemented.
- A UI. CRUD is API-only for v1.

## Architecture overview

```
POST /api/submissions
  └─ handler.CreateSubmission
       ├─ submissionRepo.Create
       ├─ email.SendSubmissionNotificationAsync     (existing)
       ├─ analyzer.AnalyzeAsync                     (existing)
       └─ dispatcher.DispatchAsync("form.submission", submission)   (NEW)
              │
              ├─ webhookRepo.ListActiveForEvent("form.submission")
              ├─ for each webhook:
              │     ├─ deliveryRepo.Create(pending)
              │     └─ go attemptLoop(webhook, delivery)
              │           └─ POST url with HMAC-signed body
              │              └─ on non-2xx/error: time.AfterFunc(backoff, retry)
              └─ (returns immediately to handler — fire-and-forget)
```

`/api/webhooks*` routes live in a Gin route group protected by a new `AdminAuth` middleware that checks `Authorization: Bearer <ADMIN_API_KEY>`. The existing `/api/submissions` and `/api/admin/:token` routes are unchanged.

## Data model

### `webhooks`

Migration `003_create_webhooks.up.sql` / `.down.sql`.

| Column          | Type           | Notes                                                                 |
|-----------------|----------------|----------------------------------------------------------------------|
| `id`            | UUID PK        | `gen_random_uuid()`                                                  |
| `name`          | VARCHAR(100)   | Human label                                                          |
| `url`           | TEXT           | Must validate as `http://`/`https://` with host                      |
| `secret`        | VARCHAR(64)    | HMAC signing secret, generated on create; **never returned by GET**  |
| `secret_prefix` | VARCHAR(8)     | First 8 chars of `secret`, returned by GET for identification        |
| `events`        | TEXT[]         | Subscribed event types (`{form.submission}` for now)                 |
| `active`        | BOOLEAN        | Soft-disable. Default `true`                                         |
| `created_at`    | TIMESTAMPTZ    | Default `NOW()`                                                      |
| `updated_at`    | TIMESTAMPTZ    | Default `NOW()`                                                      |

Indexes:
- `webhooks(active)` for the dispatch query.

Constraints:
- `CHECK (array_length(events, 1) > 0)`
- `CHECK (url ~* '^https?://')` (defensive — handler also validates).

### `webhook_deliveries`

Migration `004_create_webhook_deliveries.up.sql` / `.down.sql`.

| Column               | Type          | Notes                                                          |
|----------------------|---------------|---------------------------------------------------------------|
| `id`                 | UUID PK       | Used as `X-Webhook-Delivery-Id` header                         |
| `webhook_id`         | UUID FK       | `ON DELETE CASCADE`                                            |
| `event_type`         | VARCHAR(50)   | `form.submission`                                              |
| `event_id`           | UUID          | Submission id; lets receivers dedupe                           |
| `payload`            | JSONB         | Exact bytes sent (parsed for storage)                          |
| `status`             | VARCHAR(20)   | `pending` \| `success` \| `failed`                             |
| `attempts`           | INT           | HTTP attempts made                                             |
| `last_status_code`   | INT           | Nullable until first attempt                                   |
| `last_error`         | TEXT          | Nullable; error message on last attempt                        |
| `last_response_body` | TEXT          | Truncated to 4 KB                                              |
| `next_attempt_at`    | TIMESTAMPTZ   | Nullable; set while a retry is scheduled                       |
| `created_at`         | TIMESTAMPTZ   | Default `NOW()`                                                |
| `updated_at`         | TIMESTAMPTZ   | Default `NOW()`                                                |

Indexes:
- `webhook_deliveries(webhook_id, created_at DESC)` for the list endpoint.

Constraints:
- `CHECK (status IN ('pending','success','failed'))`

## API surface

All routes are under `/api/webhooks` and require `Authorization: Bearer <ADMIN_API_KEY>`.

Responses use the existing unwrapped JSON pattern (matches `handler.respondData` / `handler.respondErrorSimple`), not the `{success, data, error}` wrapper.

### CRUD

| Method | Path                                                | Body / Notes                                                                                          |
|--------|-----------------------------------------------------|-------------------------------------------------------------------------------------------------------|
| POST   | `/api/webhooks`                                     | `{name, url, events?, active?}`. Generates `secret`; returns it once in `WebhookWithSecret` response. |
| GET    | `/api/webhooks`                                     | List all webhooks (no pagination — small N expected).                                                 |
| GET    | `/api/webhooks/:id`                                 | Single webhook. No `secret`, only `secretPrefix`.                                                     |
| PATCH  | `/api/webhooks/:id`                                 | Partial update of `name`, `url`, `events`, `active`. Does **not** rotate the secret.                  |
| POST   | `/api/webhooks/:id/rotate-secret`                   | Generates new `secret`, invalidates old immediately. Returns `WebhookWithSecret`.                     |
| DELETE | `/api/webhooks/:id`                                 | Hard delete; deliveries cascade.                                                                      |

### Delivery history

| Method | Path                                                        | Notes                                                                                  |
|--------|-------------------------------------------------------------|----------------------------------------------------------------------------------------|
| GET    | `/api/webhooks/:id/deliveries?limit=50&before=<deliveryId>` | Paginated by `created_at DESC`. Summary fields only — no `payload`, no response body.  |
| GET    | `/api/webhooks/:id/deliveries/:deliveryId`                  | Full record including `payload` and `last_response_body`.                              |

Pagination: cursor-style. `before=<id>` returns rows created strictly before that delivery's `created_at`. Default `limit=50`, max `limit=200`.

### Request validation

Performed in the handler before touching the repository:
- `url` must parse (`url.Parse`), scheme is `http` or `https`, host is non-empty.
- `events` is non-empty; every element is in the known-events set (`form.submission`).
- `name` length 1–100.
- For `PATCH`, only the provided fields are updated; unknown fields are rejected.

### Response shapes

```go
// Returned by GET/LIST/PATCH/DELETE. No secret.
type WebhookResponse struct {
    ID           uuid.UUID `json:"id"`
    Name         string    `json:"name"`
    URL          string    `json:"url"`
    SecretPrefix string    `json:"secretPrefix"`
    Events       []string  `json:"events"`
    Active       bool      `json:"active"`
    CreatedAt    time.Time `json:"createdAt"`
    UpdatedAt    time.Time `json:"updatedAt"`
}

// Returned ONLY by POST /api/webhooks and POST /api/webhooks/:id/rotate-secret.
type WebhookWithSecretResponse struct {
    WebhookResponse
    Secret string `json:"secret"` // shown once
}

type DeliverySummary struct {
    ID             uuid.UUID  `json:"id"`
    WebhookID      uuid.UUID  `json:"webhookId"`
    EventType      string     `json:"eventType"`
    EventID        uuid.UUID  `json:"eventId"`
    Status         string     `json:"status"`
    Attempts       int        `json:"attempts"`
    LastStatusCode *int       `json:"lastStatusCode,omitempty"`
    NextAttemptAt  *time.Time `json:"nextAttemptAt,omitempty"`
    CreatedAt      time.Time  `json:"createdAt"`
    UpdatedAt      time.Time  `json:"updatedAt"`
}

type DeliveryDetail struct {
    DeliverySummary
    Payload          json.RawMessage `json:"payload"`
    LastError        *string         `json:"lastError,omitempty"`
    LastResponseBody *string         `json:"lastResponseBody,omitempty"`
}
```

## Delivery pipeline

### Trigger

In `internal/handlers/submissions.go`, after the existing fan-out:

```go
h.email.SendSubmissionNotificationAsync(submission)
h.analyzer.AnalyzeAsync(submission)
h.dispatcher.DispatchAsync(submissionID, "form.submission", submission)   // NEW
```

All three are fire-and-forget. Ordering between them is not significant.

### Dispatcher (`internal/services/webhook_dispatcher.go`)

```go
type Dispatcher struct {
    webhooks         repository.WebhookRepository
    deliveries       repository.WebhookDeliveryRepository
    httpClient       *http.Client
    backoffSchedule  []time.Duration // {5s, 30s, 2min} in prod; overridden in tests
    allowPrivateIPs  bool
}

func (d *Dispatcher) DispatchAsync(eventID uuid.UUID, eventType string, data any)
```

`DispatchAsync` runs in a goroutine using `context.Background()` (the request context is gone by then; per-attempt timeouts bound the work). It:

1. Queries `webhooks` for `active=true AND events @> ARRAY[eventType]`. If none, returns.
2. Builds the canonical payload **once**:
   ```json
   {
     "id": "<delivery_id>",
     "event": "form.submission",
     "createdAt": "<RFC3339>",
     "data": { /* full submission, same JSON tags as models.Submission */ }
   }
   ```
   Note: `id` is the *delivery* id, not the event id. The event/submission id is also embedded inside `data.id`. Receivers should dedupe on `data.id`.
3. For each matching webhook, marshals the payload to bytes, inserts a `webhook_deliveries` row with `status='pending'`, and spawns a per-webhook goroutine running the attempt loop.

### Attempt loop

For attempts 1..4 (initial attempt plus up to 3 retries):

1. Build the request:
   - `POST <webhook.url>`
   - Headers:
     - `Content-Type: application/json`
     - `User-Agent: intake-form-api-webhook/1.0`
     - `X-Webhook-Id: <webhook_id>`
     - `X-Webhook-Delivery-Id: <delivery_id>`
     - `X-Webhook-Event: form.submission`
     - `X-Webhook-Timestamp: <unix_seconds>`
     - `X-Webhook-Signature: sha256=<hex(HMAC_SHA256(secret, timestamp + "." + body))>`
   - Body: the bytes from step 2 above.
2. Execute against `httpClient` with a 10-second per-request timeout.
3. Read the response body via `io.LimitReader(resp.Body, 4096)`.
4. Classify:
   - **Success** (2xx): update delivery → `status='success'`, `attempts=n`, `last_status_code=<code>`, `last_response_body=<truncated>`, `next_attempt_at=NULL`, `last_error=NULL`. Done.
   - **Failure** (non-2xx or network error): update delivery → `attempts=n`, `last_status_code=<code or 0>`, `last_error=<msg>`, `last_response_body=<truncated or empty>`.
     - If `n < 4`: set `next_attempt_at = now + backoffSchedule[n-1]`, schedule `time.AfterFunc(backoffSchedule[n-1], func(){ attempt n+1 })`. Status stays `pending`.
     - If `n == 4`: set `status='failed'`, `next_attempt_at=NULL`. Done.

`backoffSchedule = []time.Duration{5*time.Second, 30*time.Second, 2*time.Minute}` — three delays between four attempts. First attempt is immediate; if it fails, the second runs 5 s later; if that fails, the third runs 30 s after the second; the fourth runs 2 min after the third. Total budget if all retries are exhausted: ~2.5 min.

All non-2xx is retried (including 4xx). The simpler rule keeps the dispatcher easy to reason about; a "retryable status code" policy can be added later if 4xx-spam becomes a problem in practice.

### Concurrency model

- One goroutine per delivery attempt sequence. N webhooks → N goroutines.
- `time.AfterFunc` schedules the next attempt; the goroutine spawned by `AfterFunc` does the next attempt.
- No mutex, no worker pool. Go's `http.Client` connection pool handles HTTP concurrency naturally.
- On server restart, pending retries are lost. Affected deliveries remain `pending` with a non-null `next_attempt_at` in the past. They are not auto-resumed in v1; the operator sees them in the list endpoint as stuck.

## Security & robustness

### Admin auth

New middleware `internal/middleware/admin_auth.go`:

```go
func AdminAuth(expectedKey string) gin.HandlerFunc
```

- If `expectedKey == ""`: the middleware short-circuits with **503** and an error indicating the feature is disabled. (We want a clear signal, not a "everything works because there's no key to check.")
- Reads `Authorization: Bearer <key>`. Compares with `subtle.ConstantTimeCompare`.
- 401 on missing or mismatched key. Never 403.

### SSRF defense

Webhook URLs are admin-controlled but still arbitrary. A misbehaving admin (or a compromised key) could target internal services. Defenses:

1. **At validation:** reject schemes other than `http`/`https`; require a host.
2. **At dial:** custom `http.Transport` with `DialContext` that resolves the host and refuses to dial private/loopback/link-local addresses:
   - IPv4: `127.0.0.0/8`, `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `169.254.0.0/16`, `0.0.0.0/8`, `100.64.0.0/10`.
   - IPv6: `::1/128`, `fc00::/7`, `fe80::/10`, `::ffff:0:0/96` mapped equivalents.
3. **No redirects:** `CheckRedirect` returns `http.ErrUseLastResponse`. A public URL can't 302 to an internal one.
4. **Override:** `WEBHOOK_ALLOW_PRIVATE_IPS=true` disables the IP check for local development. Default `false`.

### Secret handling

- Stored plaintext (needed for HMAC at send time). The risk profile is: a DB compromise exposes secrets, which lets an attacker forge requests to receivers. Hashing the secret server-side would prevent that, but receivers can already check the source IP — and we'd lose the ability to recompute HMAC. Plaintext is the standard trade-off (matches Stripe, GitHub, etc.).
- Generated via `crypto/rand`, 32 bytes, hex-encoded (64 chars).
- Field is `json:"-"` on `models.Webhook`. A separate `WebhookWithSecretResponse` is used for create/rotate.
- Rotation: `POST /api/webhooks/:id/rotate-secret` generates a new secret in a single update. There is **no overlap window** — the old secret is invalid as soon as the new one is returned.

### Payload size

- Outbound: no cap on request body. Submissions are ~2 KB.
- Inbound (receiver's response): capped at 4 KB via `io.LimitReader`. Anything beyond is silently truncated; we just need enough to debug.

### Disabled / deleted webhooks during retry

- `active=false`: the dispatch query excludes it for *new* events. In-flight retries continue (short budget, not worth coordinating cancellation).
- Deleted: `time.AfterFunc` fires, the per-attempt code does `deliveryRepo.GetByID` (or relies on the FK), gets `ErrNotFound`, and returns silently.

### Logging

- Failures: `log.Printf("webhook delivery failed: webhook_id=%s delivery_id=%s attempts=%d status=%d err=%v", ...)`.
- Successes: brief info log.
- Never logs `payload` contents — only ids, status codes, errors.

## Configuration

Additions to `internal/config/config.go`:

```go
type WebhookConfig struct {
    AdminAPIKey      string  // ADMIN_API_KEY; required for /api/webhooks routes
    AllowPrivateIPs  bool    // WEBHOOK_ALLOW_PRIVATE_IPS; default false
}
```

Behavior:
- `ADMIN_API_KEY` unset → webhook CRUD endpoints return 503 with a clear "admin api key not configured" error. The existing endpoints are unaffected, and the dispatcher still runs (but if no webhooks exist, it's a no-op).
- `WEBHOOK_ALLOW_PRIVATE_IPS=true` is intended for local dev only; document this in `.env.example`.

## Testing

The repo currently has no test files. This feature establishes a minimal pattern using the stdlib `testing` package only — no testify, no mock framework.

### Unit tests

1. **HMAC signing** (`webhook_dispatcher_test.go::TestSign`): table-driven against fixed `(secret, timestamp, body) → expected hex` cases. Locks the wire format so future refactors don't silently break receivers.

2. **URL / IP guard** (`webhook_dispatcher_test.go::TestIsPrivateAddr`, `TestValidateURL`): table-driven coverage of public, loopback, RFC1918, link-local, IPv6 equivalents, bad schemes, missing host.

3. **Admin auth middleware** (`middleware/admin_auth_test.go`): three cases — missing header, wrong key, correct key.

### Integration tests

4. **Dispatcher against `httptest.Server`** (`webhook_dispatcher_test.go`):
   - In-memory implementations of `WebhookRepository` and `WebhookDeliveryRepository` (under ~50 LOC, lives next to the test).
   - `backoffSchedule` overridden to `{1ms, 1ms, 1ms}` (still three entries, matching prod) so retry paths complete quickly.
   - Cases:
     - **Happy path:** receiver returns 200; expect `attempts=1`, `status=success`, signature header verifies against the recorded secret.
     - **Retry then succeed:** receiver returns 500 twice, then 200; expect `attempts=3`, `status=success`.
     - **Permanent failure:** receiver always returns 500; expect `attempts=4`, `status=failed`, `last_status_code=500`.
     - **Disabled webhook:** `active=false`; expect no delivery row, no HTTP call.
     - **SSRF blocked:** `url=http://127.0.0.1:<port>`, `allowPrivateIPs=false`; expect no HTTP call, `status=failed`, descriptive `last_error`.

### Out of scope for tests

- Live database integration tests (would require a test DB harness — separate project).
- Handler-level routing/binding tests (mostly testing Gin).
- End-to-end CRUD tests (straightforward repo passthrough; covered by code review).

## Files changed / added

**New:**
- `migrations/003_create_webhooks.up.sql` / `.down.sql`
- `migrations/004_create_webhook_deliveries.up.sql` / `.down.sql`
- `internal/models/webhook.go`
- `internal/repository/webhook_repo.go`
- `internal/repository/webhook_delivery_repo.go`
- `internal/services/webhook_dispatcher.go`
- `internal/services/webhook_dispatcher_test.go`
- `internal/handlers/webhooks.go`
- `internal/middleware/admin_auth.go`
- `internal/middleware/admin_auth_test.go`

**Modified:**
- `internal/config/config.go` — add `WebhookConfig`.
- `internal/handlers/handlers.go` — wire the dispatcher into `Handler`.
- `internal/handlers/submissions.go` — invoke `DispatchAsync` after the existing fan-out.
- `main.go` — instantiate webhook repos + dispatcher, register `/api/webhooks` route group with `AdminAuth` middleware.
- `.env.example` — document `ADMIN_API_KEY`, `WEBHOOK_ALLOW_PRIVATE_IPS`.

## Open questions

None. All design decisions captured above were confirmed in brainstorming.
