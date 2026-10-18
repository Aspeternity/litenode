// Package app implements the startup flow and subcommand routing.
package app

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/firewall"
	"github.com/Aspeternity/litenode/internal/forward"
	"github.com/Aspeternity/litenode/internal/forwardmgr"
	"github.com/Aspeternity/litenode/internal/node/builtin"
	"github.com/Aspeternity/litenode/internal/nodemgr"
	"github.com/Aspeternity/litenode/internal/reality"
	"github.com/Aspeternity/litenode/internal/share"
	"github.com/Aspeternity/litenode/internal/ssh"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/sysctl"
	"github.com/Aspeternity/litenode/internal/sysopt"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/text"
	"github.com/Aspeternity/litenode/internal/ui"
	"github.com/Aspeternity/litenode/internal/xray"
)

// Env is what LiteNode needs from the outside world.
type Env struct {
	// Runner executes system operations for real. Run wraps it in a
	// dry-run Runner when --dry-run is given.
	Runner system.Runner
	Euid   int
	Stdout io.Writer
	Stderr io.Writer
	// Args are the command-line arguments without the program name.
	Args []string
	// Version is the build version, "dev" when not injected.
	Version string
	// Executable is the absolute path of the litenode binary, used in
	// systemd units that call back into LiteNode.
	Executable string
	// LockPath is the instance lock file.
	LockPath string
	// Kinds lists the configuration kinds the rollback subcommand handles.
	Kinds change.Registry
	// Session reads menu input. Nil means no interactive menu (tests).
	Session *ui.Session

	// Optional overrides for tests; nil uses the real implementations.
	TargetChecker   TargetChecker
	TargetScanner   TargetScanner
	AddressDetector AddressDetector
	XrayFetcher     xray.Fetcher
	Rand            io.Reader
	Resolver        share.Resolver
	ForwardResolver forward.Resolver
	ForwardDialer   forwardmgr.Dialer
}

func (env Env) fetcher() xray.Fetcher {
	if env.XrayFetcher != nil {
		return env.XrayFetcher
	}
	return xray.NewHTTPFetcher()
}

func (env Env) rand() io.Reader {
	if env.Rand != nil {
		return env.Rand
	}
	return rand.Reader
}

// services builds the node service and its dependencies on r.
func (env Env) services(r system.Runner, store *state.Store) (*nodemgr.Service, error) {
	reg, err := builtin.Registry()
	if err != nil {
		return nil, err
	}
	fetcher, rnd := env.fetcher(), env.rand()
	return &nodemgr.Service{
		Runner:   r,
		Store:    store,
		Changes:  change.NewManager(r, store),
		Registry: reg,
		Xray:     xray.NewInstaller(r, fetcher, runtime.GOARCH),
		Rand:     rnd,
	}, nil
}

// kinds is the registry the rollback subcommand uses: every kind a
// confirm-or-rollback change may contain.
func (env Env) kinds(r system.Runner, store *state.Store) change.Registry {
	if env.Kinds != nil {
		return env.Kinds
	}
	reg := change.Registry{}
	if svc, err := env.services(r, store); err == nil {
		for _, k := range svc.Kinds() {
			reg.Register(k)
		}
	}
	for _, k := range (&ssh.Service{Store: store}).Kinds() {
		reg.Register(k)
	}
	for _, k := range (&firewall.Service{Store: store}).Kinds() {
		reg.Register(k) // replaces ufw with an identical kind
	}
	reg.Register(firewall.SyncKind(store))
	reg.Register(sysctl.Kind())
	return reg
}

// Run dispatches the command line and returns the process exit code.
//
//	litenode [--dry-run]           main menu
//	litenode version | -v | --version
//	litenode help | -h | --help
//	litenode internal rollback   (called by systemd; not in help)
//	litenode internal resolve    (called by systemd; not in help)
func Run(ctx context.Context, env Env) int {
	args := env.Args
	switch {
	case len(args) == 0:
		return runMenu(ctx, env, false)
	case len(args) == 1 && args[0] == "--dry-run":
		return runMenu(ctx, env, true)
	case len(args) == 1 && (args[0] == "version" || args[0] == "-v" || args[0] == "--version"):
		fmt.Fprintln(env.Stdout, text.Version(env.Version, runtime.GOOS+"/"+runtime.GOARCH))
		return 0
	case len(args) == 1 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help"):
		fmt.Fprint(env.Stdout, text.Help)
		return 0
	case len(args) >= 1 && args[0] == "internal":
		return runInternal(ctx, env, args[1:])
	}
	fmt.Fprintln(env.Stderr, text.UnknownCommand(strings.Join(args, " ")))
	return 2
}

// runInternal handles subcommands that systemd units call. They are not
// listed in help.
func runInternal(ctx context.Context, env Env, args []string) int {
	if len(args) == 1 && args[0] == "rollback" {
		return rollback(ctx, env)
	}
	if len(args) == 1 && args[0] == "resolve" {
		return resolve(ctx, env)
	}
	fmt.Fprintln(env.Stderr, text.UnknownCommand(strings.Join(append([]string{"internal"}, args...), " ")))
	return 2
}

// runMenu checks the environment, takes the instance lock, loads the state
// and shows the main menu.
func runMenu(ctx context.Context, env Env, dryRun bool) int {
	r := env.Runner
	if dryRun {
		r = system.NewDryRun(env.Runner, env.Stdout)
	}
	info, code := startupCheck(ctx, env.Euid, r, env.Stderr)
	if code != 0 {
		return code
	}
	lock, err := state.AcquireLock(env.LockPath)
	if errors.Is(err, state.ErrLocked) {
		fmt.Fprintln(env.Stderr, text.AlreadyRunning)
		return 1
	}
	if err != nil {
		fmt.Fprintln(env.Stderr, text.LockFailed(err))
		return 1
	}
	defer lock.Release()

	store := state.NewStore(r)
	if _, code := loadState(store, env.Stderr); code != 0 {
		return code
	}
	if env.Session == nil {
		return 0
	}
	svc, err := env.services(r, store)
	if err != nil {
		fmt.Fprintln(env.Stderr, text.ActionFailed(err))
		return 1
	}
	s := env.Session
	s.Describe = describe
	if dryRun {
		s.Println(s.Colors.Warn(text.DryRunBanner))
	}
	nodes := &nodeUI{ctx: ctx, s: s, svc: svc, checker: env.TargetChecker, detector: env.AddressDetector, dns: env.Resolver}
	if nodes.checker == nil {
		nodes.checker = &reality.Checker{}
	}
	nodes.scanner = env.TargetScanner
	if nodes.scanner == nil {
		// The scanner checks candidates with the same checker, so it
		// follows the ML-KEM support set before each target choice.
		c := nodes.checker
		s := reality.NewScanner(nil)
		s.Check = c.Check
		nodes.scanner = s
	}
	if nodes.detector == nil {
		nodes.detector = share.NewDetector()
	}
	sshSvc := &ssh.Service{Store: store, Confirmer: change.NewConfirmer(change.NewManager(r, store), env.Executable),
		Fetcher: env.fetcher(), Rand: env.rand(), Firewall: firewall.SyncKind(store)}
	reg := env.kinds(r, store)
	sshMenu := &sshUI{ctx: ctx, s: s, svc: sshSvc, reg: reg}
	fwSvc := &firewall.Service{Store: store, Confirmer: sshSvc.Confirmer}
	fwMenu := &firewallUI{ctx: ctx, s: s, svc: fwSvc, reg: reg}
	fwdMenu := &forwardUI{ctx: ctx, s: s, svc: env.forwards(r, store)}
	optMenu := &sysoptUI{ctx: ctx, s: s, info: info, caps: system.ProbeAll(ctx, r),
		svc: &sysopt.Service{Runner: r, Store: store, Changes: change.NewManager(r, store)}}
	// The user may have enabled ufw since the last change (D-038).
	if err := svc.SyncUFW(ctx); err != nil {
		s.Println(s.Colors.Warn(text.ActionFailed(err)))
	}
	// Both going back from the main menu and quitting end the program.
	s.Run(func() ui.Menu {
		st := nodes.state()
		info.BBREnabled = system.BBREnabled(r)
		_, installed := svc.Xray.Installed(ctx)
		active := false
		if installed {
			active, _ = system.ServiceActive(ctx, r, xray.UnitName)
		}
		return mainMenu(s, env.Version, info, st, xrayStatus(s, installed, active, st), sshMainStatus(ctx, sshSvc),
			fwSvc.BothActive(ctx, st),
			func() error { return s.Run(nodes.menu) },
			func() error { return s.Run(fwdMenu.menu) },
			func() error { return s.Run(nodes.xrayMenu) },
			func() error { return s.Run(sshMenu.menu) },
			func() error { return s.Run(fwMenu.menu) },
			func() error { return s.Run(optMenu.menu) })
	})
	return 0
}

// startupCheck requires root and a supported system, then returns the
// detected system information.
func startupCheck(ctx context.Context, euid int, r system.Runner, stderr io.Writer) (system.Info, int) {
	if euid != 0 {
		fmt.Fprintln(stderr, text.NotRoot)
		return system.Info{}, 1
	}
	info, err := system.Detect(ctx, r)
	if err != nil {
		fmt.Fprintln(stderr, text.DetectFailed(err))
		return system.Info{}, 1
	}
	if !info.OS.Supported() {
		name := info.OS.ShortName()
		if name == "" {
			name = text.UnknownOS
		}
		fmt.Fprintln(stderr, text.UnsupportedOS(name))
		return system.Info{}, 1
	}
	return info, 0
}

// loadState loads the state file and explains failures to the user.
func loadState(store *state.Store, stderr io.Writer) (*state.State, int) {
	st, err := store.Load()
	if err == nil {
		return st, 0
	}
	var corrupt *state.CorruptError
	var tooNew *state.TooNewError
	switch {
	case errors.As(err, &corrupt):
		fmt.Fprintln(stderr, text.StateCorrupt(store.Path, store.BackupDir, corrupt.Err))
	case errors.As(err, &tooNew):
		fmt.Fprintln(stderr, text.StateTooNew(store.Path))
	default:
		fmt.Fprintln(stderr, text.StateLoadFailed(err))
	}
	return nil, 1
}

// rollback is run by the systemd timer when a change was not confirmed in
// time. It deliberately ignores the instance lock: the menu that made the
// change is usually still running (or hung after SSH dropped), and the
// rollback must happen regardless.
func rollback(ctx context.Context, env Env) int {
	if env.Euid != 0 {
		fmt.Fprintln(env.Stderr, text.NotRoot)
		return 1
	}
	store := state.NewStore(env.Runner)
	confirmer := change.NewConfirmer(change.NewManager(env.Runner, store), env.Executable)
	done, err := confirmer.Rollback(ctx, env.kinds(env.Runner, store))
	switch {
	case err != nil:
		fmt.Fprintln(env.Stderr, text.RollbackFailed(err))
		return 1
	case done:
		fmt.Fprintln(env.Stdout, text.RollbackDone)
	default:
		fmt.Fprintln(env.Stdout, text.RollbackNotNeeded)
	}
	return 0
}
