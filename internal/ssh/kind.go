package ssh

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/ports"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
)

// ConfigFile is the change of the drop-in for s; it is removed when s
// manages nothing.
func ConfigFile(s state.SSH, ipv6 bool) change.File {
	data := Generate(s, ipv6)
	if data == nil {
		return change.File{Path: ConfigPath, Remove: true}
	}
	return change.File{Path: ConfigPath, Data: data, Mode: state.PrivateFileMode}
}

// PrivsepDir must exist for "sshd -t" and "sshd -T". ssh.service creates
// it when it starts; under socket activation (Ubuntu) that may not have
// happened yet, e.g. at boot when the boot-time rollback runs.
const PrivsepDir = "/run/sshd"

func ensurePrivsepDir(r system.Runner) error {
	if _, err := r.ReadDir(PrivsepDir); err == nil {
		return nil
	}
	return system.EnsureDir(r, PrivsepDir, 0o755)
}

// ReadEffective runs "sshd -T".
func ReadEffective(ctx context.Context, r system.Runner) (Effective, error) {
	if err := ensurePrivsepDir(r); err != nil {
		return Effective{}, err
	}
	res, err := r.Query(ctx, SshdPath, "-T")
	if err != nil {
		return Effective{}, fmt.Errorf("sshd -T: %w (%s)", err, strings.TrimSpace(res.Stderr))
	}
	return ParseEffective(res.Stdout)
}

// Sleep waits between checks; tests replace it.
var Sleep = time.Sleep

// SocketUnit is Ubuntu's socket activation for sshd (masked on Debian).
const SocketUnit = "ssh.socket"

// HasIPv6 reports whether the kernel has IPv6.
func HasIPv6(r system.Runner) bool {
	data, err := r.ReadFile("/proc/net/if_inet6")
	return err == nil && len(strings.TrimSpace(string(data))) > 0
}

// socketPorts are the ports ssh.socket listens on.
func socketPorts(ctx context.Context, r system.Runner) ([]int, error) {
	res, err := r.Query(ctx, "systemctl", "show", "--property=Listen", "--value", SocketUnit)
	if err != nil {
		return nil, fmt.Errorf("systemctl show %s: %w", SocketUnit, err)
	}
	var out []int
	for _, f := range strings.Fields(res.Stdout) {
		i := strings.LastIndexByte(f, ':')
		if i < 0 {
			continue // "(Stream)"
		}
		if p, err := strconv.Atoi(f[i+1:]); err == nil && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out, nil
}

func samePorts(a, b []int) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

// reload makes sshd use the configuration on disk. Under socket
// activation (Ubuntu) a generator turns sshd's ports into ssh.socket's
// ListenStream at daemon-reload, and the socket is restarted when its
// ports change (D-065); established sessions survive.
func reload(ctx context.Context, r system.Runner) error {
	socket, err := system.ServiceActive(ctx, r, SocketUnit)
	if err != nil {
		return err
	}
	if socket {
		// The socket's ports as loaded, before daemon-reload loads the
		// generator's new ones: what it listens on now.
		have, err := socketPorts(ctx, r)
		if err != nil {
			return err
		}
		if err := system.DaemonReload(ctx, r); err != nil {
			return err
		}
		e, err := ReadEffective(ctx, r)
		if err != nil {
			return err
		}
		if !samePorts(have, e.Ports) {
			return system.Systemctl(ctx, r, system.ServiceRestart, SocketUnit)
		}
	}
	active, err := system.ServiceActive(ctx, r, Unit)
	if err != nil || !active {
		return err // not running yet: it reads the configuration when it starts
	}
	return system.Systemctl(ctx, r, system.ServiceReload, Unit)
}

// waitListening waits up to two seconds for sshd (or ssh.socket) to
// listen on port.
func waitListening(r system.Runner, port int, sleep func(time.Duration)) error {
	for i := 0; ; i++ {
		l, err := ports.Listening(r)
		if err != nil {
			return err
		}
		if l[ports.Key{Proto: ports.TCP, Port: port}] {
			return nil
		}
		if i == 10 {
			return fmt.Errorf("%w: nothing listens on port %d", ErrNotEffective, port)
		}
		sleep(200 * time.Millisecond)
	}
}

// Kind is the "sshd" configuration kind. sshd cannot check a file
// elsewhere, so the drop-in is validated in place with "sshd -t" before
// sshd reloads. Apply reloads a running sshd and then checks with
// "sshd -T" that sshd uses the settings in the state on disk, so it also
// verifies a restored configuration.
func Kind(store *state.Store) *change.Kind {
	return &change.Kind{
		Name:            "sshd",
		ValidateInPlace: true,
		Validate: func(ctx context.Context, r system.Runner, _ map[string]string) error {
			main, err := r.ReadFile(MainConfigPath)
			if err != nil {
				return err
			}
			if err := CheckInclude(main); err != nil {
				return err
			}
			if err := ensurePrivsepDir(r); err != nil {
				return err
			}
			if res, err := r.Query(ctx, SshdPath, "-t"); err != nil {
				return fmt.Errorf("sshd -t: %w (%s)", err, strings.TrimSpace(res.Stderr))
			}
			return nil
		},
		Apply: func(ctx context.Context, r system.Runner) error {
			st, err := store.Load()
			if err != nil {
				return err
			}
			if err := reload(ctx, r); err != nil {
				return err
			}
			e, err := ReadEffective(ctx, r)
			if err != nil {
				return err
			}
			if err := Check(st.SSH, e); err != nil {
				return err
			}
			if st.SSH.Port > 0 {
				return waitListening(r, st.SSH.Port, Sleep)
			}
			return nil
		},
	}
}
