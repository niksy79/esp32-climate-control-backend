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

---

## Architecture

```
ESP32 firmware
     │  MQTT publish
     ▼
eclipse-mosquitto:1883
     │  subscribe climate/+/+/#
     ▼
climate-backend (Go)
  ├── internal/mqtt     parse + dispatch
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
│   │                           Single source of truth for types used across packages.
│   │
│   ├── mqtt/client.go          Paho MQTT client. Parses multi-tenant topics,
│   │                           decodes JSON payloads, calls Handlers callbacks.
│   │                           Does NOT touch the DB or managers directly.
│   │
│   ├── db/db.go                PostgreSQL connection pool, schema migration,
│   │                           and all SQL queries. Every public function takes
│   │                           (ctx, tenantID, deviceID, ...) — no exceptions.
│   │
│   ├── api/handlers.go         gorilla/mux HTTP handlers. Routes are scoped to
│   │                           /api/tenants/{tenant_id}/devices/{device_id}/...
│   │                           WebSocket upgrade at /ws/{tenant_id}.
│   │
│   ├── ws/hub.go               gorilla/websocket broadcast hub. Receives any
│   │                           value via Broadcast(v any), marshals to JSON,
│   │                           fans out to all connected clients.
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
│   │                           tenant/device. AllDeviceKeys() feeds the device
│   │                           list API endpoint.
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
├── mosquitto/config/
│   └── mosquitto.conf          Broker config: anonymous access, TCP 1883,
│                               WebSocket 9001, persistence enabled.
│
├── Dockerfile                  Multi-stage build: golang:1.22-alpine → alpine:3.19
├── docker-compose.yml          mosquitto + timescaledb + climate-backend services,
│                               Cloudflare tunnel stub (commented out).
├── .env                        Environment variables consumed by docker-compose.
└── .dockerignore
```

---

## MQTT Topic Structure

```
climate / <tenant_id> / <device_id> / <subtopic>
```

### Inbound subtopics (device → backend)

| Subtopic     | Payload type          | Handler                |
|--------------|-----------------------|------------------------|
| `sensor`     | `models.Reading`      | `OnSensorReading`      |
| `status`     | `models.SystemStatus` | `OnSystemStatus`       |
| `relays`     | `models.DeviceStates` | `OnDeviceStates`       |
| `settings`   | `models.DeviceSnapshot` | `OnSettings`         |
| `errors`     | `[]models.ErrorStatus`| `OnErrors`             |
| `compressor` | `models.CompressorCycle` | `OnCompressorCycle` |
| `identity`   | `models.DeviceIdentity` | `OnIdentity`         |

### Outbound subtopics (backend → device)

```
climate/<tenant_id>/<device_id>/cmd/<command>
```

Published via `mqtt.Client.PublishCommand(tenantID, deviceID, command, payload)`.

### Broker subscription

The backend subscribes to the single wildcard `climate/+/+/#` on connect.
The two `+` wildcards capture tenant and device IDs; `#` captures any subtopic depth.

---

## HTTP API Routes

All device routes are tenant-scoped. Replacing `{tenant_id}` and `{device_id}` is
mandatory — there are no cross-tenant routes.

```
GET  /api/tenants/{tenant_id}/devices
GET  /api/tenants/{tenant_id}/devices/{device_id}/current
GET  /api/tenants/{tenant_id}/devices/{device_id}/status
GET  /api/tenants/{tenant_id}/devices/{device_id}/history?days=N
GET  /api/tenants/{tenant_id}/devices/{device_id}/errors
GET  /api/tenants/{tenant_id}/devices/{device_id}/settings
POST /api/tenants/{tenant_id}/devices/{device_id}/settings
POST /api/tenants/{tenant_id}/devices/{device_id}/mode

WS   /ws/{tenant_id}
```

---

## Database Schema (key points)

All tables use `(tenant_id, device_id)` as the composite owner key. The `devices`
table primary key is `(tenant_id, device_id)`; every other table has a composite
foreign key referencing it.

**Auto-registration**: `db.EnsureDevice(ctx, tenantID, deviceID)` runs an
`INSERT ... ON CONFLICT DO NOTHING` before every sensor/cycle insert so devices
self-register on first MQTT contact. This is intentional — do not add a guard that
requires an explicit registration step.

---

## Running Locally

### With Docker Compose (recommended)

```bash
# Start broker + database
docker compose up -d mosquitto timescaledb

# Run backend with live reload (requires go installed)
go run ./cmd/server

# Or build and run the full stack
docker compose up --build
```

### Without Docker

Requires a running PostgreSQL instance and MQTT broker. Override env vars:

```bash
export DATABASE_URL="postgres://climate:climate@localhost:5432/climate?sslmode=disable"
export MQTT_URL="tcp://localhost:1883"
export LISTEN_ADDR=":8080"
go run ./cmd/server
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
| `POSTGRES_USER` | `climate` | Used by the timescaledb container only |
| `POSTGRES_PASSWORD` | `climate` | Used by the timescaledb container only |
| `POSTGRES_DB` | `climate` | Used by the timescaledb container only |
| `CLOUDFLARE_TUNNEL_TOKEN` | *(unset)* | Required only if the tunnel service is uncommented |

For local development without Docker the hostnames `timescaledb` and `mosquitto`
must be replaced with `localhost` (or the actual host).

---

## Development Conventions

### Error handling

- All errors are returned up the call stack. Never swallow an error silently.
- Errors from fire-and-forget goroutines (MQTT callbacks, WebSocket pumps) are
  logged with `log.Printf` and do not crash the process.
- The pattern `if err != nil { return fmt.Errorf("pkg: context: %w", err) }` is
  used throughout the `db` package for wrappable error chains.

### Logging

- `log.Printf` only. No third-party logger at this stage.
- Format: `"package: action noun/key: %v"`, e.g. `"db: insert reading t1/dev1: %v"`.
- Log at the call site, not deep inside helpers, to keep context visible.

### Tenant isolation

- Every in-memory manager key is `tenantID + "/" + deviceID` (via the private
  `tenantKey` helper in each package). Never use `deviceID` alone as a map key.
- Every DB function signature is `(ctx, tenantID, deviceID, ...)`. There are no
  queries that operate across tenants.
- HTTP handlers extract both `{tenant_id}` and `{device_id}` from the path; the
  `pathIDs` helper in `api/handlers.go` enforces this consistently.
- The `handleListDevices` endpoint filters `AllDeviceKeys()` by the request's
  `tenant_id` so one tenant cannot discover another tenant's devices.

### Adding a new MQTT subtopic

1. Add a handler field to `mqtt.Handlers` in `mqtt/client.go`.
2. Add the `case` to `dispatch`.
3. Wire the callback in `cmd/server/main.go`.

### Adding a new API endpoint

1. Register the route in `api.New()` under the
   `/api/tenants/{tenant_id}/devices/{device_id}/` prefix.
2. Use `pathIDs(r)` to extract `tenantID, deviceID`.
3. Pass both to every manager and DB call.

---

## What NOT to Change

**Tenant isolation logic** — the `tenantKey(tenantID, deviceID)` composite key
pattern in every manager, and the `(tenant_id, device_id)` compound primary/foreign
keys in the database, must not be simplified to a single-column key. Multiple tenants
can have devices with the same `device_id`; collapsing this would silently mix data.

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
