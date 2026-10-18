package ui

import (
	"fmt"
	"strings"

	"github.com/Aspeternity/litenode/internal/text"
)

const (
	// lineWidth is the width of separators and of the title box interior.
	lineWidth = 46
	// hintColumn is the minimum width of "label" before a hint starts.
	hintColumn = 16
)

// Separator is the horizontal rule between menu sections.
var Separator = strings.Repeat("─", lineWidth)

// Item is one numbered menu option.
type Item struct {
	Label string
	// Hint is shown after the label in a fixed column, e.g. text.MenuMaintHint.
	Hint string
	// Group separates items in a grouped menu (the main menu only, D-014).
	Group int
	// Hidden removes the item when the feature is unavailable. Remaining
	// items are numbered consecutively.
	Hidden bool
	// Page marks an item that opens another menu (a page). Any other item
	// is an operation and runs as a flow (D-069).
	Page bool
	// Action runs when the item is chosen.
	Action func() error
}

// Menu is one screen. Build it fresh before each display so status and
// visibility reflect the current state.
type Menu struct {
	// Title, if set, is shown in a box at the top (the main menu only).
	Title string
	// Heading, if set, is shown in a bar at the top of a submenu so it
	// stands out from the output above it (D-060).
	Heading string
	// Path holds the names of the pages above this one; the heading shows
	// them before its own name ("Main › Share", D-096). Session.Run
	// sets it.
	Path []string
	// Status lines are shown above the options.
	Status []string
	// Notice is shown below the status, e.g. why some options are hidden.
	Notice string
	Items  []Item
	// Grouped draws a separator between item groups (the main menu only).
	Grouped bool
	// ExitLabel is the label of option 0, text.Back by default.
	ExitLabel string
}

// HeadingBar centers s in a bar of "═" as wide as the separators, keeping
// at least two "═" on each side of a long heading.
func HeadingBar(s string) string {
	fill := max(lineWidth-Width(s)-2, 4)
	left := fill / 2
	return strings.Repeat("═", left) + " " + s + " " + strings.Repeat("═", fill-left)
}

// Visible returns the items that are shown, in display order. Item n on
// screen is Visible()[n-1].
func (m Menu) Visible() []Item {
	var out []Item
	for _, it := range m.Items {
		if !it.Hidden {
			out = append(out, it)
		}
	}
	return out
}

// Render returns the menu text, ending with a newline.
func (m Menu) Render() string {
	var b strings.Builder
	line := func(s string) { b.WriteString(s + "\n") }

	if m.Title != "" {
		line("╔" + strings.Repeat("═", lineWidth) + "╗")
		pad := (lineWidth + 2 - Width(m.Title)) / 2
		line(strings.Repeat(" ", max(pad, 0)) + m.Title)
		line("╚" + strings.Repeat("═", lineWidth) + "╝")
	}
	if m.Heading != "" {
		line(HeadingBar(strings.Join(append(append([]string{}, m.Path...), m.Heading), text.PathSeparator)))
	}
	for _, s := range m.Status {
		line(" " + s)
	}
	if m.Notice != "" {
		line(" " + m.Notice)
	}
	line(Separator)
	for _, l := range m.itemLines() {
		if l == "" {
			line(Separator)
			continue
		}
		line(" " + l)
	}
	// Submenus have a single separator, above option 0 (D-014). In the
	// grouped main menu option 0 belongs to the last group instead.
	if !m.Grouped {
		line(Separator)
	}
	line(" 0. " + m.exitLabel())
	return b.String()
}

func (m Menu) exitLabel() string {
	if m.ExitLabel != "" {
		return m.ExitLabel
	}
	return text.Back
}

// itemLines are the numbered items, hints aligned; "" marks a group
// separator of a grouped menu.
func (m Menu) itemLines() []string {
	var out []string
	items := m.Visible()
	labelWidth := hintColumn
	for _, it := range items {
		if it.Hint != "" {
			labelWidth = max(labelWidth, Width(it.Label)+2)
		}
	}
	// With 10 or more items, labels stay aligned: " 1.  a" / " 10. b".
	numWidth := len(fmt.Sprint(len(items))) + 1
	for i, it := range items {
		if m.Grouped && i > 0 && it.Group != items[i-1].Group {
			out = append(out, "")
		}
		label := it.Label
		if it.Hint != "" {
			label = PadRight(label, labelWidth) + it.Hint
		}
		out = append(out, fmt.Sprintf("%-*s %s", numWidth, fmt.Sprintf("%d.", i+1), label))
	}
	return out
}
