// Package fan mirrors FanManager from fan_manager.h.
// On the backend it tracks fan settings and mixing state per tenant/device.
package fan

import (
	"sync"
	"time"

	"climate-backend/internal/models"
)

// mixingState tracks the current mixing cycle for a device.
type mixingState struct {
	active    bool
	startedAt time.Time
}

// deviceFan holds fan-related state for one tenant/device.
type deviceFan struct {
	settings models.FanSettings
	mixing   mixingState
}

// Manager tracks fan settings and mixing state across tenant/device pairs.
type Manager struct {
	mu      sync.RWMutex
	devices map[string]*deviceFan // key: tenantKey(tenantID, deviceID)
}

// New creates a Manager.
func New() *Manager {
	return &Manager{devices: make(map[string]*deviceFan)}
}

// UpdateSettings stores the latest fan settings for a tenant/device pair.
func (m *Manager) UpdateSettings(tenantID, deviceID string, fs models.FanSettings) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getOrCreate(tenantID, deviceID).settings = fs
}

// GetSettings returns the current fan settings for a tenant/device pair.
func (m *Manager) GetSettings(tenantID, deviceID string) (models.FanSettings, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	df, ok := m.devices[tenantKey(tenantID, deviceID)]
	if !ok {
		return models.FanSettings{}, false
	}
	return df.settings, true
}

// StartMixing marks the device as entering a mixing cycle.
func (m *Manager) StartMixing(tenantID, deviceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getOrCreate(tenantID, deviceID).mixing = mixingState{active: true, startedAt: time.Now()}
}

// StopMixing ends the current mixing cycle.
func (m *Manager) StopMixing(tenantID, deviceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	df, ok := m.devices[tenantKey(tenantID, deviceID)]
	if !ok {
		return
	}
	df.mixing.active = false
}

// IsMixing returns true when a device is in an active mixing cycle.
func (m *Manager) IsMixing(tenantID, deviceID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	df, ok := m.devices[tenantKey(tenantID, deviceID)]
	if !ok {
		return false
	}
	return df.mixing.active
}

// MixingDuration returns how long the current mixing cycle has been running.
func (m *Manager) MixingDuration(tenantID, deviceID string) time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	df, ok := m.devices[tenantKey(tenantID, deviceID)]
	if !ok || !df.mixing.active {
		return 0
	}
	return time.Since(df.mixing.startedAt)
}

// ---------------------------------------------------------------------------
// internal
// ---------------------------------------------------------------------------

func tenantKey(tenantID, deviceID string) string { return tenantID + "/" + deviceID }

func (m *Manager) getOrCreate(tenantID, deviceID string) *deviceFan {
	k := tenantKey(tenantID, deviceID)
	df, ok := m.devices[k]
	if !ok {
		df = &deviceFan{
			settings: models.FanSettings{
				Speed:          50,
				MixingInterval: 3600,
				MixingDuration: 300,
				MixingEnabled:  true,
			},
		}
		m.devices[k] = df
	}
	return df
}
