package nativefs

import "errors"

var (
	ErrBusy        = errors.New("lock is already held")
	ErrUnsupported = errors.New("host lock operation is unsupported")
)
