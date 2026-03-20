// Package api provides the HTTP REST handlers.
// Routes are scoped by tenant: /api/tenants/{tenant_id}/devices/{device_id}/...
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"climate-backend/internal/auth"
	"climate-backend/internal/control"
	"climate-backend/internal/datastore"
	"climate-backend/internal/db"
	"climate-backend/internal/errmanager"
	"climate-backend/internal/models"
	"climate-backend/internal/sensor"
	"climate-backend/internal/status"
	"climate-backend/internal/storage"
	"climate-backend/internal/ws"
)

// Services bundles all manager dependencies.
type Services struct {
	DB        *db.DB
	Sensor    *sensor.Manager
	Control   *control.Manager
	Status    *status.Manager
	Errors    *errmanager.Manager
	Datastore *datastore.Manager
	Storage   *storage.Manager
	Hub       *ws.Hub
}

// Handler holds the HTTP handler and its dependencies.
type Handler struct {
	svc Services
}

// New creates a Handler and registers routes on the provided router.
// authHandler provides the JWT middleware and the register/login/refresh endpoints.
func New(r *mux.Router, svc Services, hub *ws.Hub, authHandler *auth.Handler) *Handler {
	h := &Handler{svc: svc}

	// ── Unauthenticated routes ────────────────────────────────────────────────
	r.HandleFunc("/api/auth/register", authHandler.Register).Methods(http.MethodPost)
	r.HandleFunc("/api/auth/login", authHandler.Login).Methods(http.MethodPost)
	r.HandleFunc("/api/auth/refresh", authHandler.Refresh).Methods(http.MethodPost)

	// WebSocket upgrade — tenant-scoped but auth handled by token query param
	// (browsers cannot set Authorization headers on WS connections).
	r.HandleFunc("/ws/{tenant_id}", hub.ServeWS)

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
	protected.HandleFunc(base+"/{device_id}/errors", h.handleErrors).Methods(http.MethodGet)
	protected.HandleFunc(base+"/{device_id}/settings", h.handleGetSettings).Methods(http.MethodGet)
	protected.HandleFunc(base+"/{device_id}/settings", h.handleSaveSettings).Methods(http.MethodPost)
	protected.HandleFunc(base+"/{device_id}/mode", h.handleSwitchMode).Methods(http.MethodPost)

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
	s, ok := h.svc.Status.Get(tenantID, deviceID)
	if !ok {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	dc, _ := h.svc.Control.GetControl(tenantID, deviceID)
	jsonResp(w, map[string]any{
		"system_status":    s,
		"operational_mode": dc.OperationalMode.String(),
		"active_mode":      dc.ActiveMode.String(),
		"device_states":    dc.DeviceStates,
		"compressor_stats": dc.CompressorStats,
		"has_errors":       h.svc.Errors.HasActiveErrors(tenantID, deviceID),
		"critical_errors":  h.svc.Errors.HasCriticalErrors(tenantID, deviceID),
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
	readings, err := h.svc.Datastore.GetLastNDays(r.Context(), tenantID, deviceID, days)
	if err != nil {
		http.Error(w, "query failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResp(w, map[string]any{
		"tenant_id": tenantID,
		"device_id": deviceID,
		"days":      days,
		"count":     len(readings),
		"readings":  readings,
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
	ts, hs, fs, ls, ss, ds := h.svc.Storage.LoadSettings(tenantID, deviceID)
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
		Fan      *models.FanSettings      `json:"fan"`
		Light    *models.LightSettings    `json:"light"`
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
		if err := h.svc.Storage.SaveFanSettings(r.Context(), tenantID, deviceID, *body.Fan); err != nil {
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
	// TODO: publish MQTT command: climate/<tenantID>/<deviceID>/cmd/switch_mode
	_ = tenantID
	_ = deviceID
	jsonResp(w, map[string]any{"mode": body.Mode, "queued": true})
}

func (h *Handler) handleListDevices(w http.ResponseWriter, r *http.Request) {
	tenantID := mux.Vars(r)["tenant_id"]
	ids, err := h.svc.DB.ListDeviceIDs(r.Context(), tenantID)
	if err != nil {
		http.Error(w, "failed to list devices: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if ids == nil {
		ids = []string{}
	}
	jsonResp(w, ids)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

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
