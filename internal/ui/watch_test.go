package ui

import (
	"context"
	"io"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestWatchDoneRedrawsInPlace(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	s, out, _ := newTestSession(pr)
	s.Interactive = true
	var echo []bool
	s.Echo = func(on bool) { echo = append(echo, on) }
	n := 0
	end, err := s.Watch(context.Background(), time.Millisecond, "footer", func() ([]string, bool, error) {
		n++
		return []string{"count " + strconv.Itoa(n), "x"}, n == 3, nil
	})
	if err != nil || end != WatchDone {
		t.Fatalf("Watch = %v, %v", end, err)
	}
	got := out.String()
	want := "count 1\nx\nfooter\n\x1b[3A\x1b[Jcount 2\nx\nfooter\n\x1b[3A\x1b[Jcount 3\nx\nfooter\n\x1b[1A\x1b[J"
	if got != want {
		t.Errorf("output %q, want %q", got, want)
	}
	if len(echo) != 2 || echo[0] || !echo[1] {
		t.Errorf("echo %v", echo)
	}
}

func TestWatchEnter(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	s, _, _ := newTestSession(pr)
	s.Interactive = true
	n := 0
	end, err := s.Watch(context.Background(), time.Hour, "", func() ([]string, bool, error) {
		n++
		go pw.Write([]byte("\n"))
		return nil, false, nil
	})
	if err != nil || end != WatchEnter || n != 1 {
		t.Errorf("Watch = %v, %v after %d updates", end, err, n)
	}
}

func TestWatchInterruptCancels(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	s, _, sigs := newTestSession(pr)
	s.Interactive = true
	end, err := s.Watch(context.Background(), time.Hour, "", func() ([]string, bool, error) {
		sigs <- syscall.SIGINT
		return nil, false, nil
	})
	if err != nil || end != WatchCancel {
		t.Fatalf("Watch = %v, %v", end, err)
	}
	go pw.Write([]byte("next\n"))
	if line, err := s.Prompt("> "); err != nil || line != "next" {
		t.Errorf("Prompt after Ctrl+C = %q, %v", line, err)
	}
}

func TestWatchPipedReadsOneLine(t *testing.T) {
	s, out, _ := newTestSession(strings.NewReader("\nrest\n"))
	end, err := s.Watch(context.Background(), time.Millisecond, "footer", func() ([]string, bool, error) {
		t.Error("update called with piped input")
		return nil, true, nil
	})
	if err != nil || end != WatchEnter || out.String() != "" {
		t.Errorf("Watch = %v, %v, output %q", end, err, out.String())
	}
	if line, _ := s.Prompt(""); line != "rest" {
		t.Errorf("next line %q", line)
	}
	s, _, _ = newTestSession(strings.NewReader(""))
	if _, err := s.Watch(context.Background(), time.Millisecond, "", nil); err != ErrQuit {
		t.Errorf("end of input: %v", err)
	}
}
