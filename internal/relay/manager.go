// Package relay mirrors RelayManager from relay_manager.h.
// On the backend it tracks the logical state of each relay per tenant/device
// and enforces minimum on/off timing rules.
package relay

import (
	"fmt"
	"sync"
	"time"

	"climate-backend/internal/models"
)

const (
	defaultMinOnTime  = 3 * time.Minute
	defaultMinOffTime = 3 * time.Minute
)

// relayState holds the current and historical state of a single relay.
type relayState struct {
	info       models.RelayInfo
	minOnTime  time.Duration
	minOffTime time.Duration
}

// deviceRelays holds all relays for one tenant/device.
type deviceRelays struct {
	relays map[models.RelayType]*relayState
}

// Manager tracks relay states across all tenant/device pairs.
type Manager struct {
	mu      sync.RWMutex
	devices map[string]*deviceRelays // key: tenantKey(tenantID, deviceID)
}

// New creates a Manager.
func New() *Manager {
	return &Manager{devices: make(map[string]*deviceRelays)}
}

// UpdateStates updates all relay states for a tenant/device from a DeviceStates snapshot.
func (m *Manager) UpdateStates(tenantID, deviceID string, ds models.DeviceStates) {
	m.mu.Lock()
	defer m.mu.Unlock()

	dr := m.getOrCreate(tenantID, deviceID)
	updates := map[models.RelayType]bool{
		models.RelayCompressor:    ds.Compressor,
		models.RelayFanCompressor: ds.FanCompressor,
		models.RelayExtraFan:      ds.ExtraFan,
		models.RelayLight:         ds.Light,
		models.RelayHeating:       ds.Heating,
		models.RelayDehumidifier:  ds.Dehumidifier,
	}
	for rt, state := range updates {
		rs := dr.getOrCreateRelay(rt)
		if rs.info.State != state {
			rs.info.State = state
			rs.info.LastChanged = time.Now()
		}
	}
}

// GetRelayInfo returns current state of a relay.
func (m *Manager) GetRelayInfo(tenantID, deviceID string, rt models.RelayType) (models.RelayInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	dr, ok := m.devices[tenantKey(tenantID, deviceID)]
	if !ok {
		return models.RelayInfo{}, fmt.Errorf("relay: unknown device %q/%q", tenantID, deviceID)
	}
	rs, ok := dr.relays[rt]
	if !ok {
		return models.RelayInfo{}, fmt.Errorf("relay: unknown relay %v for device %q/%q", rt, tenantID, deviceID)
	}
	return rs.info, nil
}

// CanToggle returns true when the relay has been in its current state long
// enough to satisfy the minimum on/off timing constraint.
func (m *Manager) CanToggle(tenantID, deviceID string, rt models.RelayType) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	dr, ok := m.devices[tenantKey(tenantID, deviceID)]
	if !ok {
		return true
	}
	rs, ok := dr.relays[rt]
	if !ok {
		return true
	}
	age := time.Since(rs.info.LastChanged)
	if rs.info.State {
		return age >= rs.minOnTime
	}
	return age >= rs.minOffTime
}

// GetAllStates returns a DeviceStates snapshot for a tenant/device pair.
func (m *Manager) GetAllStates(tenantID, deviceID string) models.DeviceStates {
	m.mu.RLock()
	defer m.mu.RUnlock()
	dr, ok := m.devices[tenantKey(tenantID, deviceID)]
	if !ok {
		return models.DeviceStates{}
	}
	ds := models.DeviceStates{}
	for rt, rs := range dr.relays {
		switch rt {
		case models.RelayCompressor:
			ds.Compressor = rs.info.State
		case models.RelayFanCompressor:
			ds.FanCompressor = rs.info.State
		case models.RelayExtraFan:
			ds.ExtraFan = rs.info.State
		case models.RelayLight:
			ds.Light = rs.info.State
		case models.RelayHeating:
			ds.Heating = rs.info.State
		case models.RelayDehumidifier:
			ds.Dehumidifier = rs.info.State
		}
	}
	return ds
}

// ---------------------------------------------------------------------------
// internal
// ---------------------------------------------------------------------------

func tenantKey(tenantID, deviceID string) string { return tenantID + "/" + deviceID }

func (m *Manager) getOrCreate(tenantID, deviceID string) *deviceRelays {
	k := tenantKey(tenantID, deviceID)
	dr, ok := m.devices[k]
	if !ok {
		dr = &deviceRelays{relays: make(map[models.RelayType]*relayState)}
		m.devices[k] = dr
	}
	return dr
}

func (dr *deviceRelays) getOrCreateRelay(rt models.RelayType) *relayState {
	rs, ok := dr.relays[rt]
	if !ok {
		rs = &relayState{
			info:       models.RelayInfo{Type: rt},
			minOnTime:  defaultMinOnTime,
			minOffTime: defaultMinOffTime,
		}
		dr.relays[rt] = rs
	}
	return rs
}
