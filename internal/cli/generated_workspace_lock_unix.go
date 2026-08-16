//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package cli

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func acquireWorkspaceLease(file *os.File, nonBlocking bool) (bool, error) {
	operation := unix.LOCK_EX
	if nonBlocking {
		operation |= unix.LOCK_NB
	}
	if err := unix.Flock(int(file.Fd()), operation); err != nil {
		if nonBlocking && (errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN)) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func releaseWorkspaceLease(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
