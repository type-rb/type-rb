//go:build linux || darwin

package repl

import "syscall"

func regularFileOpenFlags() (int, bool) {
	return syscall.O_NONBLOCK | syscall.O_NOCTTY, true
}
