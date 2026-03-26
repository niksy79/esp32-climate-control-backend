// Package db provides PostgreSQL access for the climate backend.
package db

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
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

CREATE TABLE IF NOT EXISTS device_types (
    id           TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS metric_definitions (
    id             BIGSERIAL PRIMARY KEY,
    device_type_id TEXT NOT NULL REFERENCES device_types(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    display_name   TEXT NOT NULL,
    unit           TEXT NOT NULL DEFAULT '',
    data_type      TEXT NOT NULL DEFAULT 'float',
    sort_order     INTEGER NOT NULL DEFAULT 0,
    UNIQUE (device_type_id, name)
);

CREATE TABLE IF NOT EXISTS command_definitions (
    id             BIGSERIAL PRIMARY KEY,
    device_type_id TEXT NOT NULL REFERENCES device_types(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    display_name   TEXT NOT NULL,
    payload_schema TEXT NOT NULL DEFAULT '{}',
    sort_order     INTEGER NOT NULL DEFAULT 0,
    UNIQUE (device_type_id, name)
);

ALTER TABLE devices ADD COLUMN IF NOT EXISTS
    device_type_id TEXT REFERENCES device_types(id);

INSERT INTO device_types (id, display_name, description)
VALUES ('climate_controller', 'Climate Controller', 'ESP32-based temperature and humidity controller')
ON CONFLICT (id) DO NOTHING;

INSERT INTO metric_definitions (device_type_id, name, display_name, unit, data_type, sort_order)
VALUES
    ('climate_controller', 'temperature', 'Temperature', '°C', 'float', 0),
    ('climate_controller', 'humidity', 'Humidity', '%', 'float', 1)
ON CONFLICT (device_type_id, name) DO NOTHING;

INSERT INTO command_definitions (device_type_id, name, display_name, payload_schema, sort_order)
VALUES
    ('climate_controller', 'set_mode', 'Set Mode', '{"mode": "string"}', 0),
    ('climate_controller', 'set_target_temp', 'Set Target Temperature', '{"value": "float"}', 1)
ON CONFLICT (device_type_id, name) DO NOTHING;

CREATE TABLE IF NOT EXISTS device_readings (
    id          BIGSERIAL   PRIMARY KEY,
    tenant_id   TEXT        NOT NULL,
    device_id   TEXT        NOT NULL,
    metric_name TEXT        NOT NULL,
    value       REAL        NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL,
    FOREIGN KEY (tenant_id, device_id) REFERENCES devices(tenant_id, device_id)
);
CREATE INDEX IF NOT EXISTS device_readings_tenant_device_metric_time
    ON device_readings (tenant_id, device_id, metric_name, recorded_at DESC);

CREATE TABLE IF NOT EXISTS password_reset_tokens (
    token       TEXT        PRIMARY KEY,
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id   TEXT        NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    used        BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

DO $$ BEGIN
    ALTER TABLE users ADD CONSTRAINT users_email_unique UNIQUE (email);
EXCEPTION WHEN duplicate_table OR duplicate_object THEN NULL;
END $$;
`

// ---------------------------------------------------------------------------
// Devices
// ---------------------------------------------------------------------------

// EnsureDevice creates a minimal device row if one does not already exist for
// the given (tenant_id, device_id) pair.  It is called automatically before
// any data insert so that ESP32 devices self-register on first contact without
// needing to publish an identity message first.
// DeviceExists returns true when the (tenantID, deviceID) pair has a row in devices.
func (d *DB) DeviceExists(ctx context.Context, tenantID, deviceID string) (bool, error) {
	var exists bool
	err := d.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM devices WHERE tenant_id=$1 AND device_id=$2)`,
		tenantID, deviceID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("db: device exists %s/%s: %w", tenantID, deviceID, err)
	}
	return exists, nil
}

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
// recorded_at is always set to the server's current UTC time; r.Timestamp
// from the ESP32 payload is not used for storage (device sends local time).
func (d *DB) InsertReading(ctx context.Context, tenantID, deviceID string, r models.Reading) error {
	ts := time.Now().UTC()
	_, err := d.pool.Exec(ctx, `
		INSERT INTO readings (tenant_id, device_id, temperature, humidity, fallback_time, recorded_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		tenantID, deviceID, r.Temperature, r.Humidity, r.FallbackTime, ts,
	)
	return err
}

// InsertDeviceReading записва единична метрична стойност в device_readings.
// Timestamp-ът е винаги time.Now().UTC() — ESP32 payload timestamps се игнорират.
func (d *DB) InsertDeviceReading(ctx context.Context, tenantID, deviceID, metric string, value float32) error {
	_, err := d.pool.Exec(ctx,
		`INSERT INTO device_readings (tenant_id, device_id, metric_name, value, recorded_at)
         VALUES ($1, $2, $3, $4, $5)`,
		tenantID, deviceID, metric, value, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("db: insert device_reading %s/%s/%s: %w", tenantID, deviceID, metric, err)
	}
	return nil
}

// GetReadings returns readings for a tenant/device pair within a time range.
func (d *DB) GetReadings(ctx context.Context, tenantID, deviceID string, from, to time.Time, limit int) ([]models.Reading, error) {
	log.Printf("db: GetReadings tenant=%q device=%q from=%s to=%s limit=%d",
		tenantID, deviceID, from.Format(time.RFC3339), to.Format(time.RFC3339), limit)
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
	log.Printf("db: GetReadings tenant=%q device=%q → %d rows", tenantID, deviceID, len(readings))
	return readings, rows.Err()
}

// GetDeviceReadings връща история за единична метрика от device_readings.
// days се cap-ва на 31; максимум 144 записа (същото като GetReadings).
func (d *DB) GetDeviceReadings(ctx context.Context, tenantID, deviceID, metric string, days int) ([]models.MetricReading, error) {
	if days < 1 {
		days = 1
	}
	if days > 31 {
		days = 31
	}
	rows, err := d.pool.Query(ctx,
		`SELECT value, recorded_at
         FROM device_readings
         WHERE tenant_id = $1
           AND device_id = $2
           AND metric_name = $3
           AND recorded_at > NOW() - ($4 * INTERVAL '1 day')
         ORDER BY recorded_at DESC
         LIMIT 10000`,
		tenantID, deviceID, metric, days,
	)
	if err != nil {
		return nil, fmt.Errorf("db: get device_readings %s/%s/%s: %w", tenantID, deviceID, metric, err)
	}
	defer rows.Close()

	var result []models.MetricReading
	for rows.Next() {
		var mr models.MetricReading
		if err := rows.Scan(&mr.Value, &mr.RecordedAt); err != nil {
			return nil, fmt.Errorf("db: scan device_reading %s/%s/%s: %w", tenantID, deviceID, metric, err)
		}
		result = append(result, mr)
	}
	return result, rows.Err()
}

// GetDeviceReadingsPaired връща temperature и humidity от device_readings
// наредени по recorded_at DESC, като ги обединява по nearest timestamp
// в рамките на 1 секунда. Ползва се от history endpoint без ?metric=.
func (d *DB) GetDeviceReadingsPaired(ctx context.Context, tenantID, deviceID string, days int) ([]models.Reading, error) {
	if days < 1 {
		days = 1
	}
	if days > 31 {
		days = 31
	}
	rows, err := d.pool.Query(ctx,
		`SELECT
             t.value        AS temperature,
             h.value        AS humidity,
             t.recorded_at  AS recorded_at
         FROM device_readings t
         JOIN device_readings h
           ON h.tenant_id   = t.tenant_id
          AND h.device_id   = t.device_id
          AND h.metric_name = 'humidity'
          AND h.recorded_at BETWEEN t.recorded_at - INTERVAL '2 seconds'
                                AND t.recorded_at + INTERVAL '2 seconds'
         WHERE t.tenant_id   = $1
           AND t.device_id   = $2
           AND t.metric_name = 'temperature'
           AND t.recorded_at > NOW() - ($3 * INTERVAL '1 day')
         ORDER BY t.recorded_at DESC
         LIMIT 10000`,
		tenantID, deviceID, days,
	)
	if err != nil {
		return nil, fmt.Errorf("db: get device_readings paired %s/%s: %w", tenantID, deviceID, err)
	}
	defer rows.Close()

	var result []models.Reading
	for rows.Next() {
		var r models.Reading
		if err := rows.Scan(&r.Temperature, &r.Humidity, &r.Timestamp); err != nil {
			return nil, fmt.Errorf("db: scan device_readings paired %s/%s: %w", tenantID, deviceID, err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
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

// GetCompressorCycles returns up to 200 compressor cycle records for a
// tenant/device pair within the last N days, newest first.
func (d *DB) GetCompressorCycles(ctx context.Context, tenantID, deviceID string, days int) ([]models.CompressorCycle, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT work_time_s, rest_time_s, temperature, humidity, created_at
		FROM compressor_cycles
		WHERE tenant_id = $1 AND device_id = $2
		  AND created_at >= NOW() - ($3 * INTERVAL '1 day')
		ORDER BY created_at DESC
		LIMIT 200`,
		tenantID, deviceID, days,
	)
	if err != nil {
		return nil, fmt.Errorf("db: get compressor cycles %s/%s: %w", tenantID, deviceID, err)
	}
	defer rows.Close()
	var cycles []models.CompressorCycle
	for rows.Next() {
		var c models.CompressorCycle
		if err := rows.Scan(&c.WorkTime, &c.RestTime, &c.Temp, &c.Humidity, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("db: scan compressor cycle %s/%s: %w", tenantID, deviceID, err)
		}
		cycles = append(cycles, c)
	}
	return cycles, rows.Err()
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

// GetSettings loads the persisted settings for a tenant/device pair.
// Returns ErrNoRows if no row exists yet.
func (d *DB) GetSettings(ctx context.Context, tenantID, deviceID string) (
	models.TempSettings, models.HumiditySettings, models.FanSettings,
	models.LightSettings, models.OperationalMode, models.ModeType, error,
) {
	var (
		ts         models.TempSettings
		hs         models.HumiditySettings
		fs         models.FanSettings
		ls         models.LightSettings
		opModeStr  string
		activeModeInt int
	)
	err := d.pool.QueryRow(ctx, `
		SELECT temp_target, temp_offset,
		       humidity_target, humidity_offset,
		       fan_speed, mixing_interval_s, mixing_duration_s, mixing_enabled,
		       light_mode, light_state,
		       operational_mode, active_mode
		FROM device_settings
		WHERE tenant_id = $1 AND device_id = $2`,
		tenantID, deviceID,
	).Scan(
		&ts.Target, &ts.Offset,
		&hs.Target, &hs.Offset,
		&fs.Speed, &fs.MixingInterval, &fs.MixingDuration, &fs.MixingEnabled,
		&ls.Mode, &ls.State,
		&opModeStr, &activeModeInt,
	)
	if err != nil {
		return ts, hs, fs, ls, 0, 0, err
	}
	var opMode models.OperationalMode
	switch opModeStr {
	case "fallback":
		opMode = models.OperationalModeFallback
	case "emergency":
		opMode = models.OperationalModeEmergency
	default:
		opMode = models.OperationalModeNormal
	}
	return ts, hs, fs, ls, opMode, models.ModeType(activeModeInt), nil
}

// SaveActiveMode updates only the active_mode column for a tenant/device pair.
func (d *DB) SaveActiveMode(ctx context.Context, tenantID, deviceID string, mode int) error {
	_, err := d.pool.Exec(ctx, `
		UPDATE device_settings
		SET active_mode = $1, updated_at = NOW()
		WHERE tenant_id = $2 AND device_id = $3`,
		mode, tenantID, deviceID,
	)
	return err
}

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

// LoadActiveModes returns a map of "tenantID/deviceID" → ModeType for every
// device that has a row in device_settings. Used at startup to seed the
// control manager so active_mode survives server restarts.
func (d *DB) LoadActiveModes(ctx context.Context) (map[string]models.ModeType, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT tenant_id, device_id, active_mode
		FROM device_settings`)
	if err != nil {
		return nil, fmt.Errorf("db: load active modes: %w", err)
	}
	defer rows.Close()
	result := make(map[string]models.ModeType)
	for rows.Next() {
		var tenantID, deviceID string
		var mode int
		if err := rows.Scan(&tenantID, &deviceID, &mode); err != nil {
			return nil, fmt.Errorf("db: scan active mode: %w", err)
		}
		result[tenantID+"/"+deviceID] = models.ModeType(mode)
	}
	return result, nil
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

// generateTenantID генерира кратък уникален tenant_id (6 символа, a-z0-9).
// Вътрешна помощна функция — не е метод на DB.
func generateTenantID() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 6)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		b[i] = chars[n.Int64()]
	}
	return string(b)
}

// GenerateUniqueTenantID генерира tenant_id който не съществува в DB.
func (d *DB) GenerateUniqueTenantID(ctx context.Context) (string, error) {
	for range 10 {
		id := generateTenantID()
		var exists bool
		err := d.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM users WHERE tenant_id = $1)`, id,
		).Scan(&exists)
		if err != nil {
			return "", fmt.Errorf("db: check tenant_id %s: %w", id, err)
		}
		if !exists {
			return id, nil
		}
	}
	return "", fmt.Errorf("db: failed to generate unique tenant_id after 10 attempts")
}

// CreatePasswordResetToken съхранява reset token валиден 1 час.
func (d *DB) CreatePasswordResetToken(ctx context.Context, userID, tenantID, token string) error {
	_, err := d.pool.Exec(ctx,
		`INSERT INTO password_reset_tokens (token, user_id, tenant_id, expires_at)
         VALUES ($1, $2, $3, NOW() + INTERVAL '1 hour')`,
		token, userID, tenantID,
	)
	if err != nil {
		return fmt.Errorf("db: create reset token: %w", err)
	}
	return nil
}

// ValidatePasswordResetToken проверява token и връща user_id и tenant_id.
// Маркира token-а като използван при успех.
func (d *DB) ValidatePasswordResetToken(ctx context.Context, token string) (userID, tenantID string, err error) {
	err = d.pool.QueryRow(ctx,
		`UPDATE password_reset_tokens
         SET used = TRUE
         WHERE token = $1
           AND used = FALSE
           AND expires_at > NOW()
         RETURNING user_id, tenant_id`,
		token,
	).Scan(&userID, &tenantID)
	if err == pgx.ErrNoRows {
		return "", "", ErrNoRows
	}
	if err != nil {
		return "", "", fmt.Errorf("db: validate reset token: %w", err)
	}
	return userID, tenantID, nil
}

// UpdatePassword задава нова bcrypt парола за потребител.
func (d *DB) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	tag, err := d.pool.Exec(ctx,
		`UPDATE users SET password_hash = $1 WHERE id = $2`,
		passwordHash, userID,
	)
	if err != nil {
		return fmt.Errorf("db: update password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoRows
	}
	return nil
}

// GetUserByID връща потребител по UUID.
func (d *DB) GetUserByID(ctx context.Context, userID string) (models.User, error) {
	var u models.User
	var roleStr string
	err := d.pool.QueryRow(ctx,
		`SELECT id, tenant_id, email, password_hash, role, created_at
         FROM users WHERE id = $1`, userID,
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

// GetUserByEmailGlobal търси потребител по email без да знае tenant_id.
// Използва се само за password reset flow.
func (d *DB) GetUserByEmailGlobal(ctx context.Context, email string) (models.User, error) {
	var u models.User
	var roleStr string
	err := d.pool.QueryRow(ctx,
		`SELECT id, tenant_id, email, password_hash, role, created_at
         FROM users WHERE email = $1
         ORDER BY created_at ASC
         LIMIT 1`, email,
	).Scan(&u.ID, &u.TenantID, &u.Email, &u.PasswordHash, &roleStr, &u.CreatedAt)
	if err != nil {
		return u, err
	}
	u.Role = models.Role(roleStr)
	return u, nil
}

// ErrNoRows is re-exported so callers can check for missing rows without
// importing pgx directly.
var ErrNoRows = pgx.ErrNoRows

// ---------------------------------------------------------------------------
// Device listing
// ---------------------------------------------------------------------------

// ListDeviceIDs returns all devices belonging to a tenant as DeviceSummary
// (device_id + device_type_id), ordered by last_seen DESC.
func (d *DB) ListDeviceIDs(ctx context.Context, tenantID string) ([]models.DeviceSummary, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT device_id, COALESCE(device_type_id, '') AS device_type_id
		FROM devices
		WHERE tenant_id = $1
		ORDER BY last_seen DESC`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []models.DeviceSummary
	for rows.Next() {
		var s models.DeviceSummary
		if err := rows.Scan(&s.DeviceID, &s.DeviceTypeID); err != nil {
			return nil, err
		}
		devices = append(devices, s)
	}
	return devices, rows.Err()
}

// ---------------------------------------------------------------------------
// Alert rules
// ---------------------------------------------------------------------------

const alertRuleCols = `id, tenant_id, device_id, metric, operator, threshold,
	channel, recipient, enabled, cooldown_minutes, last_fired, created_at`

func scanAlertRule(row interface {
	Scan(...any) error
}) (models.AlertRule, error) {
	var r models.AlertRule
	err := row.Scan(
		&r.ID, &r.TenantID, &r.DeviceID, &r.Metric, &r.Operator, &r.Threshold,
		&r.Channel, &r.Recipient, &r.Enabled, &r.CooldownMinutes, &r.LastFired, &r.CreatedAt,
	)
	return r, err
}

// CreateAlertRule inserts a new alert rule and returns the persisted record.
func (d *DB) CreateAlertRule(ctx context.Context, rule models.AlertRule) (models.AlertRule, error) {
	return scanAlertRule(d.pool.QueryRow(ctx, `
		INSERT INTO alert_rules
			(tenant_id, device_id, metric, operator, threshold, channel, recipient, enabled, cooldown_minutes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING `+alertRuleCols,
		rule.TenantID, rule.DeviceID, rule.Metric, rule.Operator, rule.Threshold,
		rule.Channel, rule.Recipient, rule.Enabled, rule.CooldownMinutes,
	))
}

// ListAlertRules returns all alert rules for a tenant/device pair.
func (d *DB) ListAlertRules(ctx context.Context, tenantID, deviceID string) ([]models.AlertRule, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT `+alertRuleCols+`
		FROM alert_rules
		WHERE tenant_id = $1 AND device_id = $2
		ORDER BY created_at ASC`,
		tenantID, deviceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []models.AlertRule
	for rows.Next() {
		r, err := scanAlertRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// UpdateAlertRule replaces the mutable fields of an existing rule.
// Returns ErrNoRows if no rule with the given ID exists for the tenant/device.
func (d *DB) UpdateAlertRule(ctx context.Context, rule models.AlertRule) (models.AlertRule, error) {
	r, err := scanAlertRule(d.pool.QueryRow(ctx, `
		UPDATE alert_rules SET
			metric           = $1,
			operator         = $2,
			threshold        = $3,
			channel          = $4,
			recipient        = $5,
			enabled          = $6,
			cooldown_minutes = $7
		WHERE id = $8 AND tenant_id = $9 AND device_id = $10
		RETURNING `+alertRuleCols,
		rule.Metric, rule.Operator, rule.Threshold, rule.Channel, rule.Recipient,
		rule.Enabled, rule.CooldownMinutes, rule.ID, rule.TenantID, rule.DeviceID,
	))
	if err == pgx.ErrNoRows {
		return r, ErrNoRows
	}
	return r, err
}

// DeleteAlertRule removes an alert rule.
// Returns ErrNoRows if no matching rule was found.
func (d *DB) DeleteAlertRule(ctx context.Context, tenantID, deviceID, ruleID string) error {
	tag, err := d.pool.Exec(ctx, `
		DELETE FROM alert_rules WHERE id = $1 AND tenant_id = $2 AND device_id = $3`,
		ruleID, tenantID, deviceID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNoRows
	}
	return nil
}

// UpdateAlertRuleLastFired stamps the last_fired timestamp on a rule.
func (d *DB) UpdateAlertRuleLastFired(ctx context.Context, ruleID string, t time.Time) error {
	_, err := d.pool.Exec(ctx, `
		UPDATE alert_rules SET last_fired = $1 WHERE id = $2`, t, ruleID)
	return err
}

// LoadAllAlertRules returns every alert rule across all tenants and devices.
// Used at startup to populate the in-memory engine cache.
func (d *DB) LoadAllAlertRules(ctx context.Context) ([]models.AlertRule, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT `+alertRuleCols+`
		FROM alert_rules
		ORDER BY tenant_id, device_id, created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []models.AlertRule
	for rows.Next() {
		r, err := scanAlertRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// ---------------------------------------------------------------------------
// DeviceType queries
// ---------------------------------------------------------------------------

func (d *DB) CreateDeviceType(ctx context.Context, dt models.DeviceType) error {
	_, err := d.pool.Exec(ctx, `
		INSERT INTO device_types (id, display_name, description)
		VALUES ($1, $2, $3)`,
		dt.ID, dt.DisplayName, dt.Description)
	if err != nil {
		return fmt.Errorf("db: create device type %s: %w", dt.ID, err)
	}
	return nil
}

func (d *DB) GetDeviceType(ctx context.Context, id string) (models.DeviceType, error) {
	var dt models.DeviceType
	err := d.pool.QueryRow(ctx, `
		SELECT id, display_name, description, created_at, updated_at
		FROM device_types WHERE id = $1`, id).
		Scan(&dt.ID, &dt.DisplayName, &dt.Description, &dt.CreatedAt, &dt.UpdatedAt)
	if err != nil {
		return dt, fmt.Errorf("db: get device type %s: %w", id, err)
	}
	dt.Metrics, err = d.ListMetricDefinitions(ctx, id)
	if err != nil {
		return dt, err
	}
	dt.Commands, err = d.ListCommandDefinitions(ctx, id)
	if err != nil {
		return dt, err
	}
	return dt, nil
}

func (d *DB) ListDeviceTypes(ctx context.Context) ([]models.DeviceType, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, display_name, description, created_at, updated_at
		FROM device_types ORDER BY display_name`)
	if err != nil {
		return nil, fmt.Errorf("db: list device types: %w", err)
	}
	defer rows.Close()
	var types []models.DeviceType
	for rows.Next() {
		var dt models.DeviceType
		if err := rows.Scan(&dt.ID, &dt.DisplayName, &dt.Description, &dt.CreatedAt, &dt.UpdatedAt); err != nil {
			return nil, fmt.Errorf("db: scan device type: %w", err)
		}
		types = append(types, dt)
	}
	return types, nil
}

func (d *DB) UpdateDeviceType(ctx context.Context, dt models.DeviceType) error {
	_, err := d.pool.Exec(ctx, `
		UPDATE device_types SET display_name=$2, description=$3, updated_at=NOW()
		WHERE id=$1`,
		dt.ID, dt.DisplayName, dt.Description)
	if err != nil {
		return fmt.Errorf("db: update device type %s: %w", dt.ID, err)
	}
	return nil
}

func (d *DB) SetDeviceTypeID(ctx context.Context, tenantID, deviceID, deviceTypeID string) error {
	_, err := d.pool.Exec(ctx, `
		UPDATE devices SET device_type_id=$3
		WHERE tenant_id=$1 AND device_id=$2`,
		tenantID, deviceID, deviceTypeID)
	if err != nil {
		return fmt.Errorf("db: set device type %s/%s: %w", tenantID, deviceID, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// MetricDefinition queries
// ---------------------------------------------------------------------------

func (d *DB) ListMetricDefinitions(ctx context.Context, deviceTypeID string) ([]models.MetricDefinition, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, device_type_id, name, display_name, unit, data_type, sort_order
		FROM metric_definitions WHERE device_type_id=$1 ORDER BY sort_order`, deviceTypeID)
	if err != nil {
		return nil, fmt.Errorf("db: list metrics %s: %w", deviceTypeID, err)
	}
	defer rows.Close()
	var metrics []models.MetricDefinition
	for rows.Next() {
		var m models.MetricDefinition
		if err := rows.Scan(&m.ID, &m.DeviceTypeID, &m.Name, &m.DisplayName, &m.Unit, &m.DataType, &m.SortOrder); err != nil {
			return nil, fmt.Errorf("db: scan metric: %w", err)
		}
		metrics = append(metrics, m)
	}
	return metrics, nil
}

func (d *DB) CreateMetricDefinition(ctx context.Context, m models.MetricDefinition) (models.MetricDefinition, error) {
	err := d.pool.QueryRow(ctx, `
		INSERT INTO metric_definitions (device_type_id, name, display_name, unit, data_type, sort_order)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`,
		m.DeviceTypeID, m.Name, m.DisplayName, m.Unit, m.DataType, m.SortOrder).
		Scan(&m.ID)
	if err != nil {
		return m, fmt.Errorf("db: create metric %s/%s: %w", m.DeviceTypeID, m.Name, err)
	}
	return m, nil
}

func (d *DB) DeleteMetricDefinition(ctx context.Context, id int64) error {
	_, err := d.pool.Exec(ctx, `DELETE FROM metric_definitions WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("db: delete metric %d: %w", id, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// CommandDefinition queries
// ---------------------------------------------------------------------------

func (d *DB) ListCommandDefinitions(ctx context.Context, deviceTypeID string) ([]models.CommandDefinition, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, device_type_id, name, display_name, payload_schema, sort_order
		FROM command_definitions WHERE device_type_id=$1 ORDER BY sort_order`, deviceTypeID)
	if err != nil {
		return nil, fmt.Errorf("db: list commands %s: %w", deviceTypeID, err)
	}
	defer rows.Close()
	var commands []models.CommandDefinition
	for rows.Next() {
		var c models.CommandDefinition
		if err := rows.Scan(&c.ID, &c.DeviceTypeID, &c.Name, &c.DisplayName, &c.PayloadSchema, &c.SortOrder); err != nil {
			return nil, fmt.Errorf("db: scan command: %w", err)
		}
		commands = append(commands, c)
	}
	return commands, nil
}

func (d *DB) CreateCommandDefinition(ctx context.Context, c models.CommandDefinition) (models.CommandDefinition, error) {
	err := d.pool.QueryRow(ctx, `
		INSERT INTO command_definitions (device_type_id, name, display_name, payload_schema, sort_order)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		c.DeviceTypeID, c.Name, c.DisplayName, c.PayloadSchema, c.SortOrder).
		Scan(&c.ID)
	if err != nil {
		return c, fmt.Errorf("db: create command %s/%s: %w", c.DeviceTypeID, c.Name, err)
	}
	return c, nil
}

func (d *DB) DeleteCommandDefinition(ctx context.Context, id int64) error {
	_, err := d.pool.Exec(ctx, `DELETE FROM command_definitions WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("db: delete command %d: %w", id, err)
	}
	return nil
}
