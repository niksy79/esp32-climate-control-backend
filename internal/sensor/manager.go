// Package sensor mirrors SensorManager from sensor_manager.h.
// It tracks the latest sensor readings received from devices and
// provides health assessment logic.
package sensor

import (
	"strings"
	"sync"
	"time"

	"climate-backend/internal/models"
)

const (
	warningThresholdSec = 60  // > 1 min without reading → warning
	errorThresholdSec   = 300 // > 5 min without reading → error
)

// deviceState tracks sensor state per tenant/device.
type deviceState struct {
	latest          models.SensorReading
	consecutiveErrs int
}

// Manager keeps the latest sensor reading per tenant/device and evaluates health.
type Manager struct {
	mu      sync.RWMutex
	devices map[string]*deviceState // key: tenantKey(tenantID, deviceID)
}

// New creates a Manager.
func New() *Manager {
	return &Manager{devices: make(map[string]*deviceState)}
}

// UpdateReading records the latest reading from a device.
func (m *Manager) UpdateReading(tenantID, deviceID string, r models.Reading) {
	r.Timestamp = time.Now().UTC()
	sr := models.SensorReading{
		Temperature:  r.Temperature,
		Humidity:     r.Humidity,
		Timestamp:    r.Timestamp,
		FallbackTime: r.FallbackTime,
	}
	sr.Health = healthFromReading(r)

	m.mu.Lock()
	defer m.mu.Unlock()
	ds := m.getOrCreate(tenantID, deviceID)
	if sr.Health == models.SensorHealthError {
		ds.consecutiveErrs++
	} else {
		ds.consecutiveErrs = 0
	}
	ds.latest = sr
}

// GetLatest returns the most recent reading for a tenant/device pair.
// Health is recalculated dynamically based on how stale the reading is.
func (m *Manager) GetLatest(tenantID, deviceID string) (models.SensorReading, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ds, ok := m.devices[tenantKey(tenantID, deviceID)]
	if !ok {
		return models.SensorReading{}, false
	}
	sr := ds.latest
	age := time.Since(sr.Timestamp).Seconds()
	switch {
	case age > errorThresholdSec:
		sr.Health = models.SensorHealthError
	case age > warningThresholdSec:
		sr.Health = models.SensorHealthWarning
	default:
		sr.Health = models.SensorHealthGood
	}
	return sr, true
}

// Health returns the current sensor health, factoring in reading staleness.
func (m *Manager) Health(tenantID, deviceID string) models.SensorHealth {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ds, ok := m.devices[tenantKey(tenantID, deviceID)]
	if !ok {
		return models.SensorHealthError
	}
	age := time.Since(ds.latest.Timestamp).Seconds()
	switch {
	case age > errorThresholdSec:
		return models.SensorHealthError
	case age > warningThresholdSec:
		return models.SensorHealthWarning
	default:
		return ds.latest.Health
	}
}

// IsSensorOperational returns true when the sensor has produced a valid
// reading recently and has fewer than 3 consecutive errors.
func (m *Manager) IsSensorOperational(tenantID, deviceID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ds, ok := m.devices[tenantKey(tenantID, deviceID)]
	if !ok {
		return false
	}
	return ds.consecutiveErrs < 3 && time.Since(ds.latest.Timestamp) < errorThresholdSec*time.Second
}

// SeedFromDB bulk-populates in-memory state from DB readings loaded at startup.
// keys are "tenantID/deviceID" (same format as LoadActiveModes). The DB
// recorded_at timestamp is preserved so GetLatest computes true staleness;
// calling UpdateReading would overwrite it with time.Now() and mask stale devices.
// Called once before MQTT connects; live updates continue via UpdateReading.
func (m *Manager) SeedFromDB(readings map[string]models.Reading) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, r := range readings {
		parts := strings.SplitN(key, "/", 2)
		if len(parts) != 2 {
			continue
		}
		m.devices[key] = &deviceState{
			latest: models.SensorReading{
				Temperature:  r.Temperature,
				Humidity:     r.Humidity,
				Timestamp:    r.Timestamp,
				FallbackTime: r.FallbackTime,
				Health:       healthFromReading(r),
			},
		}
	}
}

// ---------------------------------------------------------------------------
// internal helpers
// ---------------------------------------------------------------------------

func tenantKey(tenantID, deviceID string) string { return tenantID + "/" + deviceID }

func (m *Manager) getOrCreate(tenantID, deviceID string) *deviceState {
	k := tenantKey(tenantID, deviceID)
	ds, ok := m.devices[k]
	if !ok {
		ds = &deviceState{}
		m.devices[k] = ds
	}
	return ds
}

// healthFromReading applies simple range checks that mirror the ESP32 logic.
func healthFromReading(r models.Reading) models.SensorHealth {
	if r.Temperature < -40 || r.Temperature > 125 {
		return models.SensorHealthError
	}
	if r.Humidity < 0 || r.Humidity > 100 {
		return models.SensorHealthError
	}
	if r.Temperature < -35 || r.Temperature > 120 || r.Humidity < 5 || r.Humidity > 95 {
		return models.SensorHealthWarning
	}
	return models.SensorHealthGood
}
