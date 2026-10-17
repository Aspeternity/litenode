package ui

import "strings"

// Width returns the number of terminal columns s occupies. East Asian wide
// characters take two columns; ANSI color sequences take none.
func Width(s string) int {
	w := 0
	inEscape := false
	for _, r := range s {
		switch {
		case inEscape:
			if r == 'm' {
				inEscape = false
			}
		case r == '\x1b':
			inEscape = true
		case isWide(r):
			w += 2
		default:
			w++
		}
	}
	return w
}

// PadRight pads s with spaces to width columns.
func PadRight(s string, width int) string {
	if n := width - Width(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

// isWide reports whether r is an East Asian wide or fullwidth character.
// Ambiguous-width symbols used in menus (● ○ ✓ ✗ ⚠) count as narrow.
func isWide(r rune) bool {
	return (r >= 0x1100 && r <= 0x115F) || // Hangul Jamo
		(r >= 0x2E80 && r <= 0xA4CF && r != 0x303F) || // CJK radicals .. Yi, incl. CJK punctuation
		(r >= 0xAC00 && r <= 0xD7A3) || // Hangul syllables
		(r >= 0xF900 && r <= 0xFAFF) || // CJK compatibility ideographs
		(r >= 0xFE30 && r <= 0xFE4F) || // CJK compatibility forms
		(r >= 0xFF00 && r <= 0xFF60) || // fullwidth forms
		(r >= 0xFFE0 && r <= 0xFFE6) ||
		(r >= 0x20000 && r <= 0x3FFFD)
}
