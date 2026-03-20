// Package models defines Go structs and enums that mirror the C++ data structures
// from the ESP32 climate controller firmware.
package models

import "time"

// ---------------------------------------------------------------------------
// Auth
// ---------------------------------------------------------------------------

// Role controls what a user is allowed to do via the REST API.
// ADMIN can read and write; USER is read-only.
type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

// User is a human operator who accesses the REST API.
// Passwords are stored as bcrypt hashes; the field is excluded from JSON output.
type User struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Role         Role      `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}

// ---------------------------------------------------------------------------
// Sensor (sensor_manager.h)
// ---------------------------------------------------------------------------

type SensorHealth int

const (
	SensorHealthGood    SensorHealth = iota
	SensorHealthWarning SensorHealth = iota
	SensorHealthError   SensorHealth = iota
)

func (s SensorHealth) String() string {
	return [...]string{"good", "warning", "error"}[s]
}

// SensorReading holds a single temperature/humidity measurement from the DHT sensor.
type SensorReading struct {
	Temperature float32   `json:"temperature"`
	Humidity    float32   `json:"humidity"`
	Timestamp   time.Time `json:"timestamp"`
	FallbackTime bool     `json:"fallback_time"` // true when RTC unavailable
	Health      SensorHealth `json:"health"`
}

// ---------------------------------------------------------------------------
// Data store (data_manager.h)
// ---------------------------------------------------------------------------

// Reading mirrors DataManager::Reading.
type Reading struct {
	Temperature  float32   `json:"temperature"`
	Humidity     float32   `json:"humidity"`
	Timestamp    time.Time `json:"timestamp"`
	FallbackTime bool      `json:"fallback_time"`
}

// CompressorCycle mirrors DataManager::CompressorCycle.
type CompressorCycle struct {
	WorkTime  uint32    `json:"work_time"`
	RestTime  uint32    `json:"rest_time"`
	Temp      float32   `json:"temperature"`
	Humidity  float32   `json:"humidity"`
	CreatedAt time.Time `json:"created_at"`
}

// ---------------------------------------------------------------------------
// Storage / settings (storage_manager.h)
// ---------------------------------------------------------------------------

// TempSettings mirrors StorageManager::TempSettings.
type TempSettings struct {
	Target float32 `json:"target"`
	Offset float32 `json:"offset"`
}

// HumiditySettings mirrors StorageManager::HumiditySettings.
type HumiditySettings struct {
	Target float32 `json:"target"`
	Offset float32 `json:"offset"`
}

// FanSettings mirrors StorageManager::FanSettings.
type FanSettings struct {
	Speed          uint8 `json:"speed"`           // 0–100 %
	MixingInterval uint32 `json:"mixing_interval_s"`
	MixingDuration uint32 `json:"mixing_duration_s"`
	MixingEnabled  bool  `json:"mixing_enabled"`
}

type LightMode int

const (
	LightModeManual LightMode = iota
	LightModeAuto
)

// LightSettings mirrors StorageManager::LightSettings.
type LightSettings struct {
	Mode  LightMode `json:"mode"`
	State bool      `json:"state"`
}

// SystemSettings mirrors StorageManager::SystemSettings.
type SystemSettings struct {
	OperationMode string `json:"operation_mode"` // "normal" | "fallback" | "emergency"
}

// DisplaySettings mirrors StorageManager::DisplaySettings / DisplayManager::DisplaySettings.
type DisplaySettings struct {
	Brightness   uint8  `json:"brightness"`    // 0–100
	SleepTimeout uint32 `json:"sleep_timeout_s"`
	AutoSleep    bool   `json:"auto_sleep"`
}

// ---------------------------------------------------------------------------
// Control manager (control_manager.h)
// ---------------------------------------------------------------------------

type ModeType int

const (
	ModeNormal          ModeType = iota
	ModeHeating
	ModeBeercooling
	ModeRoomTemp
	ModeProductMeatFish
	ModeProductDairy
	ModeProductReadyFood
	ModeProductVegetables
)

func (m ModeType) String() string {
	return [...]string{
		"normal", "heating", "beer_cooling", "room_temp",
		"product_meat_fish", "product_dairy", "product_ready_food", "product_vegetables",
	}[m]
}

type OperationalMode int

const (
	OperationalModeNormal    OperationalMode = iota
	OperationalModeFallback
	OperationalModeEmergency
)

func (o OperationalMode) String() string {
	return [...]string{"normal", "fallback", "emergency"}[o]
}

type FallbackState int

const (
	FallbackStateIdle          FallbackState = iota
	FallbackStateCompressorOn
	FallbackStateCompressorOff
)

// ModeSettings mirrors ControlManager::ModeSettings.
type ModeSettings struct {
	Mode        ModeType `json:"mode"`
	TargetTemp  float32  `json:"target_temp"`
	Tolerance   float32  `json:"tolerance"`
	// device flags
	CompressorEnabled   bool `json:"compressor_enabled"`
	HeatingEnabled      bool `json:"heating_enabled"`
	DehumidifierEnabled bool `json:"dehumidifier_enabled"`
	ExtraFanEnabled     bool `json:"extra_fan_enabled"`
}

// CompressorStats mirrors ControlManager::CompressorStats.
type CompressorStats struct {
	WorkTime   uint32 `json:"work_time_s"`
	RestTime   uint32 `json:"rest_time_s"`
	CycleCount uint32 `json:"cycle_count"`
}

// FallbackStatistics mirrors ControlManager::FallbackStatistics.
type FallbackStatistics struct {
	EnterCount    uint32    `json:"enter_count"`
	TotalDuration uint32    `json:"total_duration_s"`
	LastEntered   time.Time `json:"last_entered"`
}

// DeviceStates mirrors ControlManager::DeviceStates.
type DeviceStates struct {
	Compressor   bool `json:"compressor"`
	FanCompressor bool `json:"fan_compressor"`
	ExtraFan     bool `json:"extra_fan"`
	Light        bool `json:"light"`
	Heating      bool `json:"heating"`
	Dehumidifier bool `json:"dehumidifier"`
}

// ---------------------------------------------------------------------------
// Relay manager (relay_manager.h)
// ---------------------------------------------------------------------------

type RelayType int

const (
	RelayCompressor   RelayType = iota
	RelayFanCompressor
	RelayExtraFan
	RelayLight
	RelayHeating
	RelayDehumidifier
)

func (r RelayType) String() string {
	return [...]string{
		"compressor", "fan_compressor", "extra_fan", "light", "heating", "dehumidifier",
	}[r]
}

// RelayInfo mirrors RelayManager::RelayInfo.
type RelayInfo struct {
	Type         RelayType `json:"type"`
	State        bool      `json:"state"`
	MinOnTime    uint32    `json:"min_on_time_ms"`
	MinOffTime   uint32    `json:"min_off_time_ms"`
	LastChanged  time.Time `json:"last_changed"`
}

// ---------------------------------------------------------------------------
// Error manager (error_manager.h)
// ---------------------------------------------------------------------------

type ErrorType int

const (
	ErrorRTC        ErrorType = iota
	ErrorSensorTemp
	ErrorSensorHum
	ErrorWiFi
	ErrorStorage
	ErrorFan
	ErrorSystem
)

func (e ErrorType) String() string {
	return [...]string{
		"rtc", "sensor_temp", "sensor_hum", "wifi", "storage", "fan", "system",
	}[e]
}

type ErrorSeverity int

const (
	SeverityInfo    ErrorSeverity = iota
	SeverityWarning
	SeverityError
)

func (s ErrorSeverity) String() string {
	return [...]string{"info", "warning", "error"}[s]
}

// ErrorStatus mirrors ErrorManager::ErrorStatus.
type ErrorStatus struct {
	Type      ErrorType     `json:"type"`
	Severity  ErrorSeverity `json:"severity"`
	Message   string        `json:"message"`
	Active    bool          `json:"active"`
	Timestamp time.Time     `json:"timestamp"`
}

// ---------------------------------------------------------------------------
// Status manager (status_manager.h)
// ---------------------------------------------------------------------------

type SystemState int

const (
	SystemStateNormal   SystemState = iota
	SystemStateWarning
	SystemStateError
	SystemStateSafeMode
	SystemStateFallback
)

func (s SystemState) String() string {
	return [...]string{"normal", "warning", "error", "safe_mode", "fallback"}[s]
}

// SystemStatus mirrors StatusManager::SystemStatus.
type SystemStatus struct {
	State        SystemState `json:"state"`
	DHTOk        bool        `json:"dht_ok"`
	RTCOk        bool        `json:"rtc_ok"`
	UptimeSeconds uint32     `json:"uptime_seconds"`
	RestartCount  uint32     `json:"restart_count"`
	Timestamp    time.Time   `json:"timestamp"`
}

// ---------------------------------------------------------------------------
// WiFi / network (wifi_manager.h)
// ---------------------------------------------------------------------------

type WiFiState int

const (
	WiFiStateBooting       WiFiState = iota
	WiFiStateBootRetry
	WiFiStateConnected
	WiFiStateReconnecting
	WiFiStateTempAP
	WiFiStatePersistentAP
)

func (w WiFiState) String() string {
	return [...]string{
		"booting", "boot_retry", "connected", "reconnecting", "temp_ap", "persistent_ap",
	}[w]
}

// DeviceIdentity holds mDNS / device discovery info.
type DeviceIdentity struct {
	TenantID   string    `json:"tenant_id"`
	DeviceID   string    `json:"device_id"`
	DeviceName string    `json:"device_name"`
	Hostname   string    `json:"hostname"`
	IPAddress  string    `json:"ip_address"`
	WiFiState  WiFiState `json:"wifi_state"`
}

// ---------------------------------------------------------------------------
// Alerts
// ---------------------------------------------------------------------------

// AlertRule defines a condition that triggers a notification channel when a
// sensor metric breaches a threshold on a specific tenant/device.
type AlertRule struct {
	ID              string     `json:"id"`
	TenantID        string     `json:"tenant_id"`
	DeviceID        string     `json:"device_id"`
	Metric          string     `json:"metric"`           // "temperature" | "humidity"
	Operator        string     `json:"operator"`         // "gt" | "lt" | "gte" | "lte"
	Threshold       float64    `json:"threshold"`
	Channel         string     `json:"channel"`          // "email" | "push"
	Recipient       string     `json:"recipient"`        // email address or push token
	Enabled         bool       `json:"enabled"`
	CooldownMinutes int        `json:"cooldown_minutes"` // default 15
	LastFired       *time.Time `json:"last_fired,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// ---------------------------------------------------------------------------
// Aggregated device snapshot (used for WebSocket broadcasts)
// ---------------------------------------------------------------------------

// DeviceSnapshot is a complete point-in-time snapshot broadcast over WebSocket
// and stored in the database.
type DeviceSnapshot struct {
	TenantID       string          `json:"tenant_id"`
	DeviceID       string          `json:"device_id"`
	Timestamp      time.Time       `json:"timestamp"`
	Sensor         SensorReading   `json:"sensor"`
	DeviceStates   DeviceStates    `json:"device_states"`
	OperationalMode OperationalMode `json:"operational_mode"`
	ActiveMode     ModeType        `json:"active_mode"`
	SystemStatus   SystemStatus    `json:"system_status"`
	Errors         []ErrorStatus   `json:"errors,omitempty"`
	FanSettings      FanSettings      `json:"fan_settings"`
	TempSettings     TempSettings     `json:"temp_settings"`
	HumiditySettings HumiditySettings `json:"humidity_settings"`
	LightSettings    LightSettings    `json:"light_settings"`
}
