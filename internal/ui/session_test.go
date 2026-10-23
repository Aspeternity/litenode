package ui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"slices"
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

func TestPromptValue(t *testing.T) {
	for _, c := range []struct{ in, def, want string }{
		{"0\n", "", ""},
		{"\n", "", ""},
		{"a\n", "", "a"},
		{"0\n", "d", ""},
		{"\n", "d", "d"},
		{"a\n", "d", "a"},
	} {
		s, _, _ := newTestSession(strings.NewReader(c.in))
		got, err := s.PromptValue("路径", c.def)
		if err != nil || got != c.want {
			t.Errorf("PromptValue(%q, def %q) = (%q, %v), want %q", c.in, c.def, got, err, c.want)
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

func TestFollowInterruptReturnsToMenu(t *testing.T) {
	s, out, sigs := newTestSession(strings.NewReader("next\n"))
	err := s.Follow(context.Background(), func(ctx context.Context) error {
		sigs <- syscall.SIGINT // Ctrl+C while following
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
			return errors.New("not canceled")
		}
	})
	if err != nil {
		t.Fatalf("Follow = %v", err)
	}
	if strings.Contains(out.String(), text.BusyInterrupt) {
		t.Error("Ctrl+C while following printed the busy notice")
	}
	// The session goes on: the next prompt reads input instead of quitting.
	if line, err := s.Prompt("> "); err != nil || line != "next" {
		t.Errorf("Prompt = %q, %v", line, err)
	}
}

func TestFollowHangupQuits(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	s, _, sigs := newTestSession(pr)
	err := s.Follow(context.Background(), func(ctx context.Context) error {
		sigs <- syscall.SIGHUP
		<-ctx.Done()
		return nil
	})
	if err != nil {
		t.Fatalf("Follow = %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		s.mu.Lock()
		q := s.quit
		s.mu.Unlock()
		if q || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := s.Prompt("> "); !errors.Is(err, ErrQuit) {
		t.Errorf("Prompt after SIGHUP = %v, want ErrQuit", err)
	}
}

// holdMenu has an operation that shows something to read, one with a
// one-line result, and one that fails.
func holdMenu(s *Session) func() Menu {
	return func() Menu {
		return Menu{Items: []Item{
			{Label: "show", Action: func() error { s.Done("ok"); s.Hold(); s.Println("link"); return nil }},
			{Label: "toggle", Action: func() error { s.Done("enabled"); return nil }},
			{Label: "fail", Action: func() error { return errors.New("boom") }},
		}}
	}
}

func TestRunHoldsAfterContentAndErrors(t *testing.T) {
	cases := []struct {
		name, in string
		pauses   int
	}{
		// After a hold the next line only returns to the menu: "1" is
		// not taken as a choice.
		{"content", "1\n1\n0\n", 1},
		{"one line", "2\n2\n0\n", 0},
		{"error", "3\n\n0\n", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, out, _ := newTestSession(strings.NewReader(tc.in))
			s.Interactive = true
			if err := s.Run(holdMenu(s)); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if got := strings.Count(out.String(), text.PressEnter); got != tc.pauses {
				t.Errorf("pauses = %d, want %d\n%s", got, tc.pauses, out.String())
			}
			// "content" ran the operation once: the line after the pause
			// was not a choice.
			if tc.name == "content" && strings.Count(out.String(), "link") != 1 {
				t.Errorf("operation ran more than once:\n%s", out.String())
			}
		})
	}
}

func TestRunNoHoldWhenNotInteractive(t *testing.T) {
	s, out, _ := newTestSession(strings.NewReader("1\n3\n0\n"))
	if err := s.Run(holdMenu(s)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Contains(out.String(), text.PressEnter) {
		t.Errorf("paused with piped input:\n%s", out.String())
	}
}

func TestHoldEOFQuits(t *testing.T) {
	s, _, _ := newTestSession(strings.NewReader("1\n"))
	s.Interactive = true
	if err := s.Run(holdMenu(s)); !errors.Is(err, ErrQuit) {
		t.Fatalf("Run = %v, want ErrQuit", err)
	}
}

func TestEndWarnHolds(t *testing.T) {
	s, out, _ := newTestSession(strings.NewReader("1\n\n0\n"))
	s.Interactive = true
	err := s.Run(func() Menu {
		return Menu{Items: []Item{{Label: "import", Action: func() error { s.EndWarn("no file"); return nil }}}}
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "└─ no file") || strings.Count(out.String(), text.PressEnter) != 1 {
		t.Errorf("EndWarn did not hold:\n%s", out.String())
	}
}

// nestedMenus is a main menu (Title) with page "A", which has page "B"
// and an operation that prints "result".
func nestedMenus(s *Session) func() Menu {
	b := func() Menu { return Menu{Heading: "B"} }
	a := func() Menu {
		return Menu{Heading: "A", Items: []Item{
			{Label: "b", Page: true, Action: func() error { return s.Run(b) }},
			{Label: "op", Action: func() error { s.Println("result"); return nil }},
		}}
	}
	return func() Menu {
		return Menu{Title: "T", Items: []Item{{Label: "a", Page: true, Action: func() error { return s.Run(a) }}}}
	}
}

// TestRunPagePathAndClear: a page heading shows the pages above it; pages
// clear the screen only when interactive, and an operation's flow does
// not (D-096).
func TestRunPagePathAndClear(t *testing.T) {
	in := "1\n1\n0\n2\n0\n0\n"
	s, out, _ := newTestSession(strings.NewReader(in))
	if err := s.Run(nestedMenus(s)); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{HeadingBar("主菜单 › A"), HeadingBar("主菜单 › A › B")} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q\n%s", want, got)
		}
	}
	if strings.Contains(got, "\x1b") {
		t.Errorf("piped session printed escape sequences:\n%q", got)
	}

	s, out, _ = newTestSession(strings.NewReader(in))
	s.Interactive = true
	if err := s.Run(nestedMenus(s)); err != nil {
		t.Fatal(err)
	}
	got = out.String()
	// Main, A, B, back to A, A after the operation, back to main.
	if n := strings.Count(got, clearScreen); n != 6 {
		t.Errorf("clears = %d, want 6\n%q", n, got)
	}
	// The operation's output comes after the last clear of page A.
	if i, j := strings.Index(got, "result"), strings.LastIndex(got, clearScreen); i < 0 || i > j {
		t.Errorf("operation output cleared too early:\n%q", got)
	}
}

// TestHoldAll: in dry-run every operation holds, pages do not.
func TestHoldAll(t *testing.T) {
	s, out, _ := newTestSession(strings.NewReader("1\n2\n\n0\n0\n"))
	s.Interactive, s.HoldAll = true, true
	if err := s.Run(nestedMenus(s)); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(out.String(), text.PressEnter); n != 1 {
		t.Errorf("pauses = %d, want 1\n%s", n, out.String())
	}
}

// TestPromptPaste: a marked paste is read to its end, line breaks and
// all, and the line it ends on is the user's Enter (FIX-04); paste mode
// is on only during the prompt.
func TestPromptPaste(t *testing.T) {
	in := "\x1b[200~ssh-ed25519 AAAA\nBBBB me\n\x1b[201~\nnext\n"
	s, _, _ := newTestSession(strings.NewReader(in))
	var modes []bool
	s.Paste = func(on bool) { modes = append(modes, on) }
	got, pasted, err := s.PromptPaste("key")
	if err != nil || !pasted || got != "ssh-ed25519 AAAA\nBBBB me" {
		t.Fatalf("PromptPaste = %q, %v, %v", got, pasted, err)
	}
	if !slices.Equal(modes, []bool{true, false}) {
		t.Errorf("paste modes = %v", modes)
	}
	if next, _ := s.Prompt("x"); next != "next" {
		t.Errorf("next line = %q", next)
	}
	// Paste and end marker on one line; typed text, back and pipes.
	for _, c := range []struct {
		in, want string
		pasted   bool
	}{
		{"\x1b[200~ssh-ed25519 AAAA me\x1b[201~\n", "ssh-ed25519 AAAA me", true},
		{"ssh-ed25519 AAAA me\nnext\n", "ssh-ed25519 AAAA me", false},
		{"0\n", "", false},
		{"\n", "", false},
	} {
		s, _, _ := newTestSession(strings.NewReader(c.in))
		got, pasted, err := s.PromptPaste("key")
		if err != nil || got != c.want || pasted != c.pasted {
			t.Errorf("%q: %q, %v, %v", c.in, got, pasted, err)
		}
	}
}

// TestPromptPasteUnmarked: on a terminal without bracketed paste, lines
// arriving together are one paste; a line typed later is not part of it.
func TestPromptPasteUnmarked(t *testing.T) {
	pr, pw := io.Pipe()
	s := NewSession(pr, &bytes.Buffer{}, make(chan os.Signal), Colors{})
	s.Interactive = true
	go func() {
		io.WriteString(pw, "-----BEGIN OPENSSH PRIVATE KEY-----\nAAAA\n-----END OPENSSH PRIVATE KEY-----\n")
		time.Sleep(3 * PasteGap)
		io.WriteString(pw, "later\n")
	}()
	got, pasted, err := s.PromptPaste("key")
	if err != nil || pasted || got != "-----BEGIN OPENSSH PRIVATE KEY-----\nAAAA\n-----END OPENSSH PRIVATE KEY-----" {
		t.Fatalf("PromptPaste = %q, %v, %v", got, pasted, err)
	}
	if next, _ := s.Prompt("x"); next != "later" {
		t.Errorf("next line = %q", next)
	}
}
