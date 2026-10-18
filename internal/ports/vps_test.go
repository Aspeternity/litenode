//go:build vps

// CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c -tags vps -o ports-vps.test ./internal/ports
// VPS_SSH_PORT=61234 ./ports-vps.test -test.v -test.run TestVPS
package ports

import (
	"os"
	"strconv"
	"testing"

	"github.com/Aspeternity/litenode/internal/system"
)

// TestVPS finds sshd's port on the test VPS: VPS_SSH_PORT, default 22.
func TestVPS(t *testing.T) {
	port := 22
	if v := os.Getenv("VPS_SSH_PORT"); v != "" {
		port, _ = strconv.Atoi(v)
	}
	l, err := Listening(system.NewReal(nil))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("listening: %v", l)
	if !l[Key{TCP, port}] {
		t.Errorf("sshd port %d not found", port)
	}
}
