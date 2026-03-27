// Package api provides the HTTP REST handlers.
// Routes are scoped by tenant: /api/tenants/{tenant_id}/devices/{device_id}/...
package api

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"golang.org/x/crypto/bcrypt"

	"climate-backend/internal/alerts"
	"climate-backend/internal/auth"
	"climate-backend/internal/control"
	"climate-backend/internal/datastore"
	"climate-backend/internal/db"
	"climate-backend/internal/errmanager"
	"climate-backend/internal/fan"
	"climate-backend/internal/models"
	"climate-backend/internal/sensor"
	"climate-backend/internal/status"
	"climate-backend/internal/storage"
	"climate-backend/internal/ws"
)

// ConfigPublisher can push payloads to a device over MQTT.
// Implemented by mqtt.Client; defined here as an interface so the api package
// does not import the mqtt package directly.
type ConfigPublisher interface {
	PublishConfig(tenantID, deviceID string, payload any) error
	PublishCommand(tenantID, deviceID, command string, payload any) error
	PublishLightCommand(tenantID, deviceID string, state *bool, mode *string) error
}

// Services bundles all manager dependencies.
type Services struct {
	DB        *db.DB
	Sensor    *sensor.Manager
	Control   *control.Manager
	Status    *status.Manager
	Errors    *errmanager.Manager
	Datastore *datastore.Manager
	Storage   *storage.Manager
	Fan       *fan.Manager
	Hub       *ws.Hub
	Alerts    *alerts.Engine
	MQTT      ConfigPublisher // nil-safe: publish is skipped when nil
}

// Handler holds the HTTP handler and its dependencies.
type Handler struct {
	svc         Services
	authHandler *auth.Handler
}

// New creates a Handler and registers routes on the provided router.
// authHandler provides the JWT middleware and the register/login/refresh endpoints.
func New(r *mux.Router, svc Services, hub *ws.Hub, authHandler *auth.Handler) *Handler {
	h := &Handler{svc: svc, authHandler: authHandler}

	// CORS middleware runs on every matched route, including the OPTIONS
	// catch-all registered at the bottom of this function.
	r.Use(corsMiddleware)

	// ── Unauthenticated routes ────────────────────────────────────────────────
	r.HandleFunc("/api/auth/register", authHandler.Register).Methods(http.MethodPost)
	r.HandleFunc("/api/auth/login", authHandler.Login).Methods(http.MethodPost)
	r.HandleFunc("/api/auth/refresh", authHandler.Refresh).Methods(http.MethodPost)
	r.HandleFunc("/api/auth/forgot-password", authHandler.ForgotPassword).Methods(http.MethodPost)
	r.HandleFunc("/api/auth/reset-password", authHandler.ResetPassword).Methods(http.MethodPost)

	// WebSocket — JWT validated from ?token= query param (browsers cannot set
	// Authorization headers on WebSocket connections).
	r.HandleFunc("/ws/{tenant_id}", h.handleWS)

	// ── JWT-protected routes ─────────────────────────────────────────────────
	// All /api/tenants/... routes require a valid Bearer token whose tenant_id
	// claim matches the {tenant_id} path variable. POST routes additionally
	// require RoleAdmin.
	protected := r.PathPrefix("/api/tenants").Subrouter()
	protected.Use(authHandler.Middleware)

	base := "/{tenant_id}/devices"
	protected.HandleFunc(base, h.handleListDevices).Methods(http.MethodGet)
	protected.HandleFunc(base+"/{device_id}/current", h.handleCurrent).Methods(http.MethodGet)
	protected.HandleFunc(base+"/{device_id}/status", h.handleStatus).Methods(http.MethodGet)
	protected.HandleFunc(base+"/{device_id}/history", h.handleHistory).Methods(http.MethodGet)
	protected.HandleFunc(base+"/{device_id}/compressor-cycles", h.handleCompressorCycles).Methods(http.MethodGet)
	protected.HandleFunc(base+"/{device_id}/errors", h.handleErrors).Methods(http.MethodGet)
	protected.HandleFunc(base+"/{device_id}/settings", h.handleGetSettings).Methods(http.MethodGet)
	protected.HandleFunc(base+"/{device_id}/settings", h.handleSaveSettings).Methods(http.MethodPost)
	protected.HandleFunc(base+"/{device_id}/mode", h.handleSwitchMode).Methods(http.MethodPost)
	protected.HandleFunc(base+"/{device_id}/light", h.handleSetLight).Methods(http.MethodPost)

	alertBase := base + "/{device_id}/alert-rules"
	protected.HandleFunc(alertBase, h.handleListAlertRules).Methods(http.MethodGet)
	protected.HandleFunc(alertBase, h.handleCreateAlertRule).Methods(http.MethodPost)
	protected.HandleFunc(alertBase+"/{rule_id}", h.handleUpdateAlertRule).Methods(http.MethodPut)
	protected.HandleFunc(alertBase+"/{rule_id}", h.handleDeleteAlertRule).Methods(http.MethodDelete)

	protected.HandleFunc(base+"/{device_id}", h.handleDeleteDevice).Methods(http.MethodDelete)
	protected.HandleFunc(base+"/{device_id}/type", h.handleSetDeviceType).Methods(http.MethodPost)
	protected.HandleFunc(base+"/{device_id}/name", h.handleUpdateDeviceName).Methods(http.MethodPatch)
	protected.HandleFunc(base+"/{device_id}/logs", h.handleGetDeviceLogs).Methods(http.MethodGet)

	userBase := "/{tenant_id}/users"
	protected.HandleFunc(userBase, h.handleListUsers).Methods(http.MethodGet)
	protected.HandleFunc(userBase, h.handleCreateUser).Methods(http.MethodPost)
	protected.HandleFunc(userBase+"/{user_id}", h.handleDeleteUser).Methods(http.MethodDelete)

	// ── Auth routes requiring JWT (no tenant isolation — user acts on own account) ──
	authProtected := r.NewRoute().Subrouter()
	authProtected.Use(authHandler.Middleware)
	authProtected.HandleFunc("/api/auth/change-password", authHandler.ChangePassword).Methods(http.MethodPost)

	// ── Device-type registry routes ──────────────────────────────────────────
	// GET is public. POST/PUT require a valid admin JWT — use a subrouter so
	// the middleware validates the token and enforces the admin role check
	// before the handler runs. The middleware skips tenant isolation here
	// because there is no {tenant_id} variable in these paths.
	r.HandleFunc("/api/device-types", h.handleListDeviceTypes).Methods(http.MethodGet)
	r.HandleFunc("/api/device-types/{type_id}", h.handleGetDeviceType).Methods(http.MethodGet)

	deviceTypesAdmin := r.NewRoute().Subrouter()
	deviceTypesAdmin.Use(authHandler.Middleware)
	deviceTypesAdmin.HandleFunc("/api/device-types", h.handleCreateDeviceType).Methods(http.MethodPost)
	deviceTypesAdmin.HandleFunc("/api/device-types/{type_id}", h.handleUpdateDeviceType).Methods(http.MethodPut)

	// Catch-all OPTIONS handler so CORS preflight requests match a route and
	// the corsMiddleware can respond with 200 OK + CORS headers.
	r.PathPrefix("/").Methods(http.MethodOptions).HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	return h
}

// ---------------------------------------------------------------------------
// handlers
// ---------------------------------------------------------------------------

func (h *Handler) handleCurrent(w http.ResponseWriter, r *http.Request) {
	tenantID, deviceID := pathIDs(r)
	sr, ok := h.svc.Sensor.GetLatest(tenantID, deviceID)
	if !ok {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	jsonResp(w, sr)
}

func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	tenantID, deviceID := pathIDs(r)
	deviceName, err := h.svc.DB.GetDeviceName(r.Context(), tenantID, deviceID)
	if err != nil {
		if errors.Is(err, db.ErrNoRows) {
			http.Error(w, "device not found", http.StatusNotFound)
			return
		}
		http.Error(w, "db error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s, _ := h.svc.Status.Get(tenantID, deviceID)
	dc, _ := h.svc.Control.GetControl(tenantID, deviceID)
	fs, _ := h.svc.Fan.GetSettings(tenantID, deviceID)
	jsonResp(w, map[string]any{
		"device_name":      deviceName,
		"system_status":    s,
		"operational_mode": dc.OperationalMode.String(),
		"active_mode":      dc.ActiveMode.String(),
		"device_states":    dc.DeviceStates,
		"compressor_stats": dc.CompressorStats,
		"fan_settings":     fs,
		"has_errors":       h.svc.Errors.HasActiveErrors(tenantID, deviceID),
		"critical_errors":  h.svc.Errors.HasCriticalErrors(tenantID, deviceID),
		"alert_firing":     h.svc.Alerts.HasRecentlyFired(tenantID, deviceID),
	})
}

func (h *Handler) handleHistory(w http.ResponseWriter, r *http.Request) {
	tenantID, deviceID := pathIDs(r)
	days := 1
	if d := r.URL.Query().Get("days"); d != "" {
		if n, err := parseInt(d); err == nil && n > 0 {
			days = n
		}
	}

	// Нов branch: ?metric=temperature или ?metric=humidity → device_readings
	metric := r.URL.Query().Get("metric")
	if metric == "temperature" || metric == "humidity" {
		readings, err := h.svc.DB.GetDeviceReadings(r.Context(), tenantID, deviceID, metric, days)
		if err != nil {
			log.Printf("api: get device_readings %s/%s/%s: %v", tenantID, deviceID, metric, err)
			http.Error(w, "query failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if readings == nil {
			readings = []models.MetricReading{}
		}
		jsonResp(w, map[string]any{
			"tenant_id": tenantID,
			"device_id": deviceID,
			"days":      days,
			"metric":    metric,
			"count":     len(readings),
			"readings":  readings,
		})
		return
	}

	// Default path — чете от device_readings (paired temp+hum), същия JSON формат
	readings, err := h.svc.DB.GetDeviceReadingsPaired(r.Context(), tenantID, deviceID, days)
	if err != nil {
		log.Printf("api: get device_readings paired %s/%s: %v", tenantID, deviceID, err)
		// Fallback към старата readings таблица ако device_readings е празна или грешка
		readings, err = h.svc.Datastore.GetLastNDays(r.Context(), tenantID, deviceID, days)
		if err != nil {
			http.Error(w, "query failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if len(readings) == 0 {
		// device_readings може да е празна за стари устройства — fallback
		readings, err = h.svc.Datastore.GetLastNDays(r.Context(), tenantID, deviceID, days)
		if err != nil {
			http.Error(w, "query failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if readings == nil {
		readings = []models.Reading{}
	}
	jsonResp(w, map[string]any{
		"tenant_id": tenantID,
		"device_id": deviceID,
		"days":      days,
		"count":     len(readings),
		"readings":  readings,
	})
}

func (h *Handler) handleCompressorCycles(w http.ResponseWriter, r *http.Request) {
	tenantID, deviceID := pathIDs(r)
	days := 7
	if d := r.URL.Query().Get("days"); d != "" {
		if n, err := parseInt(d); err == nil && n > 0 {
			days = n
		}
	}
	cycles, err := h.svc.DB.GetCompressorCycles(r.Context(), tenantID, deviceID, days)
	if err != nil {
		http.Error(w, "query failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if cycles == nil {
		cycles = []models.CompressorCycle{}
	}
	jsonResp(w, map[string]any{
		"tenant_id": tenantID,
		"device_id": deviceID,
		"days":      days,
		"count":     len(cycles),
		"cycles":    cycles,
	})
}

func (h *Handler) handleErrors(w http.ResponseWriter, r *http.Request) {
	tenantID, deviceID := pathIDs(r)
	errs := h.svc.Errors.GetActive(tenantID, deviceID)
	if errs == nil {
		errs = []models.ErrorStatus{}
	}
	jsonResp(w, errs)
}

func (h *Handler) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	tenantID, deviceID := pathIDs(r)
	ts, hs, fs, ls, ss, ds := h.svc.Storage.LoadSettings(r.Context(), tenantID, deviceID)
	jsonResp(w, map[string]any{
		"temp":     ts,
		"humidity": hs,
		"fan":      fs,
		"light":    ls,
		"system":   ss,
		"display":  ds,
	})
}

func (h *Handler) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	tenantID, deviceID := pathIDs(r)

	var body struct {
		Temp     *models.TempSettings     `json:"temp"`
		Humidity *models.HumiditySettings `json:"humidity"`
		Fan      *struct {
			Speed          uint8  `json:"speed"`
			MixingInterval uint32 `json:"mixing_interval"`
			MixingDuration uint32 `json:"mixing_duration"`
			MixingEnabled  bool   `json:"mixing_enabled"`
		} `json:"fan"`
		Light *models.LightSettings `json:"light"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	if body.Temp != nil {
		if err := h.svc.Storage.SaveTempSettings(r.Context(), tenantID, deviceID, *body.Temp); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if body.Humidity != nil {
		if err := h.svc.Storage.SaveHumiditySettings(r.Context(), tenantID, deviceID, *body.Humidity); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if body.Fan != nil {
		fs := models.FanSettings{
			Speed:          body.Fan.Speed,
			MixingInterval: body.Fan.MixingInterval,
			MixingDuration: body.Fan.MixingDuration,
			MixingEnabled:  body.Fan.MixingEnabled,
		}
		log.Printf("api: save fan settings %s/%s: speed=%d interval=%d duration=%d enabled=%v",
			tenantID, deviceID, fs.Speed, fs.MixingInterval, fs.MixingDuration, fs.MixingEnabled)
		if err := h.svc.Storage.SaveFanSettings(r.Context(), tenantID, deviceID, fs); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if body.Light != nil {
		if err := h.svc.Storage.SaveLightSettings(r.Context(), tenantID, deviceID, *body.Light); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Push config to the device over MQTT.
	// Only include fields whose settings were present in this request.
	// A publish failure does not fail the HTTP response — settings are already
	// persisted to DB and will be re-applied on next device connection.
	if h.svc.MQTT != nil && (body.Temp != nil || body.Humidity != nil || body.Fan != nil) {
		cfg := map[string]any{}
		if body.Temp != nil {
			cfg["temp_target"] = body.Temp.Target
			cfg["temp_offset"] = body.Temp.Offset
		}
		if body.Humidity != nil {
			cfg["hum_target"] = body.Humidity.Target
			cfg["hum_offset"] = body.Humidity.Offset
		}
		if body.Fan != nil {
			cfg["fan_speed"]        = body.Fan.Speed
			cfg["mixing_enabled"]   = body.Fan.MixingEnabled
			cfg["mixing_interval"]  = body.Fan.MixingInterval
			cfg["mixing_duration"]  = body.Fan.MixingDuration
		}
		if err := h.svc.MQTT.PublishConfig(tenantID, deviceID, cfg); err != nil {
			log.Printf("api: mqtt config publish %s/%s: %v", tenantID, deviceID, err)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleSwitchMode(w http.ResponseWriter, r *http.Request) {
	tenantID, deviceID := pathIDs(r)
	var body struct {
		Mode int `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	validModes := map[int]bool{0: true, 1: true, 2: true, 3: true, 10: true, 11: true, 12: true, 13: true}
	if !validModes[body.Mode] {
		http.Error(w, "invalid mode: must be one of 0, 1, 2, 3, 10, 11, 12, 13", http.StatusBadRequest)
		return
	}
	if h.svc.MQTT != nil {
		if err := h.svc.MQTT.PublishCommand(tenantID, deviceID, "mode", map[string]any{"mode": body.Mode}); err != nil {
			log.Printf("api: mqtt publish mode %s/%s: %v", tenantID, deviceID, err)
			http.Error(w, "mqtt publish failed", http.StatusInternalServerError)
			return
		}
	}
	if err := h.svc.Storage.SaveActiveMode(r.Context(), tenantID, deviceID, body.Mode); err != nil {
		log.Printf("api: save active mode %s/%s: %v", tenantID, deviceID, err)
	}
	h.svc.Control.SetActiveMode(tenantID, deviceID, models.ModeType(body.Mode))
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleSetLight(w http.ResponseWriter, r *http.Request) {
	tenantID, deviceID := pathIDs(r)
	var body struct {
		State *bool   `json:"state"`
		Mode  *string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.Mode != nil && *body.Mode != "manual" && *body.Mode != "auto" {
		http.Error(w, "mode must be 'manual' or 'auto'", http.StatusBadRequest)
		return
	}
	if h.svc.MQTT != nil {
		if err := h.svc.MQTT.PublishLightCommand(tenantID, deviceID, body.State, body.Mode); err != nil {
			log.Printf("api: mqtt publish light %s/%s: %v", tenantID, deviceID, err)
		}
	}
	if body.Mode != nil {
		modeInt := models.LightModeManual
		if *body.Mode == "auto" {
			modeInt = models.LightModeAuto
		}
		ls := models.LightSettings{Mode: modeInt}
		if body.State != nil {
			ls.State = *body.State
		}
		if err := h.svc.Storage.SaveLightSettings(r.Context(), tenantID, deviceID, ls); err != nil {
			log.Printf("api: save light settings %s/%s: %v", tenantID, deviceID, err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleListDevices(w http.ResponseWriter, r *http.Request) {
	tenantID := mux.Vars(r)["tenant_id"]
	devices, err := h.svc.DB.ListDeviceIDs(r.Context(), tenantID)
	if err != nil {
		http.Error(w, "failed to list devices: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if devices == nil {
		devices = []models.DeviceSummary{}
	}
	jsonResp(w, devices)
}

// ---------------------------------------------------------------------------
// alert-rule handlers (admin only for all four methods)
// ---------------------------------------------------------------------------

func (h *Handler) handleListAlertRules(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	tenantID, deviceID := pathIDs(r)
	rules, err := h.svc.Alerts.ListRules(r.Context(), tenantID, deviceID)
	if err != nil {
		http.Error(w, "query failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if rules == nil {
		rules = []models.AlertRule{}
	}
	jsonResp(w, rules)
}

func (h *Handler) handleCreateAlertRule(w http.ResponseWriter, r *http.Request) {
	tenantID, deviceID := pathIDs(r)
	var body struct {
		Metric          string  `json:"metric"`
		Operator        string  `json:"operator"`
		Threshold       float64 `json:"threshold"`
		Channel         string  `json:"channel"`
		Recipient       string  `json:"recipient"`
		Enabled         bool    `json:"enabled"`
		CooldownMinutes int     `json:"cooldown_minutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateAlertRule(body.Metric, body.Operator, body.Channel); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cooldown := body.CooldownMinutes
	if cooldown <= 0 {
		cooldown = 15
	}
	rule := models.AlertRule{
		TenantID:        tenantID,
		DeviceID:        deviceID,
		Metric:          body.Metric,
		Operator:        body.Operator,
		Threshold:       body.Threshold,
		Channel:         body.Channel,
		Recipient:       body.Recipient,
		Enabled:         body.Enabled,
		CooldownMinutes: cooldown,
	}
	created, err := h.svc.Alerts.CreateRule(r.Context(), rule)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonResp(w, created)
}

func (h *Handler) handleUpdateAlertRule(w http.ResponseWriter, r *http.Request) {
	tenantID, deviceID := pathIDs(r)
	ruleID := mux.Vars(r)["rule_id"]
	var body struct {
		Metric          string  `json:"metric"`
		Operator        string  `json:"operator"`
		Threshold       float64 `json:"threshold"`
		Channel         string  `json:"channel"`
		Recipient       string  `json:"recipient"`
		Enabled         bool    `json:"enabled"`
		CooldownMinutes int     `json:"cooldown_minutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateAlertRule(body.Metric, body.Operator, body.Channel); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cooldown := body.CooldownMinutes
	if cooldown <= 0 {
		cooldown = 15
	}
	rule := models.AlertRule{
		ID:              ruleID,
		TenantID:        tenantID,
		DeviceID:        deviceID,
		Metric:          body.Metric,
		Operator:        body.Operator,
		Threshold:       body.Threshold,
		Channel:         body.Channel,
		Recipient:       body.Recipient,
		Enabled:         body.Enabled,
		CooldownMinutes: cooldown,
	}
	updated, err := h.svc.Alerts.UpdateRule(r.Context(), rule)
	if err != nil {
		if errors.Is(err, db.ErrNoRows) {
			http.Error(w, "rule not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResp(w, updated)
}

func (h *Handler) handleDeleteAlertRule(w http.ResponseWriter, r *http.Request) {
	tenantID, deviceID := pathIDs(r)
	ruleID := mux.Vars(r)["rule_id"]
	err := h.svc.Alerts.DeleteRule(r.Context(), tenantID, deviceID, ruleID)
	if err != nil {
		if errors.Is(err, db.ErrNoRows) {
			http.Error(w, "rule not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleWS(w http.ResponseWriter, r *http.Request) {
	tenantID := mux.Vars(r)["tenant_id"]
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing token query parameter", http.StatusUnauthorized)
		return
	}
	claims, err := h.authHandler.ValidateToken(token)
	if err != nil {
		http.Error(w, "invalid token: "+err.Error(), http.StatusUnauthorized)
		return
	}
	if claims.TenantID != tenantID {
		http.Error(w, "forbidden: tenant mismatch", http.StatusForbidden)
		return
	}
	h.svc.Hub.Subscribe(tenantID, w, r)
}

// ---------------------------------------------------------------------------
// device-type handlers
// ---------------------------------------------------------------------------

func (h *Handler) handleListDeviceTypes(w http.ResponseWriter, r *http.Request) {
	types, err := h.svc.DB.ListDeviceTypes(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		log.Printf("api: list device types: %v", err)
		return
	}
	if types == nil {
		types = []models.DeviceType{}
	}
	jsonResp(w, types)
}

func (h *Handler) handleGetDeviceType(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["type_id"]
	dt, err := h.svc.DB.GetDeviceType(r.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		log.Printf("api: get device type %s: %v", id, err)
		return
	}
	jsonResp(w, dt)
}

func (h *Handler) handleCreateDeviceType(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	var dt models.DeviceType
	if err := json.NewDecoder(r.Body).Decode(&dt); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if dt.ID == "" || dt.DisplayName == "" {
		http.Error(w, "id and display_name are required", http.StatusBadRequest)
		return
	}
	if err := h.svc.DB.CreateDeviceType(r.Context(), dt); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		log.Printf("api: create device type %s: %v", dt.ID, err)
		return
	}
	created, err := h.svc.DB.GetDeviceType(r.Context(), dt.ID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonResp(w, created)
}

func (h *Handler) handleUpdateDeviceType(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	id := mux.Vars(r)["type_id"]
	var dt models.DeviceType
	if err := json.NewDecoder(r.Body).Decode(&dt); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	dt.ID = id
	if err := h.svc.DB.UpdateDeviceType(r.Context(), dt); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		log.Printf("api: update device type %s: %v", id, err)
		return
	}
	updated, err := h.svc.DB.GetDeviceType(r.Context(), id)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonResp(w, updated)
}

func (h *Handler) handleSetDeviceType(w http.ResponseWriter, r *http.Request) {
	tenantID, deviceID := pathIDs(r)
	var body struct {
		DeviceTypeID string `json:"device_type_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.DeviceTypeID == "" {
		http.Error(w, "device_type_id is required", http.StatusBadRequest)
		return
	}
	if err := h.svc.DB.SetDeviceTypeID(r.Context(), tenantID, deviceID, body.DeviceTypeID); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		log.Printf("api: set device type %s/%s: %v", tenantID, deviceID, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleUpdateDeviceName(w http.ResponseWriter, r *http.Request) {
	tenantID, deviceID := pathIDs(r)
	var body struct {
		DeviceName string `json:"device_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.DeviceName == "" {
		http.Error(w, "device_name is required", http.StatusBadRequest)
		return
	}
	err := h.svc.DB.UpdateDeviceName(r.Context(), tenantID, deviceID, body.DeviceName)
	if errors.Is(err, db.ErrNoRows) {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		log.Printf("api: update device name %s/%s: %v", tenantID, deviceID, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleGetDeviceLogs(w http.ResponseWriter, r *http.Request) {
	tenantID, deviceID := pathIDs(r)

	n := 100
	if s := r.URL.Query().Get("lines"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v < 1 {
			http.Error(w, "lines must be a positive integer", http.StatusBadRequest)
			return
		}
		if v > 500 {
			v = 500
		}
		n = v
	}

	path := fmt.Sprintf("logs/%s/%s.log", tenantID, deviceID)
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"lines": []string{}})
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		log.Printf("api: open log %s/%s: %v", tenantID, deviceID, err)
		return
	}
	defer f.Close()

	// sliding window — keep only the last n lines
	ring := make([]string, n)
	pos := 0
	count := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		ring[pos%n] = sc.Text()
		pos++
		count++
	}
	if err := sc.Err(); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		log.Printf("api: scan log %s/%s: %v", tenantID, deviceID, err)
		return
	}

	var lines []string
	if count == 0 {
		lines = []string{}
	} else if count <= n {
		lines = ring[:count]
	} else {
		// oldest entry starts at pos%n
		start := pos % n
		lines = make([]string, n)
		copy(lines, ring[start:])
		copy(lines[n-start:], ring[:start])
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"lines": lines})
}

// ---------------------------------------------------------------------------
// user-management handlers (admin only)
// ---------------------------------------------------------------------------

func (h *Handler) handleListUsers(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	tenantID := mux.Vars(r)["tenant_id"]
	users, err := h.svc.DB.ListUsersByTenant(r.Context(), tenantID)
	if err != nil {
		http.Error(w, "query failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if users == nil {
		users = []models.User{}
	}
	jsonResp(w, users)
}

func (h *Handler) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	tenantID := mux.Vars(r)["tenant_id"]
	var body struct {
		Email    string      `json:"email"`
		Password string      `json:"password"`
		Role     models.Role `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if body.Email == "" || body.Password == "" {
		http.Error(w, `{"error":"email and password are required"}`, http.StatusBadRequest)
		return
	}
	if len(body.Password) < 8 {
		http.Error(w, `{"error":"password must be at least 8 characters"}`, http.StatusBadRequest)
		return
	}
	role := models.RoleUser
	if body.Role == models.RoleAdmin {
		role = models.RoleAdmin
	}
	if _, err := h.svc.DB.GetUserByEmailGlobal(r.Context(), body.Email); err == nil {
		http.Error(w, `{"error":"email already registered"}`, http.StatusConflict)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}
	user, err := h.svc.DB.CreateUser(r.Context(), tenantID, body.Email, string(hash), role)
	if err != nil {
		log.Printf("api: create user %s: %v", tenantID, err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonResp(w, user)
}

func (h *Handler) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tenantID := vars["tenant_id"]
	userID := vars["user_id"]
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	if claims.UserID == userID {
		http.Error(w, `{"error":"cannot delete your own account"}`, http.StatusBadRequest)
		return
	}
	err := h.svc.DB.DeleteUser(r.Context(), tenantID, userID)
	if err != nil {
		if errors.Is(err, db.ErrNoRows) {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleDeleteDevice(w http.ResponseWriter, r *http.Request) {
	tenantID, deviceID := pathIDs(r)
	err := h.svc.DB.DeleteDevice(r.Context(), tenantID, deviceID)
	if err != nil {
		if errors.Is(err, db.ErrNoRows) {
			http.Error(w, "device not found", http.StatusNotFound)
			return
		}
		log.Printf("api: delete device %s/%s: %v", tenantID, deviceID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func (h *Handler) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok || claims.Role != models.RoleAdmin {
		http.Error(w, "forbidden: admin role required", http.StatusForbidden)
		return false
	}
	return true
}

func validateAlertRule(metric, operator, channel string) error {
	switch metric {
	case "temperature", "humidity":
	default:
		return fmt.Errorf("metric must be 'temperature' or 'humidity'")
	}
	switch operator {
	case "gt", "lt", "gte", "lte":
	default:
		return fmt.Errorf("operator must be 'gt', 'lt', 'gte', or 'lte'")
	}
	switch channel {
	case "email", "push":
	default:
		return fmt.Errorf("channel must be 'email' or 'push'")
	}
	return nil
}

func pathIDs(r *http.Request) (tenantID, deviceID string) {
	v := mux.Vars(r)
	return v["tenant_id"], v["device_id"]
}

func jsonResp(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscan(s, &n)
	return n, err
}

// Ensure time is imported (used via models.Reading.Timestamp etc.)
var _ = time.Now
