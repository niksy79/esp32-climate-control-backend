// Package status mirrors StatusManager from status_manager.h.
// It tracks overall system health per tenant/device.
package status

import (
	"sync"
	"time"

	"climate-backend/internal/models"
)

// Manager tracks SystemStatus for all known tenant/device pairs.
type Manager struct {
	mu      sync.RWMutex
	devices map[string]*models.SystemStatus // key: tenantKey(tenantID, deviceID)
}

// New creates a Manager.
func New() *Manager {
	return &Manager{devices: make(map[string]*models.SystemStatus)}
}

// Update records the latest status for a tenant/device pair.
func (m *Manager) Update(tenantID, deviceID string, s models.SystemStatus) {
	if s.Timestamp.IsZero() {
		s.Timestamp = time.Now()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := s
	m.devices[tenantKey(tenantID, deviceID)] = &cp
}

// Get returns the most recent status for a tenant/device pair.
func (m *Manager) Get(tenantID, deviceID string) (models.SystemStatus, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.devices[tenantKey(tenantID, deviceID)]
	if !ok {
		return models.SystemStatus{}, false
	}
	return *s, true
}

// IsHealthy returns true when the system state is NORMAL or WARNING.
func (m *Manager) IsHealthy(tenantID, deviceID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.devices[tenantKey(tenantID, deviceID)]
	if !ok {
		return false
	}
	return s.State == models.SystemStateNormal || s.State == models.SystemStateWarning
}

// AllDeviceKeys returns all tracked keys as (tenantID, deviceID) pairs.
func (m *Manager) AllDeviceKeys() [][2]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([][2]string, 0, len(m.devices))
	for k := range m.devices {
		// key format: "tenantID/deviceID"
		for i := 0; i < len(k); i++ {
			if k[i] == '/' {
				out = append(out, [2]string{k[:i], k[i+1:]})
				break
			}
		}
	}
	return out
}

// OnSafeMode marks a device as entering safe mode.
func (m *Manager) OnSafeMode(tenantID, deviceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := tenantKey(tenantID, deviceID)
	s, ok := m.devices[k]
	if !ok {
		s = &models.SystemStatus{}
		m.devices[k] = s
	}
	s.State = models.SystemStateSafeMode
	s.Timestamp = time.Now()
}

// ---------------------------------------------------------------------------
// internal
// ---------------------------------------------------------------------------

func tenantKey(tenantID, deviceID string) string { return tenantID + "/" + deviceID }
