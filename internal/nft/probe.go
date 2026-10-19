package nft

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

// ProbeTable is the temporary table of the node connectivity check
// (DESIGN section 9.9, D-114). It only counts packets and never drops
// them; it is not persistent configuration.
const ProbeTable = "inet litenode-probe"

// ProbeTableName is ProbeTable without the family.
const ProbeTableName = "litenode-probe"

// ProbeSourceLimit is how many source addresses are recorded per protocol
// and address family.
const ProbeSourceLimit = 5

// ProbePort is one protocol / port the check counts.
type ProbePort struct {
	Proto string // "tcp" or "udp"
	Port  int
}

// Probe counter priorities: arrival is counted before any firewall and
// reverse path filter (raw is -300), passing after the filter priority
// (0) of ufw / iptables / the LiteNode firewall.
const (
	probeArrivePriority = -300
	probePassPriority   = 100
)

// GenerateProbe returns the nft script that (re)creates the probe table
// for ps. Like Generate it first creates and deletes the table, so a
// leftover table is replaced in one transaction.
func GenerateProbe(ps []ProbePort) ([]byte, error) {
	if len(ps) == 0 {
		return nil, fmt.Errorf("probe: no ports")
	}
	seen := map[string]bool{}
	for _, p := range ps {
		if p.Proto != "tcp" && p.Proto != "udp" {
			return nil, fmt.Errorf("probe: invalid protocol %q", p.Proto)
		}
		if p.Port < 1 || p.Port > 65535 {
			return nil, fmt.Errorf("probe: invalid port %d", p.Port)
		}
		if seen[p.Proto] {
			return nil, fmt.Errorf("probe: duplicate protocol %q", p.Proto)
		}
		seen[p.Proto] = true
	}
	var b strings.Builder
	b.WriteString("table " + ProbeTable + "\ndelete table " + ProbeTable + "\n\n")
	b.WriteString("table " + ProbeTable + " {\n")
	for _, p := range ps {
		fmt.Fprintf(&b, "\tcounter %s {\n\t}\n", arriveCounter(p.Proto))
		fmt.Fprintf(&b, "\tcounter %s {\n\t}\n", passCounter(p.Proto))
		for _, fam := range []string{"4", "6"} {
			typ := "ipv4_addr"
			if fam == "6" {
				typ = "ipv6_addr"
			}
			fmt.Fprintf(&b, "\tset %s {\n\t\ttype %s\n\t\tsize %d\n\t\tflags dynamic\n\t}\n", sourceSet(p.Proto, fam), typ, ProbeSourceLimit)
		}
	}
	var arrive, pass []string
	for _, p := range ps {
		match := p.Proto + " dport " + strconv.Itoa(p.Port)
		// The counter has its own rule: adding to a full set ends the
		// rule it is in.
		arrive = append(arrive,
			match+` counter name "`+arriveCounter(p.Proto)+`"`,
			"meta nfproto ipv4 "+match+" add @"+sourceSet(p.Proto, "4")+" { ip saddr }",
			"meta nfproto ipv6 "+match+" add @"+sourceSet(p.Proto, "6")+" { ip6 saddr }")
		pass = append(pass, match+` counter name "`+passCounter(p.Proto)+`"`)
	}
	writeChain(&b, "arrive", fmt.Sprintf("type filter hook prerouting priority %d; policy accept;", probeArrivePriority), arrive)
	writeChain(&b, "pass", fmt.Sprintf("type filter hook input priority %d; policy accept;", probePassPriority), pass)
	b.WriteString("}\n")
	return []byte(b.String()), nil
}

func arriveCounter(proto string) string     { return "arrive_" + proto }
func passCounter(proto string) string       { return "pass_" + proto }
func sourceSet(proto, family string) string { return "src_" + proto + family }

// ProbeCount is what the probe table has counted for one protocol.
type ProbeCount struct {
	Arrived uint64       // packets seen in prerouting
	Passed  uint64       // packets that passed the input filters
	Sources []netip.Addr // recorded sources, IPv4 first
}

// ParseProbe parses "nft -j list table inet litenode-probe" into counts by
// protocol ("tcp", "udp"). Protocols without counters are absent.
func ParseProbe(data []byte) (map[string]ProbeCount, error) {
	var doc struct {
		Nftables []map[string]json.RawMessage `json:"nftables"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse probe table: %w", err)
	}
	out := map[string]ProbeCount{}
	sources := map[string]map[string][]netip.Addr{} // proto -> family -> addrs
	for _, obj := range doc.Nftables {
		if raw, ok := obj["counter"]; ok {
			var c struct {
				Table, Name string
				Packets     uint64
			}
			if err := json.Unmarshal(raw, &c); err != nil {
				return nil, fmt.Errorf("parse probe counter: %w", err)
			}
			if c.Table != ProbeTableName {
				continue
			}
			if proto, ok := strings.CutPrefix(c.Name, "arrive_"); ok {
				pc := out[proto]
				pc.Arrived = c.Packets
				out[proto] = pc
			} else if proto, ok := strings.CutPrefix(c.Name, "pass_"); ok {
				pc := out[proto]
				pc.Passed = c.Packets
				out[proto] = pc
			}
		}
		if raw, ok := obj["set"]; ok {
			var s struct {
				Table, Name string
				Elem        []json.RawMessage
			}
			if err := json.Unmarshal(raw, &s); err != nil {
				return nil, fmt.Errorf("parse probe set: %w", err)
			}
			rest, ok := strings.CutPrefix(s.Name, "src_")
			if s.Table != ProbeTableName || !ok || len(rest) < 2 {
				continue
			}
			proto, fam := rest[:len(rest)-1], rest[len(rest)-1:]
			for _, e := range s.Elem {
				a, err := setElemAddr(e)
				if err != nil {
					return nil, err
				}
				if sources[proto] == nil {
					sources[proto] = map[string][]netip.Addr{}
				}
				sources[proto][fam] = append(sources[proto][fam], a)
			}
		}
	}
	for proto, fams := range sources {
		pc := out[proto]
		pc.Sources = append(append([]netip.Addr(nil), fams["4"]...), fams["6"]...)
		out[proto] = pc
	}
	return out, nil
}

// setElemAddr reads a set element: a plain string, or an object
// {"elem": {"val": "..."}} when the element carries extra data.
func setElemAddr(raw json.RawMessage) (netip.Addr, error) {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		var o struct {
			Elem struct{ Val string } `json:"elem"`
		}
		if err := json.Unmarshal(raw, &o); err != nil {
			return netip.Addr{}, fmt.Errorf("parse probe set element %s: %w", raw, err)
		}
		s = o.Elem.Val
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("parse probe set element: %w", err)
	}
	return a, nil
}
