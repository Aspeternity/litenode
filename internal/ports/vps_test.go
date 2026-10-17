//go:build vps

// CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c -tags vps -o ports-vps.test ./internal/ports
package ports

import (
	"testing"

	"github.com/Aspeternity/litenode/internal/system"
)

// TestVPS finds sshd's port on the test VPS (61234).
func TestVPS(t *testing.T) {
	l, err := Listening(system.NewReal(nil))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("listening: %v", l)
	if !l[Key{TCP, 61234}] {
		t.Error("sshd port 61234 not found")
	}
}
