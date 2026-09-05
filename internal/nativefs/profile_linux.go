//go:build linux

package nativefs

import (
	"os"
	"syscall"
)

func localLockProfile(file *os.File) error {
	var status syscall.Statfs_t
	if err := syscall.Fstatfs(int(file.Fd()), &status); err != nil {
		return err
	}
	// The ext filesystem family and tmpfs use local VFS flock semantics.
	// Network, FUSE, and unclassified filesystems are not guessed from a path.
	switch uint64(status.Type) {
	case 0xef53, 0x01021994:
		return nil
	default:
		return ErrUnsupported
	}
}
