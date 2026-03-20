#!/usr/bin/env bash
# test-ws.sh — end-to-end WebSocket smoke test
#
# Prerequisites:
#   - climate-backend running on $BASE_URL (default: http://localhost:8080)
#   - Mosquitto running on $MQTT_HOST:$MQTT_PORT (default: localhost:1883)
#   - websocat installed (https://github.com/vi/websocat)
#     OR wscat installed (npm install -g wscat)
#
# Usage:
#   ./scripts/test-ws.sh [tenant_id] [device_id]
#
# Example:
#   ./scripts/test-ws.sh tenant1 device1

set -euo pipefail

TENANT="${1:-tenant1}"
DEVICE="${2:-device1}"
BASE_URL="${BASE_URL:-http://localhost:8080}"
MQTT_HOST="${MQTT_HOST:-localhost}"
MQTT_PORT="${MQTT_PORT:-1883}"

# ── 1. Register a test user ───────────────────────────────────────────────────
echo "==> Registering test user for tenant '$TENANT'..."
REGISTER_RESP=$(curl -sf -X POST "$BASE_URL/api/auth/register" \
  -H "Content-Type: application/json" \
  -d "{\"tenant_id\":\"$TENANT\",\"email\":\"ws-test@example.com\",\"password\":\"testpass123\",\"role\":\"user\"}") || true

# If already registered, log in instead.
if [ -z "$REGISTER_RESP" ]; then
  echo "    (user may already exist, logging in instead)"
  REGISTER_RESP=$(curl -sf -X POST "$BASE_URL/api/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"tenant_id\":\"$TENANT\",\"email\":\"ws-test@example.com\",\"password\":\"testpass123\"}")
fi

TOKEN=$(echo "$REGISTER_RESP" | grep -o '"access_token":"[^"]*"' | cut -d'"' -f4)
if [ -z "$TOKEN" ]; then
  echo "ERROR: could not obtain access token" >&2
  exit 1
fi
echo "    access token obtained"

# ── 2. Connect WebSocket in background ───────────────────────────────────────
WS_URL="ws${BASE_URL#http}/ws/$TENANT?token=$TOKEN"
OUTFILE=$(mktemp)

if command -v websocat &>/dev/null; then
  echo "==> Connecting via websocat: $WS_URL"
  tail -f /dev/null | websocat --no-close "$WS_URL" > "$OUTFILE" &
  WS_PID=$!
elif command -v wscat &>/dev/null; then
  echo "==> Connecting via wscat: $WS_URL"
  wscat --connect "$WS_URL" --no-check > "$OUTFILE" &
  WS_PID=$!
else
  echo "ERROR: neither websocat nor wscat found. Install one to run this test." >&2
  exit 1
fi

sleep 2  # Let the connection establish

# ── 3. Publish a sensor MQTT message ─────────────────────────────────────────
PAYLOAD='{"temperature":21.5,"humidity":58.2,"timestamp":"2024-01-01T00:00:00Z"}'
TOPIC="climate/$TENANT/$DEVICE/sensor"
echo "==> Publishing to $TOPIC: $PAYLOAD"
mosquitto_pub -h "$MQTT_HOST" -p "$MQTT_PORT" -t "$TOPIC" -m "$PAYLOAD"

sleep 2  # Wait for backend to process and push over WS

# ── 4. Check output ───────────────────────────────────────────────────────────
kill "$WS_PID" 2>/dev/null || true
wait "$WS_PID" 2>/dev/null || true

OUTPUT=$(cat "$OUTFILE")
rm -f "$OUTFILE"

echo "==> Messages received:"
echo "$OUTPUT"

if echo "$OUTPUT" | grep -q '"type":"sensor"'; then
  echo ""
  echo "PASS: received sensor message over WebSocket"
else
  echo ""
  echo "WARN: could not confirm sensor message in output — check above manually"
fi
