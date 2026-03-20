// Package errmanager mirrors ErrorManager from error_manager.h.
// It tracks active errors per tenant/device, keyed by ErrorType.
package errmanager

import (
	"sync"
	"time"

	"climate-backend/internal/models"
)

// Manager tracks errors for all tenant/device pairs.
type Manager struct {
	mu sync.RWMutex
	// devices[tenantKey(tenantID,deviceID)][errorType] → latest status
	devices map[string]map[models.ErrorType]*models.ErrorStatus
}

// New creates a Manager.
func New() *Manager {
	return &Manager{
		devices: make(map[string]map[models.ErrorType]*models.ErrorStatus),
	}
}

// SetError records or updates an error for a tenant/device pair.
func (m *Manager) SetError(tenantID, deviceID string, e models.ErrorStatus) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	dm := m.getOrCreate(tenantID, deviceID)
	cp := e
	dm[e.Type] = &cp
}

// ClearError marks an error as inactive.
func (m *Manager) ClearError(tenantID, deviceID string, et models.ErrorType) {
	m.mu.Lock()
	defer m.mu.Unlock()
	dm, ok := m.devices[tenantKey(tenantID, deviceID)]
	if !ok {
		return
	}
	if es, ok := dm[et]; ok {
		es.Active = false
	}
}

// ReplaceAll replaces all errors for a tenant/device with the provided list.
func (m *Manager) ReplaceAll(tenantID, deviceID string, errs []models.ErrorStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	dm := make(map[models.ErrorType]*models.ErrorStatus, len(errs))
	for i := range errs {
		cp := errs[i]
		dm[cp.Type] = &cp
	}
	m.devices[tenantKey(tenantID, deviceID)] = dm
}

// HasActiveErrors returns true when any error is currently active.
func (m *Manager) HasActiveErrors(tenantID, deviceID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	dm, ok := m.devices[tenantKey(tenantID, deviceID)]
	if !ok {
		return false
	}
	for _, e := range dm {
		if e.Active {
			return true
		}
	}
	return false
}

// HasCriticalErrors returns true when any ERROR-severity error is active.
func (m *Manager) HasCriticalErrors(tenantID, deviceID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	dm, ok := m.devices[tenantKey(tenantID, deviceID)]
	if !ok {
		return false
	}
	for _, e := range dm {
		if e.Active && e.Severity == models.SeverityError {
			return true
		}
	}
	return false
}

// GetActive returns all active errors for a tenant/device pair.
func (m *Manager) GetActive(tenantID, deviceID string) []models.ErrorStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	dm, ok := m.devices[tenantKey(tenantID, deviceID)]
	if !ok {
		return nil
	}
	var out []models.ErrorStatus
	for _, e := range dm {
		if e.Active {
			out = append(out, *e)
		}
	}
	return out
}

// GetAll returns all errors (active and inactive) for a tenant/device pair.
func (m *Manager) GetAll(tenantID, deviceID string) []models.ErrorStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	dm, ok := m.devices[tenantKey(tenantID, deviceID)]
	if !ok {
		return nil
	}
	out := make([]models.ErrorStatus, 0, len(dm))
	for _, e := range dm {
		out = append(out, *e)
	}
	return out
}

// ---------------------------------------------------------------------------
// internal
// ---------------------------------------------------------------------------

func tenantKey(tenantID, deviceID string) string { return tenantID + "/" + deviceID }

func (m *Manager) getOrCreate(tenantID, deviceID string) map[models.ErrorType]*models.ErrorStatus {
	k := tenantKey(tenantID, deviceID)
	dm, ok := m.devices[k]
	if !ok {
		dm = make(map[models.ErrorType]*models.ErrorStatus)
		m.devices[k] = dm
	}
	return dm
}
