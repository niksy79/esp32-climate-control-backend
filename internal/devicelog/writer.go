// Package devicelog writes plain-text log messages received from ESP32 devices
// to per-device log files under logs/<tenantID>/<deviceID>.log.
package devicelog

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Write appends a single log line to logs/<tenantID>/<deviceID>.log.
// The directory is created if it does not exist.
// Format: "2026-03-20T22:07:34Z [deviceID] message\n"
func Write(tenantID, deviceID, message string) error {
	dir := filepath.Join("logs", tenantID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	path := filepath.Join(dir, deviceID+".log")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	line := fmt.Sprintf("%s [%s] %s\n", time.Now().UTC().Format(time.RFC3339), deviceID, message)
	_, err = f.WriteString(line)
	return err
}
