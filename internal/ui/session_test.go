package ui

import (
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Aspeternity/litenode/internal/text"
)

// syncBuffer is safe for the signal watcher and the menu loop to share.
type syncBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (w *syncBuffer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func (w *syncBuffer) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

func newTestSession(in io.Reader) (*Session, *syncBuffer, chan os.Signal) {
	out := &syncBuffer{}
	sigs := make(chan os.Signal, 1)
	return NewSession(in, out, sigs, Colors{}), out, sigs
}

func waitPrompting(t *testing.T, s *Session) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		p := s.prompting
		s.mu.Unlock()
		if p {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("session never reached a prompt")
}

func simpleMenu(calls *[]string) func() Menu {
	return func() Menu {
		return Menu{Items: []Item{
			{Label: "a", Action: func() error { *calls = append(*calls, "a"); return nil }},
			{Label: "hidden", Hidden: true, Action: func() error { *calls = append(*calls, "hidden"); return nil }},
			{Label: "b", Action: func() error { *calls = append(*calls, "b"); return errors.New("boom") }},
		}}
	}
}

func TestRunInvalidInputAsksAgain(t *testing.T) {
	var calls []string
	s, out, _ := newTestSession(strings.NewReader("9\nabc\n\n-1\n2\n1\n0\n"))
	if err := s.Run(simpleMenu(&calls)); err != nil {
		t.Fatal(err)
	}
	// Option 2 is "b": the hidden item does not take a number.
	if strings.Join(calls, ",") != "b,a" {
		t.Errorf("calls = %v, want [b a]", calls)
	}
	if n := strings.Count(out.String(), text.InvalidChoice(2)); n != 3 {
		t.Errorf("invalid-choice message shown %d times, want 3 (empty line is silent)", n)
	}
	if !strings.Contains(out.String(), text.ActionFailed(errors.New("boom"))) {
		t.Error("action error not shown")
	}
	// The menu is redrawn after each action, not after invalid input.
	if n := strings.Count(out.String(), " 0. 返回"); n != 3 {
		t.Errorf("menu drawn %d times, want 3", n)
	}
}

func TestRunEOFQuits(t *testing.T) {
	var calls []string
	s, _, _ := newTestSession(strings.NewReader(""))
	if err := s.Run(simpleMenu(&calls)); !errors.Is(err, ErrQuit) {
		t.Errorf("err = %v, want ErrQuit", err)
	}
}

func TestInterruptAtPromptQuits(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	s, _, sigs := newTestSession(pr)
	done := make(chan error)
	go func() { _, err := s.Prompt("> "); done <- err }()
	waitPrompting(t, s)
	sigs <- syscall.SIGINT
	select {
	case err := <-done:
		if !errors.Is(err, ErrQuit) {
			t.Errorf("err = %v, want ErrQuit", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("prompt did not return after interrupt")
	}
}

func TestInterruptDuringActionIsDeferred(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	s, out, sigs := newTestSession(pr)

	finished := false
	menu := func() Menu {
		return Menu{Items: []Item{{Label: "slow", Action: func() error {
			sigs <- syscall.SIGINT // Ctrl+C while the operation runs
			deadline := time.Now().Add(2 * time.Second)
			for !strings.Contains(out.String(), text.BusyInterrupt) && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			finished = true
			return nil
		}}}}
	}
	done := make(chan error)
	go func() { done <- s.Run(menu) }()
	waitPrompting(t, s)
	pw.Write([]byte("1\n"))

	select {
	case err := <-done:
		if !errors.Is(err, ErrQuit) {
			t.Errorf("err = %v, want ErrQuit", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("menu did not quit after the operation finished")
	}
	if !finished {
		t.Error("operation was cut off")
	}
	if !strings.Contains(out.String(), text.BusyInterrupt) {
		t.Error("busy notice not shown")
	}
}

func TestConfirm(t *testing.T) {
	for in, want := range map[string]bool{"y\n": true, "YES\n": true, "\n": false, "n\n": false, "ok\n": false} {
		s, out, _ := newTestSession(strings.NewReader(in))
		got, err := s.Confirm("删除节点？")
		if err != nil || got != want {
			t.Errorf("Confirm(%q) = (%v, %v), want %v", in, got, err, want)
		}
		if !strings.Contains(out.String(), "删除节点？ [y/N]: ") {
			t.Errorf("prompt = %q", out.String())
		}
	}
}

func TestRunErrBackLeavesMenu(t *testing.T) {
	s, _, _ := newTestSession(strings.NewReader("1\n"))
	err := s.Run(func() Menu {
		return Menu{Items: []Item{{Label: "x", Action: func() error { return ErrBack }}}}
	})
	if err != nil {
		t.Errorf("err = %v, want nil (back)", err)
	}
}
