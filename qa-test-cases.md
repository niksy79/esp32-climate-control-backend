# QA Test Cases — climate-backend

## 1. Purpose

This file contains **executable QA test cases** for LLM-agent-driven validation of the `climate-backend` system and its web app.

It assumes:
- dedicated QA environment
- deterministic seed data
- MQTT broker available
- WebSocket access available
- frontend and backend both running

It explicitly excludes:
- payments
- search
- filters

because these are not part of the product scope.

---

## 2. Test accounts

| ID | Tenant | Email | Password | Role |
|---|---|---|---|---|
| U1 | qa-tenant-a | admin@qa-a.local | secret123 | admin |
| U2 | qa-tenant-a | user@qa-a.local | secret123 | user |
| U3 | qa-tenant-b | admin@qa-b.local | secret123 | admin |
| U4 | qa-tenant-b | user@qa-b.local | secret123 | user |

---

## 3. Seed devices

| ID | Tenant | Device ID | Notes |
|---|---|---|---|
| D1 | qa-tenant-a | device-a1 | primary happy-path device |
| D2 | qa-tenant-a | device-a2 | secondary device |
| D3 | qa-tenant-b | device-b1 | cross-tenant isolation checks |

---

## 4. Execution rules for the agent

For every test case, the agent must record:
- test case id
- preconditions
- exact steps
- expected result
- actual result
- pass/fail
- evidence

Evidence should include at least one of:
- screenshot
- API response
- console/network log
- DB verification
- MQTT capture
- WebSocket capture

---

## 5. Smoke suite

### TC-SMOKE-01 — Backend health by auth login
**Priority:** Critical  
**Preconditions:** QA stack is running.

**Steps**
1. Send login request with U1 credentials.
2. Capture response.

**Expected**
- Response is successful.
- Access token and refresh token are returned.

---

### TC-SMOKE-02 — Frontend login page loads
**Priority:** Critical  
**Preconditions:** Frontend app is running.

**Steps**
1. Open `/login`.
2. Wait for UI render.

**Expected**
- Login form is visible.
- No fatal frontend error appears.

---

### TC-SMOKE-03 — Login via UI redirects to dashboard
**Priority:** Critical  
**Preconditions:** U1 exists.

**Steps**
1. Open `/login`.
2. Enter U1 credentials.
3. Submit form.

**Expected**
- Login succeeds.
- User is redirected to `/`.
- Auth state is stored and protected area becomes accessible.

---

### TC-SMOKE-04 — Dashboard loads device list
**Priority:** Critical  
**Preconditions:** U1 is logged in, D1 and D2 exist.

**Steps**
1. Open dashboard.
2. Wait for data fetch to complete.

**Expected**
- Device cards are rendered.
- At least D1 and D2 are visible.
- No device from `qa-tenant-b` is visible.

---

### TC-SMOKE-05 — Device detail opens
**Priority:** Critical  
**Preconditions:** U1 logged in, D1 exists.

**Steps**
1. Open `/device/device-a1`.

**Expected**
- Device detail page loads.
- Tabs are visible.
- No fatal UI crash occurs.

---

### TC-SMOKE-06 — MQTT sensor event reaches WebSocket subscriber
**Priority:** Critical  
**Preconditions:** U1 logged in; valid JWT available.

**Steps**
1. Open WebSocket connection for `qa-tenant-a`.
2. Publish valid sensor MQTT message for `qa-tenant-a/device-a1`.
3. Capture WebSocket event.

**Expected**
- WebSocket receives one sensor message for device-a1.
- Payload contains `type`, `device_id`, `temperature`, `humidity`, `timestamp`.

---

## 6. Authentication test cases

### TC-AUTH-01 — Register new admin user
**Priority:** High

**Steps**
1. POST `/api/auth/register` with:
   - tenant_id: `qa-tenant-a`
   - email: unique new email
   - password: valid password
   - role: `admin`

**Expected**
- HTTP 201.
- Token pair returned.
- New user can log in.

---

### TC-AUTH-02 — Register duplicate email in same tenant
**Priority:** High  
**Preconditions:** U1 already exists.

**Steps**
1. POST `/api/auth/register` with same tenant and email as U1.

**Expected**
- Request is rejected.
- No duplicate user is created.

---

### TC-AUTH-03 — Register same email in different tenant
**Priority:** Medium  
**Preconditions:** U1 exists.

**Steps**
1. Register `admin@qa-a.local` under a different tenant than `qa-tenant-a`.

**Expected**
- Request succeeds if tenant/email uniqueness is respected per tenant.
- User creation in different tenant is allowed.

---

### TC-AUTH-04 — Login with valid credentials
**Priority:** Critical

**Steps**
1. POST `/api/auth/login` with U1 credentials.

**Expected**
- Success response.
- Access token and refresh token returned.

---

### TC-AUTH-05 — Login with wrong password
**Priority:** Critical

**Steps**
1. POST `/api/auth/login` with valid tenant/email and invalid password.

**Expected**
- Login rejected.
- No token returned.

---

### TC-AUTH-06 — Login with wrong tenant
**Priority:** Critical

**Steps**
1. POST `/api/auth/login` with U1 email/password but tenant `qa-tenant-b`.

**Expected**
- Login rejected.

---

### TC-AUTH-07 — Refresh with valid refresh token
**Priority:** High

**Steps**
1. Obtain valid refresh token from login.
2. POST `/api/auth/refresh`.

**Expected**
- New token pair returned.
- New access token works on protected endpoints.

---

### TC-AUTH-08 — Refresh with invalid token
**Priority:** High

**Steps**
1. POST `/api/auth/refresh` with invalid or malformed token.

**Expected**
- Request rejected.

---

### TC-AUTH-09 — Protected endpoint without token
**Priority:** Critical

**Steps**
1. GET protected endpoint without Authorization header.

**Expected**
- Unauthorized response.

---

### TC-AUTH-10 — Protected endpoint with malformed token
**Priority:** Critical

**Steps**
1. GET protected endpoint with malformed Bearer token.

**Expected**
- Unauthorized response.

---

### TC-AUTH-11 — WebSocket with valid token
**Priority:** High

**Steps**
1. Open `WS /ws/qa-tenant-a?token=<valid_jwt>`.

**Expected**
- Connection succeeds.

---

### TC-AUTH-12 — WebSocket with invalid token
**Priority:** High

**Steps**
1. Open WebSocket using invalid token.

**Expected**
- Connection is rejected or closed immediately.

---

## 7. Authorization and tenant isolation test cases

### TC-PERM-01 — User role can read allowed resources
**Priority:** Critical

**Steps**
1. Log in as U2.
2. GET:
   - devices
   - current
   - status
   - history
   - errors
   - settings

**Expected**
- Read endpoints succeed for own tenant resources.

---

### TC-PERM-02 — User role cannot update settings
**Priority:** Critical

**Steps**
1. Log in as U2.
2. POST settings update for D1.

**Expected**
- Request rejected due to insufficient permissions.

---

### TC-PERM-03 — User role cannot switch mode
**Priority:** Critical

**Steps**
1. Log in as U2.
2. POST mode change for D1.

**Expected**
- Request rejected.

---

### TC-PERM-04 — User role cannot manage alert rules
**Priority:** Critical

**Steps**
1. Log in as U2.
2. Attempt create/update/delete alert rule.

**Expected**
- Write actions are rejected.

---

### TC-PERM-05 — Tenant path mismatch is rejected
**Priority:** Critical

**Steps**
1. Log in as U1 (tenant A).
2. Call `/api/tenants/qa-tenant-b/devices`.

**Expected**
- Request rejected due to tenant mismatch.

---

### TC-PERM-06 — Admin from tenant A cannot access tenant B device details
**Priority:** Critical

**Steps**
1. Log in as U1.
2. Request D3 data via tenant B path.

**Expected**
- Request rejected.
- No cross-tenant data leakage.

---

### TC-PERM-07 — Public device-type list works without auth
**Priority:** High

**Steps**
1. GET `/api/device-types` without token.

**Expected**
- Request succeeds.

---

### TC-PERM-08 — Public device-type detail works without auth
**Priority:** High

**Steps**
1. GET `/api/device-types/climate_controller` without token.

**Expected**
- Request succeeds.

---

### TC-PERM-09 — Device-type create requires admin JWT
**Priority:** High

**Steps**
1. Try POST `/api/device-types` without token.
2. Try with U2 token.
3. Try with U1 token.

**Expected**
- No token: rejected.
- User token: rejected.
- Admin token: succeeds.

---

## 8. Device read-flow test cases

### TC-READ-01 — List devices ordered by last_seen
**Priority:** High  
**Preconditions:** D1 and D2 exist with different `last_seen`.

**Steps**
1. Log in as U1.
2. GET `/api/tenants/qa-tenant-a/devices`.

**Expected**
- Devices are returned for tenant A only.
- Ordering follows documented `last_seen DESC`.

---

### TC-READ-02 — Get current reading
**Priority:** High

**Steps**
1. GET current endpoint for D1.

**Expected**
- Latest reading for D1 is returned.
- Data matches current in-memory state.

---

### TC-READ-03 — Get status
**Priority:** High

**Steps**
1. GET status endpoint for D1.

**Expected**
- Status payload includes system status and available runtime state.

---

### TC-READ-04 — Get history with valid days
**Priority:** High

**Steps**
1. GET history with `days=1` for D1.

**Expected**
- History records returned for D1.
- No records from other devices/tenants.

---

### TC-READ-05 — Get history with days above max cap
**Priority:** High

**Steps**
1. GET history with `days=999`.

**Expected**
- Backend enforces documented cap.
- Response does not exceed intended max range behavior.

---

### TC-READ-06 — Get compressor cycles
**Priority:** Medium

**Steps**
1. GET compressor cycles for D1.

**Expected**
- Seeded cycle data returned.
- Data belongs only to D1.

---

### TC-READ-07 — Get active errors
**Priority:** Medium

**Steps**
1. GET errors for D1.

**Expected**
- Active errors returned from current in-memory error state.

---

### TC-READ-08 — Get settings
**Priority:** High

**Steps**
1. GET settings for D1.

**Expected**
- Full settings payload returned.
- Values match seeded state.

---

### TC-READ-09 — Read non-existent device
**Priority:** High

**Steps**
1. GET current/status/history/settings/errors for unknown device.

**Expected**
- Appropriate error response.
- No server crash.

---

## 9. Admin write-flow test cases

### TC-WRITE-01 — Update settings with partial temperature payload
**Priority:** Critical

**Steps**
1. Log in as U1.
2. POST settings for D1 with only temp-related fields.
3. Read back settings.
4. Capture outbound MQTT on `climate/qa-tenant-a/device-a1/config`.

**Expected**
- HTTP 204.
- DB-backed settings updated.
- MQTT config message published.
- Payload contains only fields included in request.

---

### TC-WRITE-02 — Update settings with humidity payload
**Priority:** Critical

**Steps**
1. POST settings for D1 with humidity fields.
2. Capture MQTT config topic.

**Expected**
- HTTP 204.
- Settings updated.
- MQTT config publish occurs.

---

### TC-WRITE-03 — Update settings with fan/light fields
**Priority:** High

**Steps**
1. POST settings with fan/light fields supported by API.
2. Read back settings.

**Expected**
- HTTP 204.
- Persisted values reflect supported fields.

---

### TC-WRITE-04 — MQTT publish failure still returns 204 for settings
**Priority:** High  
**Preconditions:** Controlled test setup able to simulate MQTT publish failure.

**Steps**
1. Temporarily force MQTT publish failure.
2. POST settings update with temp/humidity fields.

**Expected**
- HTTP 204 still returned, per documented behavior.
- Failure is observable in logs/evidence if available.

---

### TC-WRITE-05 — Switch active mode as admin
**Priority:** Critical

**Steps**
1. POST mode change for D1 as U1.
2. Read resulting device state if exposed.
3. Capture outbound mode-related MQTT topic if available.

**Expected**
- Request succeeds.
- Mode change is persisted/applied as documented.
- MQTT command publish is attempted.

---

### TC-WRITE-06 — Create alert rule
**Priority:** Critical

**Steps**
1. POST valid alert rule for D1.
2. GET alert rules list.

**Expected**
- Rule is created.
- Rule appears in subsequent list.

---

### TC-WRITE-07 — Create alert rule with omitted cooldown
**Priority:** High

**Steps**
1. POST alert rule without `cooldown_minutes`.
2. Read back created rule.

**Expected**
- Rule created.
- `cooldown_minutes` defaults to 15.

---

### TC-WRITE-08 — Create alert rule with cooldown <= 0
**Priority:** High

**Steps**
1. POST alert rule with `cooldown_minutes = 0`.
2. Read back created rule.

**Expected**
- Rule created.
- `cooldown_minutes` stored as 15.

---

### TC-WRITE-09 — Update alert rule
**Priority:** High

**Steps**
1. Create or use existing rule.
2. PUT updated values.
3. GET rules list.

**Expected**
- Changes are persisted.
- Updated values visible in list/readback.

---

### TC-WRITE-10 — Delete alert rule
**Priority:** High

**Steps**
1. Delete existing rule.
2. GET rules list again.

**Expected**
- Rule no longer appears.

---

### TC-WRITE-11 — Update non-existent alert rule
**Priority:** High

**Steps**
1. PUT unknown rule id.

**Expected**
- Appropriate error response.

---

### TC-WRITE-12 — Delete non-existent alert rule
**Priority:** High

**Steps**
1. DELETE unknown rule id.

**Expected**
- Appropriate error response.

---

### TC-WRITE-13 — Create device type as admin
**Priority:** High

**Steps**
1. POST `/api/device-types` with:
   - id: `smart_scale`
   - display_name
   - description

**Expected**
- Device type created successfully.

---

### TC-WRITE-14 — Update device type display fields
**Priority:** High

**Steps**
1. PUT `/api/device-types/smart_scale` with changed display_name/description.

**Expected**
- Update succeeds.
- New display fields are visible on GET detail.

---

### TC-WRITE-15 — Device type id is immutable
**Priority:** High

**Steps**
1. PUT existing device type with different `id` in body.

**Expected**
- Path `type_id` remains authoritative.
- Existing resource is updated without id mutation.

---

### TC-WRITE-16 — Assign device type to device
**Priority:** High

**Steps**
1. POST device type assignment for D1 with `climate_controller` or `smart_scale`.

**Expected**
- Assignment succeeds.
- Device reflects assigned `device_type_id` where observable.

---

## 10. MQTT ingestion test cases

### TC-MQTT-01 — First sensor publish auto-registers unknown device
**Priority:** Critical

**Steps**
1. Publish valid sensor message for new device `device-a-new`.
2. Query devices list.

**Expected**
- New device appears automatically.
- No manual registration required.

---

### TC-MQTT-02 — Sensor publish updates current reading
**Priority:** Critical

**Steps**
1. Publish sensor payload for D1.
2. GET current endpoint.

**Expected**
- Current endpoint reflects latest published values.

---

### TC-MQTT-03 — Sensor publish persists history row
**Priority:** Critical

**Steps**
1. Publish sensor payload for D1.
2. GET history.

**Expected**
- New reading is present in history for D1.

---

### TC-MQTT-04 — Sensor broadcast timestamp uses backend UTC time
**Priority:** High

**Steps**
1. Publish sensor payload with arbitrary device-local timestamp if payload supports it.
2. Capture WebSocket message.

**Expected**
- Broadcast timestamp is backend UTC time.
- Device payload time is not trusted for broadcast timestamp.

---

### TC-MQTT-05 — Status publish updates status endpoint
**Priority:** High

**Steps**
1. Publish valid status message for D1.
2. GET status endpoint.

**Expected**
- Status endpoint reflects latest published status data.

---

### TC-MQTT-06 — Errors publish updates errors endpoint
**Priority:** High

**Steps**
1. Publish errors payload for D1.
2. GET errors endpoint.

**Expected**
- Active errors reflect current payload.

---

### TC-MQTT-07 — Compressor publish updates compressor cycles
**Priority:** Medium

**Steps**
1. Publish compressor payload for D1.
2. GET compressor cycles.

**Expected**
- Cycle data appears in endpoint response.

---

### TC-MQTT-08 — Identity publish is processed without breaking device state
**Priority:** Medium

**Steps**
1. Publish identity payload for D1.
2. Re-check device listing/status.

**Expected**
- No ingestion failure occurs.
- Observable device metadata updates if implemented.

---

### TC-MQTT-09 — Unknown subtopic is ignored
**Priority:** High

**Steps**
1. Publish payload to unknown subtopic under valid tenant/device topic.
2. Check system stability and normal endpoints.

**Expected**
- Message is silently discarded.
- No corruption or crash occurs.

---

### TC-MQTT-10 — Invalid payload does not crash system
**Priority:** High

**Steps**
1. Publish malformed payload to known subtopic.
2. Observe logs and normal API behavior.

**Expected**
- System remains available.
- Invalid message does not corrupt later reads.

---

## 11. WebSocket test cases

### TC-WS-01 — Tenant A subscriber receives tenant A sensor event
**Priority:** Critical

**Steps**
1. Connect WebSocket with valid tenant A JWT.
2. Publish sensor message for tenant A device.

**Expected**
- Exactly one matching sensor event is received.

---

### TC-WS-02 — Tenant A subscriber does not receive tenant B event
**Priority:** Critical

**Steps**
1. Keep tenant A WebSocket open.
2. Publish sensor message for tenant B device.

**Expected**
- No tenant B event is delivered to tenant A subscriber.

---

### TC-WS-03 — WebSocket message format is correct
**Priority:** High

**Steps**
1. Trigger sensor event.
2. Validate received message structure.

**Expected**
- Message contains:
  - `type`
  - `device_id`
  - `temperature`
  - `humidity`
  - `timestamp`

---

### TC-WS-04 — Reconnect after disconnect
**Priority:** Medium

**Steps**
1. Open WebSocket.
2. Disconnect it.
3. Reconnect.
4. Publish sensor event.

**Expected**
- Reconnected session receives new live event normally.

---

## 12. Frontend test cases

### TC-FE-01 — Login page visible to public users only
**Priority:** Medium

**Steps**
1. Open `/login` while logged out.
2. Log in.
3. Try to revisit `/login`.

**Expected**
- Logged-out user can access login page.
- Logged-in user is redirected away due to PublicOnly guard.

---

### TC-FE-02 — Dashboard route requires auth
**Priority:** Critical

**Steps**
1. Open `/` while logged out.

**Expected**
- Redirect to login or protected-route handling occurs.

---

### TC-FE-03 — Device detail route requires auth
**Priority:** Critical

**Steps**
1. Open `/device/device-a1` while logged out.

**Expected**
- Redirect to login or equivalent auth handling occurs.

---

### TC-FE-04 — Dashboard renders device cards
**Priority:** High

**Steps**
1. Log in as U1.
2. Open dashboard.

**Expected**
- Device cards show temperature, humidity, relay/status badges, and timestamp where data exists.

---

### TC-FE-05 — Dashboard live indicator visible
**Priority:** Medium

**Steps**
1. Open dashboard with live backend connectivity.

**Expected**
- WebSocket live indicator is present and reflects connection state.

---

### TC-FE-06 — Device detail tabs are accessible
**Priority:** High

**Steps**
1. Open D1 detail page.
2. Click each tab:
   - История
   - Настройки
   - Алерти
   - Режими
   - Диагностика

**Expected**
- Each tab opens successfully.
- No blank crash state.

---

### TC-FE-07 — History tab renders chart
**Priority:** Medium

**Steps**
1. Open History tab for D1 with seeded data.

**Expected**
- Chart renders.
- Last 24h / 1-day behavior works as currently implemented.

---

### TC-FE-08 — Settings tab loads current values
**Priority:** High

**Steps**
1. Open Settings tab for D1.

**Expected**
- Existing backend settings are displayed in form fields that are implemented in UI.

---

### TC-FE-09 — Settings update via UI
**Priority:** High

**Steps**
1. Change a supported setting in UI.
2. Submit.
3. Reload tab or refetch settings.

**Expected**
- Update succeeds.
- Changed value persists.

---

### TC-FE-10 — Alerts tab CRUD via UI
**Priority:** High

**Steps**
1. Open Alerts tab.
2. Create rule.
3. Edit rule.
4. Delete rule.

**Expected**
- UI CRUD works end-to-end.
- Results match backend state.

---

### TC-FE-11 — Modes tab confirm dialog and mode switch
**Priority:** High

**Steps**
1. Open Modes tab.
2. Choose mode.
3. Confirm switch.

**Expected**
- Confirmation flow appears.
- Mode switch succeeds and UI reflects resulting state where implemented.

---

### TC-FE-12 — Diagnostics tab renders compressor chart and errors
**Priority:** Medium

**Steps**
1. Open Diagnostics tab for D1 with seeded cycles/errors.

**Expected**
- Compressor cycles chart renders.
- Active errors list renders with severity badges.

---

### TC-FE-13 — 401 triggers refresh retry
**Priority:** High  
**Preconditions:** Controlled setup able to force expired access token with valid refresh token.

**Steps**
1. Use expired access token in browser storage.
2. Perform protected request.

**Expected**
- Axios interceptor performs refresh flow.
- Request is retried successfully if refresh token is valid.

---

### TC-FE-14 — Invalid refresh results in logout or auth failure handling
**Priority:** High  
**Preconditions:** Expired access token and invalid refresh token.

**Steps**
1. Trigger protected request.

**Expected**
- Refresh fails.
- User is returned to login or auth failure state.

---

## 13. Negative and edge-case test cases

### TC-NEG-01 — Register with missing required fields
**Priority:** High

**Steps**
1. POST register with missing email, password, or tenant_id.

**Expected**
- Request rejected.

---

### TC-NEG-02 — Login with missing required fields
**Priority:** High

**Steps**
1. POST login with incomplete payload.

**Expected**
- Request rejected.

---

### TC-NEG-03 — Create alert rule with invalid JSON
**Priority:** High

**Steps**
1. POST malformed JSON body.

**Expected**
- Request rejected cleanly.

---

### TC-NEG-04 — Create alert rule with invalid threshold type
**Priority:** High

**Steps**
1. Send threshold as string instead of number.

**Expected**
- Validation or decoding error returned.

---

### TC-NEG-05 — History with invalid days value
**Priority:** High

**Steps**
1. Call history endpoint with:
   - `days=0`
   - `days=-1`
   - `days=text`

**Expected**
- Invalid values handled safely.
- No crash.

---

### TC-NEG-06 — Create device type without required id
**Priority:** High

**Steps**
1. POST `/api/device-types` without `id`.

**Expected**
- Request rejected.

---

### TC-NEG-07 — Assign non-existent device type
**Priority:** Medium

**Steps**
1. Assign invalid `device_type_id` to D1.

**Expected**
- Request rejected if validation is enforced.
- No corrupted device state.

---

### TC-NEG-08 — Settings request with unsupported fields only
**Priority:** Medium

**Steps**
1. POST settings body with unsupported or unknown fields only.

**Expected**
- Safe handling.
- No unintended state mutation.

---

### TC-NEG-09 — Empty history / empty errors / empty cycles state
**Priority:** Medium

**Steps**
1. Use device with no seeded entries for one or more endpoints.

**Expected**
- Empty state returned safely.
- Frontend handles empty state without crash.

---

### TC-NEG-10 — Duplicate MQTT sensor messages
**Priority:** Medium

**Steps**
1. Publish same sensor payload multiple times quickly.
2. Inspect current/history behavior.

**Expected**
- System remains stable.
- Current state reflects latest message.
- History persistence behavior is consistent with implementation.

---

## 14. Minimal regression pack after every fix

Run at minimum:

1. TC-SMOKE-01
2. TC-SMOKE-03
3. TC-SMOKE-04
4. TC-SMOKE-05
5. TC-AUTH-09
6. TC-PERM-05
7. one related read test
8. one related write test
9. one MQTT/WebSocket test if integration code was touched

---

## 15. Suggested execution batches for the agent

### Batch A — Smoke
- TC-SMOKE-01 to TC-SMOKE-06

### Batch B — Auth
- TC-AUTH-01 to TC-AUTH-12

### Batch C — Permissions
- TC-PERM-01 to TC-PERM-09

### Batch D — Read flows
- TC-READ-01 to TC-READ-09

### Batch E — Write flows
- TC-WRITE-01 to TC-WRITE-16

### Batch F — MQTT / WS
- TC-MQTT-01 to TC-MQTT-10
- TC-WS-01 to TC-WS-04

### Batch G — Frontend
- TC-FE-01 to TC-FE-14

### Batch H — Negative
- TC-NEG-01 to TC-NEG-10

---

## 16. Execution note

The agent should not mark these as bugs:
- lack of payments
- lack of search
- lack of filters
- lack of tenant switching UI
- missing device type integration in frontend
- settings fields not yet exposed in frontend if backend supports more fields than UI currently renders

These are scope exclusions or documented gaps, not regressions.
