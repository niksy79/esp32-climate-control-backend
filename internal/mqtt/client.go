// Package mqtt subscribes to topics published by the ESP32 climate controller
// and dispatches decoded payloads to registered handlers.
//
// Multi-tenant topic layout:
//
//	climate/<tenant_id>/<device_id>/sensor       – SensorReading JSON
//	climate/<tenant_id>/<device_id>/status       – SystemStatus JSON
//	climate/<tenant_id>/<device_id>/relays       – DeviceStates JSON
//	climate/<tenant_id>/<device_id>/settings     – aggregated settings JSON
//	climate/<tenant_id>/<device_id>/errors       – []ErrorStatus JSON
//	climate/<tenant_id>/<device_id>/compressor   – CompressorCycle JSON
//	climate/<tenant_id>/<device_id>/identity     – DeviceIdentity JSON
package mqtt

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"climate-backend/internal/models"
)

// Handlers groups all inbound-message callbacks.
// Every callback receives tenantID and deviceID extracted from the topic.
type Handlers struct {
	OnSensorReading   func(tenantID, deviceID string, r models.Reading)
	OnSystemStatus    func(tenantID, deviceID string, s models.SystemStatus)
	OnDeviceStates    func(tenantID, deviceID string, ds models.DeviceStates)
	OnSettings        func(tenantID, deviceID string, snap models.DeviceSnapshot)
	OnErrors          func(tenantID, deviceID string, errs []models.ErrorStatus)
	OnCompressorCycle func(tenantID, deviceID string, c models.CompressorCycle)
	OnIdentity        func(tenantID, deviceID string, id models.DeviceIdentity)
	OnLog             func(tenantID, deviceID, message string)
}

// Client wraps a Paho MQTT client and routes messages to Handlers.
type Client struct {
	paho     paho.Client
	h        Handlers
	topicPfx string // e.g. "climate"
}

// Config holds MQTT broker connection parameters.
type Config struct {
	BrokerURL   string // e.g. "tcp://localhost:1883"
	ClientID    string
	Username    string
	Password    string
	TopicPrefix string // default "climate"
}

// New creates a connected MQTT client and subscribes to all tenant/device topics.
func New(cfg Config, h Handlers) (*Client, error) {
	if cfg.TopicPrefix == "" {
		cfg.TopicPrefix = "climate"
	}
	c := &Client{h: h, topicPfx: cfg.TopicPrefix}

	opts := paho.NewClientOptions().
		AddBroker(cfg.BrokerURL).
		SetClientID(cfg.ClientID).
		SetUsername(cfg.Username).
		SetPassword(cfg.Password).
		SetCleanSession(true).
		SetKeepAlive(15 * time.Second).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetMaxReconnectInterval(30 * time.Second).
		SetDefaultPublishHandler(c.dispatch).
		SetOnConnectHandler(c.onConnect).
		SetConnectionLostHandler(func(_ paho.Client, err error) {
			log.Printf("mqtt: connection lost: %v", err)
		})

	c.paho = paho.NewClient(opts)
	tok := c.paho.Connect()
	tok.Wait()
	if err := tok.Error(); err != nil {
		return nil, fmt.Errorf("mqtt: connect to %s: %w", cfg.BrokerURL, err)
	}
	return c, nil
}

// Disconnect cleanly disconnects from the broker.
func (c *Client) Disconnect() {
	c.paho.Disconnect(500)
}

// PublishConfig publishes a JSON config payload to a specific tenant's device.
// Topic: <prefix>/<tenantID>/<deviceID>/config  (QoS 1, not retained)
// The ESP32 subscribes to this topic and applies received fields immediately.
func (c *Client) PublishConfig(tenantID, deviceID string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	topic := fmt.Sprintf("%s/%s/%s/config", c.topicPfx, tenantID, deviceID)
	tok := c.paho.Publish(topic, 1, false, b)
	tok.Wait()
	return tok.Error()
}

// PublishLightCommand publishes a light control command to a specific tenant's device.
// Topic: <prefix>/<tenantID>/<deviceID>/cmd/light  (QoS 1, not retained)
// Omit state or mode by passing nil to exclude them from the payload.
func (c *Client) PublishLightCommand(tenantID, deviceID string, state *bool, mode *string) error {
	payload := map[string]interface{}{}
	if state != nil {
		payload["state"] = *state
	}
	if mode != nil {
		payload["mode"] = *mode
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	topic := fmt.Sprintf("%s/%s/%s/cmd/light", c.topicPfx, tenantID, deviceID)
	tok := c.paho.Publish(topic, 1, false, data)
	tok.Wait()
	return tok.Error()
}

// PublishCommand publishes a JSON command to a specific tenant's device.
// Topic: <prefix>/<tenantID>/<deviceID>/cmd/<command>  (QoS 1, not retained)
// Returns an error if the broker does not acknowledge within 5 seconds.
func (c *Client) PublishCommand(tenantID, deviceID, command string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	topic := fmt.Sprintf("%s/%s/%s/cmd/%s", c.topicPfx, tenantID, deviceID, command)
	tok := c.paho.Publish(topic, 1, false, b)
	if !tok.WaitTimeout(5 * time.Second) {
		return fmt.Errorf("mqtt: publish command %s/%s/%s: timeout", tenantID, deviceID, command)
	}
	return tok.Error()
}

// ---------------------------------------------------------------------------
// internal
// ---------------------------------------------------------------------------

// mqttSettingsPayload matches the flat JSON published by the ESP32 firmware
// on the climate/.../settings topic. V2 firmware sends mixing_interval and
// mixing_duration in minutes; light_mode as a string ("manual"/"auto").
type mqttSettingsPayload struct {
	TempTarget      float32 `json:"temp_target"`
	TempOffset      float32 `json:"temp_offset"`
	HumTarget       float32 `json:"hum_target"`
	HumOffset       float32 `json:"hum_offset"`
	FanSpeed        uint8   `json:"fan_speed"`
	MixingInterval  uint32  `json:"mixing_interval"`
	MixingDuration  uint32  `json:"mixing_duration"`
	MixingEnabled   bool    `json:"mixing_enabled"`
	LightMode       string  `json:"light_mode"`
	LightState      bool    `json:"light_state"`
	ActiveMode      int     `json:"active_mode"`
}

func (c *Client) onConnect(cl paho.Client) {
	// Subscribe before logging so any messages that arrive during the SUBACK
	// round-trip are caught by the DefaultPublishHandler fallback.
	topic := fmt.Sprintf("%s/+/+/#", c.topicPfx)
	tok := cl.Subscribe(topic, 1, c.dispatch)
	tok.Wait()
	if err := tok.Error(); err != nil {
		log.Printf("mqtt: subscribe %s: %v", topic, err)
		return
	}
	log.Printf("mqtt: connected and subscribed to %s", topic)
}

func (c *Client) dispatch(_ paho.Client, msg paho.Message) {
	parts := strings.Split(msg.Topic(), "/")
	// expected: <prefix>/<tenant_id>/<device_id>/<subtopic>
	if len(parts) < 4 {
		return
	}
	tenantID := parts[1]
	deviceID := parts[2]
	subtopic := parts[3]
	payload := msg.Payload()
	log.Printf("mqtt: rx %s/%s/%s %s", tenantID, deviceID, subtopic, payload)

	switch subtopic {
	case "sensor":
		if c.h.OnSensorReading == nil {
			return
		}
		var r models.Reading
		if err := json.Unmarshal(payload, &r); err != nil {
			log.Printf("mqtt: decode sensor/%s/%s: %v", tenantID, deviceID, err)
			return
		}
		c.h.OnSensorReading(tenantID, deviceID, r)

	case "status":
		if c.h.OnSystemStatus == nil {
			return
		}
		var s models.SystemStatus
		if err := json.Unmarshal(payload, &s); err != nil {
			log.Printf("mqtt: decode status/%s/%s: %v", tenantID, deviceID, err)
			return
		}
		c.h.OnSystemStatus(tenantID, deviceID, s)

	case "relays":
		if c.h.OnDeviceStates == nil {
			return
		}
		var ds models.DeviceStates
		if err := json.Unmarshal(payload, &ds); err != nil {
			log.Printf("mqtt: decode relays/%s/%s: %v", tenantID, deviceID, err)
			return
		}
		c.h.OnDeviceStates(tenantID, deviceID, ds)

	case "settings":
		if c.h.OnSettings == nil {
			return
		}
		var p mqttSettingsPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			log.Printf("mqtt: decode settings/%s/%s: %v", tenantID, deviceID, err)
			return
		}
		lightMode := models.LightMode(p.LightMode)
		if lightMode != models.LightModeAuto {
			lightMode = models.LightModeManual
		}
		snap := models.DeviceSnapshot{
			TenantID: tenantID,
			DeviceID: deviceID,
			TempSettings: models.TempSettings{
				Target: p.TempTarget,
				Offset: p.TempOffset,
			},
			HumiditySettings: models.HumiditySettings{
				Target: p.HumTarget,
				Offset: p.HumOffset,
			},
			FanSettings: models.FanSettings{
				Speed:          p.FanSpeed,
				MixingInterval: p.MixingInterval,
				MixingDuration: p.MixingDuration,
				MixingEnabled:  p.MixingEnabled,
			},
			LightSettings: models.LightSettings{
				Mode:  lightMode,
				State: p.LightState,
			},
			ActiveMode: models.ModeType(p.ActiveMode),
		}
		c.h.OnSettings(tenantID, deviceID, snap)

	case "errors":
		if c.h.OnErrors == nil {
			return
		}
		var errs []models.ErrorStatus
		if err := json.Unmarshal(payload, &errs); err != nil {
			log.Printf("mqtt: decode errors/%s/%s: %v", tenantID, deviceID, err)
			return
		}
		c.h.OnErrors(tenantID, deviceID, errs)

	case "compressor":
		if c.h.OnCompressorCycle == nil {
			return
		}
		var cyc models.CompressorCycle
		if err := json.Unmarshal(payload, &cyc); err != nil {
			log.Printf("mqtt: decode compressor/%s/%s: %v", tenantID, deviceID, err)
			return
		}
		c.h.OnCompressorCycle(tenantID, deviceID, cyc)

	case "identity":
		if c.h.OnIdentity == nil {
			return
		}
		var id models.DeviceIdentity
		if err := json.Unmarshal(payload, &id); err != nil {
			log.Printf("mqtt: decode identity/%s/%s: %v", tenantID, deviceID, err)
			return
		}
		// Stamp the tenant/device IDs from the topic (authoritative source).
		id.TenantID = tenantID
		id.DeviceID = deviceID
		c.h.OnIdentity(tenantID, deviceID, id)

	case "logs":
		if c.h.OnLog == nil {
			return
		}
		c.h.OnLog(tenantID, deviceID, string(payload))

	default:
		// unknown subtopic – silently ignore
	}
}
