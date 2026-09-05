//go:build !linux && !darwin

package nativefs

import "os"

func TryLock(_ *os.Root, _ string) (*os.File, error) { return nil, ErrUnsupported }
