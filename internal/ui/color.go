package ui

// Colors applies a small set of ANSI colors used to mark status. The zero
// value prints plain text.
type Colors struct {
	Enabled bool
}

// NewColors enables colors unless NO_COLOR is set to a non-empty value
// (https://no-color.org).
func NewColors(noColor string) Colors {
	return Colors{Enabled: noColor == ""}
}

func (c Colors) wrap(code, s string) string {
	if !c.Enabled || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

// Good marks a healthy state, such as running or enabled.
func (c Colors) Good(s string) string { return c.wrap("32", s) }

// Off marks an inactive state, such as stopped, disabled or not installed.
func (c Colors) Off(s string) string { return c.wrap("90", s) }

// Warn marks a warning.
func (c Colors) Warn(s string) string { return c.wrap("33", s) }

// Bad marks an error.
func (c Colors) Bad(s string) string { return c.wrap("31", s) }
