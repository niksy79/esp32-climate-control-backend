// Package db provides PostgreSQL access for the climate backend.
package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"climate-backend/internal/models"
)

// DB wraps a pgxpool connection pool.
type DB struct {
	pool *pgxpool.Pool
}

// New connects to PostgreSQL using the given DSN and runs schema migrations.
func New(ctx context.Context, dsn string) (*DB, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("db: open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("db: ping: %w", err)
	}
	d := &DB{pool: pool}
	if err := d.migrate(ctx); err != nil {
		return nil, fmt.Errorf("db: migrate: %w", err)
	}
	return d, nil
}

// Close releases all connections.
func (d *DB) Close() { d.pool.Close() }

// ---------------------------------------------------------------------------
// Schema migrations
// ---------------------------------------------------------------------------

func (d *DB) migrate(ctx context.Context) error {
	_, err := d.pool.Exec(ctx, schema)
	return err
}

const schema = `
CREATE TABLE IF NOT EXISTS devices (
    tenant_id   TEXT NOT NULL,
    device_id   TEXT NOT NULL,
    device_name TEXT NOT NULL DEFAULT '',
    hostname    TEXT NOT NULL DEFAULT '',
    ip_address  TEXT NOT NULL DEFAULT '',
    wifi_state  INTEGER NOT NULL DEFAULT 0,
    last_seen   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, device_id)
);

CREATE TABLE IF NOT EXISTS readings (
    id            BIGSERIAL PRIMARY KEY,
    tenant_id     TEXT        NOT NULL,
    device_id     TEXT        NOT NULL,
    temperature   REAL        NOT NULL,
    humidity      REAL        NOT NULL,
    fallback_time BOOLEAN     NOT NULL DEFAULT FALSE,
    recorded_at   TIMESTAMPTZ NOT NULL,
    FOREIGN KEY (tenant_id, device_id) REFERENCES devices(tenant_id, device_id)
);
CREATE INDEX IF NOT EXISTS readings_tenant_device_time ON readings (tenant_id, device_id, recorded_at DESC);

CREATE TABLE IF NOT EXISTS compressor_cycles (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   TEXT    NOT NULL,
    device_id   TEXT    NOT NULL,
    work_time_s INTEGER NOT NULL,
    rest_time_s INTEGER NOT NULL,
    temperature REAL    NOT NULL,
    humidity    REAL    NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (tenant_id, device_id) REFERENCES devices(tenant_id, device_id)
);

CREATE TABLE IF NOT EXISTS system_status (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       TEXT    NOT NULL,
    device_id       TEXT    NOT NULL,
    state           INTEGER NOT NULL,
    dht_ok          BOOLEAN NOT NULL,
    rtc_ok          BOOLEAN NOT NULL,
    uptime_seconds  INTEGER NOT NULL,
    restart_count   INTEGER NOT NULL,
    recorded_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (tenant_id, device_id) REFERENCES devices(tenant_id, device_id)
);

CREATE TABLE IF NOT EXISTS errors (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   TEXT    NOT NULL,
    device_id   TEXT    NOT NULL,
    error_type  INTEGER NOT NULL,
    severity    INTEGER NOT NULL,
    message     TEXT    NOT NULL,
    active      BOOLEAN NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    FOREIGN KEY (tenant_id, device_id) REFERENCES devices(tenant_id, device_id)
);

CREATE TABLE IF NOT EXISTS device_settings (
    tenant_id           TEXT NOT NULL,
    device_id           TEXT NOT NULL,
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

CREATE TABLE IF NOT EXISTS users (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     TEXT        NOT NULL,
    email         TEXT        NOT NULL,
    password_hash TEXT        NOT NULL,
    role          TEXT        NOT NULL DEFAULT 'user',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, email)
);
`

// ---------------------------------------------------------------------------
// Devices
// ---------------------------------------------------------------------------

// EnsureDevice creates a minimal device row if one does not already exist for
// the given (tenant_id, device_id) pair.  It is called automatically before
// any data insert so that ESP32 devices self-register on first contact without
// needing to publish an identity message first.
func (d *DB) EnsureDevice(ctx context.Context, tenantID, deviceID string) error {
	_, err := d.pool.Exec(ctx, `
		INSERT INTO devices (tenant_id, device_id)
		VALUES ($1, $2)
		ON CONFLICT (tenant_id, device_id) DO NOTHING`,
		tenantID, deviceID,
	)
	return err
}

// UpsertDevice inserts or updates a device record.
func (d *DB) UpsertDevice(ctx context.Context, dev models.DeviceIdentity) error {
	_, err := d.pool.Exec(ctx, `
		INSERT INTO devices (tenant_id, device_id, device_name, hostname, ip_address, wifi_state, last_seen)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (tenant_id, device_id) DO UPDATE SET
			device_name = EXCLUDED.device_name,
			hostname    = EXCLUDED.hostname,
			ip_address  = EXCLUDED.ip_address,
			wifi_state  = EXCLUDED.wifi_state,
			last_seen   = NOW()`,
		dev.TenantID, dev.DeviceID, dev.DeviceName, dev.Hostname, dev.IPAddress, int(dev.WiFiState),
	)
	return err
}

// ---------------------------------------------------------------------------
// Readings
// ---------------------------------------------------------------------------

// InsertReading stores a single sensor reading.
// If r.Timestamp is zero (e.g. the ESP32 had no RTC sync), it falls back to
// the server's current UTC time so recorded_at is never 0001-01-01.
func (d *DB) InsertReading(ctx context.Context, tenantID, deviceID string, r models.Reading) error {
	ts := r.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	_, err := d.pool.Exec(ctx, `
		INSERT INTO readings (tenant_id, device_id, temperature, humidity, fallback_time, recorded_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		tenantID, deviceID, r.Temperature, r.Humidity, r.FallbackTime, ts,
	)
	return err
}

// GetReadings returns readings for a tenant/device pair within a time range.
func (d *DB) GetReadings(ctx context.Context, tenantID, deviceID string, from, to time.Time, limit int) ([]models.Reading, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT temperature, humidity, fallback_time, recorded_at
		FROM readings
		WHERE tenant_id = $1 AND device_id = $2 AND recorded_at BETWEEN $3 AND $4
		ORDER BY recorded_at DESC
		LIMIT $5`,
		tenantID, deviceID, from, to, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var readings []models.Reading
	for rows.Next() {
		var r models.Reading
		if err := rows.Scan(&r.Temperature, &r.Humidity, &r.FallbackTime, &r.Timestamp); err != nil {
			return nil, err
		}
		readings = append(readings, r)
	}
	return readings, rows.Err()
}

// ---------------------------------------------------------------------------
// Compressor cycles
// ---------------------------------------------------------------------------

// InsertCompressorCycle stores a compressor cycle record.
func (d *DB) InsertCompressorCycle(ctx context.Context, tenantID, deviceID string, c models.CompressorCycle) error {
	_, err := d.pool.Exec(ctx, `
		INSERT INTO compressor_cycles (tenant_id, device_id, work_time_s, rest_time_s, temperature, humidity, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		tenantID, deviceID, c.WorkTime, c.RestTime, c.Temp, c.Humidity, c.CreatedAt,
	)
	return err
}

// ---------------------------------------------------------------------------
// System status
// ---------------------------------------------------------------------------

// InsertSystemStatus stores a status snapshot.
func (d *DB) InsertSystemStatus(ctx context.Context, tenantID, deviceID string, s models.SystemStatus) error {
	_, err := d.pool.Exec(ctx, `
		INSERT INTO system_status (tenant_id, device_id, state, dht_ok, rtc_ok, uptime_seconds, restart_count, recorded_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())`,
		tenantID, deviceID, int(s.State), s.DHTOk, s.RTCOk, s.UptimeSeconds, s.RestartCount,
	)
	return err
}

// GetLatestSystemStatus returns the most recent status for a tenant/device pair.
func (d *DB) GetLatestSystemStatus(ctx context.Context, tenantID, deviceID string) (models.SystemStatus, error) {
	var s models.SystemStatus
	var state int
	err := d.pool.QueryRow(ctx, `
		SELECT state, dht_ok, rtc_ok, uptime_seconds, restart_count, recorded_at
		FROM system_status
		WHERE tenant_id = $1 AND device_id = $2
		ORDER BY recorded_at DESC
		LIMIT 1`,
		tenantID, deviceID,
	).Scan(&state, &s.DHTOk, &s.RTCOk, &s.UptimeSeconds, &s.RestartCount, &s.Timestamp)
	if err != nil {
		return s, err
	}
	s.State = models.SystemState(state)
	return s, nil
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

// InsertError stores an error event.
func (d *DB) InsertError(ctx context.Context, tenantID, deviceID string, e models.ErrorStatus) error {
	_, err := d.pool.Exec(ctx, `
		INSERT INTO errors (tenant_id, device_id, error_type, severity, message, active, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		tenantID, deviceID, int(e.Type), int(e.Severity), e.Message, e.Active, e.Timestamp,
	)
	return err
}

// GetActiveErrors returns all active errors for a tenant/device pair.
func (d *DB) GetActiveErrors(ctx context.Context, tenantID, deviceID string) ([]models.ErrorStatus, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT error_type, severity, message, active, occurred_at
		FROM errors
		WHERE tenant_id = $1 AND device_id = $2 AND active = TRUE
		ORDER BY occurred_at DESC`,
		tenantID, deviceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var errs []models.ErrorStatus
	for rows.Next() {
		var e models.ErrorStatus
		var et, sev int
		if err := rows.Scan(&et, &sev, &e.Message, &e.Active, &e.Timestamp); err != nil {
			return nil, err
		}
		e.Type = models.ErrorType(et)
		e.Severity = models.ErrorSeverity(sev)
		errs = append(errs, e)
	}
	return errs, rows.Err()
}

// ---------------------------------------------------------------------------
// Device settings
// ---------------------------------------------------------------------------

// UpsertSettings persists the full settings block for a tenant/device pair.
func (d *DB) UpsertSettings(ctx context.Context, tenantID, deviceID string,
	ts models.TempSettings, hs models.HumiditySettings,
	fs models.FanSettings, ls models.LightSettings,
	mode models.OperationalMode, activeMode models.ModeType,
) error {
	_, err := d.pool.Exec(ctx, `
		INSERT INTO device_settings (
			tenant_id, device_id,
			temp_target, temp_offset,
			humidity_target, humidity_offset,
			fan_speed, mixing_interval_s, mixing_duration_s, mixing_enabled,
			light_mode, light_state, operational_mode, active_mode, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,NOW())
		ON CONFLICT (tenant_id, device_id) DO UPDATE SET
			temp_target       = EXCLUDED.temp_target,
			temp_offset       = EXCLUDED.temp_offset,
			humidity_target   = EXCLUDED.humidity_target,
			humidity_offset   = EXCLUDED.humidity_offset,
			fan_speed         = EXCLUDED.fan_speed,
			mixing_interval_s = EXCLUDED.mixing_interval_s,
			mixing_duration_s = EXCLUDED.mixing_duration_s,
			mixing_enabled    = EXCLUDED.mixing_enabled,
			light_mode        = EXCLUDED.light_mode,
			light_state       = EXCLUDED.light_state,
			operational_mode  = EXCLUDED.operational_mode,
			active_mode       = EXCLUDED.active_mode,
			updated_at        = NOW()`,
		tenantID, deviceID,
		ts.Target, ts.Offset,
		hs.Target, hs.Offset,
		fs.Speed, fs.MixingInterval, fs.MixingDuration, fs.MixingEnabled,
		int(ls.Mode), ls.State,
		mode.String(), int(activeMode),
	)
	return err
}

// ---------------------------------------------------------------------------
// Users
// ---------------------------------------------------------------------------

// CreateUser inserts a new user and returns the created record.
// Returns an error (with a duplicate key message) if the email already exists
// within the tenant.
func (d *DB) CreateUser(ctx context.Context, tenantID, email, passwordHash string, role models.Role) (models.User, error) {
	var u models.User
	var roleStr string
	err := d.pool.QueryRow(ctx, `
		INSERT INTO users (tenant_id, email, password_hash, role)
		VALUES ($1, $2, $3, $4)
		RETURNING id, tenant_id, email, password_hash, role, created_at`,
		tenantID, email, passwordHash, string(role),
	).Scan(&u.ID, &u.TenantID, &u.Email, &u.PasswordHash, &roleStr, &u.CreatedAt)
	if err != nil {
		return u, err
	}
	u.Role = models.Role(roleStr)
	return u, nil
}

// GetUserByEmail returns a user by tenant + email.
// Returns pgx.ErrNoRows if no matching user exists.
func (d *DB) GetUserByEmail(ctx context.Context, tenantID, email string) (models.User, error) {
	var u models.User
	var roleStr string
	err := d.pool.QueryRow(ctx, `
		SELECT id, tenant_id, email, password_hash, role, created_at
		FROM users
		WHERE tenant_id = $1 AND email = $2`,
		tenantID, email,
	).Scan(&u.ID, &u.TenantID, &u.Email, &u.PasswordHash, &roleStr, &u.CreatedAt)
	if err != nil {
		return u, err
	}
	u.Role = models.Role(roleStr)
	return u, nil
}

// ErrNoRows is re-exported so callers can check for missing users without
// importing pgx directly.
var ErrNoRows = pgx.ErrNoRows

// ---------------------------------------------------------------------------
// Device listing
// ---------------------------------------------------------------------------

// ListDeviceIDs returns all device_ids belonging to a tenant, ordered
// by the time they were last seen.
func (d *DB) ListDeviceIDs(ctx context.Context, tenantID string) ([]string, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT device_id
		FROM devices
		WHERE tenant_id = $1
		ORDER BY last_seen DESC`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
