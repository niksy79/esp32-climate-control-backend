package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/smtp"

	"golang.org/x/crypto/bcrypt"

	"climate-backend/internal/db"
	"climate-backend/internal/models"
)

// Handler exposes the register / login / refresh / password endpoints.
type Handler struct {
	db       *db.DB
	svc      *Service
	smtpHost string
	smtpPort string
	smtpUser string
	smtpPass string
	smtpFrom string
	appURL   string
}

// NewHandler creates a Handler.
// Deprecated: use New() with SMTP parameters.
func NewHandler(svc *Service, database *db.DB) *Handler {
	return &Handler{svc: svc, db: database}
}

// New creates a Handler with SMTP and app URL configuration.
func New(database *db.DB, svc *Service, smtpHost, smtpPort, smtpUser, smtpPass, smtpFrom, appURL string) *Handler {
	return &Handler{
		db:       database,
		svc:      svc,
		smtpHost: smtpHost,
		smtpPort: smtpPort,
		smtpUser: smtpUser,
		smtpPass: smtpPass,
		smtpFrom: smtpFrom,
		appURL:   appURL,
	}
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

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// Register creates a new user with an auto-generated tenant_id and returns a token pair.
// POST /api/auth/register  body: {"email":"...","password":"..."}
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
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

	// Провери дали email вече съществува глобално
	if _, err := h.db.GetUserByEmailGlobal(r.Context(), body.Email); err == nil {
		http.Error(w, `{"error":"email already registered"}`, http.StatusConflict)
		return
	}

	// Генерирай уникален tenant_id автоматично
	tenantID, err := h.db.GenerateUniqueTenantID(r.Context())
	if err != nil {
		log.Printf("auth: generate tenant_id: %v", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	role := models.RoleAdmin // първият потребител в tenant-а е admin
	if body.Role == models.RoleUser {
		role = models.RoleUser
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	user, err := h.db.CreateUser(r.Context(), tenantID, body.Email, string(hash), role)
	if err != nil {
		log.Printf("auth: create user: %v", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	tokens, err := h.tokenPair(user)
	if err != nil {
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(tokens)
}

// Login validates credentials and returns a token pair.
// POST /api/auth/login
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Email == "" || req.Password == "" {
		http.Error(w, "email and password are required", http.StatusBadRequest)
		return
	}

	user, err := h.db.GetUserByEmailGlobal(r.Context(), req.Email)
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

// ForgotPassword изпраща email с reset link.
// POST /api/auth/forgot-password  body: {"email":"..."}
// Винаги връща 200 за да не разкрие дали email съществува.
func (h *Handler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Email == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	go func() {
		ctx := context.Background()
		user, err := h.db.GetUserByEmailGlobal(ctx, body.Email)
		if err != nil {
			return // потребителят не съществува — тихо
		}

		// Генерирай криптографски случаен token
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			log.Printf("auth: generate reset token: %v", err)
			return
		}
		token := hex.EncodeToString(raw)

		if err := h.db.CreatePasswordResetToken(ctx, user.ID, user.TenantID, token); err != nil {
			log.Printf("auth: store reset token: %v", err)
			return
		}

		if h.smtpHost == "" {
			log.Printf("auth: SMTP not configured, reset token for %s: %s", body.Email, token)
			return
		}

		resetURL := h.appURL + "/reset-password?token=" + token
		subject := "Възстановяване на парола"
		emailBody := fmt.Sprintf(
			"Получихте заявка за смяна на паролата ви.\n\n"+
				"Кликнете на линка за да смените паролата си (валиден 1 час):\n%s\n\n"+
				"Ако не сте поискали смяна на парола, игнорирайте този имейл.",
			resetURL,
		)
		if err := sendEmail(h.smtpHost, h.smtpPort, h.smtpUser, h.smtpPass, h.smtpFrom, user.Email, subject, emailBody); err != nil {
			log.Printf("auth: send reset email to %s: %v", user.Email, err)
		}
	}()

	w.WriteHeader(http.StatusOK)
}

// ResetPassword задава нова парола чрез reset token.
// POST /api/auth/reset-password  body: {"token":"...","password":"..."}
func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	if body.Token == "" || body.Password == "" {
		http.Error(w, `{"error":"token and password are required"}`, http.StatusBadRequest)
		return
	}
	if len(body.Password) < 8 {
		http.Error(w, `{"error":"password must be at least 8 characters"}`, http.StatusBadRequest)
		return
	}

	userID, _, err := h.db.ValidatePasswordResetToken(r.Context(), body.Token)
	if err != nil {
		http.Error(w, `{"error":"invalid or expired token"}`, http.StatusBadRequest)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	if err := h.db.UpdatePassword(r.Context(), userID, string(hash)); err != nil {
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// ChangePassword позволява на logged-in потребител да смени паролата си.
// POST /api/auth/change-password  body: {"old_password":"...","new_password":"..."}
// Изисква валиден Bearer token.
func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	var body struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	if body.OldPassword == "" || body.NewPassword == "" {
		http.Error(w, `{"error":"old_password and new_password are required"}`, http.StatusBadRequest)
		return
	}
	if len(body.NewPassword) < 8 {
		http.Error(w, `{"error":"password must be at least 8 characters"}`, http.StatusBadRequest)
		return
	}

	user, err := h.db.GetUserByID(r.Context(), claims.UserID)
	if err != nil {
		http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(body.OldPassword)); err != nil {
		http.Error(w, `{"error":"incorrect current password"}`, http.StatusUnauthorized)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}
	if err := h.db.UpdatePassword(r.Context(), user.ID, string(hash)); err != nil {
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
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

func sendEmail(host, port, user, pass, from, to, subject, body string) error {
	addr := host + ":" + port
	msg := []byte("To: " + to + "\r\n" +
		"From: " + from + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" +
		body + "\r\n")
	var auth smtp.Auth
	if user != "" {
		auth = smtp.PlainAuth("", user, pass, host)
	}
	return smtp.SendMail(addr, auth, from, []string{to}, msg)
}

func jsonResp(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
