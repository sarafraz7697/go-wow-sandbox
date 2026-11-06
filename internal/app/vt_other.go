//go:build !windows

package app

import "io"

// enableVirtualTerminal reports whether ANSI escapes can be used; Unix
// terminals always interpret them.
func enableVirtualTerminal(io.Writer) bool { return true }
