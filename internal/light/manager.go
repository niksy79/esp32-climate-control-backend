// Package light mirrors LightManager from light_manager.h.
// On the backend it tracks light mode and state per tenant/device.
package light

import (
	"sync"

	"climate-backend/internal/models"
)

// deviceLight tracks light state for one tenant/device.
type deviceLight struct {
	settings models.LightSettings
}

// Manager tracks light settings across all tenant/device pairs.
type Manager struct {
	mu      sync.RWMutex
	devices map[string]*deviceLight // key: tenantKey(tenantID, deviceID)
}

// New creates a Manager.
func New() *Manager {
	return &Manager{devices: make(map[string]*deviceLight)}
}

// UpdateSettings stores the latest light settings for a tenant/device pair.
func (m *Manager) UpdateSettings(tenantID, deviceID string, ls models.LightSettings) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getOrCreate(tenantID, deviceID).settings = ls
}

// GetSettings returns the current light settings for a tenant/device pair.
func (m *Manager) GetSettings(tenantID, deviceID string) (models.LightSettings, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	dl, ok := m.devices[tenantKey(tenantID, deviceID)]
	if !ok {
		return models.LightSettings{}, false
	}
	return dl.settings, true
}

// SetMode changes the light mode (manual / auto).
func (m *Manager) SetMode(tenantID, deviceID string, mode models.LightMode) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getOrCreate(tenantID, deviceID).settings.Mode = mode
}

// SetLight changes the on/off state.
func (m *Manager) SetLight(tenantID, deviceID string, on bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getOrCreate(tenantID, deviceID).settings.State = on
}

// Toggle flips the current state.
func (m *Manager) Toggle(tenantID, deviceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	dl := m.getOrCreate(tenantID, deviceID)
	dl.settings.State = !dl.settings.State
}

// IsOn returns true when the light is on.
func (m *Manager) IsOn(tenantID, deviceID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	dl, ok := m.devices[tenantKey(tenantID, deviceID)]
	if !ok {
		return false
	}
	return dl.settings.State
}

// ---------------------------------------------------------------------------
// internal
// ---------------------------------------------------------------------------

func tenantKey(tenantID, deviceID string) string { return tenantID + "/" + deviceID }

func (m *Manager) getOrCreate(tenantID, deviceID string) *deviceLight {
	k := tenantKey(tenantID, deviceID)
	dl, ok := m.devices[k]
	if !ok {
		dl = &deviceLight{}
		m.devices[k] = dl
	}
	return dl
}
