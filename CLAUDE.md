# climate-backend — Developer & AI Agent Reference

## Project Overview

Multi-tenant IoT backend for ESP32-based climate controllers used in cold storage
and refrigeration units (walk-in coolers, fermentation chambers, meat/dairy rooms).

Each physical ESP32 device runs the companion firmware in `../esp32-climate-controller`.
The backend receives telemetry over MQTT, persists it to TimescaleDB, and exposes a
REST + WebSocket API for dashboards and mobile clients.

Key capabilities:
- Ingest sensor readings (temperature, humidity) from multiple tenants' devices
- Track relay states (compressor, fan, light, heating, dehumidifier)
- Store compressor cycle history for predictive maintenance
- Broadcast live snapshots to WebSocket subscribers
- Auto-register unknown devices on first contact (development / initial pairing flow)
- Expose per-tenant, per-device settings and history over HTTP
- JWT authentication with tenant isolation and role-based access control
- Alert engine: evaluate threshold rules on every sensor reading, notify via email or push

---

## Architecture

```
ESP32 firmware
     │  MQTT publish (anonymous, API-key auth planned)
     ▼
eclipse-mosquitto:1883
     │  subscribe climate/+/+/#
     ▼
climate-backend (Go)
  ├── internal/mqtt     parse + dispatch
  ├── internal/auth     JWT sign/validate + HTTP middleware
  ├── internal/alerts   alert rule evaluation + notification dispatch
  ├── internal/*mgr     in-memory state per tenant/device
  ├── internal/db       TimescaleDB writes
  └── internal/api      REST + WebSocket
     │
     ├── TimescaleDB (PostgreSQL 16 + timescaledb extension)
     └── WebSocket clients (browser dashboards, mobile)
```

Single binary. No message queue between MQTT ingestion and DB writes — the Go
process is the only consumer. Horizontal scaling is out of scope for now.

---

## Project Structure

```
climate-backend/
├── cmd/server/main.go          Entry point. Wires all managers, MQTT handlers,
│                               HTTP server. The only place that reads env vars.
│
├── internal/
│   ├── models/models.go        All shared structs and enums. Mirrors the C++
│   │                           data structures from esp32-climate-controller/include/.
│   │                           Also defines Role, User, and AlertRule.
│   │                           Single source of truth for types used across packages.
│   │
│   ├── auth/
│   │   ├── service.go          JWT signing/validation. GenerateAccessToken (15 min),
│   │   │                       GenerateRefreshToken (7 days), ValidateToken.
│   │   │                       Uses HMAC-SHA256. Rejects empty JWT_SECRET at startup.
│   │   ├── middleware.go       HTTP middleware: extracts Bearer token, validates it,
│   │   │                       enforces tenant_id claim == URL {tenant_id},
│   │   │                       enforces RoleAdmin for mutating methods (POST/PUT/
│   │   │                       PATCH/DELETE). Stores claims in request context.
│   │   └── handler.go          Register / Login / Refresh HTTP handlers.
│   │                           Register: bcrypt hash, CreateUser, return token pair.
│   │                           Login: constant-time error for bad user or bad password.
│   │                           Refresh: re-validates token AND re-checks user in DB.
│   │
│   ├── alerts/engine.go        Alert rule engine. Loads all rules from DB at startup
│   │                           into an in-memory cache. Evaluate() is called on every
│   │                           sensor reading; matching rules fire in a goroutine.
│   │                           Cooldown tracked in memory, last_fired persisted to DB.
│   │                           Email via net/smtp; push is a log placeholder (FCM TBD).
│   │
│   ├── mqtt/client.go          Paho MQTT client. Parses multi-tenant topics,
│   │                           decodes JSON payloads, calls Handlers callbacks.
│   │                           Does NOT touch the DB or managers directly.
│   │
│   ├── db/db.go                PostgreSQL connection pool, schema migration,
│   │                           and all SQL queries. Every public function takes
│   │                           (ctx, tenantID, ...) — no exceptions.
│   │                           Re-exports pgx.ErrNoRows as db.ErrNoRows.
│   │
│   ├── api/handlers.go         gorilla/mux HTTP handlers. Auth routes registered
│   │                           on the plain router; tenant routes on a protected
│   │                           subrouter with JWT middleware applied via Use().
│   │                           Device list queries the DB, not the in-memory map.
│   │
│   ├── ws/hub.go               gorilla/websocket per-tenant broadcast hub.
│   │                           BroadcastToTenant fans out only to that tenant's
│   │                           connected clients. Subscribe validates JWT from
│   │                           ?token= query param before upgrading.
│   │
│   ├── sensor/manager.go       Mirrors SensorManager. Caches the latest reading
│   │                           and evaluates SensorHealth per tenant/device.
│   │
│   ├── control/manager.go      Mirrors ControlManager. Tracks operational mode,
│   │                           active product mode, device relay states, compressor
│   │                           stats, and fallback mode transitions.
│   │
│   ├── relay/manager.go        Mirrors RelayManager. Tracks relay on/off state
│   │                           and enforces minimum on/off timing constraints.
│   │
│   ├── fan/manager.go          Mirrors FanManager. Tracks fan speed settings
│   │                           and mixing cycle state.
│   │
│   ├── light/manager.go        Mirrors LightManager. Tracks light mode (manual/
│   │                           auto) and on/off state.
│   │
│   ├── status/manager.go       Mirrors StatusManager. Tracks SystemState per
│   │                           tenant/device. In-memory only — populated by MQTT,
│   │                           reset on restart. Do NOT use for device enumeration.
│   │
│   ├── errmanager/manager.go   Mirrors ErrorManager. Stores active/inactive
│   │                           ErrorStatus records keyed by ErrorType.
│   │
│   ├── storage/manager.go      Mirrors StorageManager. In-memory settings cache
│   │                           backed by PostgreSQL. One row per tenant/device
│   │                           in device_settings.
│   │
│   └── datastore/manager.go    Mirrors DataManager. Wraps DB reads/writes for
│                               sensor readings and compressor cycles. Handles
│                               auto-registration (EnsureDevice) and zero-timestamp
│                               fallback before every insert.
│
├── scripts/
│   └── test-ws.sh              End-to-end WebSocket smoke test. Connects via
│                               websocat or wscat, publishes a sensor MQTT message,
│                               and checks that the live message is received.
│
├── mosquitto/config/
│   └── mosquitto.conf          Broker config: anonymous access, TCP 1883,
│                               WebSocket 9001, persistence enabled.
│
├── Makefile                    dev / build / test / lint / docker-up / docker-down
├── Dockerfile                  Multi-stage build: golang:1.22-alpine → alpine:3.19
├── docker-compose.yml          mosquitto + timescaledb + climate-backend services,
│                               Cloudflare tunnel stub (commented out).
├── .env                        Environment variables for docker-compose ONLY.
│                               Not loaded automatically by go run — use make dev
│                               or create a .env.local file instead.
└── .dockerignore
```

---

## JWT Authentication

### How it works

1. A user registers via `POST /api/auth/register` with `tenant_id`, `email`,
   `password`, and optional `role` (`"admin"` or `"user"`, default `"user"`).
2. Passwords are stored as bcrypt hashes (`bcrypt.DefaultCost = 10`).
3. `POST /api/auth/login` validates credentials and returns an **access token**
   (15 min TTL) and a **refresh token** (7 day TTL), both signed with HS256.
4. `POST /api/auth/refresh` accepts a refresh token, re-validates the user still
   exists in the database, and issues a new token pair.
5. All `/api/tenants/...` routes require `Authorization: Bearer <access_token>`.

### JWT claims

```json
{
  "user_id":   "uuid",
  "tenant_id": "tenant1",
  "email":     "user@example.com",
  "role":      "admin",
  "sub":       "uuid",
  "iat":       1234567890,
  "exp":       1234568790
}
```

### Tenant isolation enforcement

The middleware extracts `{tenant_id}` from the URL path and compares it to the
`tenant_id` claim in the token. A mismatch returns **403 Forbidden**. This means
a valid token for `tenant_a` cannot access any route under `/api/tenants/tenant_b/`.

### Role enforcement

| HTTP method | Required role |
|---|---|
| `GET` | `user` or `admin` (except alert-rules GET, which requires admin) |
| `POST`, `PUT`, `PATCH`, `DELETE` | `admin` only |

### Device-to-backend auth

MQTT stays anonymous (no JWT). Device auth via API key or TLS client certificates
is planned but not yet implemented. Do not add JWT validation to the MQTT path.

---

## Alert Engine

### How it works

1. On startup, `alerts.Engine.LoadAll(ctx)` fetches every rule from the
   `alert_rules` table and stores them in an in-memory map keyed by
   `"tenantID/deviceID"`.
2. On every inbound MQTT sensor reading, `alertEngine.Evaluate(tenantID, deviceID, r)`
   is called from the MQTT handler (before the WebSocket broadcast).
3. For each enabled rule that matches the device, the engine checks the condition
   and the per-rule cooldown. If both pass, a goroutine is spawned to send the
   notification and update `last_fired` in the database.
4. When a rule is created, updated, or deleted via the API, the engine reloads
   the cache for that tenant/device from the database.

### Supported metrics

| Value | Sensor field |
|-------|-------------|
| `temperature` | `reading.Temperature` |
| `humidity` | `reading.Humidity` |

### Supported operators

| Value | Condition |
|-------|-----------|
| `gt`  | value > threshold |
| `lt`  | value < threshold |
| `gte` | value >= threshold |
| `lte` | value <= threshold |

### Cooldown

Each rule has a `cooldown_minutes` field (default 15). After a rule fires, it
will not fire again until the cooldown has elapsed. The last-fired time is tracked
in memory (fast path) and persisted to `alert_rules.last_fired` (survives restarts).

### Notification channels

| Channel | Behaviour |
|---------|-----------|
| `email` | Sends a plain-text email via SMTP (`net/smtp`). Requires `SMTP_HOST` to be set. Skips silently if `SMTP_HOST` is empty. |
| `push`  | **Placeholder only** — logs the event. FCM integration is not yet implemented. |

---

## HTTP API Routes

### Unauthenticated

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/auth/register` | Create user, returns token pair |
| `POST` | `/api/auth/login` | Validate credentials, returns token pair |
| `POST` | `/api/auth/refresh` | Exchange refresh token for new token pair |
| `WS`   | `/ws/{tenant_id}?token=<jwt>` | WebSocket stream; JWT validated from query param |

### Authenticated — any role

All routes below require `Authorization: Bearer <token>` where `tenant_id` in the
token matches `{tenant_id}` in the path.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/tenants/{tenant_id}/devices` | List device IDs (from DB) |
| `GET` | `/api/tenants/{tenant_id}/devices/{device_id}/current` | Latest sensor reading |
| `GET` | `/api/tenants/{tenant_id}/devices/{device_id}/status` | System status + relay states |
| `GET` | `/api/tenants/{tenant_id}/devices/{device_id}/history?days=N` | Reading history (max 31 days, 144 records) |
| `GET` | `/api/tenants/{tenant_id}/devices/{device_id}/errors` | Active errors |
| `GET` | `/api/tenants/{tenant_id}/devices/{device_id}/settings` | All settings |

### Authenticated — admin role only

| Method | Path | Description |
|--------|------|-------------|
| `POST`   | `/api/tenants/{tenant_id}/devices/{device_id}/settings` | Save settings |
| `POST`   | `/api/tenants/{tenant_id}/devices/{device_id}/mode` | Switch operating mode |
| `GET`    | `/api/tenants/{tenant_id}/devices/{device_id}/alert-rules` | List alert rules |
| `POST`   | `/api/tenants/{tenant_id}/devices/{device_id}/alert-rules` | Create alert rule |
| `PUT`    | `/api/tenants/{tenant_id}/devices/{device_id}/alert-rules/{rule_id}` | Update alert rule |
| `DELETE` | `/api/tenants/{tenant_id}/devices/{device_id}/alert-rules/{rule_id}` | Delete alert rule |

#### Alert rule body (POST / PUT)

```json
{
  "metric":           "temperature",
  "operator":         "gt",
  "threshold":        8.0,
  "channel":          "email",
  "recipient":        "admin@example.com",
  "enabled":          true,
  "cooldown_minutes": 30
}
```

`cooldown_minutes` defaults to 15 if omitted or ≤ 0.

---

## MQTT Topic Structure

```
climate / <tenant_id> / <device_id> / <subtopic>
```

### Inbound subtopics (device → backend)

| Subtopic     | Payload type            | Handler                |
|--------------|-------------------------|------------------------|
| `sensor`     | `models.Reading`        | `OnSensorReading`      |
| `status`     | `models.SystemStatus`   | `OnSystemStatus`       |
| `relays`     | `models.DeviceStates`   | `OnDeviceStates`       |
| `settings`   | `models.DeviceSnapshot` | `OnSettings`           |
| `errors`     | `[]models.ErrorStatus`  | `OnErrors`             |
| `compressor` | `models.CompressorCycle`| `OnCompressorCycle`    |
| `identity`   | `models.DeviceIdentity` | `OnIdentity`           |

### Outbound subtopics (backend → device)

```
climate/<tenant_id>/<device_id>/cmd/<command>
```

Published via `mqtt.Client.PublishCommand(tenantID, deviceID, command, payload)`.

### Broker subscription

The backend subscribes to the single wildcard `climate/+/+/#` on connect.
The two `+` wildcards capture tenant and device IDs; `#` captures any subtopic depth.

---

## Database Schema (key points)

All tables use `(tenant_id, device_id)` as the composite owner key. The `devices`
table primary key is `(tenant_id, device_id)`; every other table has a composite
foreign key referencing it.

The `users` table uses `(tenant_id, email)` as a unique constraint. The same email
address may exist in different tenants.

The `alert_rules` table stores one row per rule. `last_fired` is nullable and
updated each time a rule fires. The engine seeds its in-memory cooldown map from
`last_fired` on startup so cooldowns survive restarts.

**Auto-registration**: `db.EnsureDevice(ctx, tenantID, deviceID)` runs an
`INSERT ... ON CONFLICT DO NOTHING` before every sensor/cycle insert so devices
self-register on first MQTT contact. This is intentional — do not add a guard that
requires an explicit registration step.

---

## Running Locally

The simplest way to run locally is with `make dev`, which sets all required
environment variables and starts the server in one command:

```bash
# Start the broker and database first
make docker-up

# Then run the backend with pre-configured local env vars
make dev
```

`make dev` sets `DATABASE_URL`, `MQTT_URL`, `JWT_SECRET=dev-secret`, and
`LISTEN_ADDR=:8080` inline, so no shell exports are needed.

### `.env.local` — personal overrides

Create `.env.local` in the project root to override any variable without
modifying the Makefile or `.env`. It is loaded automatically at startup and
is listed in `.gitignore` so it is never committed.

```bash
# .env.local example
SMTP_HOST=smtp.example.com
SMTP_PORT=587
SMTP_USER=alerts@example.com
SMTP_PASS=secret
SMTP_FROM=alerts@example.com
JWT_SECRET=my-local-secret
```

`.env.local` is **not** read by `go run` directly — it is loaded by the Go
binary itself via `godotenv` at startup. `make dev` inline variables take
precedence over `.env.local` because they are set in the process environment
before the binary reads the file.

### Known issue: `.env` is not auto-loaded

`.env` is consumed by `docker compose` only and is never loaded by the Go
binary. Use `make dev` or `.env.local` for local development.

### With Docker Compose (recommended for full stack)

```bash
# Start broker + database only, run backend locally
make docker-up
make dev

# Build and run the full stack (reads .env automatically)
docker compose up --build
```

### Quick auth flow (curl)

```bash
# Register an admin user
curl -s -X POST http://localhost:8080/api/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"tenant_id":"tenant1","email":"admin@example.com","password":"secret","role":"admin"}'

# Login and capture the access token
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"tenant_id":"tenant1","email":"admin@example.com","password":"secret"}' \
  | grep -o '"access_token":"[^"]*"' | cut -d'"' -f4)

# List devices
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/tenants/tenant1/devices

# Create an alert rule
curl -s -X POST http://localhost:8080/api/tenants/tenant1/devices/device1/alert-rules \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"metric":"temperature","operator":"gt","threshold":8.0,"channel":"email","recipient":"admin@example.com","enabled":true,"cooldown_minutes":30}'
```

### Publish a test sensor message

```bash
mosquitto_pub -h localhost -t "climate/tenant1/device1/sensor" \
  -m '{"temperature":4.2,"humidity":82.5,"fallback_time":false}'
```

The device auto-registers on first publish. No prior setup needed.

---

## Environment Variables

| Variable | Default (docker-compose) | Description |
|---|---|---|
| `DATABASE_URL` | `postgres://climate:climate@timescaledb:5432/climate?sslmode=disable` | pgx DSN for TimescaleDB |
| `MQTT_URL` | `tcp://mosquitto:1883` | Paho broker URL |
| `LISTEN_ADDR` | `:8080` | HTTP server bind address |
| `JWT_SECRET` | *(none — startup fails if empty)* | HMAC-SHA256 signing secret. Generate with `openssl rand -hex 32` |
| `SMTP_HOST` | *(unset)* | SMTP server hostname. Email alerts are skipped if empty. |
| `SMTP_PORT` | `587` | SMTP server port (STARTTLS). |
| `SMTP_USER` | *(unset)* | SMTP username. If empty, no auth is sent (relay mode). |
| `SMTP_PASS` | *(unset)* | SMTP password. |
| `SMTP_FROM` | *(unset)* | From address for alert emails. |
| `POSTGRES_USER` | `climate` | Used by the timescaledb container only |
| `POSTGRES_PASSWORD` | `climate` | Used by the timescaledb container only |
| `POSTGRES_DB` | `climate` | Used by the timescaledb container only |
| `CLOUDFLARE_TUNNEL_TOKEN` | *(unset)* | Required only if the tunnel service is uncommented |

For local development without Docker, replace `timescaledb` and `mosquitto` with
`localhost` in `DATABASE_URL` and `MQTT_URL`.

---

## Development Conventions

### Error handling

- All errors are returned up the call stack. Never swallow an error silently.
- Errors from fire-and-forget goroutines (MQTT callbacks, WebSocket pumps, alert
  notifications) are logged with `log.Printf` and do not crash the process.
- The pattern `if err != nil { return fmt.Errorf("pkg: context: %w", err) }` is
  used throughout the `db` package for wrappable error chains.

### Logging

- `log.Printf` only. No third-party logger at this stage.
- Format: `"package: action noun/key: %v"`, e.g. `"db: insert reading t1/dev1: %v"`.
- Log at the call site, not deep inside helpers, to keep context visible.

### Tenant isolation

- Every in-memory manager key is `tenantID + "/" + deviceID` (via the private
  `tenantKey` helper in each package). Never use `deviceID` alone as a map key.
- Every DB function signature is `(ctx, tenantID, ...)`. There are no queries
  that operate across tenants.
- HTTP handlers extract both `{tenant_id}` and `{device_id}` from the path; the
  `pathIDs` helper in `api/handlers.go` enforces this consistently.
- The JWT middleware enforces that the token's `tenant_id` claim matches the URL
  path variable before any handler runs.

### DB is the source of truth for persistent data

In-memory managers (`status`, `sensor`, `control`, `alerts`, etc.) are **caches**
populated by live MQTT traffic or DB loads at startup. They reset on every server
restart and may be empty.

**Never use in-memory state to answer questions about what exists in the system.**
Always query the database for:

- Device enumeration (`handleListDevices` → `db.ListDeviceIDs`)
- Settings that must survive a restart (`storage.Manager` reads from DB on first access)
- User lookup (`auth` always hits the DB on login and refresh)
- Alert rules (`alerts.Engine` loads from DB at startup; CRUD operations reload per pair)

This was the root cause of a real bug: `handleListDevices` originally read from
`status.Manager.AllDeviceKeys()` and returned an empty array for devices that were
in the database but had not sent an MQTT status message since the server started.

### Adding a new MQTT subtopic

1. Add a handler field to `mqtt.Handlers` in `mqtt/client.go`.
2. Add the `case` to `dispatch`.
3. Wire the callback in `cmd/server/main.go`.

### Adding a new API endpoint

1. Decide if the route is public (auth endpoints) or protected (tenant routes).
2. Public: register directly on `r` in `api.New()`.
3. Protected: register on the `protected` subrouter — middleware is applied automatically.
4. Use `pathIDs(r)` to extract `tenantID, deviceID`.
5. Pass both to every manager and DB call.
6. If the data must reflect the persisted state, query the DB — do not read from
   an in-memory manager.
7. If the endpoint must be admin-only for GET requests, call `h.requireAdmin(w, r)`
   explicitly (the middleware only enforces admin for mutating HTTP methods).

---

## What NOT to Change

**Tenant isolation logic** — the `tenantKey(tenantID, deviceID)` composite key
pattern in every manager, and the `(tenant_id, device_id)` compound primary/foreign
keys in the database, must not be simplified to a single-column key. Multiple tenants
can have devices with the same `device_id`; collapsing this would silently mix data.

**JWT middleware tenant check** — the line in `auth/middleware.go` that compares
`claims.TenantID` to the URL `{tenant_id}` variable is the primary enforcement point
for tenant isolation on the HTTP layer. Removing or weakening it allows any
authenticated user to read or modify any other tenant's data.

**MQTT topic structure** — `climate/<tenant_id>/<device_id>/<subtopic>` is the
contract with the ESP32 firmware. Changing the segment positions or count breaks
all deployed devices. The subscription wildcard `climate/+/+/#` depends on exactly
two single-level wildcards before the subtopic.

**`EnsureDevice` before inserts** — `datastore.AddReading` and `AddCompressorCycle`
call `db.EnsureDevice` unconditionally. Removing this check causes FK violations for
any device that has not yet published an `identity` message.

**Zero-timestamp fallback** — `datastore.AddReading` stamps `time.Now().UTC()` when
`r.Timestamp` is zero before passing the reading to the DB layer. `db.InsertReading`
has a second guard for the same reason. Both are needed: the datastore guard keeps
the in-memory state consistent; the DB guard protects against direct callers.

**Device-to-backend MQTT auth** — ESP32 devices use anonymous MQTT, not JWT. Do not
add JWT validation to any MQTT code path.

**Alert rule cache invalidation** — after any CRUD operation on `alert_rules`, the
engine's in-memory cache must be reloaded for the affected tenant/device. The CRUD
methods on `alerts.Engine` do this automatically. Do not call `db.*AlertRule`
functions directly from handlers — always go through the engine so the cache stays
consistent with the database.
