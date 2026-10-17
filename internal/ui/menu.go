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
	// Action runs when the item is chosen.
	Action func() error
}

// Menu is one screen. Build it fresh before each display so status and
// visibility reflect the current state.
type Menu struct {
	// Title, if set, is shown in a box at the top (the main menu only).
	Title string
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
	for _, s := range m.Status {
		line(" " + s)
	}
	if m.Notice != "" {
		line(" " + m.Notice)
	}
	line(Separator)

	items := m.Visible()
	labelWidth := hintColumn
	for _, it := range items {
		if it.Hint != "" {
			labelWidth = max(labelWidth, Width(it.Label)+2)
		}
	}
	for i, it := range items {
		if m.Grouped && i > 0 && it.Group != items[i-1].Group {
			line(Separator)
		}
		label := it.Label
		if it.Hint != "" {
			label = PadRight(label, labelWidth) + it.Hint
		}
		line(fmt.Sprintf(" %d. %s", i+1, label))
	}
	// Submenus have a single separator, above option 0 (D-014). In the
	// grouped main menu option 0 belongs to the last group instead.
	if !m.Grouped {
		line(Separator)
	}
	exit := m.ExitLabel
	if exit == "" {
		exit = text.Back
	}
	line(" 0. " + exit)
	return b.String()
}
