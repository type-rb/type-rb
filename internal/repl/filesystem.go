package repl

import (
	"errors"
	"fmt"
	"os"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

type filesystemRuntimeProvider struct{}

func init() {
	registerRuntimeProvider(func() runtimeProvider { return &filesystemRuntimeProvider{} })
}

func (*filesystemRuntimeProvider) Name() string { return "trb/std/filesystem" }

func (*filesystemRuntimeProvider) Handles(intrinsic string) bool {
	return intrinsic == "trb.std.filesystem.open"
}

func (*filesystemRuntimeProvider) Configure([]*ir.Program) error { return nil }

func (*filesystemRuntimeProvider) Close() error { return nil }

func (*filesystemRuntimeProvider) Call(_ *Evaluator, invocation runtimeInvocation) (Value, error) {
	return Value{}, fmt.Errorf("filesystem intrinsic %s requires a structured block", invocation.Name)
}

func (*filesystemRuntimeProvider) Block(evaluator *Evaluator, invocation runtimeBlockInvocation) (Value, error) {
	if len(invocation.Arguments) < 2 {
		return Value{}, errors.New("FileSystem.open requires path and mode")
	}
	path, ok := invocation.Arguments[0].Value.Data.(string)
	if !ok {
		return Value{}, errors.New("FileSystem.open path must be String")
	}
	mode, ok := invocation.Arguments[1].Value.Data.(*enumValue)
	if !ok {
		return Value{}, errors.New("FileSystem.open mode must be FileSystem::OpenMode")
	}
	flags := os.O_RDONLY
	switch mode.Name {
	case "Read":
	case "Write":
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	case "CreateNew":
		flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	default:
		return Value{}, fmt.Errorf("unknown FileSystem::OpenMode %s", mode.Name)
	}
	file, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		kind := "Other"
		if errors.Is(err, os.ErrExist) {
			kind = "AlreadyExists"
		}
		return evaluator.filesystemErrKind(invocation.Type, "open", path, err, kind)
	}
	value, evaluateErr := invocation.Evaluate([]Value{{Type: types.FromName("FileSystem::File"), Data: file}})
	closeErr := file.Close()
	if evaluateErr != nil {
		return Value{}, evaluateErr
	}
	if result, isResult := value.Data.(*enumValue); isResult && types.Equivalent(value.Type, invocation.Type) && result.Name == "Err" {
		return value, nil
	}
	if closeErr != nil {
		return evaluator.filesystemErrKind(invocation.Type, "close", path, closeErr, "Other")
	}
	return evaluator.filesystemOK(invocation.Type, value)
}
