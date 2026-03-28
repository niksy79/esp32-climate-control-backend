# QA Test Framework Plan — climate-backend

## 1) Scope and source of truth

This plan is based on the current backend API, database schema, MQTT/WebSocket behavior, and frontend web app flows documented in:
- `CLAUDE.md`
- `CLAUDE-api.md`
- `CLAUDE-web.md`

It is intended for **LLM-agent-driven QA** with deterministic execution order, stable test data, explicit assertions, and structured bug reporting.

---

## 2) System under test

### Backend
- Auth: register, login, refresh
- Tenant-scoped device APIs
- Admin-only settings, mode, alert rules, device-type assignment
- Public device-type endpoints
- WebSocket live stream per tenant
- MQTT-driven device ingestion and command/config publishing

### Frontend
- Routes: `/login`, `/`, `/device/:id`
- Dashboard
- Device detail tabs:
  - История
  - Настройки
  - Алерти
  - Режими
  - Диагностика

### Out of scope for now
These should be tracked as **known gaps**, not treated as regressions unless explicitly implemented:
- No payment flow
- No search flow
- No filter flow
- No tenant switching UI
- No user/profile page
- No device registration UI
- No frontend device-type integration
- Settings tab missing light and operational-mode fields
- Relay states are display-only
- History range selector mentioned as TODO, current behavior is last 24h / hardcoded 1 day
- UI is Bulgarian only

---

## 3) QA objectives

The agent must verify:

1. **Authentication correctness**
2. **Authorization and tenant isolation**
3. **Read flows for all device data**
4. **Admin write flows**
5. **Alert rule CRUD**
6. **Device type endpoints**
7. **MQTT ingestion behavior**
8. **WebSocket live updates**
9. **Frontend rendering and UX state**
10. **Error handling and validation behavior**
11. **Regression stability after fixes**

---

## 4) Required test environment

## 4.1 Separate environment
Use a dedicated QA stack, never production.

Recommended services:
- `climate-backend`
- `timescaledb` / PostgreSQL
- `mosquitto`
- frontend app
- optional mail catcher for alert email verification

Recommended environment naming:
- tenant A: `qa-tenant-a`
- tenant B: `qa-tenant-b`

## 4.2 Environment variables
Prepare QA-specific values:
- `DATABASE_URL`
- `MQTT_URL`
- `LISTEN_ADDR`
- `JWT_SECRET`
- `SMTP_HOST`
- `SMTP_PORT`
- `SMTP_USER`
- `SMTP_PASS`
- `SMTP_FROM`

Notes:
- QA must not reuse production secrets.
- SMTP can point to a safe sandbox or mail catcher.
- Use localhost/dev service names consistently depending on Docker or local run mode.

## 4.3 Database policy
- Start each full run from a clean DB snapshot or recreate the schema.
- Re-seed deterministic users, devices, settings, alert rules, and history.
- Keep IDs stable so agent assertions are repeatable.
- Record the exact seed version used for each run.

## 4.4 MQTT policy
The agent must be able to:
- publish inbound device messages to MQTT
- subscribe to outbound config/command topics
- verify payload content and topic correctness

## 4.5 WebSocket policy
The agent must be able to:
- open a WebSocket connection with JWT
- subscribe as tenant A and tenant B separately
- verify delivery only to the correct tenant

---

## 5) Seed data specification

## 5.1 Users
Create these users before test execution:

| Tenant | Email | Password | Role | Purpose |
|---|---|---|---|---|
| qa-tenant-a | admin@qa-a.local | secret123 | admin | full admin flows |
| qa-tenant-a | user@qa-a.local | secret123 | user | read-only and permission tests |
| qa-tenant-b | admin@qa-b.local | secret123 | admin | tenant isolation |
| qa-tenant-b | user@qa-b.local | secret123 | user | tenant isolation |

## 5.2 Devices
Create at least:

| Tenant | Device ID | Purpose |
|---|---|---|
| qa-tenant-a | device-a1 | primary device for all happy paths |
| qa-tenant-a | device-a2 | ordering / last_seen / multi-device checks |
| qa-tenant-b | device-b1 | cross-tenant isolation checks |

## 5.3 Device types
Ensure at least:
- `climate_controller` (already seeded by system)
- one extra custom type for CRUD verification, e.g. `smart_scale`

## 5.4 Seed readings and diagnostics
For `qa-tenant-a / device-a1`, seed:
- multiple readings across recent timestamps
- at least 1 compressor cycle
- at least 1 system status row
- at least 1 active error
- device settings row
- at least 1 alert rule

## 5.5 Live-stream test data
Prepare MQTT payloads for:
- sensor
- status
- relays
- settings
- errors
- compressor
- identity
- logs

Also prepare invalid payloads and unknown subtopics for negative tests.

---

## 6) User flows to be tested

## 6.1 Authentication flows
1. Register new user
2. Login with valid credentials
3. Login with invalid password
4. Login with wrong tenant
5. Refresh token with valid refresh token
6. Refresh token with invalid/expired token
7. Access protected route without token
8. Access protected route with malformed token
9. WebSocket connect with valid token
10. WebSocket connect with invalid token

## 6.2 Dashboard / device overview flows
1. Load dashboard after login
2. List devices for current tenant only
3. Show device cards with latest data
4. Show live indicator state
5. Auto-refresh / polling continuity
6. Relative timestamp rendering

## 6.3 Device detail read flows
1. Open DeviceDetail
2. Load History tab
3. Load Settings tab
4. Load Alerts tab
5. Load Modes tab
6. Load Diagnostics tab
7. Fetch current reading
8. Fetch status
9. Fetch settings
10. Fetch errors
11. Fetch compressor cycles

## 6.4 Admin write flows
1. Update settings with partial payload
2. Update settings temp/humidity and verify MQTT config publish
3. Update settings fan/light payload where supported by API
4. Change active mode
5. Assign device type
6. Create alert rule
7. Edit alert rule
8. Delete alert rule
9. Create device type
10. Update device type

## 6.5 Permission flows
1. User role can read tenant resources
2. User role cannot call admin endpoints
3. Admin of tenant A cannot access tenant B resources
4. Token tenant mismatch in path is rejected
5. Public device type endpoints accessible without auth
6. Protected device type writes require admin JWT

## 6.6 MQTT ingestion flows
1. First sensor publish auto-registers device
2. Sensor publish updates current reading
3. Sensor publish persists history row
4. Status publish updates status view
5. Errors publish updates active errors
6. Compressor publish persists cycle data
7. Identity publish updates device metadata if applicable
8. Unknown subtopic is discarded without corruption

## 6.7 WebSocket flows
1. Tenant A receives sensor event for tenant A
2. Tenant A does not receive tenant B event
3. Message format is correct
4. Backend timestamp is UTC-generated
5. Reconnect behavior after disconnect

## 6.8 Error and edge-case flows
1. Missing required fields
2. Invalid JSON body
3. Invalid path params
4. Invalid `days` values
5. Non-existent device
6. Non-existent alert rule
7. Duplicate registration in same tenant
8. Same email in different tenant
9. Invalid alert rule operator / metric if validation exists
10. MQTT publish failure on settings still returns 204 when documented
11. Hardcoded dashboard threshold behavior does not falsely imply alert rule linkage

---

## 7) Test execution order for the agent

## Phase 1 — Smoke
- Backend starts
- Frontend loads
- Register or login works
- Dashboard opens
- Devices endpoint responds
- Device detail opens
- MQTT sensor publish works
- WebSocket receives a live event

Exit rule: stop if any smoke test fails.

## Phase 2 — Authentication and permissions
- register / login / refresh
- role restrictions
- tenant isolation
- public vs protected endpoints

## Phase 3 — Core read flows
- dashboard
- current/status/history/settings/errors/compressor
- frontend tab rendering

## Phase 4 — Core write flows
- settings update
- mode change
- alert rules CRUD
- device type creation/update/assignment

## Phase 5 — MQTT and WebSocket integration
- device auto-registration
- message fan-out
- outbound MQTT verification
- tenant-isolated live events

## Phase 6 — Negative tests
- invalid JSON
- invalid token
- wrong tenant
- bad params
- missing device/rule/type cases

## Phase 7 — Regression
- rerun all impacted smoke + targeted functional tests after each fix

---

## 8) Acceptance criteria by function

## 8.1 Register
**Acceptance criteria**
- Valid payload creates user and returns token pair.
- Role is stored as requested when supported by API flow.
- Duplicate email in same tenant is rejected.
- Same email in different tenant is allowed.

## 8.2 Login
**Acceptance criteria**
- Valid tenant/email/password returns token pair.
- Wrong password is rejected.
- Wrong tenant is rejected.
- Returned access token grants access only within token tenant.

## 8.3 Refresh
**Acceptance criteria**
- Valid refresh token returns new token pair.
- Invalid or expired refresh token is rejected.
- Newly issued access token works on protected endpoints.

## 8.4 Protected reads
**Acceptance criteria**
- Authenticated user can read allowed tenant resources.
- Response structure matches documented fields.
- Data belongs only to requested tenant and device.
- Unauthorized access is rejected.

## 8.5 Dashboard
**Acceptance criteria**
- Dashboard loads after login.
- Only tenant devices are shown.
- Device card shows latest available temperature, humidity, state, and timestamp.
- Live status indicator changes based on WebSocket connectivity/state.

## 8.6 Device history
**Acceptance criteria**
- History tab loads without frontend crash.
- Backend returns max allowed range behavior for `days`.
- Chart renders seeded data for target device.
- Cross-device or cross-tenant data leakage does not occur.

## 8.7 Settings read/write
**Acceptance criteria**
- Settings endpoint returns current stored settings.
- Admin can submit partial settings payload.
- DB values change accordingly.
- If temp or humidity fields are included, backend publishes config MQTT message.
- HTTP 204 is returned even when MQTT publish fails, as currently documented.
- Non-admin user is rejected.

## 8.8 Mode switch
**Acceptance criteria**
- Admin can switch mode successfully.
- State is persisted in DB/control state as expected.
- MQTT command publish is attempted on documented topic.
- Non-admin user is rejected.

## 8.9 Alert rules CRUD
**Acceptance criteria**
- Admin can list, create, update, and delete rules.
- Created rule is visible in subsequent GET.
- `cooldown_minutes` defaults to 15 when omitted or <= 0.
- Non-admin user is rejected.

## 8.10 Device types
**Acceptance criteria**
- Public GET list and detail work without auth.
- Admin can create device type with immutable `id`.
- PUT updates display_name / description only.
- Device can be assigned a valid device_type_id.
- Invalid or non-existent device_type_id is rejected if backend validation exists.

## 8.11 MQTT ingestion
**Acceptance criteria**
- First valid sensor event auto-creates device if missing.
- Sensor event updates latest reading and persists history.
- Other documented subtopics update their corresponding stores/views.
- Unknown subtopic does not break ingestion pipeline.

## 8.12 WebSocket
**Acceptance criteria**
- Valid JWT can open tenant-scoped stream.
- Sensor publish triggers correctly formatted live message.
- Timestamp is backend UTC timestamp, not device payload time.
- Only same-tenant subscribers receive event.

---

## 9) Functional test matrix

| Functional area | Scenario | Expected result |
|---|---|---|
| Register | New admin in tenant A | 201, token pair returned, user created |
| Register | Duplicate email in same tenant | Request rejected |
| Register | Same email in different tenant | Success |
| Login | Valid credentials | Token pair returned |
| Login | Wrong password | Rejected |
| Login | Wrong tenant | Rejected |
| Refresh | Valid refresh token | New token pair |
| Refresh | Invalid token | Rejected |
| Devices list | Admin fetches tenant A devices | Only tenant A devices returned |
| Devices list | User fetches tenant A devices | Success |
| Devices list | Tenant A token to tenant B path | Rejected |
| Current reading | Existing device with seeded data | Latest reading returned |
| Status | Existing device | Status payload returned |
| History | `days=1` | Data returned for device |
| History | `days>31` | Backend cap behavior enforced |
| Compressor cycles | default days | Data returned |
| Errors | active errors exist | Active errors returned |
| Settings GET | existing settings | Full settings payload returned |
| Settings POST | admin partial temp/humidity update | 204, DB updated, MQTT config published |
| Settings POST | admin fan/light update | 204, DB updated where supported |
| Settings POST | normal user tries update | Rejected |
| Mode POST | admin switches mode | Success, persisted, MQTT command attempted |
| Mode POST | normal user tries | Rejected |
| Alert rules GET | admin lists rules | Rule list returned |
| Alert rules POST | valid create | Rule created |
| Alert rules PUT | valid update | Rule updated |
| Alert rules DELETE | valid delete | Rule removed |
| Alert rules POST | cooldown omitted | Stored as 15 |
| Device types GET list | public call | Success |
| Device types GET detail | public call | Success |
| Device types POST | admin creates `smart_scale` | Created |
| Device types PUT | admin updates display text | Updated |
| Device types PUT | tries to change `id` via body | Path id remains source of truth |
| Assign device type | valid type assignment | Device updated |
| MQTT sensor | publish sensor for new device | Device auto-registered |
| MQTT sensor | publish sensor for existing device | latest current + history updated |
| MQTT errors | publish active errors | errors endpoint reflects data |
| MQTT compressor | publish compressor cycle | compressor endpoint reflects data |
| MQTT unknown subtopic | publish unknown topic | ignored, no corruption |
| WebSocket | valid tenant A subscriber + tenant A sensor | message received |
| WebSocket | tenant A subscriber + tenant B sensor | no message |
| Frontend login | valid login via UI | redirected to dashboard |
| Frontend dashboard | data available | cards render correctly |
| Frontend device detail | open all tabs | no crash, expected sections visible |
| Frontend alerts | create/edit/delete via UI | CRUD reflected in UI and API |
| Frontend modes | confirm switch mode | state change reflected |
| Frontend diagnostics | seeded errors/cycles | charts/lists render |
| Invalid JSON | malformed POST body | rejected with error |
| Missing token | protected endpoint | unauthorized |
| Invalid token | protected endpoint | unauthorized |
| Missing device | GET/POST unknown device | appropriate error |
| Missing alert rule | PUT/DELETE unknown rule | appropriate error |

---

## 10) Negative and edge-case checklist

## Auth / security
- empty email/password
- invalid JWT format
- expired refresh token
- token from tenant A on tenant B path
- no auth header
- malformed Bearer header

## Input validation
- malformed JSON bodies
- string instead of numeric threshold
- invalid operator value
- negative cooldown
- invalid `days` values: 0, -1, very large, text
- missing required fields in register/login/alert rules/device type creation

## Resource handling
- unknown device
- unknown device type
- unknown alert rule
- device exists without recent in-memory state
- empty history / empty cycles / empty errors

## Integration behavior
- MQTT unavailable during settings write
- WebSocket disconnect/reconnect
- duplicate inbound MQTT messages
- unknown MQTT subtopic
- stale frontend token triggering refresh retry

---

## 11) Agent operating rules

The LLM QA agent must follow these rules:

1. Never test randomly.
2. Execute tests in the order defined in section 7.
3. Before each test, record:
   - test id
   - scenario
   - preconditions
   - exact request or UI steps
   - expected result
4. After each test, record:
   - actual result
   - pass/fail
   - evidence
5. Evidence should include one or more of:
   - API request/response
   - screenshot
   - console log
   - network trace
   - DB check
   - MQTT topic/payload capture
   - WebSocket message capture
6. Any failure must produce a structured bug report.
7. After a fix, rerun:
   - the failed test
   - the directly related flow
   - a minimal smoke regression

---

## 12) Bug report template for the agent

```md
### Bug ID
AUTO-GENERATED

### Title
Short factual summary

### Severity
Critical / Major / Minor

### Area
Auth / Dashboard / Settings / Alerts / Modes / MQTT / WebSocket / Device Types / Other

### Preconditions
State before execution

### Steps to reproduce
1.
2.
3.

### Expected result
...

### Actual result
...

### Evidence
- screenshot:
- response body:
- logs:
- mqtt/ws capture:

### Suspected scope
What feature/module is likely affected

### Regression tests required
List of tests to rerun after fix
```

---

## 13) Practical agent prompt skeleton

Use this as the controlling instruction for the QA agent:

```md
You are a QA execution agent for the climate-backend project.

Rules:
- Follow the provided QA plan strictly.
- Do not invent product behavior beyond the documented source of truth.
- Treat known gaps/TODOs as non-bugs unless the implemented UI/API contradicts the docs.
- Run tests in this order: smoke -> auth -> permissions -> read flows -> write flows -> MQTT/WebSocket -> negative tests -> regression.
- For every test, produce: preconditions, steps, expected, actual, status, evidence.
- For every failure, produce a structured bug report.
- Verify not only UI behavior, but also API response, data persistence, and MQTT/WebSocket side effects where relevant.
- Explicitly verify tenant isolation and role restrictions.
- Reuse deterministic seed data and test accounts only.
```

---

## 14) Known implementation notes the agent must respect

1. There is **no payment functionality** in the current project.
2. There is **no search/filter workflow** documented in the current project.
3. Some backend data is served from **in-memory managers**, not only the database.
4. `POST /settings` may still return **204 even if MQTT publish fails**.
5. WebSocket event timestamps are generated by the **backend in UTC**, not taken from device payloads.
6. Device auto-registration happens on first publish.
7. Device types exist in backend, but are **not integrated in frontend yet**.
8. Settings UI currently does **not expose all backend settings fields**.

---

## 15) Recommended file naming for follow-up artifacts

- `qa-test-framework-plan.md` — this file
- `qa-test-cases.md` — executable detailed test cases
- `qa-bug-log.md` — collected failures
- `qa-regression-checklist.md` — rerun pack after fixes

---

## 16) Final instruction

Start with:
1. environment verification
2. seed verification
3. smoke suite
4. then continue sequentially without skipping evidence collection
