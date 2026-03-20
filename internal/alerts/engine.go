// Package alerts evaluates per-tenant alert rules against incoming sensor
// readings and dispatches notifications over the configured channel.
package alerts

import (
	"context"
	"fmt"
	"log"
	"net/smtp"
	"sync"
	"time"

	"climate-backend/internal/db"
	"climate-backend/internal/models"
)

// SMTPConfig holds outbound mail server settings read from environment variables.
type SMTPConfig struct {
	Host string // SMTP_HOST
	Port int    // SMTP_PORT (default 587)
	User string // SMTP_USER
	Pass string // SMTP_PASS
	From string // SMTP_FROM
}

// Engine loads alert rules from the database, evaluates them on every sensor
// reading, and fires notifications when a rule's threshold is breached and its
// cooldown has elapsed.
type Engine struct {
	db   *db.DB
	smtp SMTPConfig

	mu        sync.RWMutex
	rules     map[string][]models.AlertRule // key: "tenantID/deviceID"

	firedMu   sync.Mutex
	lastFired map[string]time.Time // key: rule ID
}

// New creates an Engine. Call LoadAll before passing readings to Evaluate.
func New(database *db.DB, smtpConf SMTPConfig) *Engine {
	return &Engine{
		db:        database,
		smtp:      smtpConf,
		rules:     make(map[string][]models.AlertRule),
		lastFired: make(map[string]time.Time),
	}
}

// LoadAll fetches every rule from the database and populates the in-memory
// cache. Call once at startup after the database is ready.
func (e *Engine) LoadAll(ctx context.Context) error {
	rules, err := e.db.LoadAllAlertRules(ctx)
	if err != nil {
		return fmt.Errorf("alerts: load all rules: %w", err)
	}
	m := make(map[string][]models.AlertRule, len(rules))
	for _, r := range rules {
		k := ruleKey(r.TenantID, r.DeviceID)
		m[k] = append(m[k], r)
	}
	e.mu.Lock()
	e.rules = m
	e.mu.Unlock()
	log.Printf("alerts: loaded %d rules", len(rules))
	return nil
}

// reload refreshes the in-memory cache for a single tenant/device pair from
// the database. Called after any CRUD mutation.
func (e *Engine) reload(ctx context.Context, tenantID, deviceID string) {
	rules, err := e.db.ListAlertRules(ctx, tenantID, deviceID)
	if err != nil {
		log.Printf("alerts: reload %s/%s: %v", tenantID, deviceID, err)
		return
	}
	k := ruleKey(tenantID, deviceID)
	e.mu.Lock()
	e.rules[k] = rules
	e.mu.Unlock()
}

// Evaluate checks all enabled rules for the tenant/device against the reading
// and fires any whose threshold is breached and cooldown has elapsed.
// Safe to call concurrently from multiple goroutines.
func (e *Engine) Evaluate(tenantID, deviceID string, r models.Reading) {
	k := ruleKey(tenantID, deviceID)
	e.mu.RLock()
	src := e.rules[k]
	rules := make([]models.AlertRule, len(src))
	copy(rules, src)
	e.mu.RUnlock()

	now := time.Now().UTC()
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		value, ok := metricValue(rule.Metric, r)
		if !ok {
			continue
		}
		if !checkOperator(rule.Operator, value, rule.Threshold) {
			continue
		}

		cooldown := time.Duration(rule.CooldownMinutes) * time.Minute
		e.firedMu.Lock()
		last, seen := e.lastFired[rule.ID]
		if seen && now.Sub(last) < cooldown {
			e.firedMu.Unlock()
			continue
		}
		e.lastFired[rule.ID] = now
		e.firedMu.Unlock()

		go e.fire(rule, tenantID, deviceID, value, now)
	}
}

// fire sends the notification and records last_fired in the database.
// Runs in its own goroutine so it never blocks the MQTT handler.
func (e *Engine) fire(rule models.AlertRule, tenantID, deviceID string, value float64, firedAt time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := e.db.UpdateAlertRuleLastFired(ctx, rule.ID, firedAt); err != nil {
		log.Printf("alerts: update last_fired rule=%s: %v", rule.ID, err)
	}

	subject := fmt.Sprintf("[climate-alert] %s/%s %s %s %.2f",
		tenantID, deviceID, rule.Metric, rule.Operator, rule.Threshold)
	body := fmt.Sprintf(
		"Alert triggered\n\nDevice:    %s / %s\nMetric:    %s\nCondition: %s %.2f\nValue:     %.2f\nTime:      %s\n",
		tenantID, deviceID,
		rule.Metric, rule.Operator, rule.Threshold,
		value, firedAt.Format(time.RFC3339),
	)

	switch rule.Channel {
	case "email":
		if e.smtp.Host == "" {
			log.Printf("alerts: SMTP not configured, skipping email rule=%s", rule.ID)
			return
		}
		if err := e.sendEmail(rule.Recipient, subject, body); err != nil {
			log.Printf("alerts: send email rule=%s to=%s: %v", rule.ID, rule.Recipient, err)
		} else {
			log.Printf("alerts: email sent rule=%s to=%s", rule.ID, rule.Recipient)
		}
	case "push":
		log.Printf("alerts: push rule=%s tenant=%s device=%s metric=%s value=%.2f [FCM not yet implemented]",
			rule.ID, tenantID, deviceID, rule.Metric, value)
	default:
		log.Printf("alerts: unknown channel=%q rule=%s", rule.Channel, rule.ID)
	}
}

// sendEmail delivers a plain-text alert email via SMTP.
// Uses PLAIN auth when SMTP_USER is set; falls back to no auth for relay servers.
func (e *Engine) sendEmail(to, subject, body string) error {
	addr := fmt.Sprintf("%s:%d", e.smtp.Host, e.smtp.Port)
	msg := []byte(
		"From: " + e.smtp.From + "\r\n" +
			"To: " + to + "\r\n" +
			"Subject: " + subject + "\r\n" +
			"\r\n" +
			body + "\r\n",
	)
	var auth smtp.Auth
	if e.smtp.User != "" {
		auth = smtp.PlainAuth("", e.smtp.User, e.smtp.Pass, e.smtp.Host)
	}
	return smtp.SendMail(addr, auth, e.smtp.From, []string{to}, msg)
}

// ---------------------------------------------------------------------------
// CRUD — called by HTTP handlers; each mutates the DB then reloads the cache
// ---------------------------------------------------------------------------

// ListRules returns all alert rules for a tenant/device pair.
func (e *Engine) ListRules(ctx context.Context, tenantID, deviceID string) ([]models.AlertRule, error) {
	return e.db.ListAlertRules(ctx, tenantID, deviceID)
}

// CreateRule persists a new rule and refreshes the in-memory cache.
func (e *Engine) CreateRule(ctx context.Context, rule models.AlertRule) (models.AlertRule, error) {
	created, err := e.db.CreateAlertRule(ctx, rule)
	if err != nil {
		return created, err
	}
	e.reload(ctx, rule.TenantID, rule.DeviceID)
	return created, nil
}

// UpdateRule replaces a rule's mutable fields and refreshes the in-memory cache.
func (e *Engine) UpdateRule(ctx context.Context, rule models.AlertRule) (models.AlertRule, error) {
	updated, err := e.db.UpdateAlertRule(ctx, rule)
	if err != nil {
		return updated, err
	}
	e.reload(ctx, rule.TenantID, rule.DeviceID)
	return updated, nil
}

// DeleteRule removes a rule and refreshes the in-memory cache.
func (e *Engine) DeleteRule(ctx context.Context, tenantID, deviceID, ruleID string) error {
	if err := e.db.DeleteAlertRule(ctx, tenantID, deviceID, ruleID); err != nil {
		return err
	}
	e.reload(ctx, tenantID, deviceID)
	return nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func ruleKey(tenantID, deviceID string) string { return tenantID + "/" + deviceID }

func metricValue(metric string, r models.Reading) (float64, bool) {
	switch metric {
	case "temperature":
		return float64(r.Temperature), true
	case "humidity":
		return float64(r.Humidity), true
	}
	return 0, false
}

func checkOperator(op string, value, threshold float64) bool {
	switch op {
	case "gt":
		return value > threshold
	case "lt":
		return value < threshold
	case "gte":
		return value >= threshold
	case "lte":
		return value <= threshold
	}
	return false
}
