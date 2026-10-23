package ui

import (
	"fmt"
	"io"
	"syscall"
	"unsafe"
)

// SetEcho turns terminal echo on or off for fd, so a password typed at a
// prompt is not shown. It fails harmlessly when fd is not a terminal.
func SetEcho(fd int, on bool) error {
	return setLflag(fd, syscall.ECHO, on)
}

// SetPaste turns bracketed paste mode on or off: the terminal on out then
// marks pasted text, and the markers are not echoed as "^[[200~" while
// it is on (ECHOCTL off on fd). Terminals without the mode ignore it.
func SetPaste(fd int, out io.Writer, on bool) error {
	if on {
		fmt.Fprint(out, "\x1b[?2004h")
	} else {
		fmt.Fprint(out, "\x1b[?2004l")
	}
	return setLflag(fd, syscall.ECHOCTL, !on)
}

// setLflag sets or clears a local mode flag of terminal fd.
func setLflag(fd int, flag uint32, set bool) error {
	var t syscall.Termios
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.TCGETS, uintptr(unsafe.Pointer(&t))); e != 0 {
		return e
	}
	if set {
		t.Lflag |= flag
	} else {
		t.Lflag &^= flag
	}
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.TCSETS, uintptr(unsafe.Pointer(&t))); e != 0 {
		return e
	}
	return nil
}

// IsTerminal reports whether fd is a terminal.
func IsTerminal(fd int) bool {
	var t syscall.Termios
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.TCGETS, uintptr(unsafe.Pointer(&t)))
	return e == 0
}
