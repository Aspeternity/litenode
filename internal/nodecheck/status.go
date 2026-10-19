// Package nodecheck checks whether client packets reach a node's port and
// pass the local firewall, by counting them in a temporary nftables table
// (DESIGN section 9.9, D-114). This file holds the pure logic: status of
// each protocol, when the check ends and the conclusions.
package nodecheck

import (
	"net/netip"
	"slices"
	"time"

	"github.com/Aspeternity/litenode/internal/nft"
	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/ports"
)

// Thresholds of DESIGN 9.9.
const (
	// MinPackets passed packets make a protocol open; as many arrived
	// packets without any passing for BlockedQuiet make it blocked.
	MinPackets   = 5
	BlockedQuiet = 3 * time.Second
	// Linger is how long counting goes on after every protocol is decided,
	// to record more sources.
	Linger = 2 * time.Second
	// UDPHintAfter and NoPacketsHintAfter are when the waiting hints
	// appear; Limit ends a check nobody is watching.
	UDPHintAfter       = 30 * time.Second
	NoPacketsHintAfter = 60 * time.Second
	Limit              = 30 * time.Minute
)

// NodePorts are the protocols and port the check counts for n: those the
// node serves (ports.NodeKeys).
func NodePorts(n node.Node) []nft.ProbePort {
	var out []nft.ProbePort
	for _, k := range ports.NodeKeys(n, n.Port) {
		out = append(out, nft.ProbePort{Proto: string(k.Proto), Port: k.Port})
	}
	return out
}

// Status is the state of one protocol.
type Status int

const (
	Unreached Status = iota // no packet has arrived
	Waiting                 // packets arrived, not decided yet
	Open                    // MinPackets passed the local firewall
	Blocked                 // packets arrive but none pass
)

// Proto is the count and status of one protocol.
type Proto struct {
	Name   string // "tcp" or "udp"
	Count  nft.ProbeCount
	Status Status
}

// Hint is a waiting hint.
type Hint int

const (
	// HintUDP: TCP is open but no UDP packet arrived after UDPHintAfter.
	HintUDP Hint = iota + 1
	// HintNoPackets: nothing arrived after NoPacketsHintAfter.
	HintNoPackets
)

// Snapshot is the state of the check at one moment.
type Snapshot struct {
	Protos  []Proto
	Elapsed time.Duration
	Hints   []Hint
	// Done is set when the check ends by itself: every protocol decided
	// (plus Linger), or Limit reached (TimedOut).
	Done     bool
	TimedOut bool
}

// Tracker turns successive counter readings into snapshots.
type Tracker struct {
	protos    []string
	start     time.Time
	passed    map[string]uint64
	passedAt  map[string]time.Time // last time Passed grew
	decidedAt time.Time            // zero while some protocol is undecided
}

// NewTracker starts tracking protos at start (when the table was loaded).
func NewTracker(protos []string, start time.Time) *Tracker {
	t := &Tracker{protos: protos, start: start, passed: map[string]uint64{}, passedAt: map[string]time.Time{}}
	for _, p := range protos {
		t.passedAt[p] = start
	}
	return t
}

// Update records counts read at now and returns the snapshot.
func (t *Tracker) Update(counts map[string]nft.ProbeCount, now time.Time) Snapshot {
	s := Snapshot{Elapsed: now.Sub(t.start)}
	decided := true
	for _, name := range t.protos {
		c := counts[name]
		if c.Passed > t.passed[name] {
			t.passed[name] = c.Passed
			t.passedAt[name] = now
		}
		p := Proto{Name: name, Count: c}
		switch {
		case c.Passed >= MinPackets:
			p.Status = Open
		case c.Arrived >= MinPackets && now.Sub(t.passedAt[name]) >= BlockedQuiet:
			p.Status = Blocked
		case c.Arrived > 0:
			p.Status = Waiting
		}
		if p.Status != Open && p.Status != Blocked {
			decided = false
		}
		s.Protos = append(s.Protos, p)
	}
	switch {
	case !decided:
		t.decidedAt = time.Time{}
	case t.decidedAt.IsZero():
		t.decidedAt = now
	}
	s.Done = decided && now.Sub(t.decidedAt) >= Linger
	if !s.Done && s.Elapsed >= Limit {
		s.Done, s.TimedOut = true, true
	}
	s.Hints = hints(s)
	return s
}

func hints(s Snapshot) []Hint {
	var out []Hint
	status := map[string]Status{}
	arrived := false
	for _, p := range s.Protos {
		status[p.Name] = p.Status
		arrived = arrived || p.Count.Arrived > 0
	}
	if s.Elapsed >= UDPHintAfter && status["tcp"] == Open {
		if st, ok := status["udp"]; ok && st == Unreached {
			out = append(out, HintUDP)
		}
	}
	if s.Elapsed >= NoPacketsHintAfter && !arrived {
		out = append(out, HintNoPackets)
	}
	return out
}

// Final settles the protocols still waiting when the check is ended by the
// user or the limit: some packets passed counts as open, none as blocked.
func Final(s Snapshot) []Proto {
	out := slices.Clone(s.Protos)
	for i, p := range out {
		if p.Status == Waiting {
			if p.Count.Passed > 0 {
				out[i].Status = Open
			} else {
				out[i].Status = Blocked
			}
		}
	}
	return out
}

// Env is what the conclusions need to know about the local firewalls.
type Env struct {
	UFW      bool     // ufw is active
	Firewall bool     // the LiteNode firewall is enabled
	Foreign  []string // other inbound filtering (firewall.DetectForeignFilter)
}

// Reason is the conclusion for one protocol.
type Reason int

const (
	ReasonOpen Reason = iota + 1
	ReasonBlockedUFW
	ReasonBlockedFirewall
	// ReasonBlockedOther: neither ufw nor the LiteNode firewall is on, so
	// other inbound rules (Finding.Foreign, may be empty) block the port.
	ReasonBlockedOther
	// ReasonUnreached: nothing arrived; the security group, client address
	// and port, or relay are to check.
	ReasonUnreached
	// ReasonUnreachedUDP: UDP did not arrive although TCP did; the causes
	// are listed without concluding it is blocked.
	ReasonUnreachedUDP
)

// Finding is the conclusion for one protocol.
type Finding struct {
	Proto   string
	Reasons []Reason // one, or one per active firewall when blocked
	Sources []netip.Addr
	Foreign []string // with ReasonBlockedOther
}

// Conclude gives the findings for the settled protocols (Final).
func Conclude(protos []Proto, env Env) []Finding {
	tcpArrived := false
	for _, p := range protos {
		if p.Name == "tcp" && p.Status != Unreached {
			tcpArrived = true
		}
	}
	var out []Finding
	for _, p := range protos {
		f := Finding{Proto: p.Name, Sources: p.Count.Sources}
		switch p.Status {
		case Open:
			f.Reasons = []Reason{ReasonOpen}
		case Blocked:
			if env.UFW {
				f.Reasons = append(f.Reasons, ReasonBlockedUFW)
			}
			if env.Firewall {
				f.Reasons = append(f.Reasons, ReasonBlockedFirewall)
			}
			if len(f.Reasons) == 0 {
				f.Reasons = []Reason{ReasonBlockedOther}
				f.Foreign = env.Foreign
			}
		default:
			if p.Name == "udp" && tcpArrived {
				f.Reasons = []Reason{ReasonUnreachedUDP}
			} else {
				f.Reasons = []Reason{ReasonUnreached}
			}
		}
		out = append(out, f)
	}
	return out
}

// AnyBlocked reports whether a settled protocol is blocked, i.e. whether
// Conclude needs the firewall environment.
func AnyBlocked(protos []Proto) bool {
	return slices.ContainsFunc(protos, func(p Proto) bool { return p.Status == Blocked })
}
