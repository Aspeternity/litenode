package xray

import "fmt"

// Node outbound IP modes, stored in xray.outbound_ip (DESIGN section 12,
// D-084). They set the domainStrategy of the freedom outbound and are
// independent of the system gai.conf preference.
const (
	OutboundDefault    = ""
	OutboundPreferIPv4 = "prefer_ipv4"
	OutboundPreferIPv6 = "prefer_ipv6"
	OutboundIPv4Only   = "ipv4_only"
	OutboundIPv6Only   = "ipv6_only"
)

// OutboundModes are the modes in menu order.
var OutboundModes = []string{OutboundDefault, OutboundPreferIPv4, OutboundPreferIPv6, OutboundIPv4Only, OutboundIPv6Only}

// outboundStrategies maps a mode to the freedom domainStrategy. The
// default (AsIs) leaves the field out.
var outboundStrategies = map[string]string{
	OutboundDefault:    "",
	OutboundPreferIPv4: "UseIPv4v6",
	OutboundPreferIPv6: "UseIPv6v4",
	OutboundIPv4Only:   "ForceIPv4",
	OutboundIPv6Only:   "ForceIPv6",
}

func domainStrategy(mode string) (string, error) {
	s, ok := outboundStrategies[mode]
	if !ok {
		return "", fmt.Errorf("unknown outbound ip mode %q", mode)
	}
	return s, nil
}

// OnlyIPv6 reports the family an "only" mode requires: ok is false for the
// modes that do not restrict the family.
func OnlyIPv6(mode string) (ipv6, ok bool) {
	switch mode {
	case OutboundIPv4Only:
		return false, true
	case OutboundIPv6Only:
		return true, true
	}
	return false, false
}

// OutboundCheck tells whether mode suits the exits the machine has
// (egress detection, WARP included; D-134): an "only" mode for a family
// without an exit is refused, a "prefer" mode for one needs confirmation.
// When neither family has an exit the detection tells nothing and
// nothing is refused.
func OutboundCheck(mode string, ipv4, ipv6 bool) (refuse, confirm bool) {
	if !ipv4 && !ipv6 {
		return false, false
	}
	switch mode {
	case OutboundIPv4Only:
		return !ipv4, false
	case OutboundIPv6Only:
		return !ipv6, false
	case OutboundPreferIPv4:
		return false, !ipv4
	case OutboundPreferIPv6:
		return false, !ipv6
	}
	return false, false
}
