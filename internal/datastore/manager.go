// Package datastore mirrors DataManager from data_manager.h.
// On the backend, readings are stored in PostgreSQL rather than LittleFS.
// The manager provides time-range queries keyed by tenant/device pair.
package datastore

import (
	"context"
	"time"

	"climate-backend/internal/db"
	"climate-backend/internal/models"
)

// MaxReadings is the maximum number of readings returned for a history query
// (mirrors DataManager::MAX_READINGS = 144).
const MaxReadings = 144

// Manager wraps the database and exposes DataManager-equivalent operations.
type Manager struct {
	db *db.DB
}

// New creates a Manager.
func New(database *db.DB) *Manager {
	return &Manager{db: database}
}

// AddReading stores a new reading for a tenant/device pair.
// If the device is not yet registered it is auto-registered with minimal fields,
// enabling ESP32 devices to self-register on first contact.
// Mirrors DataManager::addReading.
func (m *Manager) AddReading(ctx context.Context, tenantID, deviceID string, r models.Reading) error {
	// Ensure Timestamp is populated before reaching the DB layer so that both
	// the in-memory managers and the stored row agree on the same value.
	if r.Timestamp.IsZero() {
		r.Timestamp = time.Now().UTC()
	}
	if err := m.db.EnsureDevice(ctx, tenantID, deviceID); err != nil {
		return err
	}
	return m.db.InsertReading(ctx, tenantID, deviceID, r)
}

// AddCompressorCycle stores a compressor cycle record.
// Auto-registers the device if it is not yet known.
func (m *Manager) AddCompressorCycle(ctx context.Context, tenantID, deviceID string, c models.CompressorCycle) error {
	if err := m.db.EnsureDevice(ctx, tenantID, deviceID); err != nil {
		return err
	}
	return m.db.InsertCompressorCycle(ctx, tenantID, deviceID, c)
}

// GetHistory returns up to MaxReadings readings for a tenant/device pair in a time window.
// Mirrors DataManager::getHistoryJson / getReadingsForPeriod.
func (m *Manager) GetHistory(ctx context.Context, tenantID, deviceID string, from, to time.Time) ([]models.Reading, error) {
	return m.db.GetReadings(ctx, tenantID, deviceID, from, to, MaxReadings)
}

// GetLastDay returns the past 24 hours of data for a tenant/device pair.
func (m *Manager) GetLastDay(ctx context.Context, tenantID, deviceID string) ([]models.Reading, error) {
	now := time.Now()
	return m.GetHistory(ctx, tenantID, deviceID, now.Add(-24*time.Hour), now)
}

// GetLastNDays returns up to MaxReadings readings from the last n days.
// Mirrors DataManager::DAYS_TO_STORE = 31.
func (m *Manager) GetLastNDays(ctx context.Context, tenantID, deviceID string, n int) ([]models.Reading, error) {
	if n > 31 {
		n = 31
	}
	now := time.Now()
	return m.GetHistory(ctx, tenantID, deviceID, now.Add(-time.Duration(n)*24*time.Hour), now)
}
