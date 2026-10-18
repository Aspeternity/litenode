package ssh

import (
	"context"
	"errors"
	"fmt"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/ports"
	"github.com/Aspeternity/litenode/internal/ufw"
)

// Errors of SetPort.
var (
	ErrPortRange = errors.New("port must be 1-65535")
	ErrPortSame  = errors.New("sshd already uses this port")
)

// PortTakenError lists why the new port cannot be used.
type PortTakenError struct {
	Conflicts []ports.Conflict
}

func (e *PortTakenError) Error() string {
	return fmt.Sprintf("port taken: %+v", e.Conflicts)
}

// SSHOwner names the SSH port in port claims.
const SSHOwner = "ssh"

// CurrentPort is the port sshd listens on now (the first, if several).
func (s *Service) CurrentPort(ctx context.Context) (int, error) {
	e, err := ReadEffective(ctx, s.runner())
	if err != nil {
		return 0, err
	}
	if len(e.Ports) == 0 {
		return 0, errors.New("sshd listens on no port")
	}
	return e.Ports[0], nil
}

// SetPort moves sshd to port under confirm-or-rollback (DESIGN section
// 13). The port must be free; sshd's own current port does not count.
// The ufw and firewall rules for the port are part of the same change
// (portParts), so a rollback removes them again. It returns the old port.
func (s *Service) SetPort(ctx context.Context, port int) (int, error) {
	if port < 1 || port > 65535 {
		return 0, ErrPortRange
	}
	r := s.runner()
	e, err := ReadEffective(ctx, r)
	if err != nil {
		return 0, err
	}
	if len(e.Ports) == 1 && e.Ports[0] == port {
		return 0, ErrPortSame
	}
	st, err := s.Store.Load()
	if err != nil {
		return 0, err
	}
	listening, err := ports.Listening(r)
	if err != nil {
		return 0, err
	}
	var self []ports.Key
	for _, p := range e.Ports {
		self = append(self, ports.Key{Proto: ports.TCP, Port: p})
	}
	checker := ports.Checker{Listening: listening, Claims: ports.StateClaims(st.Nodes, st.Forwards)}
	if c := checker.Check([]ports.Key{{Proto: ports.TCP, Port: port}}, SSHOwner, self); len(c) > 0 {
		return 0, &PortTakenError{Conflicts: c}
	}
	old := e.Ports[0]
	st.SSH.Port = port
	_, err = s.Confirmer.Apply(ctx, change.Change{
		Parts: s.portParts(ConfigFile(st.SSH, HasIPv6(r))),
		State: st,
	})
	return old, err
}

// OldPortRules lists the user's own ufw rules that still allow the old
// SSH port, to delete only if the user agrees (D-038). It returns nothing
// unless ufw is active.
func (s *Service) OldPortRules(ctx context.Context, port int) ([]ufw.UserRule, int, error) {
	r := s.runner()
	if !ufw.Active(ctx, r) {
		return nil, 0, nil
	}
	return ufw.PortRules(r, port)
}
