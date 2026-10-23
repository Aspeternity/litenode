//go:build !linux

package ui

import (
	"errors"
	"io"
)

// SetEcho is only implemented on Linux, the only supported system.
func SetEcho(fd int, on bool) error { return errors.New("not supported") }

// SetPaste is only implemented on Linux.
func SetPaste(fd int, out io.Writer, on bool) error { return errors.New("not supported") }

// IsTerminal is only implemented on Linux; elsewhere it reports false.
func IsTerminal(fd int) bool { return false }
