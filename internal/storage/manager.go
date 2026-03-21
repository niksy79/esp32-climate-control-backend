// Package storage mirrors StorageManager from storage_manager.h.
// On the backend, settings are persisted to PostgreSQL instead of ESP32 Preferences.
package storage

import (
	"context"
	"errors"
	"log"
	"sync"

	"climate-backend/internal/db"
	"climate-backend/internal/models"
)

// Manager caches settings in memory and persists them to the database.
type Manager struct {
	mu      sync.RWMutex
	db      *db.DB
	devices map[string]*deviceSettings // key: tenantKey(tenantID, deviceID)
}

type deviceSettings struct {
	temp       models.TempSettings
	humidity   models.HumiditySettings
	fan        models.FanSettings
	light      models.LightSettings
	system     models.SystemSettings
	display    models.DisplaySettings
	mode       models.OperationalMode
	activeMode models.ModeType
}

// New creates a Manager backed by the given database.
func New(database *db.DB) *Manager {
	return &Manager{
		db:      database,
		devices: make(map[string]*deviceSettings),
	}
}

// LoadSettings returns all settings for a tenant/device pair.
// On cache miss it loads from the database before returning.
func (m *Manager) LoadSettings(ctx context.Context, tenantID, deviceID string) (
	models.TempSettings, models.HumiditySettings, models.FanSettings,
	models.LightSettings, models.SystemSettings, models.DisplaySettings,
) {
	m.mu.RLock()
	_, cached := m.devices[tenantKey(tenantID, deviceID)]
	m.mu.RUnlock()

	if !cached && m.db != nil {
		ts, hs, fs, ls, opMode, activeMode, err := m.db.GetSettings(ctx, tenantID, deviceID)
		if err != nil && !errors.Is(err, db.ErrNoRows) {
			log.Printf("storage: load settings %s/%s: %v", tenantID, deviceID, err)
		} else if err == nil {
			m.mu.Lock()
			ds := m.getOrCreate(tenantID, deviceID)
			ds.temp = ts
			ds.humidity = hs
			ds.fan = fs
			ds.light = ls
			ds.mode = opMode
			ds.activeMode = activeMode
			m.mu.Unlock()
		}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	ds := m.getOrCreate(tenantID, deviceID)
	return ds.temp, ds.humidity, ds.fan, ds.light, ds.system, ds.display
}

// SaveTempSettings updates temperature settings and persists them.
func (m *Manager) SaveTempSettings(ctx context.Context, tenantID, deviceID string, ts models.TempSettings) error {
	m.mu.Lock()
	m.getOrCreate(tenantID, deviceID).temp = ts
	ds := m.devices[tenantKey(tenantID, deviceID)]
	m.mu.Unlock()
	return m.persist(ctx, tenantID, deviceID, ds)
}

// SaveHumiditySettings updates humidity settings and persists them.
func (m *Manager) SaveHumiditySettings(ctx context.Context, tenantID, deviceID string, hs models.HumiditySettings) error {
	m.mu.Lock()
	m.getOrCreate(tenantID, deviceID).humidity = hs
	ds := m.devices[tenantKey(tenantID, deviceID)]
	m.mu.Unlock()
	return m.persist(ctx, tenantID, deviceID, ds)
}

// SaveFanSettings updates fan settings and persists them.
func (m *Manager) SaveFanSettings(ctx context.Context, tenantID, deviceID string, fs models.FanSettings) error {
	m.mu.Lock()
	m.getOrCreate(tenantID, deviceID).fan = fs
	ds := m.devices[tenantKey(tenantID, deviceID)]
	m.mu.Unlock()
	return m.persist(ctx, tenantID, deviceID, ds)
}

// SaveLightSettings updates light settings and persists them.
func (m *Manager) SaveLightSettings(ctx context.Context, tenantID, deviceID string, ls models.LightSettings) error {
	m.mu.Lock()
	m.getOrCreate(tenantID, deviceID).light = ls
	ds := m.devices[tenantKey(tenantID, deviceID)]
	m.mu.Unlock()
	return m.persist(ctx, tenantID, deviceID, ds)
}

// UpdateFromSnapshot ingests a full device snapshot and refreshes the cache.
func (m *Manager) UpdateFromSnapshot(ctx context.Context, tenantID, deviceID string, snap models.DeviceSnapshot) error {
	m.mu.Lock()
	ds := m.getOrCreate(tenantID, deviceID)
	ds.temp = snap.TempSettings
	ds.humidity = snap.HumiditySettings
	ds.fan = snap.FanSettings
	ds.light = snap.LightSettings
	ds.mode = snap.OperationalMode
	ds.activeMode = snap.ActiveMode
	m.mu.Unlock()
	return m.persist(ctx, tenantID, deviceID, ds)
}

// ---------------------------------------------------------------------------
// internal
// ---------------------------------------------------------------------------

func tenantKey(tenantID, deviceID string) string { return tenantID + "/" + deviceID }

func (m *Manager) getOrCreate(tenantID, deviceID string) *deviceSettings {
	k := tenantKey(tenantID, deviceID)
	ds, ok := m.devices[k]
	if !ok {
		ds = &deviceSettings{
			temp:     models.TempSettings{Target: 4.0},
			humidity: models.HumiditySettings{Target: 80.0},
			fan: models.FanSettings{
				Speed: 50, MixingInterval: 3600, MixingDuration: 300, MixingEnabled: true,
			},
			display: models.DisplaySettings{Brightness: 80, SleepTimeout: 30, AutoSleep: true},
		}
		m.devices[k] = ds
	}
	return ds
}

func (m *Manager) persist(ctx context.Context, tenantID, deviceID string, ds *deviceSettings) error {
	if m.db == nil {
		return nil
	}
	return m.db.UpsertSettings(ctx, tenantID, deviceID,
		ds.temp, ds.humidity, ds.fan, ds.light,
		ds.mode, ds.activeMode,
	)
}
