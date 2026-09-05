//go:build linux || darwin

package nativefs

import (
	"os"
	"path"

	"golang.org/x/sys/unix"
)

func openLockLeaf(root *os.Root, relative string) (*os.File, error) {
	parent, leaf := path.Split(relative)
	if parent == "" {
		parent = "."
	}
	directory, err := root.OpenFile(parent, os.O_RDONLY|unix.O_DIRECTORY|unix.O_NONBLOCK|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, err
	}
	// Resolve parents through Root, then address exactly one name relative
	// to that descriptor. Root.OpenFile itself follows in-root symlinks even
	// when its caller supplies O_NOFOLLOW.
	fd, openErr := unix.Openat(int(directory.Fd()), leaf, os.O_RDWR|os.O_CREATE|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_NOCTTY|unix.O_CLOEXEC, 0o666)
	closeErr := directory.Close()
	if openErr != nil {
		return nil, openErr
	}
	file := os.NewFile(uintptr(fd), relative)
	if closeErr != nil {
		_ = file.Close()
		return nil, closeErr
	}
	return file, nil
}
