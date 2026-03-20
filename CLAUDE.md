# climate-backend — Developer & AI Agent Reference

## Project Overview

Multi-tenant IoT backend for ESP32-based climate controllers used in cold storage
and refrigeration units (walk-in coolers, fermentation chambers, meat/dairy rooms).

Each physical ESP32 device runs the companion firmware in `../esp32-climate-controller`.
The backend receives telemetry over MQTT, persists it to TimescaleDB (PostgreSQL 16),
and exposes a REST + WebSocket API for dashboards and mobile clients.

Key capabilities:
- Ingest sensor readings (temperature, humidity) from multiple tenants' devices
- Track relay states (compressor, fan, light, heating, dehumidifier)
- Store compressor cycle history for predictive maintenance
- Broadcast live sensor snapshots to WebSocket subscribers per tenant
- Auto-register unknown devices on first MQTT contact
- Expose per-tenant, per-device settings and history over HTTP
- JWT authentication (HS256) with tenant isolation and role-based access control
- Alert engine: evaluate threshold rules on every sensor reading, notify via email (SMTP) or push (placeholder)

---

## Architecture

```
ESP32 firmware
     │  MQTT publish (anonymous)
     ▼
eclipse-mosquitto:1883
     │  subscribe climate/+/+/#
     ▼
climate-backend (Go)
  ├── internal/mqtt       parse topics + dispatch to callbacks
  ├── internal/auth       JWT sign/validate, HTTP middleware, register/login/refresh
  ├── internal/alerts     alert rule evaluation + SMTP email notifications
  ├── internal/datastore  sensor reading and compressor cycle persistence
  ├── internal/db         all SQL queries, schema migration, connection pool
  ├── internal/api        gorilla/mux REST handlers
  ├── internal/ws         gorilla/websocket per-tenant broadcast hub
  ├── internal/devicelog  plain-text log file writer for ESP32 log messages
  └── internal/*mgr       in-memory state caches (sensor, control, relay, fan,
                          light, status, errmanager, storage)
     │
     ├── TimescaleDB (PostgreSQL 16 + timescaledb extension)
     └── WebSocket clients (browser dashboards, mobile)
```

Single binary. No message queue — Go is the sole MQTT consumer. Horizontal
scaling is out of scope.

---

## Project Structure

```
climate-backend/
├── cmd/server/main.go          Entry point. Reads all env vars, wires every
│                               manager, MQTT client, and HTTP server. The only
│                               file that calls os.Getenv / godotenv.
│
├── internal/
│   ├── models/models.go        All shared structs and enums. Mirrors C++ types
│   │                           from esp32-climate-controller/include/. Also
│   │                           defines Role, User, AlertRule. Single source of
│   │                           truth for types used across packages.
│   │
│   ├── auth/
│   │   ├── service.go          JWT signing/validation. GenerateAccessToken
│   │   │                       (15 min TTL), GenerateRefreshToken (7 day TTL),
│   │   │                       ValidateToken. HMAC-SHA256. Fails at startup if
│   │   │                       JWT_SECRET is empty.
│   │   ├── middleware.go       HTTP middleware: extracts Bearer token, validates
│   │   │                       it, enforces token tenant_id == URL {tenant_id},
│   │   │                       enforces RoleAdmin for POST/PUT/PATCH/DELETE.
│   │   │                       Stores *Claims in request context.
│   │   └── handler.go          Register / Login / Refresh HTTP handlers.
│   │                           Register: bcrypt hash, CreateUser, 201 + token pair.
│   │                           Login: constant-time error for bad user/password.
│   │                           Refresh: re-validates token AND re-checks user in DB.
│   │
│   ├── alerts/engine.go        Alert rule engine. Loads all rules from DB at
│   │                           startup into in-memory map keyed by
│   │                           "tenantID/deviceID". Evaluate() called on every
│   │                           sensor reading; matching rules fire in a goroutine.
│   │                           Cooldown tracked in memory; last_fired persisted
│   │                           to DB. Email via net/smtp; push is a log placeholder.
│   │
│   ├── mqtt/client.go          Paho MQTT client. Subscribes to climate/+/+/#,
│   │                           parses multi-tenant topics, JSON-decodes payloads,
│   │                           dispatches to registered Handlers callbacks.
│   │                           Does NOT touch DB or managers directly.
│   │
│   ├── db/db.go                pgxpool connection pool, schema migration (all
│   │                           CREATE TABLE IF NOT EXISTS run at startup), and
│   │                           every SQL query. All public functions take
│   │                           (ctx, tenantID, ...). Re-exports pgx.ErrNoRows
│   │                           as db.ErrNoRows.
│   │
│   ├── api/handlers.go         gorilla/mux HTTP handlers. Auth routes on plain
│   │                           router; tenant routes on protected subrouter with
│   │                           JWT middleware via Use(). pathIDs() helper extracts
│   │                           tenantID + deviceID from every route.
│   │
│   ├── ws/hub.go               Per-tenant broadcast hub. BroadcastToTenant fans
│   │                           out only to that tenant's WebSocket clients.
│   │                           Subscribe validates JWT from ?token= before upgrade.
│   │   hub_test.go             Unit tests for the hub.
│   │
│   ├── datastore/manager.go    Wraps DB reads/writes for sensor readings and
│   │                           compressor cycles. Stamps every reading AND
│   │                           every compressor cycle with time.Now().UTC() —
│   │                           ESP32 payload timestamps are ignored for storage.
│   │                           Calls EnsureDevice before every insert.
│   │
│   ├── sensor/manager.go       In-memory latest SensorReading + SensorHealth
│   │                           evaluation (warning >60 s stale, error >300 s).
│   │
│   ├── control/manager.go      In-memory DeviceControl: OperationalMode,
│   │                           ActiveMode, ModeSettings, DeviceStates,
│   │                           CompressorStats, FallbackStatistics.
│   │
│   ├── relay/manager.go        In-memory relay on/off state + minimum on/off
│   │                           timing constraints (CanToggle logic).
│   │
│   ├── fan/manager.go          In-memory FanSettings and mixing cycle state
│   │                           (active, startedAt) per tenant/device.
│   │
│   ├── light/manager.go        In-memory LightSettings (mode manual/auto, state).
│   │
│   ├── status/manager.go       In-memory SystemStatus per tenant/device.
│   │                           Populated by MQTT; resets on restart. Do NOT use
│   │                           for device enumeration.
│   │
│   ├── errmanager/manager.go   In-memory map[ErrorType]*ErrorStatus per
│   │                           tenant/device. ReplaceAll on every errors message.
│   │
│   ├── storage/manager.go      In-memory settings cache (TempSettings,
│   │                           HumiditySettings, FanSettings, LightSettings,
│   │                           SystemSettings, DisplaySettings) backed by
│   │                           device_settings table. Persists on every Save*.
│   │
│   └── devicelog/writer.go     Writes plain-text ESP32 log messages to
│                               logs/<tenantID>/<deviceID>.log (relative to
│                               working directory). Creates directories on first
│                               write. One line per message:
│                               "2026-03-20T22:07:34Z [deviceID] message\n"
│                               Exported function: Write(tenantID, deviceID,
│                               message string) error
│
├── scripts/
│   └── test-ws.sh              End-to-end WebSocket smoke test (websocat/wscat).
│
├── mosquitto/config/
│   └── mosquitto.conf          Anonymous access, TCP 1883, WS 9001, persistence.
│
├── Makefile                    dev / build / test / lint / docker-up / docker-down
├── Dockerfile                  Multi-stage: golang:1.22-alpine → alpine:3.19
├── docker-compose.yml          mosquitto + timescaledb + climate-backend services.
│                               Cloudflare tunnel stub is commented out.
├── .env                        Consumed by docker-compose only. Never auto-loaded
│                               by the Go binary. Use make dev or .env.local.
└── .dockerignore
```

---

## Models Reference (internal/models/models.go)

### Auth

```go
type Role   string  // RoleAdmin = "admin", RoleUser = "user"
type User   struct  // ID (UUID), TenantID, Email, PasswordHash (json:"-"), Role, CreatedAt
```

### Sensor

```go
type SensorHealth  int    // SensorHealthGood, SensorHealthWarning, SensorHealthError
type SensorReading struct  // Temperature, Humidity, Timestamp, FallbackTime, Health
type Reading       struct  // Temperature, Humidity, Timestamp, FallbackTime  (DB form)
type CompressorCycle struct // WorkTime json:"work_time", RestTime json:"rest_time" (uint32 seconds),
                            // Temp, Humidity (float32), CreatedAt
                            // JSON tags are work_time/rest_time — NOT work_time_s/rest_time_s
```

### Settings

```go
type TempSettings     struct  // Target, Offset (float32)
type HumiditySettings struct  // Target, Offset (float32)
type FanSettings      struct  // Speed (uint8), MixingInterval, MixingDuration (uint32), MixingEnabled
type LightMode        int     // LightModeManual, LightModeAuto
type LightSettings    struct  // Mode, State
type SystemSettings   struct  // OperationMode string ("normal"|"fallback"|"emergency")
type DisplaySettings  struct  // Brightness (uint8), SleepTimeout (uint32), AutoSleep
```

### Control

```go
type ModeType int  // ModeNormal, ModeHeating, ModeBeercooling, ModeRoomTemp,
                   // ModeProductMeatFish, ModeProductDairy, ModeProductReadyFood,
                   // ModeProductVegetables  (8 values, iota)
type OperationalMode int  // OperationalModeNormal, OperationalModeFallback, OperationalModeEmergency
type FallbackState   int  // FallbackStateIdle, FallbackStateCompressorOn, FallbackStateCompressorOff
type ModeSettings    struct  // Mode, TargetTemp, Tolerance, CompressorEnabled,
                             // HeatingEnabled, DehumidifierEnabled, ExtraFanEnabled
type CompressorStats      struct  // WorkTime, RestTime, CycleCount (uint32)
type FallbackStatistics   struct  // EnterCount, TotalDuration, LastEntered
type DeviceStates         struct  // Compressor, FanCompressor, ExtraFan, Light, Heating,
                                  // Dehumidifier (all bool)
```

### Relay

```go
type RelayType int   // RelayCompressor, RelayFanCompressor, RelayExtraFan,
                     // RelayLight, RelayHeating, RelayDehumidifier  (6 values)
type RelayInfo struct // Type, State, MinOnTime, MinOffTime, LastChanged
```

### Errors

```go
type ErrorType     int  // ErrorRTC, ErrorSensorTemp, ErrorSensorHum, ErrorWiFi,
                        // ErrorStorage, ErrorFan, ErrorSystem  (7 values)
type ErrorSeverity int  // SeverityInfo, SeverityWarning, SeverityError
type ErrorStatus struct  // Type, Severity, Message, Active, Timestamp
```

### Status / Network

```go
type SystemState int  // SystemStateNormal, SystemStateWarning, SystemStateError,
                      // SystemStateSafeMode, SystemStateFallback  (5 values)
type SystemStatus struct  // State, DHTOk, RTCOk, UptimeSeconds, RestartCount, Timestamp
type WiFiState    int     // WiFiStateBooting, WiFiStateBootRetry, WiFiStateConnected,
                          // WiFiStateReconnecting, WiFiStateTempAP, WiFiStatePersistentAP
type DeviceIdentity struct // TenantID, DeviceID, DeviceName, Hostname, IPAddress, WiFiState
```

### Alerts

```go
type AlertRule struct  // ID (UUID), TenantID, DeviceID, Metric, Operator, Threshold (float64),
                       // Channel, Recipient, Enabled, CooldownMinutes, LastFired (*time.Time),
                       // CreatedAt
```

### Aggregated

```go
type DeviceSnapshot struct  // TenantID, DeviceID, Timestamp, Sensor (SensorReading),
                             // DeviceStates, OperationalMode, ActiveMode, SystemStatus,
                             // Errors ([]ErrorStatus), FanSettings, TempSettings,
                             // HumiditySettings, LightSettings
```

---

## JWT Authentication

### Flow

1. Register via `POST /api/auth/register` — fields: `tenant_id`, `email`, `password`,
   optional `role` (`"admin"` | `"user"`, default `"user"`).
2. Passwords stored as bcrypt hashes (`bcrypt.DefaultCost = 10`).
3. Login via `POST /api/auth/login` — returns **access token** (15 min) and
   **refresh token** (7 days), both HS256-signed.
4. Refresh via `POST /api/auth/refresh` — re-validates user still exists in DB,
   issues a new token pair.
5. All `/api/tenants/...` routes require `Authorization: Bearer <access_token>`.

### JWT claims payload

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

The middleware compares the `{tenant_id}` URL path variable to `claims.TenantID`.
A mismatch returns **403 Forbidden** before any handler runs.

### Role enforcement

| HTTP method | Required role |
|---|---|
| `GET` | `user` or `admin` (except alert-rules GET — admin only) |
| `POST`, `PUT`, `PATCH`, `DELETE` | `admin` only |

### Device-to-backend auth

MQTT is anonymous. API key / TLS client certificate auth is planned but not
implemented. Do not add JWT validation to any MQTT code path.

---

## Alert Engine

### Lifecycle

1. `alerts.Engine.LoadAll(ctx)` at startup fetches every rule from `alert_rules`,
   populates an in-memory `map[string][]AlertRule` keyed by `"tenantID/deviceID"`,
   and seeds the cooldown map from `last_fired`.
2. `alertEngine.Evaluate(tenantID, deviceID, reading)` is called from the MQTT
   `OnSensorReading` callback before the WebSocket broadcast.
3. For each enabled matching rule: if the threshold condition passes and the cooldown
   has elapsed, a goroutine is spawned to fire the notification and update `last_fired`.
4. After any CRUD operation the engine calls `reload(ctx, tenantID, deviceID)` to
   refresh the cache for that pair.

### Supported metrics

| Value | Field |
|---|---|
| `temperature` | `reading.Temperature` |
| `humidity` | `reading.Humidity` |

### Supported operators

| Value | Condition |
|---|---|
| `gt`  | value > threshold |
| `lt`  | value < threshold |
| `gte` | value >= threshold |
| `lte` | value <= threshold |

### Notification channels

| Channel | Behaviour |
|---|---|
| `email` | Sends plain-text email via `net/smtp`. Uses `PlainAuth` if `SMTP_USER` is set, relay mode otherwise. Skipped silently if `SMTP_HOST` is empty. |
| `push`  | **Placeholder** — logs the event only. FCM not implemented. |

### Cooldown

`cooldown_minutes` defaults to 15. Tracked in memory (fast path) and persisted
to `alert_rules.last_fired` so cooldowns survive restarts.

---

## HTTP API Routes

### Unauthenticated

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/auth/register` | Create user; returns token pair (201) |
| `POST` | `/api/auth/login` | Validate credentials; returns token pair |
| `POST` | `/api/auth/refresh` | Exchange refresh token for new token pair |
| `WS`   | `/ws/{tenant_id}?token=<jwt>` | WebSocket live stream; JWT validated from query param |

### Authenticated — any role

All routes require `Authorization: Bearer <token>` with matching tenant_id.

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/tenants/{tenant_id}/devices` | List device IDs from DB (ordered by last_seen DESC) |
| `GET` | `/api/tenants/{tenant_id}/devices/{device_id}/current` | Latest sensor reading from **in-memory** sensor manager |
| `GET` | `/api/tenants/{tenant_id}/devices/{device_id}/status` | System status (DB) + relay states + compressor stats + error flags (in-memory) |
| `GET` | `/api/tenants/{tenant_id}/devices/{device_id}/history?days=N` | Readings from DB; N capped at 31, max 144 records |
| `GET` | `/api/tenants/{tenant_id}/devices/{device_id}/compressor-cycles?days=N` | Compressor cycles from DB; defaults to 7 days, max 200 records |
| `GET` | `/api/tenants/{tenant_id}/devices/{device_id}/errors` | Active errors from in-memory error manager |
| `GET` | `/api/tenants/{tenant_id}/devices/{device_id}/settings` | All settings from in-memory storage manager (backed by DB) |

### Authenticated — admin role only

| Method | Path | Description |
|---|---|---|
| `POST`   | `/api/tenants/{tenant_id}/devices/{device_id}/settings` | Persist settings to DB (accepts partial: temp, humidity, fan, light). If temp or humidity fields are present, also publishes a config payload to `climate/<tenant>/<device>/config` via MQTT (QoS 1). HTTP 204 is returned regardless of MQTT publish result. |
| `POST`   | `/api/tenants/{tenant_id}/devices/{device_id}/mode` | **TODO stub** — does not yet publish MQTT command |
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

### WebSocket live message format

Pushed to all subscribers of a tenant after every sensor reading:

```json
{
  "type":        "sensor",
  "device_id":   "773C",
  "temperature": 4.2,
  "humidity":    82.5,
  "timestamp":   "2026-03-20T14:00:00Z"
}
```

`timestamp` is always `time.Now().UTC()` stamped by the backend at broadcast time —
the ESP32 payload timestamp is discarded (device sends local time, not UTC).

---

## MQTT Topic Structure

```
climate / <tenant_id> / <device_id> / <subtopic>
```

### Inbound subtopics (device → backend)

| Subtopic     | Payload type             | Handler              |
|---|---|---|
| `sensor`     | `models.Reading`         | `OnSensorReading`    |
| `status`     | `models.SystemStatus`    | `OnSystemStatus`     |
| `relays`     | `models.DeviceStates`    | `OnDeviceStates`     |
| `settings`   | `models.DeviceSnapshot`  | `OnSettings`         |
| `errors`     | `[]models.ErrorStatus`   | `OnErrors`           |
| `compressor` | `models.CompressorCycle` | `OnCompressorCycle`  |
| `identity`   | `models.DeviceIdentity`  | `OnIdentity`         |
| `logs`       | plain text string        | `OnLog`              |

Unknown subtopics are silently discarded.

### Outbound subtopics (backend → device)

| Topic | Method | Payload | Trigger |
|---|---|---|---|
| `climate/<tenant>/<device>/cmd/<command>` | `PublishCommand(tenantID, deviceID, command, payload)` | any JSON | (stub — mode switch not yet wired) |
| `climate/<tenant>/<device>/config` | `PublishConfig(tenantID, deviceID, payload)` | `{"temp_target":18.5,"temp_offset":0.5,"hum_target":83,"hum_offset":3}` | `POST /settings` with temp or humidity fields |

`PublishConfig` publishes QoS 1, retained=false. Only the fields present in the
`POST /settings` request body are included in the payload — omitted fields are not
sent. The `api` package calls it through the `ConfigPublisher` interface (defined in
`api/handlers.go`) so the api package does not import the mqtt package directly.

### Broker subscription

`climate/+/+/#` — two single-level wildcards for tenant/device IDs, `#` for
any subtopic depth. The segment positions are a firmware contract; do not change.

---

## Database Schema

Schema is applied at startup via `CREATE TABLE IF NOT EXISTS` statements in
`db.New()`. The TimescaleDB extension is present (container image) but
**hypertables are not yet created** — all tables are plain PostgreSQL.

### devices

```sql
CREATE TABLE IF NOT EXISTS devices (
    tenant_id   TEXT        NOT NULL,
    device_id   TEXT        NOT NULL,
    device_name TEXT        NOT NULL DEFAULT '',
    hostname    TEXT        NOT NULL DEFAULT '',
    ip_address  TEXT        NOT NULL DEFAULT '',
    wifi_state  INTEGER     NOT NULL DEFAULT 0,
    last_seen   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, device_id)
);
```

### readings

```sql
CREATE TABLE IF NOT EXISTS readings (
    id            BIGSERIAL   PRIMARY KEY,
    tenant_id     TEXT        NOT NULL,
    device_id     TEXT        NOT NULL,
    temperature   REAL        NOT NULL,
    humidity      REAL        NOT NULL,
    fallback_time BOOLEAN     NOT NULL DEFAULT FALSE,
    recorded_at   TIMESTAMPTZ NOT NULL,
    FOREIGN KEY (tenant_id, device_id) REFERENCES devices(tenant_id, device_id)
);
CREATE INDEX IF NOT EXISTS readings_tenant_device_time
    ON readings (tenant_id, device_id, recorded_at DESC);
```

`recorded_at` is always set to the server's `time.Now().UTC()`. The ESP32
payload timestamp is ignored (device sends local time, not UTC).

### compressor_cycles

```sql
CREATE TABLE IF NOT EXISTS compressor_cycles (
    id          BIGSERIAL   PRIMARY KEY,
    tenant_id   TEXT        NOT NULL,
    device_id   TEXT        NOT NULL,
    work_time_s INTEGER     NOT NULL,
    rest_time_s INTEGER     NOT NULL,
    temperature REAL        NOT NULL,
    humidity    REAL        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (tenant_id, device_id) REFERENCES devices(tenant_id, device_id)
);
```

### system_status

```sql
CREATE TABLE IF NOT EXISTS system_status (
    id              BIGSERIAL   PRIMARY KEY,
    tenant_id       TEXT        NOT NULL,
    device_id       TEXT        NOT NULL,
    state           INTEGER     NOT NULL,
    dht_ok          BOOLEAN     NOT NULL,
    rtc_ok          BOOLEAN     NOT NULL,
    uptime_seconds  INTEGER     NOT NULL,
    restart_count   INTEGER     NOT NULL,
    recorded_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (tenant_id, device_id) REFERENCES devices(tenant_id, device_id)
);
```

### errors

```sql
CREATE TABLE IF NOT EXISTS errors (
    id          BIGSERIAL   PRIMARY KEY,
    tenant_id   TEXT        NOT NULL,
    device_id   TEXT        NOT NULL,
    error_type  INTEGER     NOT NULL,
    severity    INTEGER     NOT NULL,
    message     TEXT        NOT NULL,
    active      BOOLEAN     NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    FOREIGN KEY (tenant_id, device_id) REFERENCES devices(tenant_id, device_id)
);
```

### device_settings

```sql
CREATE TABLE IF NOT EXISTS device_settings (
    tenant_id           TEXT    NOT NULL,
    device_id           TEXT    NOT NULL,
    temp_target         REAL    NOT NULL DEFAULT 4.0,
    temp_offset         REAL    NOT NULL DEFAULT 0.0,
    humidity_target     REAL    NOT NULL DEFAULT 80.0,
    humidity_offset     REAL    NOT NULL DEFAULT 0.0,
    fan_speed           INTEGER NOT NULL DEFAULT 50,
    mixing_interval_s   INTEGER NOT NULL DEFAULT 3600,
    mixing_duration_s   INTEGER NOT NULL DEFAULT 300,
    mixing_enabled      BOOLEAN NOT NULL DEFAULT TRUE,
    light_mode          INTEGER NOT NULL DEFAULT 0,
    light_state         BOOLEAN NOT NULL DEFAULT FALSE,
    operational_mode    TEXT    NOT NULL DEFAULT 'normal',
    active_mode         INTEGER NOT NULL DEFAULT 0,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, device_id),
    FOREIGN KEY (tenant_id, device_id) REFERENCES devices(tenant_id, device_id)
);
```

### users

```sql
CREATE TABLE IF NOT EXISTS users (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     TEXT        NOT NULL,
    email         TEXT        NOT NULL,
    password_hash TEXT        NOT NULL,
    role          TEXT        NOT NULL DEFAULT 'user',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, email)
);
```

Same email may exist in different tenants — uniqueness is per (tenant_id, email).

### alert_rules

```sql
CREATE TABLE IF NOT EXISTS alert_rules (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        TEXT        NOT NULL,
    device_id        TEXT        NOT NULL,
    metric           TEXT        NOT NULL,
    operator         TEXT        NOT NULL,
    threshold        REAL        NOT NULL,
    channel          TEXT        NOT NULL DEFAULT 'email',
    recipient        TEXT        NOT NULL DEFAULT '',
    enabled          BOOLEAN     NOT NULL DEFAULT TRUE,
    cooldown_minutes INTEGER     NOT NULL DEFAULT 15,
    last_fired       TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (tenant_id, device_id) REFERENCES devices(tenant_id, device_id)
);
```

---

## Running Locally

### Prerequisites

- Docker + Docker Compose (for mosquitto + timescaledb)
- Go 1.22+

### Quickstart

```bash
# Start broker and database containers
make docker-up

# Run the backend (sets DATABASE_URL, MQTT_URL, JWT_SECRET, LISTEN_ADDR inline)
make dev
```

`make dev` uses `MQTT_URL=tcp://192.168.68.117:1883` (a hardcoded local broker IP)
and `JWT_SECRET=dev-secret`. Override via `.env.local` if needed.

### `.env.local` — personal overrides

Create `.env.local` in the project root; it is loaded by the binary at startup
via `godotenv` and is gitignored. Variables already set in the process environment
(e.g. by `make dev`) take precedence.

```bash
# .env.local example
DATABASE_URL=postgres://climate:climate@localhost:5432/climate?sslmode=disable
MQTT_URL=tcp://localhost:1883
JWT_SECRET=my-local-secret
SMTP_HOST=smtp.example.com
SMTP_PORT=587
SMTP_USER=alerts@example.com
SMTP_PASS=secret
SMTP_FROM=alerts@example.com
```

`.env` is consumed by `docker compose` only — never auto-loaded by the binary.

### Full stack with Docker Compose

```bash
docker compose up --build
```

Starts mosquitto, timescaledb, and climate-backend in dependency order (health
checks enforced). Cloudflare tunnel service is present but commented out.

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

Device auto-registers on first publish. No prior setup needed.

### Makefile targets

| Target | Action |
|---|---|
| `make dev` | Run backend locally with hardcoded dev env vars |
| `make build` | Compile binary to `bin/climate-backend` |
| `make test` | `go test ./...` |
| `make lint` | `go vet ./...` |
| `make docker-up` | Start mosquitto + timescaledb containers only |
| `make docker-down` | Stop and remove all containers |

---

## Production Deployment

### Infrastructure

| Component | Detail |
|---|---|
| Host | Proxmox LXC container 104 (`docker-services`), Debian |
| Stack | Docker Compose: `mosquitto` + `timescaledb` + `climate-backend` |
| Internal port | `8081` (mapped from container's `8080`) |
| Public URL | `https://climate.gotocloud.xyz` |
| Reverse proxy | Nginx Proxy Manager — proxies `climate.gotocloud.xyz` → `localhost:8081` |
| Tunnel | Cloudflare Tunnel — routes public HTTPS to Nginx Proxy Manager on the LXC host |

### Production vs local differences

| Setting | Local (`make dev`) | Production (docker-compose) |
|---|---|---|
| `DATABASE_URL` host | `localhost` | `timescaledb` (Docker service name) |
| `MQTT_URL` host | `192.168.68.117` (hardcoded LAN IP) | `mosquitto` (Docker service name) |
| `LISTEN_ADDR` | `:8080` | `:8080` (mapped to `8081` on host) |
| `JWT_SECRET` | `dev-secret` | Strong secret via `.env` |

The Cloudflare Tunnel service is defined in `docker-compose.yml` but commented out —
the tunnel token is injected separately on the LXC host and runs outside Compose.

---

## Environment Variables

| Variable | Default in `make dev` | Description |
|---|---|---|
| `DATABASE_URL` | `postgres://climate:climate@localhost:5432/climate?sslmode=disable` | pgx DSN |
| `MQTT_URL` | `tcp://192.168.68.117:1883` | Paho broker URL |
| `LISTEN_ADDR` | `:8080` | HTTP bind address |
| `JWT_SECRET` | `dev-secret` | HS256 signing key. **Startup fails if empty.** Use `openssl rand -hex 32` for production. |
| `SMTP_HOST` | *(unset)* | SMTP hostname. Email alerts are silently skipped if empty. |
| `SMTP_PORT` | `587` | SMTP port (STARTTLS). |
| `SMTP_USER` | *(unset)* | SMTP username. Empty = unauthenticated relay mode. |
| `SMTP_PASS` | *(unset)* | SMTP password. |
| `SMTP_FROM` | *(unset)* | From address for alert emails. |
| `POSTGRES_USER` | `climate` | docker-compose / timescaledb container only |
| `POSTGRES_PASSWORD` | `climate` | docker-compose / timescaledb container only |
| `POSTGRES_DB` | `climate` | docker-compose / timescaledb container only |
| `CLOUDFLARE_TUNNEL_TOKEN` | *(unset)* | Only if tunnel service is uncommented in docker-compose.yml |

In docker-compose, `DATABASE_URL` uses host `timescaledb` and `MQTT_URL` uses
`mosquitto` instead of `localhost`.

---

## What Is NOT Yet Implemented

| Feature | Status |
|---|---|
| `POST /mode` MQTT command dispatch | Handler exists, parses request, but **never publishes to MQTT**. Stub only. |
| FCM push notifications | `push` channel fires a `log.Printf` only. No FCM client or token management. Deferred until Flutter app is built. |
| React web app | Planned. Will live in `web/` directory. Not yet started. |
| Device-to-backend MQTT auth | Broker allows anonymous connections. API key / TLS client cert auth is planned. |
| TimescaleDB hypertables | Extension is available in the container image but `SELECT create_hypertable(...)` is never called. `readings` and `compressor_cycles` are plain PostgreSQL tables. |
| Cloudflare tunnel | Service definition exists in `docker-compose.yml` but is commented out. |
| Command acknowledgement | No mechanism for devices to confirm receipt of a `cmd/` message. |
| Device deregistration / deletion | No API or MQTT handler to remove a device from the database. |
| HTTP endpoint for device logs | `logs/<tenantID>/<deviceID>.log` files are written to disk but not exposed over HTTP. |

---

## Development Conventions

### Error handling

- All errors returned up the call stack. Never swallow silently.
- Goroutine-based fire-and-forget paths (MQTT callbacks, WebSocket pumps, alert
  notifications) log with `log.Printf` and do not crash the process.
- DB package wraps errors: `fmt.Errorf("db: action tenant/device: %w", err)`.

### Logging

- `log.Printf` only. No third-party logger.
- Format: `"package: action noun/key: %v"`, e.g. `"db: insert reading t1/dev1: %v"`.
- Log at the call site for visible context.

### Tenant isolation

- Every in-memory manager key is `tenantID + "/" + deviceID` via a private
  `tenantKey` helper. Never use `deviceID` alone as a map key.
- Every DB function signature is `(ctx, tenantID, ...)`. No cross-tenant queries.
- `pathIDs(r)` in `api/handlers.go` extracts both IDs from every route.
- JWT middleware enforces `claims.TenantID == URL {tenant_id}` before any handler.

### DB is the source of truth for persistent data

In-memory managers are **caches** populated by MQTT traffic or DB loads at startup.
They reset on restart and may be empty.

**Never use in-memory state to answer questions about what exists in the system.**
Always query the DB for:

- Device enumeration → `db.ListDeviceIDs`
- Settings that must survive restart → `storage.Manager` reads from DB on first access
- User lookup → always hits DB on login and refresh
- Alert rules → `alerts.Engine` loads from DB at startup; CRUD reloads per pair

Root cause of a fixed bug: `handleListDevices` originally called
`status.Manager.AllDeviceKeys()` and returned an empty array for devices that had
not sent an MQTT status since the last restart.

### Timestamp handling

`datastore.AddReading` unconditionally sets `r.Timestamp = time.Now().UTC()` before
any DB or in-memory write, discarding the ESP32 payload timestamp (device sends EET
local time, not UTC). `db.InsertReading` also uses `time.Now().UTC()` directly as
a second independent safeguard. Both guards must remain.

`datastore.AddCompressorCycle` unconditionally sets `c.CreatedAt = time.Now().UTC()`
for the same reason — the MQTT payload does not include a `created_at` field, so
without this stamp the column defaults to the Go zero time (`0001-01-01`).

### Adding a new MQTT subtopic

1. Add a handler field to `mqtt.Handlers` in `mqtt/client.go`.
2. Add the `case` to the `dispatch` switch.
3. Wire the callback in `cmd/server/main.go`.

### Adding a new API endpoint

1. Decide public (auth) vs protected (tenant).
2. Public: register on `r` in `api.New()`.
3. Protected: register on the `protected` subrouter (middleware applied automatically).
4. Use `pathIDs(r)` for tenantID + deviceID.
5. Pass both to every manager and DB call.
6. Persistent state → query the DB. Live state → in-memory manager.
7. Admin-only GET → call `h.requireAdmin(w, r)` explicitly (middleware only
   enforces admin for mutating methods).

---

## What NOT to Change

**Tenant isolation logic** — `tenantKey(tenantID, deviceID)` composite key in every
manager, and `(tenant_id, device_id)` compound FK in every DB table, must not be
collapsed to a single column. Multiple tenants may share the same device_id.

**JWT middleware tenant check** — `claims.TenantID == path {tenant_id}` in
`auth/middleware.go` is the HTTP-layer enforcement point. Weakening it allows
cross-tenant data access with any valid token.

**MQTT topic structure** — `climate/<tenant_id>/<device_id>/<subtopic>` is the
firmware contract. The subscription `climate/+/+/#` depends on exactly two
single-level wildcards. Changing segment positions breaks all deployed devices.

**`EnsureDevice` before inserts** — `datastore.AddReading` and `AddCompressorCycle`
call `db.EnsureDevice` unconditionally. Removing it causes FK violations for devices
that have not yet published an `identity` message.

**Server-side UTC timestamp** — `datastore.AddReading`, `db.InsertReading`, and
`datastore.AddCompressorCycle` all stamp `time.Now().UTC()` unconditionally.
The ESP32 sends local time (EET UTC+2) which must not be trusted for `recorded_at`
or `created_at`. Restoring any ESP32 timestamp would silently break time-range queries.

**Device-to-backend MQTT auth** — ESP32 devices use anonymous MQTT. Do not add JWT
validation to any MQTT code path.

**Alert rule cache invalidation** — after any CRUD on `alert_rules`, the engine's
in-memory cache must be reloaded via `reload(ctx, tenantID, deviceID)`. The CRUD
methods on `alerts.Engine` do this automatically. Do not call `db.*AlertRule`
functions directly from handlers — always go through the engine.
