package warp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/netip"
	"time"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/egress"
	"github.com/Aspeternity/litenode/internal/system"
)

// Errors of Service.
var (
	// ErrNoKernel means the kernel has no WireGuard (containers).
	ErrNoKernel = errors.New("kernel has no wireguard support")
	// ErrNoEndpoint means no endpoint completed a handshake that carries
	// traffic.
	ErrNoEndpoint = errors.New("no warp endpoint works")
	// ErrNotEnabled means there is no LiteNode WARP configuration.
	ErrNotEnabled = errors.New("warp is not enabled")
	// ErrNoHandshake means the tunnel did not complete a handshake.
	ErrNoHandshake = errors.New("no wireguard handshake")
)

// Service enables, checks and disables WARP. Fields other than Runner and
// Changes have working defaults.
type Service struct {
	Runner  system.Runner
	Changes *change.Manager
	// Client talks to the registration API.
	Client *http.Client
	// Trace asks Cloudflare over one family (egress.TraceFamily).
	Trace func(ctx context.Context, v6 bool) (egress.Trace, error)
	Rand  io.Reader
	Now   func() time.Time
	Sleep func(time.Duration)
	// HandshakeWait bounds the wait for the first handshake.
	HandshakeWait time.Duration
	// Executable is the program the check timer runs (default
	// /usr/local/bin/litenode).
	Executable string
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) sleep(d time.Duration) {
	if s.Sleep != nil {
		s.Sleep(d)
		return
	}
	time.Sleep(d)
}

func (s *Service) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func (s *Service) trace(ctx context.Context, v6 bool) (egress.Trace, error) {
	if s.Trace != nil {
		return s.Trace(ctx, v6)
	}
	return egress.TraceFamily(ctx, v6)
}

// Conf returns the current configuration; ErrNotEnabled if there is none.
func (s *Service) Conf() (Conf, error) {
	return readConf(s.Runner)
}

func readConf(r system.Runner) (Conf, error) {
	data, err := r.ReadFile(ConfPath)
	if errors.Is(err, fs.ErrNotExist) {
		return Conf{}, ErrNotEnabled
	}
	if err != nil {
		return Conf{}, fmt.Errorf("read %s: %w", ConfPath, err)
	}
	return ParseConf(data)
}

// Status is shown in the WARP menu.
type Status struct {
	Enabled bool
	Conf    Conf
	// Active reports whether the wg-quick service runs.
	Active bool
	// Handshake is the latest handshake, zero if none.
	Handshake time.Time
}

// Status reads the configuration, service state and latest handshake.
func (s *Service) Status(ctx context.Context) (Status, error) {
	c, err := s.Conf()
	if errors.Is(err, ErrNotEnabled) {
		return Status{}, nil
	}
	if err != nil {
		return Status{}, err
	}
	st := Status{Enabled: true, Conf: c}
	st.Active, _ = system.ServiceActive(ctx, s.Runner, Unit)
	if res, err := s.Runner.Query(ctx, "wg", "show", Interface, "latest-handshakes"); err == nil {
		st.Handshake = ParseHandshake(res.Stdout)
	}
	return st, nil
}

// Kind is the "warp" configuration kind. Apply starts the tunnel when the
// configuration exists and checks that it carries traffic of its family;
// otherwise it stops and disables the service.
func (s *Service) Kind() *change.Kind {
	return &change.Kind{
		Name: "warp",
		Apply: func(ctx context.Context, r system.Runner) error {
			c, err := readConf(r)
			if errors.Is(err, ErrNotEnabled) {
				return down(ctx, r)
			}
			if err != nil {
				return err
			}
			if err := system.Systemctl(ctx, r, system.ServiceEnable, Unit); err != nil {
				return err
			}
			if err := system.Systemctl(ctx, r, system.ServiceRestart, Unit); err != nil {
				return err
			}
			if err := s.check(ctx, c); err != nil {
				return err
			}
			return s.installCheck(ctx, r)
		},
	}
}

// downConf is a stand-in configuration for "wg-quick down": the real
// file is already gone when the change flow removed or restored it, and
// wg-quick refuses to run without one. Taking the interface down only
// needs the live interface.
const downConf = "/run/litenode/" + Interface + ".conf"

// down stops the tunnel after its configuration was removed: the unit's
// stop fails without the file, so the interface, its rules and nftables
// table are removed with the stand-in file.
func down(ctx context.Context, r system.Runner) error {
	RemoveCheck(ctx, r)
	// The unit may not exist (wireguard-tools removed); errors are
	// expected here.
	_, _ = r.Exec(ctx, "systemctl", "disable", "--now", Unit)
	if _, err := r.Query(ctx, "ip", "link", "show", "dev", Interface); err == nil {
		if err := r.MkdirAll("/run/litenode", 0o700); err != nil {
			return fmt.Errorf("create /run/litenode: %w", err)
		}
		if err := r.WriteFile(downConf, []byte("[Interface]\n"), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", downConf, err)
		}
		_, err := r.Exec(ctx, "wg-quick", "down", downConf)
		_ = r.Remove(downConf)
		if err != nil {
			return fmt.Errorf("take down %s: %w", Interface, err)
		}
	}
	_, _ = r.Exec(ctx, "systemctl", "reset-failed", Unit)
	return nil
}

// check waits for a handshake, then asks Cloudflare over the supplied
// family.
func (s *Service) check(ctx context.Context, c Conf) error {
	wait := s.HandshakeWait
	if wait <= 0 {
		wait = 10 * time.Second
	}
	deadline := s.now().Add(wait)
	for {
		res, err := s.Runner.Query(ctx, "wg", "show", Interface, "latest-handshakes")
		if err == nil && !ParseHandshake(res.Stdout).IsZero() {
			break
		}
		if !s.now().Before(deadline) {
			return fmt.Errorf("%s: %w", c.Endpoint, ErrNoHandshake)
		}
		s.sleep(time.Second)
	}
	// Routes may take a moment after the first handshake.
	var err error
	for range 3 {
		if _, err = s.trace(ctx, c.IPv6); err == nil {
			return nil
		}
		s.sleep(time.Second)
	}
	return fmt.Errorf("%s exit through %s: %w", FamilyName(c.IPv6), c.Endpoint, err)
}

// prepare installs wireguard-tools when missing and loads the kernel
// module.
func (s *Service) prepare(ctx context.Context) error {
	if _, err := s.Runner.Query(ctx, "wg", "--version"); err != nil {
		if _, err := s.Runner.Exec(ctx, "env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "-y", "--no-install-recommends", "wireguard-tools"); err != nil {
			return fmt.Errorf("install wireguard-tools: %w", err)
		}
	}
	if _, err := s.Runner.Exec(ctx, "modprobe", "wireguard"); err != nil {
		return fmt.Errorf("%w: %v", ErrNoKernel, err)
	}
	return nil
}

func (s *Service) request(ctx context.Context, method, url, token string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "okhttp/3.12.1")
	req.Header.Set("CF-Client-Version", ClientVersion)
	if body != nil {
		req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, url, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, url, err)
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("%s %s: HTTP %d", method, url, resp.StatusCode)
	}
	return data, nil
}

// register creates a WARP device for a new key pair.
func (s *Service) register(ctx context.Context) (Keys, Account, error) {
	keys, err := NewKeys(s.Rand)
	if err != nil {
		return Keys{}, Account{}, err
	}
	data, err := s.request(ctx, http.MethodPost, APIURL, "", RegisterBody(keys.Public, s.now()))
	if err != nil {
		return Keys{}, Account{}, fmt.Errorf("register warp device: %w", err)
	}
	a, err := ParseAccount(data)
	return keys, a, err
}

// unregister deletes the device; failures are ignored (the device only
// stays listed at Cloudflare).
func (s *Service) unregister(ctx context.Context, c Conf) {
	if c.AccountID == "" || c.Token == "" {
		return
	}
	_, _ = s.request(ctx, http.MethodDelete, APIURL+"/"+c.AccountID, c.Token, nil)
}

// Enable registers a device and brings up a tunnel for the family this
// host lacks (ipv6: supply IPv6 to an IPv4-only host), trying endpoints
// until one works. try is called before each attempt. When none works,
// nothing is left behind.
func (s *Service) Enable(ctx context.Context, ipv6 bool, try func(netip.AddrPort)) error {
	if _, err := s.Conf(); err == nil {
		return s.Reselect(ctx, try)
	}
	if err := s.prepare(ctx); err != nil {
		return err
	}
	keys, a, err := s.register(ctx)
	if err != nil {
		return err
	}
	addr := a.Addr4
	if ipv6 {
		addr = a.Addr6
	}
	c := Conf{AccountID: a.ID, Token: a.Token, IPv6: ipv6, PrivateKey: keys.Private, Address: addr, PeerKey: a.PeerKey}
	// The endpoint is reached over the family this host has.
	if err := s.connect(ctx, c, Candidates(a.Endpoints, a.Ports, !ipv6, netip.AddrPort{}), try); err != nil {
		s.unregister(ctx, c)
		return err
	}
	return nil
}

// Reselect tries the endpoints again, the current one last.
func (s *Service) Reselect(ctx context.Context, try func(netip.AddrPort)) error {
	c, err := s.Conf()
	if err != nil {
		return err
	}
	if err := s.prepare(ctx); err != nil {
		return err
	}
	return s.connect(ctx, c, Candidates(nil, nil, !c.IPv6, c.Endpoint), try)
}

// connect applies c with each endpoint in turn. A failed attempt is
// undone by the change flow, which restores the previous file.
func (s *Service) connect(ctx context.Context, c Conf, endpoints []netip.AddrPort, try func(netip.AddrPort)) error {
	if err := s.Runner.MkdirAll("/etc/wireguard", 0o700); err != nil {
		return fmt.Errorf("create /etc/wireguard: %w", err)
	}
	var last error
	for _, ep := range endpoints {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if try != nil {
			try(ep)
		}
		c.Endpoint = ep
		_, err := s.Changes.Apply(ctx, change.Change{Parts: []change.Part{{Kind: s.Kind(),
			Files: []change.File{{Path: ConfPath, Data: c.Render(), Mode: 0o600}}}}})
		if err == nil {
			return nil
		}
		last = err
	}
	return fmt.Errorf("%w: %v", ErrNoEndpoint, last)
}

// Disable stops the tunnel, removes its configuration and deletes the
// device at Cloudflare (best effort).
func (s *Service) Disable(ctx context.Context) error {
	c, err := s.Conf()
	if err != nil && !errors.Is(err, ErrNotEnabled) {
		// An unreadable file is still LiteNode's: remove it.
		c = Conf{}
	}
	if _, err := s.Changes.Apply(ctx, change.Change{Parts: []change.Part{{Kind: s.Kind(),
		Files: []change.File{{Path: ConfPath, Remove: true}}}}}); err != nil {
		return err
	}
	s.unregister(ctx, c)
	return nil
}
