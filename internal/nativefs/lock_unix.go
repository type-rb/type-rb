//go:build linux || darwin

package nativefs

import (
	"errors"
	"os"
	"syscall"
)

// TryLock acquires a fresh open-file description for a stable, regular lock
// file. Closing the returned descriptor releases the lock; the name is kept.
func TryLock(root *os.Root, path string) (*os.File, error) {
	file, err := openLockLeaf(root, path)
	if err != nil {
		return nil, err
	}
	accepted := false
	defer func() {
		if !accepted {
			_ = file.Close()
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("lock handle is not a regular file")
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrBusy
		}
		if errors.Is(err, syscall.ENOTSUP) || errors.Is(err, syscall.ENOSYS) {
			return nil, ErrUnsupported
		}
		return nil, err
	}
	accepted = true
	return file, nil
}
