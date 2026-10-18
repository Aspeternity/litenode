package forwardmgr

import (
	"context"
	"net"
	"strconv"
	"time"

	"github.com/Aspeternity/litenode/internal/forward"
)

// ProbeTimeout limits each connection attempt of Probe.
const ProbeTimeout = 5 * time.Second

// Dialer opens TCP connections; *net.Dialer implements it.
type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// ProbeResult is the outcome of connecting to one target address.
type ProbeResult struct {
	// Address is "host:port" as dialed.
	Address string
	Latency time.Duration
	Err     error
}

func (s *Service) dialer() Dialer {
	if s.Dialer != nil {
		return s.Dialer
	}
	return &net.Dialer{}
}

// Probe connects from this machine to each address the rule forwards to
// (IPv4 and IPv6, the resolved ones for a domain) on the first target
// port, over TCP (DESIGN section 11). UDP cannot be tested reliably.
func (s *Service) Probe(ctx context.Context, f forward.Forward) []ProbeResult {
	from, _, err := forward.ParsePorts(f.TargetPort)
	if err != nil {
		return []ProbeResult{{Address: f.TargetHost, Err: err}}
	}
	var res []ProbeResult
	for _, ip := range []string{f.IPv4(), f.IPv6()} {
		if ip == "" {
			continue
		}
		addr := net.JoinHostPort(ip, strconv.Itoa(from))
		ctx, cancel := context.WithTimeout(ctx, ProbeTimeout)
		start := time.Now()
		c, err := s.dialer().DialContext(ctx, "tcp", addr)
		r := ProbeResult{Address: addr, Latency: time.Since(start), Err: err}
		cancel()
		if c != nil {
			c.Close()
		}
		res = append(res, r)
	}
	return res
}
