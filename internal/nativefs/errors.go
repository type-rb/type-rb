package nativefs

import "errors"

var (
	ErrBusy        = errors.New("lock is already held")
	ErrUnsupported = errors.New("filesystem profile is unsupported")
)
