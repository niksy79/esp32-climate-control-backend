# QA Bug Log — climate-backend

---

### Bug ID
BUG-001

### Title
Assigning non-existent device_type_id returns HTTP 500 instead of 400/404

### Severity
Minor

### Area
Device Types / API

### Preconditions
- Admin user authenticated for tenant A
- device-a1 exists in tenant A
- `device_type_id` value `"nonexistent_type_xyz"` does not exist in `device_types` table

### Steps to reproduce
1. Login as admin (admin@qa-a.local)
2. POST `/api/tenants/{tenant_id}/devices/device-a1/type` with body:
   ```json
   {"device_type_id": "nonexistent_type_xyz"}
   ```

### Expected result
- HTTP 400 or 404 with a descriptive error message (e.g., "device type not found")
- The FK constraint violation should be caught and translated to a client-friendly error

### Actual result
- HTTP 500 with body: `internal error`
- Backend log: `api: set device type g2dugp/device-a1: db: set device type g2dugp/device-a1: ERROR: insert or update on table "devices" violates foreign key constraint "devices_device_type_id_fkey" (SQLSTATE 23503)`

### Evidence
- Request: `POST /api/tenants/g2dugp/devices/device-a1/type` with `{"device_type_id": "nonexistent_type_xyz"}`
- Response: HTTP 500, body=`internal error`, Content-Type=`text/plain; charset=utf-8`
- Server log: FK constraint violation (SQLSTATE 23503) bubbles up as unhandled internal error

### Suspected scope
`internal/api/handlers.go:handleSetDeviceType` (lines 688-707) — the handler calls `DB.SetDeviceTypeID` and catches all errors as HTTP 500. It should detect the FK violation (SQLSTATE 23503 or check existence beforehand) and return HTTP 400 with `"device type not found"`.

### Regression tests required
- TC-NEG-07 (assign non-existent device type)
- TC-WRITE-16 (assign valid device type — ensure fix doesn't break happy path)
- TC-PERM-09 (device-type create — ensure type registry still works)

---

### Bug ID
BUG-002

### Title
History endpoint returns uncapped `days` value in JSON response when input exceeds 31

### Severity
Minor

### Area
API / History

### Preconditions
- Admin user authenticated
- device-a1 exists with history data

### Steps to reproduce
1. GET `/api/tenants/{tenant_id}/devices/device-a1/history?days=999`

### Expected result
- Response JSON field `days` reflects the actual capped value (31), matching the DB query behavior documented in CLAUDE-api.md: "N capped at 31"

### Actual result
- Response: `{"days": 999, "count": 11, ...}`
- The `days` field echoes the raw input (999) even though the DB functions (`GetDeviceReadings`, `GetDeviceReadingsPaired`) internally cap to 31
- Data returned is correctly limited to 31 days, but the response metadata is misleading

### Evidence
- `days=31` → `{"days": 31, "count": 11}` (correct)
- `days=32` → `{"days": 32, "count": 11}` (uncapped in response)
- `days=999` → `{"days": 999, "count": 11}` (uncapped in response)
- DB code (`db.go:378-379`, `db.go:415-416`) caps internally: `if days > 31 { days = 31 }`
- Handler (`api/handlers.go:191-245`) returns original `days` value, not the capped one

### Suspected scope
`internal/api/handlers.go:handleHistory` — the handler should cap `days` at 31 before passing to DB functions and returning in the response, or the DB functions should return the capped value.

### Regression tests required
- TC-NEG-05 (history with invalid days)
- TC-READ-04 (history with valid days)
- TC-READ-05 (history with days above cap)

---

### Bug ID
BUG-003

### Title
History, settings, errors, and compressor-cycles endpoints return HTTP 200 with defaults for non-existent devices instead of 404

### Severity
Minor

### Area
API / Device validation

### Preconditions
- Admin user authenticated for tenant A
- `device-nonexistent` does NOT exist in the `devices` table

### Steps to reproduce
1. GET `/api/tenants/{tenant_id}/devices/device-nonexistent/history?days=1`
2. GET `/api/tenants/{tenant_id}/devices/device-nonexistent/settings`
3. GET `/api/tenants/{tenant_id}/devices/device-nonexistent/errors`
4. GET `/api/tenants/{tenant_id}/devices/device-nonexistent/compressor-cycles`

### Expected result
- HTTP 404 for all endpoints when the device does not exist, consistent with `/current` and `/status` which correctly return 404

### Actual result
- `/history`: HTTP 200, `{"count": 0, "readings": [], "device_id": "device-nonexistent"}`
- `/settings`: HTTP 200, returns default settings values (target=4, offset=0, etc.)
- `/errors`: HTTP 200, `[]`
- `/compressor-cycles`: HTTP 200, `{"count": 0, "cycles": []}`

Meanwhile, `/current` returns 404 and `/status` returns 404 for the same non-existent device.

### Evidence
- `GET .../device-nonexistent/current` → HTTP 404 (correct)
- `GET .../device-nonexistent/status` → HTTP 404 (correct)
- `GET .../device-nonexistent/history?days=1` → HTTP 200, empty (inconsistent)
- `GET .../device-nonexistent/settings` → HTTP 200, defaults (inconsistent)
- `GET .../device-nonexistent/errors` → HTTP 200, empty (inconsistent)
- `GET .../device-nonexistent/compressor-cycles` → HTTP 200, empty (inconsistent)

### Suspected scope
`internal/api/handlers.go` — the handlers for history, settings, errors, and compressor-cycles do not validate device existence before querying. The `/current` and `/status` handlers check in-memory state which naturally returns not-found. A device existence check (e.g., `db.GetDevice` or equivalent) should be added to the other handlers, or they should return 404 when the DB query returns no device match.

### Regression tests required
- TC-NEG-10 (unknown device endpoints)
- TC-READ-09 (read non-existent device)
- TC-SMOKE-05 (device detail opens — ensure fix doesn't break existing devices)

---

### Bug ID
BUG-004

### Title
Fan settings POST/GET JSON tag mismatch: POST accepts `mixing_interval`/`mixing_duration` but GET returns `mixing_interval_s`/`mixing_duration_s`

### Severity
Major

### Area
Settings / API

### Preconditions
- Admin user authenticated
- device-a1 exists

### Steps to reproduce
1. POST `/api/tenants/{tenant_id}/devices/device-a1/settings` with body:
   ```json
   {"fan": {"speed": 80, "mixing_interval_s": 1800, "mixing_duration_s": 150, "mixing_enabled": true}}
   ```
2. GET `/api/tenants/{tenant_id}/devices/device-a1/settings`
3. Observe `fan.mixing_interval_s` and `fan.mixing_duration_s` in response

### Expected result
- POST with `mixing_interval_s` and `mixing_duration_s` should persist values 1800 and 150
- GET should return `{"mixing_interval_s": 1800, "mixing_duration_s": 150}`

### Actual result
- POST returns HTTP 204 (appears successful) but `mixing_interval_s` and `mixing_duration_s` are silently ignored
- GET returns `{"mixing_interval_s": 0, "mixing_duration_s": 0}` — values were never stored
- POST with `mixing_interval` / `mixing_duration` (no `_s` suffix) DOES work correctly

### Root cause
`internal/api/handlers.go:handleSaveSettings` (lines 307-312) defines an inline struct for fan with JSON tags `json:"mixing_interval"` and `json:"mixing_duration"` (no `_s` suffix). However, `models.FanSettings` uses `json:"mixing_interval_s"` and `json:"mixing_duration_s"` — and the GET endpoint returns models.FanSettings. This creates an asymmetry: the client must use different field names for POST vs what it reads from GET.

### Evidence
- POST `{"fan":{"mixing_interval_s":1800}}` → 204, but readback shows `mixing_interval_s: 0`
- POST `{"fan":{"mixing_interval":1800}}` → 204, readback shows `mixing_interval_s: 1800`
- Handler code: `handlers.go:309` — `MixingInterval uint32 \`json:"mixing_interval"\``
- Model code: `models.go:102` — `MixingInterval uint32 \`json:"mixing_interval_s"\``

### Suspected scope
`internal/api/handlers.go:handleSaveSettings` lines 307-312 — the inline struct's JSON tags should match the model's tags (`mixing_interval_s`, `mixing_duration_s`).

### Regression tests required
- TC-WRITE-03 (fan/light settings update)
- TC-WRITE-01 (temp settings — ensure no regression)
- TC-FE-09 (settings update via UI — verify frontend uses correct field names)

---

### Bug ID
BUG-005

### Title
Creating a duplicate device type returns HTTP 500 instead of 409 Conflict

### Severity
Minor

### Area
Device Types / API

### Preconditions
- Admin user authenticated
- Device type `smart_scale` already exists in `device_types` table

### Steps to reproduce
1. POST `/api/device-types` with body:
   ```json
   {"id": "smart_scale", "display_name": "Dup", "description": "Dup test"}
   ```

### Expected result
- HTTP 409 Conflict with descriptive error message (e.g., "device type already exists")

### Actual result
- HTTP 500 with body: `internal error`
- Backend log: `api: create device type smart_scale: db: create device type smart_scale: ERROR: duplicate key value violates unique constraint "device_types_pkey" (SQLSTATE 23505)`

### Evidence
- Request: `POST /api/device-types` with `{"id":"smart_scale",...}`
- Response: HTTP 500, body=`internal error`
- Server log: PK violation (SQLSTATE 23505) treated as internal error

### Suspected scope
`internal/api/handlers.go:handleCreateDeviceType` (lines 637-661) — same pattern as BUG-001: all DB errors treated as 500. Should detect unique constraint violation (SQLSTATE 23505) and return 409.

### Regression tests required
- TC-WRITE-13 (create device type)
- TC-PERM-09 (device-type create with different roles)
