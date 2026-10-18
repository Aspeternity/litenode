package ui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/Aspeternity/litenode/internal/text"
)

// ErrQuit means the user asked to leave LiteNode: Ctrl+C (or SIGHUP /
// SIGTERM) or end of input. Menus return it unchanged so the program can
// unwind and exit.
var ErrQuit = errors.New("quit")

// ErrBack, returned by a menu action, leaves that menu as if the user had
// chosen 0 (e.g. after deleting the item the menu is about).
var ErrBack = errors.New("back")

// Session reads user input and handles interrupt signals.
//
// An interrupt while waiting for input quits at once. An interrupt while an
// operation runs (any time outside a prompt) only prints a notice; the
// operation finishes or rolls back, and the next prompt returns ErrQuit.
// This keeps a configuration change from being cut off halfway.
type Session struct {
	Out    io.Writer
	Colors Colors
	// Describe maps an action's error to a message (optional).
	Describe func(error) string
	// Echo turns terminal echo on or off around PromptSecret (optional;
	// the program sets it to SetEcho on stdin).
	Echo func(on bool)
	// Interactive means input and output are a terminal. Only then does
	// an operation that showed something to read wait for Enter before
	// the menu comes back (D-096); piped input runs straight through.
	Interactive bool

	lines <-chan string
	flow  *flowState
	// hold: the running operation showed something to read.
	hold bool

	mu        sync.Mutex
	prompting bool
	quit      bool
	wake      chan struct{}
	// follow cancels the running Follow operation; Ctrl+C ends it instead
	// of quitting.
	follow context.CancelFunc
}

// NewSession reads lines from in and interrupts from sigs. The caller
// subscribes sigs to the signals that should quit (signal.Notify).
func NewSession(in io.Reader, out io.Writer, sigs <-chan os.Signal, colors Colors) *Session {
	lines := make(chan string)
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(in)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()
	s := &Session{Out: out, Colors: colors, lines: lines, wake: make(chan struct{}, 1)}
	go s.watch(sigs)
	return s
}

func (s *Session) watch(sigs <-chan os.Signal) {
	for sig := range sigs {
		s.mu.Lock()
		if s.follow != nil {
			s.follow()
			if sig == os.Interrupt {
				s.mu.Unlock()
				continue
			}
		}
		already := s.quit
		s.quit = true
		prompting := s.prompting
		s.mu.Unlock()
		if prompting {
			select {
			case s.wake <- struct{}{}:
			default:
			}
		} else if !already {
			fmt.Fprintln(s.Out, "\n"+s.Colors.Warn(text.BusyInterrupt))
		}
	}
}

// Follow runs fn, an open-ended operation such as following a log, until it
// returns. Ctrl+C cancels fn's context and returns to the menu instead of
// quitting (D-090); other quit signals still quit.
func (s *Session) Follow(ctx context.Context, fn func(context.Context) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.mu.Lock()
	if s.quit {
		s.mu.Unlock()
		return ErrQuit
	}
	s.follow = cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.follow = nil
		s.mu.Unlock()
	}()
	err := fn(ctx)
	if ctx.Err() != nil {
		fmt.Fprintln(s.Out)
		return nil
	}
	return err
}

// Prompt prints label and returns the trimmed input line.
func (s *Session) Prompt(label string) (string, error) {
	s.mu.Lock()
	if s.quit {
		s.mu.Unlock()
		return "", ErrQuit
	}
	s.prompting = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.prompting = false
		s.mu.Unlock()
	}()

	fmt.Fprint(s.Out, s.promptLabel(label))
	select {
	case line, ok := <-s.lines:
		if !ok {
			fmt.Fprintln(s.Out)
			return "", ErrQuit
		}
		return strings.TrimSpace(line), nil
	case <-s.wake:
		fmt.Fprintln(s.Out)
		return "", ErrQuit
	}
}

// PromptSecret is Prompt without showing what is typed.
func (s *Session) PromptSecret(label string) (string, error) {
	if s.Echo != nil {
		s.Echo(false)
		defer func() {
			s.Echo(true)
			fmt.Fprintln(s.Out) // the newline the user typed was not echoed
		}()
	}
	return s.Prompt(label)
}

// Confirm asks a yes/no question. Only "y" or "yes" means yes; the default
// is no (D-014 dangerous operations, DESIGN section 7).
func (s *Session) Confirm(question string) (bool, error) {
	answer, err := s.Prompt(question + text.ConfirmSuffix)
	if err != nil {
		return false, err
	}
	switch strings.ToLower(answer) {
	case "y", "yes":
		return true, nil
	}
	return false, nil
}

// Println prints a line; in a flow, in the gutter.
func (s *Session) Println(a ...any) {
	if s.flow != nil {
		s.flushStep()
		fmt.Fprintln(s.Out, s.gutter(strings.TrimSuffix(fmt.Sprintln(a...), "\n")))
		return
	}
	fmt.Fprintln(s.Out, a...)
}

// Run shows a menu until the user picks 0 or quits. build is called before
// every display so the status is current. Run returns nil when the user
// goes back, ErrQuit when they quit. An action's error is shown with
// Describe and the menu continues.
func (s *Session) Run(build func() Menu) error {
	for {
		m := build()
		items := m.Visible()
		choice, err := s.Choose(m)
		if err != nil {
			return err
		}
		if choice == 0 {
			return nil
		}
		it := items[choice-1]
		flow := !it.Page && s.flow == nil
		if flow {
			s.openFlow(it.Label)
		}
		s.hold = false
		err = it.Action()
		if flow {
			s.closeFlow(err)
		}
		hold := s.hold
		s.hold = false
		if errors.Is(err, ErrQuit) {
			return err
		}
		if err != nil && !errors.Is(err, ErrBack) {
			s.Println(s.Colors.Bad(s.describe(err)))
			hold = true
		}
		if hold {
			if err := s.pause(); err != nil {
				return err
			}
		}
		if errors.Is(err, ErrBack) {
			return nil
		}
	}
}

// Hold marks the running operation as having shown something to read
// (links, logs, keys, results): when it ends, the menu comes back only
// after Enter (D-096). An operation that ends with a one-line result
// does not call it; an error always holds.
func (s *Session) Hold() { s.hold = true }

// pause waits for Enter when the session is interactive.
func (s *Session) pause() error {
	if !s.Interactive {
		return nil
	}
	_, err := s.Prompt("\n" + text.PressEnter)
	return err
}

// Describe turns an action's error into the message shown to the user.
// The application sets it to map known errors to text; by default the
// error text is shown.
func (s *Session) describe(err error) string {
	if s.Describe != nil {
		return s.Describe(err)
	}
	return text.ActionFailed(err)
}

// Choose shows m once and returns the chosen option: 1..n for visible
// items in order, 0 for back.
func (s *Session) Choose(m Menu) (int, error) {
	if s.flow != nil {
		fmt.Fprint(s.Out, s.renderFlow(m))
		return s.choose(len(m.Visible()))
	}
	fmt.Fprint(s.Out, "\n"+m.Render())
	return s.choose(len(m.Visible()))
}

// PromptDefault asks with a default shown in brackets; an empty answer
// returns the default.
func (s *Session) PromptDefault(label, def string) (string, error) {
	in, err := s.Prompt(text.WithDefault(label, def))
	if err != nil || in != "" {
		return in, err
	}
	return def, nil
}

// PromptNumber asks for a number between min and max, asking again after
// invalid input. An empty answer returns def (when def >= min).
func (s *Session) PromptNumber(label string, def, min, max int) (int, error) {
	for {
		var in string
		var err error
		if def >= min {
			in, err = s.PromptDefault(label, strconv.Itoa(def))
		} else {
			in, err = s.Prompt(label + ": ")
		}
		if err != nil {
			return 0, err
		}
		if n, err := strconv.Atoi(in); err == nil && n >= min && n <= max {
			return n, nil
		}
		s.Println(s.Colors.Warn(text.InvalidNumber(min, max)))
	}
}

// choose reads an option number, asking again after invalid input. An
// empty line asks again silently.
func (s *Session) choose(maxChoice int) (int, error) {
	for {
		in, err := s.Prompt(text.PromptChoice(maxChoice))
		if err != nil {
			return 0, err
		}
		if in == "" {
			continue
		}
		n, err := strconv.Atoi(in)
		if err == nil && n >= 0 && n <= maxChoice {
			return n, nil
		}
		s.Println(s.Colors.Warn(text.InvalidChoice(maxChoice)))
	}
}
