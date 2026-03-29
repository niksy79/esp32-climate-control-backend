// Package models defines Go structs and enums that mirror the C++ data structures
// from the ESP32 climate controller firmware.
package models

import (
	"encoding/json"
	"fmt"
	"time"
)

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
	Temperature  float32      `json:"temperature"`
	Humidity     float32      `json:"humidity"`
	Timestamp    time.Time    `json:"timestamp"`
	FallbackTime uint32       `json:"fallback_time"` // 0 = not in fallback, >0 = seconds in fallback
	Health       SensorHealth `json:"health"`
}

// ---------------------------------------------------------------------------
// Data store (data_manager.h)
// ---------------------------------------------------------------------------

// Reading mirrors DataManager::Reading.
type Reading struct {
	Temperature  float32   `json:"temperature"`
	Humidity     float32   `json:"humidity"`
	Timestamp    time.Time `json:"timestamp"`
	FallbackTime uint32    `json:"fallback_time"`
}

// UnmarshalJSON accepts fallback_time as V1 bool or V2 int, and timestamp
// with or without timezone suffix.
func (r *Reading) UnmarshalJSON(data []byte) error {
	type raw struct {
		Temperature  float32         `json:"temperature"`
		Humidity     float32         `json:"humidity"`
		Timestamp    json.RawMessage `json:"timestamp"`
		FallbackTime json.RawMessage `json:"fallback_time"`
	}
	var v raw
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	r.Temperature = v.Temperature
	r.Humidity = v.Humidity
	// Parse timestamp: try RFC3339, then without timezone
	if len(v.Timestamp) > 0 {
		var ts time.Time
		if json.Unmarshal(v.Timestamp, &ts) == nil {
			r.Timestamp = ts
		} else {
			var s string
			if json.Unmarshal(v.Timestamp, &s) == nil {
				if t, err := time.Parse("2006-01-02T15:04:05", s); err == nil {
					r.Timestamp = t
				}
			}
		}
	}
	// Parse fallback_time: bool (V1) or int (V2)
	if len(v.FallbackTime) > 0 {
		var b bool
		if json.Unmarshal(v.FallbackTime, &b) == nil {
			if b {
				r.FallbackTime = 1
			}
			return nil
		}
		var n uint32
		if json.Unmarshal(v.FallbackTime, &n) == nil {
			r.FallbackTime = n
		}
	}
	return nil
}

// CompressorCycle mirrors DataManager::CompressorCycle.
type CompressorCycle struct {
	WorkTime  uint32    `json:"work_time"`
	RestTime  uint32    `json:"rest_time"`
	Temp      float32   `json:"temperature"`
	Humidity  float32   `json:"humidity"`
	CreatedAt time.Time `json:"created_at"`
}

// MetricReading е резултат от device_readings при GET history?metric=
type MetricReading struct {
	Value      float32   `json:"value"`
	RecordedAt time.Time `json:"recorded_at"`
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
	Speed          uint8  `json:"speed"`            // 0–100 %
	MixingInterval uint32 `json:"mixing_interval"`  // minutes
	MixingDuration uint32 `json:"mixing_duration"`  // minutes
	MixingEnabled  bool   `json:"mixing_enabled"`
}

type LightMode string

const (
	LightModeManual LightMode = "manual"
	LightModeAuto   LightMode = "auto"
)

// UnmarshalJSON accepts both V1 int (0=manual, 1=auto) and V2 string.
func (l *LightMode) UnmarshalJSON(data []byte) error {
	var s string
	if json.Unmarshal(data, &s) == nil {
		*l = LightMode(s)
		return nil
	}
	var n int
	if json.Unmarshal(data, &n) == nil {
		if n == 1 {
			*l = LightModeAuto
		} else {
			*l = LightModeManual
		}
		return nil
	}
	return fmt.Errorf("light_mode: expected string or int, got %s", data)
}

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
	ModeNormal          ModeType = 0
	ModeHeating         ModeType = 1
	ModeBeercooling     ModeType = 2
	ModeRoomTemp        ModeType = 3
	ModeProductMeatFish ModeType = 10
	ModeProductDairy    ModeType = 11
	ModeProductReadyFood ModeType = 12
	ModeProductVegetables ModeType = 13
)

func (m ModeType) String() string {
	switch m {
	case ModeNormal:
		return "normal"
	case ModeHeating:
		return "heating"
	case ModeBeercooling:
		return "beer_cooling"
	case ModeRoomTemp:
		return "room_temp"
	case ModeProductMeatFish:
		return "product_meat_fish"
	case ModeProductDairy:
		return "product_dairy"
	case ModeProductReadyFood:
		return "product_ready_food"
	case ModeProductVegetables:
		return "product_vegetables"
	default:
		return "unknown"
	}
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

type SystemState string

const (
	SystemStateNormal   SystemState = "NORMAL"
	SystemStateWarning  SystemState = "WARNING"
	SystemStateError    SystemState = "ERROR"
	SystemStateSafeMode SystemState = "SAFE_MODE"
	SystemStateFallback SystemState = "FALLBACK"
)

func (s SystemState) String() string {
	return string(s)
}

// UnmarshalJSON accepts both V1 int and V2 string representations.
func (s *SystemState) UnmarshalJSON(data []byte) error {
	var str string
	if json.Unmarshal(data, &str) == nil {
		*s = SystemState(str)
		return nil
	}
	var n int
	if json.Unmarshal(data, &n) == nil {
		switch n {
		case 0:
			*s = SystemStateNormal
		case 1:
			*s = SystemStateWarning
		case 2:
			*s = SystemStateError
		case 3:
			*s = SystemStateSafeMode
		case 4:
			*s = SystemStateFallback
		default:
			*s = SystemStateNormal
		}
		return nil
	}
	return fmt.Errorf("system_state: expected string or int, got %s", data)
}

// SystemStatus mirrors StatusManager::SystemStatus.
type SystemStatus struct {
	State        SystemState `json:"state"`
	DHTOk        bool        `json:"dht_ok"`
	RTCOk        bool        `json:"rtc_ok"`
	UptimeSeconds uint32     `json:"uptime_seconds"`
	RestartCount  int32      `json:"restart_count"`
	Timestamp    time.Time   `json:"timestamp"`
}

// ---------------------------------------------------------------------------
// WiFi / network (wifi_manager.h)
// ---------------------------------------------------------------------------

type WiFiState string

const (
	WiFiStateBooting       WiFiState = "BOOTING"
	WiFiStateBootRetry     WiFiState = "BOOT_RETRY"
	WiFiStateConnected     WiFiState = "CONNECTED"
	WiFiStateReconnecting  WiFiState = "RECONNECTING"
	WiFiStateTempAP        WiFiState = "TEMP_AP"
	WiFiStatePersistentAP  WiFiState = "PERSISTENT_AP"
)

func (w WiFiState) String() string {
	return string(w)
}

// UnmarshalJSON accepts both V1 int and V2 string representations.
func (w *WiFiState) UnmarshalJSON(data []byte) error {
	var s string
	if json.Unmarshal(data, &s) == nil {
		*w = WiFiState(s)
		return nil
	}
	var n int
	if json.Unmarshal(data, &n) == nil {
		switch n {
		case 0:
			*w = WiFiStateBooting
		case 1:
			*w = WiFiStateBootRetry
		case 2:
			*w = WiFiStateConnected
		case 3:
			*w = WiFiStateReconnecting
		case 4:
			*w = WiFiStateTempAP
		case 5:
			*w = WiFiStatePersistentAP
		default:
			*w = WiFiState(fmt.Sprintf("UNKNOWN_%d", n))
		}
		return nil
	}
	return fmt.Errorf("wifi_state: expected string or int, got %s", data)
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
// Device type registry
// ---------------------------------------------------------------------------

// MetricDefinition describes a sensor metric exposed by a device type.
type MetricDefinition struct {
	ID           int64  `json:"id"`
	DeviceTypeID string `json:"device_type_id"`
	Name         string `json:"name"`
	DisplayName  string `json:"display_name"`
	Unit         string `json:"unit"`
	DataType     string `json:"data_type"`
	SortOrder    int    `json:"sort_order"`
}

// CommandDefinition describes a command that can be sent to a device type.
type CommandDefinition struct {
	ID            int64  `json:"id"`
	DeviceTypeID  string `json:"device_type_id"`
	Name          string `json:"name"`
	DisplayName   string `json:"display_name"`
	PayloadSchema string `json:"payload_schema"`
	SortOrder     int    `json:"sort_order"`
}

// DeviceType groups metric and command definitions for a class of device.
type DeviceType struct {
	ID          string              `json:"id"`
	DisplayName string              `json:"display_name"`
	Description string              `json:"description"`
	Metrics     []MetricDefinition  `json:"metrics,omitempty"`
	Commands    []CommandDefinition `json:"commands,omitempty"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

// DeviceSummary is returned by the list-devices endpoint.
// It includes the device_type_id so the frontend can display type labels
// without a per-device follow-up request.
type DeviceSummary struct {
	DeviceID     string `json:"device_id"`
	DeviceTypeID string `json:"device_type_id"`
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
