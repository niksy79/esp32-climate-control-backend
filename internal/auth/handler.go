package auth

import (
	"encoding/json"
	"errors"
	"net/http"

	"golang.org/x/crypto/bcrypt"

	"climate-backend/internal/db"
	"climate-backend/internal/models"
)

// Handler exposes the register / login / refresh HTTP endpoints.
type Handler struct {
	svc *Service
	db  *db.DB
}

// NewHandler creates a Handler.
func NewHandler(svc *Service, database *db.DB) *Handler {
	return &Handler{svc: svc, db: database}
}

// Middleware delegates to the underlying Service middleware so that callers
// only need to hold a *Handler rather than both a *Handler and a *Service.
func (h *Handler) Middleware(next http.Handler) http.Handler {
	return h.svc.Middleware(next)
}

// ValidateToken delegates to the underlying Service so callers (e.g. the
// WebSocket handler) can validate tokens without importing auth.Service.
func (h *Handler) ValidateToken(token string) (*Claims, error) {
	return h.svc.ValidateToken(token)
}

// ---------------------------------------------------------------------------
// Request / response types
// ---------------------------------------------------------------------------

type registerRequest struct {
	TenantID string      `json:"tenant_id"`
	Email    string      `json:"email"`
	Password string      `json:"password"`
	Role     models.Role `json:"role"` // omit → defaults to "user"
}

type loginRequest struct {
	TenantID string `json:"tenant_id"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// Register creates a new user and returns a token pair.
// POST /api/auth/register
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.TenantID == "" || req.Email == "" || req.Password == "" {
		http.Error(w, "tenant_id, email and password are required", http.StatusBadRequest)
		return
	}
	if req.Role == "" {
		req.Role = models.RoleUser
	}
	if req.Role != models.RoleAdmin && req.Role != models.RoleUser {
		http.Error(w, "role must be 'admin' or 'user'", http.StatusBadRequest)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	user, err := h.db.CreateUser(r.Context(), req.TenantID, req.Email, string(hash), req.Role)
	if err != nil {
		// Most likely a duplicate (tenant_id, email) — return 409.
		http.Error(w, "could not create user: "+err.Error(), http.StatusConflict)
		return
	}

	resp, err := h.tokenPair(user)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonResp(w, resp)
}

// Login validates credentials and returns a token pair.
// POST /api/auth/login
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.TenantID == "" || req.Email == "" || req.Password == "" {
		http.Error(w, "tenant_id, email and password are required", http.StatusBadRequest)
		return
	}

	user, err := h.db.GetUserByEmail(r.Context(), req.TenantID, req.Email)
	if err != nil {
		// Return the same message whether the user doesn't exist or the
		// password is wrong — don't leak which one failed.
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	resp, err := h.tokenPair(user)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonResp(w, resp)
}

// Refresh validates a refresh token and issues a new token pair.
// POST /api/auth/refresh
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.RefreshToken == "" {
		http.Error(w, "refresh_token is required", http.StatusBadRequest)
		return
	}

	claims, err := h.svc.ValidateToken(body.RefreshToken)
	if err != nil {
		http.Error(w, "invalid refresh token: "+err.Error(), http.StatusUnauthorized)
		return
	}

	// Re-validate the user still exists in the database.
	user, err := h.db.GetUserByEmail(r.Context(), claims.TenantID, claims.Email)
	if err != nil {
		if errors.Is(err, db.ErrNoRows) {
			http.Error(w, "user no longer exists", http.StatusUnauthorized)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	resp, err := h.tokenPair(user)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonResp(w, resp)
}

// ---------------------------------------------------------------------------
// internal helpers
// ---------------------------------------------------------------------------

func (h *Handler) tokenPair(user models.User) (tokenResponse, error) {
	access, err := h.svc.GenerateAccessToken(user.ID, user.TenantID, user.Email, user.Role)
	if err != nil {
		return tokenResponse{}, err
	}
	refresh, err := h.svc.GenerateRefreshToken(user.ID, user.TenantID, user.Email, user.Role)
	if err != nil {
		return tokenResponse{}, err
	}
	return tokenResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
	}, nil
}

func jsonResp(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
