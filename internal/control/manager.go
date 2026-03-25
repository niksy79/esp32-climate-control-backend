// Package control mirrors ControlManager from control_manager.h.
// On the backend it tracks the operational mode, active product mode,
// device states, and compressor statistics per tenant/device.
package control

import (
	"sync"
	"time"

	"climate-backend/internal/models"
)

// DeviceControl holds all control-related state for one device.
type DeviceControl struct {
	OperationalMode models.OperationalMode
	ActiveMode      models.ModeType
	ModeSettings    models.ModeSettings
	DeviceStates    models.DeviceStates
	CompressorStats models.CompressorStats
	FallbackStats   models.FallbackStatistics
	LastUpdated     time.Time
}

// Manager tracks control state for all known tenant/device pairs.
type Manager struct {
	mu      sync.RWMutex
	devices map[string]*DeviceControl // key: tenantKey(tenantID, deviceID)
}

// New creates a Manager.
func New() *Manager {
	return &Manager{devices: make(map[string]*DeviceControl)}
}

// UpdateDeviceStates records the latest relay/device states from a snapshot.
func (m *Manager) UpdateDeviceStates(tenantID, deviceID string, ds models.DeviceStates) {
	m.mu.Lock()
	defer m.mu.Unlock()
	dc := m.getOrCreate(tenantID, deviceID)
	dc.DeviceStates = ds
	dc.LastUpdated = time.Now()
}

// UpdateSnapshot updates mode and settings from a full device snapshot.
func (m *Manager) UpdateSnapshot(tenantID, deviceID string, snap models.DeviceSnapshot) {
	m.mu.Lock()
	defer m.mu.Unlock()
	dc := m.getOrCreate(tenantID, deviceID)
	dc.OperationalMode = snap.OperationalMode
	dc.ActiveMode = snap.ActiveMode
	dc.DeviceStates = snap.DeviceStates
	dc.LastUpdated = time.Now()
}

// SeedActiveModes bulk-sets active modes from a "tenantID/deviceID" → ModeType
// map loaded from the database. Called once at startup before MQTT connects.
func (m *Manager) SeedActiveModes(modes map[string]models.ModeType) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, mode := range modes {
		dc, ok := m.devices[key]
		if !ok {
			dc = &DeviceControl{}
			m.devices[key] = dc
		}
		dc.ActiveMode = mode
	}
}

// SetActiveMode updates only the active mode for a tenant/device pair.
func (m *Manager) SetActiveMode(tenantID, deviceID string, mode models.ModeType) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getOrCreate(tenantID, deviceID).ActiveMode = mode
}

// RecordCompressorCycle updates running compressor statistics.
func (m *Manager) RecordCompressorCycle(tenantID, deviceID string, cyc models.CompressorCycle) {
	m.mu.Lock()
	defer m.mu.Unlock()
	dc := m.getOrCreate(tenantID, deviceID)
	dc.CompressorStats.CycleCount++
	dc.CompressorStats.WorkTime += cyc.WorkTime
	dc.CompressorStats.RestTime += cyc.RestTime
}

// GetControl returns the control state for a tenant/device pair.
func (m *Manager) GetControl(tenantID, deviceID string) (DeviceControl, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	dc, ok := m.devices[tenantKey(tenantID, deviceID)]
	if !ok {
		return DeviceControl{}, false
	}
	return *dc, true
}

// IsCompressorRunning returns true when the compressor relay is active.
func (m *Manager) IsCompressorRunning(tenantID, deviceID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	dc, ok := m.devices[tenantKey(tenantID, deviceID)]
	if !ok {
		return false
	}
	return dc.DeviceStates.Compressor
}

// ---------------------------------------------------------------------------
// Fallback mode tracking
// ---------------------------------------------------------------------------

// EnterFallback marks a device as being in fallback mode.
func (m *Manager) EnterFallback(tenantID, deviceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	dc := m.getOrCreate(tenantID, deviceID)
	dc.OperationalMode = models.OperationalModeFallback
	dc.FallbackStats.EnterCount++
	dc.FallbackStats.LastEntered = time.Now()
}

// ExitFallback marks a device as having left fallback mode and accumulates duration.
func (m *Manager) ExitFallback(tenantID, deviceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	dc, ok := m.devices[tenantKey(tenantID, deviceID)]
	if !ok {
		return
	}
	if !dc.FallbackStats.LastEntered.IsZero() {
		dur := uint32(time.Since(dc.FallbackStats.LastEntered).Seconds())
		dc.FallbackStats.TotalDuration += dur
	}
	dc.OperationalMode = models.OperationalModeNormal
}

// ---------------------------------------------------------------------------
// internal
// ---------------------------------------------------------------------------

func tenantKey(tenantID, deviceID string) string { return tenantID + "/" + deviceID }

func (m *Manager) getOrCreate(tenantID, deviceID string) *DeviceControl {
	k := tenantKey(tenantID, deviceID)
	dc, ok := m.devices[k]
	if !ok {
		dc = &DeviceControl{}
		m.devices[k] = dc
	}
	return dc
}
