package share

import (
	"strings"

	"rsc.io/qr"
)

// quietZone is the light border in modules around the code. The standard
// asks for 4; 2 keeps the code narrow enough for 80-column terminals and
// is read reliably by phone scanners.
const quietZone = 2

// QR renders text as a QR code for the terminal (D-052). Two module rows
// share one line using half-block characters.
//
// With color, modules are drawn with explicit black and white so the code
// scans on both light and dark terminals. Without color (NO_COLOR) dark
// modules are drawn as spaces and light ones as blocks, which suits the
// dark background of most SSH terminals.
func QR(text string, color bool) (string, error) {
	code, err := qr.Encode(text, qr.L)
	if err != nil {
		return "", err
	}
	n := code.Size + 2*quietZone
	dark := func(x, y int) bool {
		x, y = x-quietZone, y-quietZone
		return x >= 0 && y >= 0 && x < code.Size && y < code.Size && code.Black(x, y)
	}
	var b strings.Builder
	for y := 0; y < n; y += 2 {
		if color {
			b.WriteString("\x1b[30;47m") // black on white
		}
		for x := 0; x < n; x++ {
			top, bottom := dark(x, y), y+1 < n && dark(x, y+1)
			b.WriteString(halfBlock(top, bottom, color))
		}
		if color {
			b.WriteString("\x1b[0m")
		}
		b.WriteByte('\n')
	}
	return b.String(), nil
}

// halfBlock draws two vertically stacked modules. With color the
// foreground is black, so "dark" is drawn; without color the terminal's
// light foreground is drawn for "light" modules.
func halfBlock(top, bottom, color bool) string {
	if !color {
		top, bottom = !top, !bottom
	}
	switch {
	case top && bottom:
		return "█"
	case top:
		return "▀"
	case bottom:
		return "▄"
	}
	return " "
}

// Matrix returns the code as rows of booleans (true = dark), including the
// quiet zone. Used to check the rendering.
func Matrix(text string) ([][]bool, error) {
	code, err := qr.Encode(text, qr.L)
	if err != nil {
		return nil, err
	}
	n := code.Size + 2*quietZone
	m := make([][]bool, n)
	for y := range m {
		m[y] = make([]bool, n)
		for x := range m[y] {
			cx, cy := x-quietZone, y-quietZone
			m[y][x] = cx >= 0 && cy >= 0 && cx < code.Size && cy < code.Size && code.Black(cx, cy)
		}
	}
	return m, nil
}
