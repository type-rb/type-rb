//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package cli

import "os"

func acquireWorkspaceLease(_ *os.File, nonBlocking bool) (bool, error) {
	return !nonBlocking, nil
}

func releaseWorkspaceLease(_ *os.File) error { return nil }
