package xray

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/Aspeternity/litenode/internal/node"
)

// Outbound tags.
const (
	tagDirect = "direct"
	tagBlock  = "block"
)

// blockedIPs are private, loopback, link-local, shared, documentation,
// benchmarking, multicast and reserved ranges (IANA special-purpose
// registries). Proxied traffic must not reach the VPS itself or its
// internal network. Written as IP ranges, not geoip:private (D-008).
// IPv4-mapped IPv6 (::ffff:0:0/96) is not listed: Xray rejects that mask
// and matches mapped addresses as IPv4, which the ranges above cover.
var blockedIPs = []string{
	"0.0.0.0/8",
	"10.0.0.0/8",
	"100.64.0.0/10",
	"127.0.0.0/8",
	"169.254.0.0/16",
	"172.16.0.0/12",
	"192.0.0.0/24",
	"192.0.2.0/24",
	"192.168.0.0/16",
	"198.18.0.0/15",
	"198.51.100.0/24",
	"203.0.113.0/24",
	"224.0.0.0/4",
	"240.0.0.0/4",
	"::/128",
	"::1/128",
	"100::/64",
	"2001:db8::/32",
	"fc00::/7",
	"fe80::/10",
	"ff00::/8",
}

type config struct {
	Log       logConfig  `json:"log"`
	Routing   routing    `json:"routing"`
	Inbounds  []inbound  `json:"inbounds"`
	Outbounds []outbound `json:"outbounds"`
}

type logConfig struct {
	LogLevel string `json:"loglevel"`
	Access   string `json:"access"`
}

type routing struct {
	DomainStrategy string `json:"domainStrategy"`
	Rules          []rule `json:"rules"`
}

type rule struct {
	Domain      []string `json:"domain,omitempty"`
	IP          []string `json:"ip,omitempty"`
	OutboundTag string   `json:"outboundTag"`
}

type inbound struct {
	Tag            string          `json:"tag"`
	Port           int             `json:"port"`
	Protocol       string          `json:"protocol"`
	Settings       any             `json:"settings"`
	StreamSettings json.RawMessage `json:"streamSettings"`
}

type outbound struct {
	Tag      string           `json:"tag"`
	Protocol string           `json:"protocol"`
	Settings *freedomSettings `json:"settings,omitempty"`
}

type freedomSettings struct {
	DomainStrategy string `json:"domainStrategy"`
}

// GenerateConfig builds the Xray configuration from the nodes in the
// state (DESIGN section 9.6): warning-level log without access log, no
// stats or API, no geo files, outbounds freedom and blackhole only, and a
// route that blocks private and reserved addresses. The domain strategy
// IPIfNonMatch resolves domains so that names pointing at such addresses
// are blocked too (D-049). Disabled nodes and nodes without enabled
// clients get no inbound. outboundIP is the node outbound IP mode (D-084).
func GenerateConfig(reg *node.Registry, nodes []node.Node, outboundIP string) ([]byte, error) {
	strategy, err := domainStrategy(outboundIP)
	if err != nil {
		return nil, err
	}
	cfg := config{
		Log: logConfig{LogLevel: "warning", Access: "none"},
		Routing: routing{
			DomainStrategy: "IPIfNonMatch",
			Rules: []rule{
				{Domain: []string{"full:localhost"}, OutboundTag: tagBlock},
				{IP: blockedIPs, OutboundTag: tagBlock},
			},
		},
		Inbounds: []inbound{},
		// The first outbound is the default.
		Outbounds: []outbound{{Tag: tagDirect, Protocol: "freedom"}, {Tag: tagBlock, Protocol: "blackhole"}},
	}
	if strategy != "" {
		cfg.Outbounds[0].Settings = &freedomSettings{DomainStrategy: strategy}
	}
	for _, n := range nodes {
		if !n.Enabled || !hasEnabledClient(n) {
			continue
		}
		in, err := buildInbound(reg, n)
		if err != nil {
			return nil, fmt.Errorf("node %d: %w", n.ID, err)
		}
		cfg.Inbounds = append(cfg.Inbounds, in)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode xray config: %w", err)
	}
	return append(data, '\n'), nil
}

func hasEnabledClient(n node.Node) bool {
	for _, c := range n.Clients {
		if c.Enabled {
			return true
		}
	}
	return false
}

// InboundTag names a node's inbound.
func InboundTag(id int64) string {
	return "node-" + strconv.FormatInt(id, 10)
}

func buildInbound(reg *node.Registry, n node.Node) (inbound, error) {
	if err := reg.CheckNode(n); err != nil {
		return inbound{}, err
	}
	p, _ := reg.Protocol(n.Protocol)
	t, _ := reg.Transport(n.Transport)
	s, _ := reg.Security(n.Security)

	settings, err := p.InboundSettings(n)
	if err != nil {
		return inbound{}, err
	}
	stream, err := streamSettings(t, s, n)
	if err != nil {
		return inbound{}, err
	}
	return inbound{
		Tag:            InboundTag(n.ID),
		Port:           n.Port,
		Protocol:       p.Name(),
		Settings:       settings,
		StreamSettings: stream,
	}, nil
}

// jsonField is one key of a JSON object built in a fixed order.
type jsonField struct {
	key   string
	value any
}

// streamSettings assembles {"network", "security", "<transport>Settings",
// "<security>Settings"} in this key order.
func streamSettings(t node.Transport, s node.Security, n node.Node) (json.RawMessage, error) {
	tKey, tSettings, err := t.StreamSettings(n)
	if err != nil {
		return nil, err
	}
	security, sKey, sSettings, err := s.StreamSecurity(n)
	if err != nil {
		return nil, err
	}
	fields := []jsonField{{"network", t.Network()}, {"security", security}}
	if tKey != "" {
		fields = append(fields, jsonField{tKey, tSettings})
	}
	if sKey != "" {
		fields = append(fields, jsonField{sKey, sSettings})
	}
	return orderedObject(fields)
}

func orderedObject(fields []jsonField) (json.RawMessage, error) {
	buf := []byte{'{'}
	for i, f := range fields {
		if i > 0 {
			buf = append(buf, ',')
		}
		k, err := json.Marshal(f.key)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(f.value)
		if err != nil {
			return nil, err
		}
		buf = append(append(append(buf, k...), ':'), v...)
	}
	return append(buf, '}'), nil
}
