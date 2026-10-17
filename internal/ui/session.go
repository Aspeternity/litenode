package ui

import (
	"bufio"
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

// Session reads user input and handles interrupt signals.
//
// An interrupt while waiting for input quits at once. An interrupt while an
// operation runs (any time outside a prompt) only prints a notice; the
// operation finishes or rolls back, and the next prompt returns ErrQuit.
// This keeps a configuration change from being cut off halfway.
type Session struct {
	Out    io.Writer
	Colors Colors

	lines <-chan string

	mu        sync.Mutex
	prompting bool
	quit      bool
	wake      chan struct{}
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
	for range sigs {
		s.mu.Lock()
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

	fmt.Fprint(s.Out, label)
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

// Println prints a line.
func (s *Session) Println(a ...any) {
	fmt.Fprintln(s.Out, a...)
}

// Run shows a menu until the user picks 0 or quits. build is called before
// every display so the status is current. Run returns nil when the user
// goes back, ErrQuit when they quit.
func (s *Session) Run(build func() Menu) error {
	for {
		m := build()
		items := m.Visible()
		fmt.Fprint(s.Out, "\n"+m.Render())

		choice, err := s.choose(len(items))
		if err != nil {
			return err
		}
		if choice == 0 {
			return nil
		}
		if err := items[choice-1].Action(); err != nil {
			if errors.Is(err, ErrQuit) {
				return err
			}
			fmt.Fprintln(s.Out, s.Colors.Bad(text.ActionFailed(err)))
		}
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
		fmt.Fprintln(s.Out, s.Colors.Warn(text.InvalidChoice(maxChoice)))
	}
}
