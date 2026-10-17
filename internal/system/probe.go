package system

import (
	"context"
	"slices"
	"strings"
)

// Capability is a system feature whose availability is decided by probing
// the running system rather than by the virtualization type.
type Capability string

const (
	CapBBR Capability = "bbr"
)

// Probe reports whether a capability is available.
type Probe func(ctx context.Context, r Runner) bool

var probes = map[Capability]Probe{
	CapBBR: probeBBR,
}

// Capabilities is the result of probing all registered capabilities.
type Capabilities map[Capability]bool

// Has reports whether c is available. Unknown capabilities are unavailable.
func (c Capabilities) Has(cp Capability) bool { return c[cp] }

// ProbeAll runs every registered probe.
func ProbeAll(ctx context.Context, r Runner) Capabilities {
	caps := Capabilities{}
	for c, p := range probes {
		caps[c] = p(ctx, r)
	}
	return caps
}

// probeBBR reports whether the kernel offers BBR, either already loaded or
// as a loadable tcp_bbr module.
func probeBBR(ctx context.Context, r Runner) bool {
	if data, err := r.ReadFile("/proc/sys/net/ipv4/tcp_available_congestion_control"); err == nil {
		if slices.Contains(strings.Fields(string(data)), "bbr") {
			return true
		}
	}
	_, err := r.Query(ctx, "modinfo", "-n", "tcp_bbr")
	return err == nil
}
