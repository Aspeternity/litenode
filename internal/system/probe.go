package system

import (
	"context"
	"slices"
	"strconv"
	"strings"
)

// Capability is a system feature whose availability is decided by probing
// the running system rather than by the virtualization type.
type Capability string

const (
	CapBBR  Capability = "bbr"
	CapSwap Capability = "swap"
	// CapNetAdmin: this process may change the network stack, which
	// nftables needs (firewall, forwarding, the connectivity check).
	CapNetAdmin Capability = "net_admin"
	// CapDNS: /etc/resolv.conf belongs to this system, not to a container
	// runtime that bind-mounts it.
	CapDNS Capability = "dns"
)

// Probe reports whether a capability is available.
type Probe func(ctx context.Context, r Runner) bool

var probes = map[Capability]Probe{
	CapBBR:      probeBBR,
	CapSwap:     probeSwap,
	CapNetAdmin: probeNetAdmin,
	CapDNS:      probeDNS,
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

// capNetAdminBit is CAP_NET_ADMIN in the capability sets of
// /proc/<pid>/status.
const capNetAdminBit = 12

// probeNetAdmin reads the effective capabilities rather than the
// virtualization type: an LXC container usually has this one, a Docker or
// Podman container usually does not. If they cannot be read, it reports
// true so that nothing is hidden by mistake.
func probeNetAdmin(_ context.Context, r Runner) bool { return HasNetAdmin(r) }

// HasNetAdmin reports whether this process has CAP_NET_ADMIN (see
// probeNetAdmin).
func HasNetAdmin(r Runner) bool {
	data, err := r.ReadFile("/proc/self/status")
	if err != nil {
		return true
	}
	for _, line := range strings.Split(string(data), "\n") {
		if hex, ok := strings.CutPrefix(line, "CapEff:"); ok {
			caps, err := strconv.ParseUint(strings.TrimSpace(hex), 16, 64)
			return err != nil || caps&(1<<capNetAdminBit) != 0
		}
	}
	return true
}

// probeDNS reports whether /etc/resolv.conf can be managed. A container
// runtime bind-mounts it: rename cannot replace a mount point, and the
// runtime writes the file again whenever the container starts.
func probeDNS(_ context.Context, r Runner) bool {
	data, err := r.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return true
	}
	for _, line := range strings.Split(string(data), "\n") {
		// Field 5 is the mount point.
		if f := strings.Fields(line); len(f) > 4 && f[4] == "/etc/resolv.conf" {
			return false
		}
	}
	return true
}

// probeSwap reports whether swap files can be used: the kernel has swap
// support and this is not a container, where swapon is refused (the
// host owns the memory).
func probeSwap(ctx context.Context, r Runner) bool {
	if _, err := r.ReadFile("/proc/swaps"); err != nil {
		return false
	}
	return !DetectVirt(ctx, r).Container
}
