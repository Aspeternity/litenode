package ui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Aspeternity/litenode/internal/text"
)

// Flows (D-069): a menu item that is not a page runs as a flow, drawn as
// one connected column so that its steps read as one operation:
//
//	┌─ Add node
//	│
//	◆ 1/4 Node type
//	│   1. VLESS + TCP + Reality (Vision)
//	│   0. Cancel
//	│   Choose [0-1]: 1
//	│
//	◆ 2/4 Port [443]:
//	│
//	└─ ✓ Node added.
//
// Everything printed or asked during the flow gets the "│" gutter.
// Done closes the flow; what follows (links, QR codes, keys) is printed
// without the gutter so that it copies and scans cleanly.

// Flow drawing.
const (
	flowGutter = "│   "
	flowOpen   = "┌─ "
	flowClose  = "└─ "
	flowStep   = "◆ "
	flowSub    = "◇ "
)

type flowState struct {
	// step is a step marker not printed yet, and stepName its name: a
	// prompt starting with the name is printed on the marker's line.
	step, stepName string
}

func (s *Session) inFlow() bool { return s.flow != nil }

// openFlow starts a flow titled title.
func (s *Session) openFlow(title string) {
	fmt.Fprintln(s.Out, "\n"+flowOpen+title)
	s.flow = &flowState{}
}

// closeFlow ends a flow that Done or End did not close: canceled, or
// failed with err (the error is shown after it).
func (s *Session) closeFlow(err error) {
	if s.flow == nil {
		return
	}
	s.flow = nil
	switch {
	case err == nil || errors.Is(err, ErrBack):
		fmt.Fprintln(s.Out, "│\n"+flowClose+text.FlowCanceled)
	case errors.Is(err, ErrQuit):
		fmt.Fprintln(s.Out, flowClose)
	default:
		fmt.Fprintln(s.Out, "│\n"+flowClose+s.Colors.Bad(text.FlowFailed))
	}
}

// Step marks step n of total (total 0: unnumbered) in the current flow.
// Outside a flow it does nothing.
func (s *Session) Step(n, total int, name string) {
	if s.flow == nil {
		return
	}
	s.flushStep()
	marker := flowStep
	if total > 0 {
		marker += fmt.Sprintf("%d/%d ", n, total)
	}
	s.flow.step, s.flow.stepName = marker+name, name
}

// flushStep prints a pending step marker.
func (s *Session) flushStep() {
	if s.flow != nil && s.flow.step != "" {
		fmt.Fprintln(s.Out, "│\n"+s.flow.step)
		s.flow.step, s.flow.stepName = "", ""
	}
}

// Done closes the current flow as successful with msg. Outside a flow it
// prints msg. The menu comes back only after Enter: the next page clears
// the screen, and the result would only flash on it (D-135).
func (s *Session) Done(msg string) {
	s.end(s.Colors.Good("✓ " + msg))
	s.Hold()
}

// End closes the current flow with msg, neither success nor cancel (e.g.
// nothing to do, or a change undone at the user's request). Like Done, the
// menu comes back only after Enter (D-128).
func (s *Session) End(msg string) {
	s.end(msg)
	s.Hold()
}

// EndWarn closes the current flow with a warning msg, a problem the user
// has to read: the menu comes back only after Enter (D-096).
func (s *Session) EndWarn(msg string) {
	s.End(s.Colors.Warn(msg))
}

func (s *Session) end(msg string) {
	if s.flow == nil {
		fmt.Fprintln(s.Out, msg)
		return
	}
	s.flushStep()
	s.flow = nil
	fmt.Fprintln(s.Out, "│\n"+flowClose+msg)
}

// gutter prefixes every line of text in a flow.
func (s *Session) gutter(text string) string {
	if s.flow == nil {
		return text
	}
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = flowGutter + l
	}
	return strings.Join(lines, "\n")
}

// promptLabel is how a prompt is shown: in a flow, on a pending step's
// line when it asks for that step, otherwise in the gutter.
func (s *Session) promptLabel(label string) string {
	if s.flow == nil {
		return label
	}
	if f := s.flow; f.step != "" && strings.HasPrefix(label, f.stepName) {
		marker := strings.TrimSuffix(f.step, f.stepName)
		f.step, f.stepName = "", ""
		return "│\n" + marker + label
	}
	s.flushStep()
	return flowGutter + label
}

// renderFlow draws a choice inside a flow: no bars or separators. The
// choice of a pending step is that step (option 0 is "cancel"); any other choice is
// a sub-step (a "◇" heading, option 0 is "back").
func (s *Session) renderFlow(m Menu) string {
	var b strings.Builder
	exit := m.exitLabel()
	if s.flow.step != "" {
		b.WriteString("│\n" + s.flow.step + "\n")
		s.flow.step, s.flow.stepName = "", ""
		if m.ExitLabel == "" {
			exit = text.Cancel
		}
	} else if m.Heading != "" {
		b.WriteString("│\n" + flowGutter + flowSub + m.Heading + "\n")
	}
	for _, l := range append(append(append([]string{}, m.Status...), noticeLines(m)...), m.itemLines()...) {
		if l != "" {
			b.WriteString(flowGutter + l + "\n")
		}
	}
	b.WriteString(flowGutter + "0. " + exit + "\n")
	return b.String()
}

func noticeLines(m Menu) []string {
	if m.Notice == "" {
		return nil
	}
	return []string{m.Notice}
}
