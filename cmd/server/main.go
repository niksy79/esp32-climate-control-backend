package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"

	"climate-backend/internal/api"
	"climate-backend/internal/auth"
	"climate-backend/internal/control"
	"climate-backend/internal/datastore"
	"climate-backend/internal/db"
	"climate-backend/internal/errmanager"
	"climate-backend/internal/fan"
	"climate-backend/internal/light"
	"climate-backend/internal/models"
	mqttclient "climate-backend/internal/mqtt"
	"climate-backend/internal/relay"
	"climate-backend/internal/sensor"
	"climate-backend/internal/status"
	"climate-backend/internal/storage"
	"climate-backend/internal/ws"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// -----------------------------------------------------------------
	// .env.local — optional local overrides, never committed
	// Loaded before os.Getenv calls so it can supply JWT_SECRET etc.
	// Silently ignored when the file does not exist.
	// -----------------------------------------------------------------
	if err := godotenv.Load(".env.local"); err != nil && !os.IsNotExist(err) {
		log.Printf("warning: could not load .env.local: %v", err)
	}

	// -----------------------------------------------------------------
	// Config from environment
	// -----------------------------------------------------------------
	dbDSN := envOr("DATABASE_URL", "postgres://climate:climate@localhost:5432/climate?sslmode=disable")
	mqttURL := envOr("MQTT_URL", "tcp://localhost:1883")
	listenAddr := envOr("LISTEN_ADDR", ":8080")
	jwtSecret := envOr("JWT_SECRET", "")

	// -----------------------------------------------------------------
	// Database
	// -----------------------------------------------------------------
	database, err := db.New(ctx, dbDSN)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer database.Close()
	log.Printf("database connected")

	// -----------------------------------------------------------------
	// Auth
	// -----------------------------------------------------------------
	authSvc, err := auth.NewService(jwtSecret)
	if err != nil {
		log.Fatalf("auth: %v", err)
	}
	authHandler := auth.NewHandler(authSvc, database)

	// -----------------------------------------------------------------
	// Managers
	// -----------------------------------------------------------------
	sensorMgr := sensor.New()
	controlMgr := control.New()
	statusMgr := status.New()
	errorMgr := errmanager.New()
	relayMgr := relay.New()
	fanMgr := fan.New()
	lightMgr := light.New()
	storageMgr := storage.New(database)
	datastoreMgr := datastore.New(database)
	hub := ws.NewHub()

	// -----------------------------------------------------------------
	// MQTT – topics: climate/<tenant_id>/<device_id>/<subtopic>
	// -----------------------------------------------------------------
	mqttCli, err := mqttclient.New(mqttclient.Config{
		BrokerURL: mqttURL,
		ClientID:  "climate-backend",
	}, mqttclient.Handlers{
		OnSensorReading: func(tenantID, deviceID string, r models.Reading) {
			sensorMgr.UpdateReading(tenantID, deviceID, r)
			if err := datastoreMgr.AddReading(ctx, tenantID, deviceID, r); err != nil {
				log.Printf("datastore: add reading %s/%s: %v", tenantID, deviceID, err)
			}
			snap := buildSnapshot(tenantID, deviceID, r, sensorMgr, controlMgr, statusMgr, errorMgr, fanMgr)
			hub.Broadcast(snap)
		},

		OnSystemStatus: func(tenantID, deviceID string, s models.SystemStatus) {
			statusMgr.Update(tenantID, deviceID, s)
			if err := database.InsertSystemStatus(ctx, tenantID, deviceID, s); err != nil {
				log.Printf("db: insert status %s/%s: %v", tenantID, deviceID, err)
			}
		},

		OnDeviceStates: func(tenantID, deviceID string, ds models.DeviceStates) {
			controlMgr.UpdateDeviceStates(tenantID, deviceID, ds)
			relayMgr.UpdateStates(tenantID, deviceID, ds)
			lightMgr.SetLight(tenantID, deviceID, ds.Light)
		},

		OnSettings: func(tenantID, deviceID string, snap models.DeviceSnapshot) {
			controlMgr.UpdateSnapshot(tenantID, deviceID, snap)
			fanMgr.UpdateSettings(tenantID, deviceID, snap.FanSettings)
			lightMgr.UpdateSettings(tenantID, deviceID, snap.LightSettings)
			if err := storageMgr.UpdateFromSnapshot(ctx, tenantID, deviceID, snap); err != nil {
				log.Printf("storage: save settings %s/%s: %v", tenantID, deviceID, err)
			}
		},

		OnErrors: func(tenantID, deviceID string, errs []models.ErrorStatus) {
			errorMgr.ReplaceAll(tenantID, deviceID, errs)
			for _, e := range errs {
				if err := database.InsertError(ctx, tenantID, deviceID, e); err != nil {
					log.Printf("db: insert error %s/%s: %v", tenantID, deviceID, err)
				}
			}
		},

		OnCompressorCycle: func(tenantID, deviceID string, c models.CompressorCycle) {
			controlMgr.RecordCompressorCycle(tenantID, deviceID, c)
			if err := datastoreMgr.AddCompressorCycle(ctx, tenantID, deviceID, c); err != nil {
				log.Printf("datastore: add cycle %s/%s: %v", tenantID, deviceID, err)
			}
		},

		OnIdentity: func(tenantID, deviceID string, id models.DeviceIdentity) {
			if err := database.UpsertDevice(ctx, id); err != nil {
				log.Printf("db: upsert device %s/%s: %v", tenantID, deviceID, err)
			}
		},
	})
	if err != nil {
		log.Fatalf("mqtt: %v", err)
	}
	defer mqttCli.Disconnect()
	log.Printf("mqtt connected to %s", mqttURL)

	_ = relayMgr

	// -----------------------------------------------------------------
	// HTTP API + WebSocket
	// -----------------------------------------------------------------
	r := mux.NewRouter()
	api.New(r, api.Services{
		DB:        database,
		Sensor:    sensorMgr,
		Control:   controlMgr,
		Status:    statusMgr,
		Errors:    errorMgr,
		Datastore: datastoreMgr,
		Storage:   storageMgr,
		Hub:       hub,
	}, hub, authHandler)

	srv := &http.Server{
		Addr:         listenAddr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("listening on %s", listenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func buildSnapshot(
	tenantID, deviceID string,
	r models.Reading,
	sensorMgr *sensor.Manager,
	controlMgr *control.Manager,
	statusMgr *status.Manager,
	errorMgr *errmanager.Manager,
	fanMgr *fan.Manager,
) models.DeviceSnapshot {
	snap := models.DeviceSnapshot{
		TenantID:  tenantID,
		DeviceID:  deviceID,
		Timestamp: r.Timestamp,
		Sensor: models.SensorReading{
			Temperature:  r.Temperature,
			Humidity:     r.Humidity,
			Timestamp:    r.Timestamp,
			FallbackTime: r.FallbackTime,
			Health:       sensorMgr.Health(tenantID, deviceID),
		},
	}
	if dc, ok := controlMgr.GetControl(tenantID, deviceID); ok {
		snap.DeviceStates = dc.DeviceStates
		snap.OperationalMode = dc.OperationalMode
		snap.ActiveMode = dc.ActiveMode
	}
	if s, ok := statusMgr.Get(tenantID, deviceID); ok {
		snap.SystemStatus = s
	}
	snap.Errors = errorMgr.GetActive(tenantID, deviceID)
	if fs, ok := fanMgr.GetSettings(tenantID, deviceID); ok {
		snap.FanSettings = fs
	}
	return snap
}
