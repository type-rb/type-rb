//go:build darwin

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
	name := make([]byte, 0, len(status.Fstypename))
	for _, character := range status.Fstypename {
		if character == 0 {
			break
		}
		name = append(name, byte(character))
	}
	const mountLocal = 0x00001000 // MNT_LOCAL from Darwin sys/mount.h.
	if string(name) != "apfs" || status.Flags&mountLocal == 0 {
		return ErrUnsupported
	}
	return nil
}
