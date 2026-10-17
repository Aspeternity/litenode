package ui

import "strings"

// Table aligns rows into columns separated by two spaces, accounting for
// wide characters. The first row is usually the header.
func Table(rows [][]string) []string {
	var widths []int
	for _, r := range rows {
		for i, c := range r {
			if i >= len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], Width(c))
		}
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		var b strings.Builder
		for j, c := range r {
			if j == len(r)-1 {
				b.WriteString(c)
				break
			}
			b.WriteString(PadRight(c, widths[j]+2))
		}
		out[i] = strings.TrimRight(b.String(), " ")
	}
	return out
}
