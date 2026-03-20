// Package ws implements a per-tenant WebSocket broadcast hub.
package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// LiveMessage is the wire format pushed to WebSocket subscribers when a new
// sensor reading arrives via MQTT.
type LiveMessage struct {
	Type        string    `json:"type"`
	DeviceID    string    `json:"device_id"`
	Temperature float32   `json:"temperature"`
	Humidity    float32   `json:"humidity"`
	Timestamp   time.Time `json:"timestamp"`
}

// client is a single authenticated WebSocket connection scoped to one tenant.
type client struct {
	tenantID string
	conn     *websocket.Conn
	send     chan []byte
}

// registration carries a new client and its tenant to the run loop.
type registration struct {
	tenantID string
	c        *client
}

// tenantPayload carries a serialised message destined for one tenant's clients.
type tenantPayload struct {
	tenantID string
	data     []byte
}

// Hub maintains per-tenant sets of WebSocket clients and fans out messages to
// each tenant independently.
type Hub struct {
	mu      sync.RWMutex
	tenants map[string]map[*client]struct{} // tenantID → connected clients

	register   chan *registration
	unregister chan *client
	broadcast  chan *tenantPayload
}

// NewHub creates and starts a Hub.
func NewHub() *Hub {
	h := &Hub{
		tenants:    make(map[string]map[*client]struct{}),
		register:   make(chan *registration, 16),
		unregister: make(chan *client, 16),
		broadcast:  make(chan *tenantPayload, 256),
	}
	go h.run()
	return h
}

// run is the single goroutine that owns the tenants map.
// All mutations go through the channels so no external lock is needed for
// the map itself; the RWMutex protects ClientCount reads.
func (h *Hub) run() {
	for {
		select {
		case reg := <-h.register:
			h.mu.Lock()
			if h.tenants[reg.tenantID] == nil {
				h.tenants[reg.tenantID] = make(map[*client]struct{})
			}
			h.tenants[reg.tenantID][reg.c] = struct{}{}
			h.mu.Unlock()
			log.Printf("ws: client connected tenant=%s total=%d",
				reg.tenantID, len(h.tenants[reg.tenantID]))

		case c := <-h.unregister:
			h.mu.Lock()
			if clients, ok := h.tenants[c.tenantID]; ok {
				if _, ok := clients[c]; ok {
					delete(clients, c)
					close(c.send)
					if len(clients) == 0 {
						delete(h.tenants, c.tenantID)
					}
					log.Printf("ws: client disconnected tenant=%s remaining=%d",
						c.tenantID, len(h.tenants[c.tenantID]))
				}
			}
			h.mu.Unlock()

		case tp := <-h.broadcast:
			h.mu.RLock()
			for c := range h.tenants[tp.tenantID] {
				select {
				case c.send <- tp.data:
				default:
					// Slow client: drop rather than block the broadcast loop.
					log.Printf("ws: slow client for tenant=%s, dropping message", tp.tenantID)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Subscribe upgrades an HTTP connection to WebSocket and registers it under
// tenantID. Must be called after JWT validation — the hub trusts the caller.
func (h *Hub) Subscribe(tenantID string, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws: upgrade tenant=%s: %v", tenantID, err)
		return
	}
	c := &client{
		tenantID: tenantID,
		conn:     conn,
		send:     make(chan []byte, 64),
	}
	h.register <- &registration{tenantID: tenantID, c: c}
	go c.writePump(h)
	go c.readPump(h)
}

// BroadcastToTenant serialises v as JSON and delivers it to all clients
// connected under tenantID. Other tenants are not affected.
// Non-blocking: drops the message if the internal channel is full.
func (h *Hub) BroadcastToTenant(tenantID string, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		log.Printf("ws: marshal for tenant=%s: %v", tenantID, err)
		return
	}
	select {
	case h.broadcast <- &tenantPayload{tenantID: tenantID, data: b}:
	default:
		log.Printf("ws: broadcast channel full for tenant=%s, dropping", tenantID)
	}
}

// ClientCount returns the number of active connections for a tenant.
func (h *Hub) ClientCount(tenantID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.tenants[tenantID])
}

// ---------------------------------------------------------------------------
// client pumps — one pair of goroutines per connection
// ---------------------------------------------------------------------------

// writePump drains the send channel and writes each message to the WebSocket.
// It exits when the channel is closed by the unregister handler.
func (c *client) writePump(h *Hub) {
	defer c.conn.Close()
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

// readPump reads and discards incoming client frames (clients are receive-only).
// Any read error (including normal close) triggers unregistration.
func (c *client) readPump(h *Hub) {
	defer func() {
		h.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(512)
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}
