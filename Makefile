.PHONY: dev build test lint docker-up docker-down

# ── Local development ────────────────────────────────────────────────────────
# Exports environment variables pointing at localhost, then runs the server.
# Alternatively, create a .env.local file in the project root — it is loaded
# automatically at startup and never committed to git.
dev:
	DATABASE_URL="postgres://climate:climate@localhost:5432/climate?sslmode=disable" \
	MQTT_URL="tcp://192.168.68.117:1883" \
	JWT_SECRET="dev-secret" \
	LISTEN_ADDR=":8080" \
	go run ./cmd/server/main.go

# ── Build ────────────────────────────────────────────────────────────────────
build:
	go build -o bin/climate-backend ./cmd/server/main.go

# ── Test ─────────────────────────────────────────────────────────────────────
test:
	go test ./...

# ── Lint ─────────────────────────────────────────────────────────────────────
lint:
	go vet ./...

# ── Docker ───────────────────────────────────────────────────────────────────
docker-up:
	docker compose up -d mosquitto timescaledb

docker-down:
	docker compose down
