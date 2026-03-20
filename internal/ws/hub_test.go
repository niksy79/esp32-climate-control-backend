package ws

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// dialWS connects a test WebSocket client to the given test server URL.
func dialWS(t *testing.T, serverURL string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(serverURL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return conn
}

// TestBroadcastToTenant verifies that a message sent to tenantA is received
// by tenantA's client but NOT by tenantB's client.
func TestBroadcastToTenant(t *testing.T) {
	hub := NewHub()

	// Spin up two test HTTP servers — one per tenant.
	srvA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.Subscribe("tenantA", w, r)
	}))
	defer srvA.Close()

	srvB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.Subscribe("tenantB", w, r)
	}))
	defer srvB.Close()

	connA := dialWS(t, srvA.URL)
	defer connA.Close()

	connB := dialWS(t, srvB.URL)
	defer connB.Close()

	// Allow registration goroutines to complete.
	time.Sleep(50 * time.Millisecond)

	// Broadcast only to tenantA.
	msg := LiveMessage{Type: "sensor", DeviceID: "d1", Temperature: 20.5, Humidity: 55.0}
	hub.BroadcastToTenant("tenantA", msg)

	// tenantA should receive the message within a short deadline.
	connA.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, data, err := connA.ReadMessage()
	if err != nil {
		t.Fatalf("tenantA did not receive message: %v", err)
	}
	if !strings.Contains(string(data), `"sensor"`) {
		t.Errorf("unexpected message content: %s", data)
	}

	// tenantB should NOT receive anything.
	connB.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, _, err = connB.ReadMessage()
	if err == nil {
		t.Fatal("tenantB unexpectedly received a message meant for tenantA")
	}
}

// TestClientCountAndDisconnect verifies ClientCount increments on connect and
// drops to zero after the connection closes.
func TestClientCountAndDisconnect(t *testing.T) {
	hub := NewHub()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.Subscribe("t1", w, r)
	}))
	defer srv.Close()

	conn := dialWS(t, srv.URL)
	time.Sleep(50 * time.Millisecond)

	if got := hub.ClientCount("t1"); got != 1 {
		t.Fatalf("expected 1 client, got %d", got)
	}

	conn.Close()
	time.Sleep(100 * time.Millisecond)

	if got := hub.ClientCount("t1"); got != 0 {
		t.Fatalf("expected 0 clients after disconnect, got %d", got)
	}
}
