package ui

import (
	"context"
	"fmt"
	"time"
)

// WatchEnd is how Watch ended.
type WatchEnd int

const (
	WatchDone   WatchEnd = iota // update reported done
	WatchEnter                  // the user pressed Enter
	WatchCancel                 // Ctrl+C (or another quit signal)
)

// Watch shows the lines update returns, redrawn in place every interval,
// followed by footer, until update reports done, the user presses Enter or
// Ctrl+C. The footer is erased when it ends. Input is not echoed
// meanwhile, so Enter does not move the cursor. Ctrl+C cancels like
// Follow (D-090); other quit signals also cancel, and the next prompt
// quits. With piped input nothing is shown or refreshed: Watch reads one
// line (WatchEnter).
func (s *Session) Watch(ctx context.Context, interval time.Duration, footer string, update func() (lines []string, done bool, err error)) (WatchEnd, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.mu.Lock()
	if s.quit {
		s.mu.Unlock()
		return WatchCancel, ErrQuit
	}
	s.follow = cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.follow = nil
		s.mu.Unlock()
	}()
	if !s.Interactive {
		select {
		case _, ok := <-s.lines:
			if !ok {
				return WatchCancel, ErrQuit
			}
			return WatchEnter, nil
		case <-ctx.Done():
			return WatchCancel, nil
		}
	}
	if s.Echo != nil {
		s.Echo(false)
		defer s.Echo(true)
	}
	shown := 0
	clear := func() {
		if shown > 0 {
			fmt.Fprintf(s.Out, "\x1b[%dA\x1b[J", shown)
		}
	}
	defer func() {
		// Erase the footer only.
		if shown > 0 {
			fmt.Fprint(s.Out, "\x1b[1A\x1b[J")
		}
	}()
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		lines, done, err := update()
		if err != nil {
			return WatchCancel, err
		}
		clear()
		for _, l := range append(lines, footer) {
			fmt.Fprintln(s.Out, s.gutter(l))
		}
		shown = len(lines) + 1
		if done {
			return WatchDone, nil
		}
		select {
		case <-tick.C:
		case _, ok := <-s.lines:
			if !ok {
				return WatchCancel, ErrQuit
			}
			return WatchEnter, nil
		case <-ctx.Done():
			return WatchCancel, nil
		}
	}
}
