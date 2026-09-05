package repl

import (
	"errors"
	"os"

	"github.com/type-rb/type-rb/internal/nativefs"
)

func (e *Evaluator) anchoredLockBlock(invocation runtimeBlockInvocation) (Value, error) {
	if len(invocation.Arguments) != 2 {
		return Value{}, errors.New("Dir.try_lock requires a receiver and relative path")
	}
	root, ok := invocation.Arguments[0].Value.Data.(*os.Root)
	if !ok {
		return Value{}, errors.New("Dir.try_lock receiver is not an open directory")
	}
	path, ok := invocation.Arguments[1].Value.Data.(string)
	if !ok {
		return Value{}, errors.New("Dir.try_lock requires RelativePath")
	}
	file, err := nativefs.TryLock(root, path)
	if err != nil {
		return e.filesystemDomainError(invocation.Type, "try_lock", path, err, filesystemErrorKind(err), true, true)
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()
	value, evaluateErr := invocation.Evaluate(nil)
	closeErr := file.Close()
	closed = true
	if evaluateErr != nil {
		return Value{}, evaluateErr
	}
	if invocation.BodyReturned != nil && invocation.BodyReturned() {
		value.Type = invocation.Type
		return value, nil
	}
	if closeErr != nil {
		return e.filesystemDomainError(invocation.Type, "close", path, closeErr, filesystemErrorKind(closeErr), true, true)
	}
	return e.filesystemOK(invocation.Type, value)
}
