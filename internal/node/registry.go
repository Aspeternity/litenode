package node

import (
	"errors"
	"fmt"
	"io"
	"slices"
)

// Param is one query parameter of a share link. Links keep the order in
// which components add their parameters.
type Param struct {
	Key, Value string
}

// Component is a protocol, transport or security layer.
type Component interface {
	// Name is the identifier stored in the state file, e.g. "vless".
	Name() string
	// Label is the display name, e.g. "VLESS".
	Label() string
}

// Protocol is a proxy protocol such as VLESS.
type Protocol interface {
	Component
	// InboundSettings returns the "settings" object of the node's Xray
	// inbound. Only enabled clients are included.
	InboundSettings(n Node) (any, error)
	// ShareLink builds a client's share link from the address, port,
	// the protocol's own parameters followed by params from the security
	// layer and transport, and the remark as the URL fragment.
	ShareLink(n Node, c Client, host string, port int, params []Param) (string, error)
}

// Transport is a transport such as raw TCP or XHTTP.
type Transport interface {
	Component
	// Network is the value of streamSettings.network.
	Network() string
	// StreamSettings returns the transport's object in streamSettings and
	// its key (e.g. "xhttpSettings"), or key "" if it needs none.
	StreamSettings(n Node) (key string, settings any, err error)
	// ShareParams returns the transport's share link parameters.
	ShareParams(n Node) ([]Param, error)
}

// Security is a security layer such as REALITY, TLS or none.
type Security interface {
	Component
	// StreamSecurity returns streamSettings.security and the layer's
	// settings object with its key (e.g. "realitySettings"), or key "" if
	// it needs none.
	StreamSecurity(n Node) (security, key string, settings any, err error)
	// ShareParams returns the security layer's share link parameters.
	ShareParams(n Node) ([]Param, error)
}

// Combo is a protocol + transport + security combination with a flow.
type Combo struct {
	Protocol  string
	Transport string
	Security  string
	// Flow is the client flow, "" for none.
	Flow string
}

// Compat declares a legal protocol + transport + security combination and
// the flows allowed with it ("" means no flow).
type Compat struct {
	Protocol  string
	Transport string
	Security  string
	Flows     []string
}

// Preset is a legal combination with defaults, offered in the menu. Users
// pick presets; they never assemble components freely.
type Preset struct {
	// ID is stable, e.g. "vless-tcp-reality-vision".
	ID    string
	Label string
	Combo Combo
	// Build creates a complete node from the user's answers, generating
	// keys and IDs with the preset's defaults.
	Build func(in BuildInput) (Node, error)
}

// BuildInput is what the add-node flow collects before building a node.
type BuildInput struct {
	// NodeID and ClientID are fresh stable IDs (State.AllocID).
	NodeID   int64
	ClientID int64
	Port     int
	// Remark is the node remark; the first client gets the same remark
	// (D-039).
	Remark string
	// Target is the REALITY target "host:port" (security layers that use
	// one).
	Target string
	// Rand is the randomness source (crypto/rand.Reader).
	Rand io.Reader
}

// ErrIllegalCombo means the combination is not in the compatibility table.
var ErrIllegalCombo = errors.New("illegal protocol/transport/security combination")

// Registry holds the compiled-in components, the compatibility table and
// the presets. Components are registered explicitly in package builtin.
type Registry struct {
	protocols  map[string]Protocol
	transports map[string]Transport
	securities map[string]Security
	compat     []Compat
	presets    []Preset
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		protocols:  map[string]Protocol{},
		transports: map[string]Transport{},
		securities: map[string]Security{},
	}
}

func register[T Component](m map[string]T, kind string, c T) error {
	if c.Name() == "" {
		return fmt.Errorf("%s with empty name", kind)
	}
	if _, dup := m[c.Name()]; dup {
		return fmt.Errorf("%s %q registered twice", kind, c.Name())
	}
	m[c.Name()] = c
	return nil
}

// AddProtocol registers a protocol.
func (r *Registry) AddProtocol(p Protocol) error { return register(r.protocols, "protocol", p) }

// AddTransport registers a transport.
func (r *Registry) AddTransport(t Transport) error { return register(r.transports, "transport", t) }

// AddSecurity registers a security layer.
func (r *Registry) AddSecurity(s Security) error { return register(r.securities, "security", s) }

// AddCompat declares a legal combination. Its components must already be
// registered.
func (r *Registry) AddCompat(c Compat) error {
	if err := r.checkComponents(c.Protocol, c.Transport, c.Security); err != nil {
		return err
	}
	r.compat = append(r.compat, c)
	return nil
}

// AddPreset registers a preset. Its combination must be legal and its ID
// unique.
func (r *Registry) AddPreset(p Preset) error {
	if p.ID == "" {
		return errors.New("preset with empty ID")
	}
	if slices.ContainsFunc(r.presets, func(q Preset) bool { return q.ID == p.ID }) {
		return fmt.Errorf("preset %q registered twice", p.ID)
	}
	if err := r.Check(p.Combo); err != nil {
		return fmt.Errorf("preset %q: %w", p.ID, err)
	}
	r.presets = append(r.presets, p)
	return nil
}

// Presets returns the presets in registration order. None is marked as
// recommended; the user picks the node type (D-060).
func (r *Registry) Presets() []Preset {
	return slices.Clone(r.presets)
}

// PresetFlow is the flow of the first preset with the given protocol,
// transport and security, "" if none. A node without clients uses it for
// its next client (D-060).
func (r *Registry) PresetFlow(protocol, transport, security string) string {
	for _, p := range r.presets {
		if p.Combo.Protocol == protocol && p.Combo.Transport == transport && p.Combo.Security == security {
			return p.Combo.Flow
		}
	}
	return ""
}

// Preset returns the preset with the given ID.
func (r *Registry) Preset(id string) (Preset, bool) {
	i := slices.IndexFunc(r.presets, func(p Preset) bool { return p.ID == id })
	if i < 0 {
		return Preset{}, false
	}
	return r.presets[i], true
}

// Protocol returns a registered protocol.
func (r *Registry) Protocol(name string) (Protocol, bool) {
	p, ok := r.protocols[name]
	return p, ok
}

// Transport returns a registered transport.
func (r *Registry) Transport(name string) (Transport, bool) {
	t, ok := r.transports[name]
	return t, ok
}

// Security returns a registered security layer.
func (r *Registry) Security(name string) (Security, bool) {
	s, ok := r.securities[name]
	return s, ok
}

func (r *Registry) checkComponents(protocol, transport, security string) error {
	if _, ok := r.protocols[protocol]; !ok {
		return fmt.Errorf("unknown protocol %q", protocol)
	}
	if _, ok := r.transports[transport]; !ok {
		return fmt.Errorf("unknown transport %q", transport)
	}
	if _, ok := r.securities[security]; !ok {
		return fmt.Errorf("unknown security %q", security)
	}
	return nil
}

// Check reports whether a combination is legal: all components registered,
// listed in the compatibility table, and the flow allowed with it.
func (r *Registry) Check(c Combo) error {
	if err := r.checkComponents(c.Protocol, c.Transport, c.Security); err != nil {
		return err
	}
	for _, e := range r.compat {
		if e.Protocol == c.Protocol && e.Transport == c.Transport && e.Security == c.Security {
			if slices.Contains(e.Flows, c.Flow) {
				return nil
			}
			return fmt.Errorf("%w: flow %q not allowed with %s+%s+%s",
				ErrIllegalCombo, c.Flow, c.Protocol, c.Transport, c.Security)
		}
	}
	return fmt.Errorf("%w: %s+%s+%s", ErrIllegalCombo, c.Protocol, c.Transport, c.Security)
}

// CheckNode checks a node's combination with each client's flow, and that
// the settings of its security layer are present.
func (r *Registry) CheckNode(n Node) error {
	if len(n.Clients) == 0 {
		return fmt.Errorf("node %d has no clients", n.ID)
	}
	for _, c := range n.Clients {
		combo := Combo{Protocol: n.Protocol, Transport: n.Transport, Security: n.Security, Flow: c.Flow}
		if err := r.Check(combo); err != nil {
			return fmt.Errorf("node %d client %d: %w", n.ID, c.ID, err)
		}
	}
	if n.Security == "reality" && n.Reality == nil {
		return fmt.Errorf("node %d: reality settings missing", n.ID)
	}
	// Without a security layer only VLESS Encryption protects the traffic.
	if n.Security == "none" && n.VLESSEncryption == nil {
		return fmt.Errorf("node %d: vless encryption settings missing", n.ID)
	}
	if n.VLESSEncryption != nil && n.Protocol != "vless" {
		return fmt.Errorf("node %d: vless encryption on protocol %q", n.ID, n.Protocol)
	}
	return nil
}
