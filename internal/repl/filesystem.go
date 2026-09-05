package repl

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/stdlib"
	"github.com/type-rb/type-rb/internal/types"
)

type filesystemRuntimeProvider struct{}

type scopedFile struct {
	*os.File
	path   string
	rooted bool
}

func (file *scopedFile) failure(e *Evaluator, typ types.Type, operation string, cause error, kind string) (Value, error) {
	native := kind == ""
	if native {
		kind = filesystemErrorKind(cause)
	}
	return e.filesystemDomainError(typ, operation, file.path, cause, kind, file.rooted, native)
}

func init() {
	registerRuntimeProvider(func() runtimeProvider { return &filesystemRuntimeProvider{} })
}

func (*filesystemRuntimeProvider) Name() string { return "trb/std/file" }

func (*filesystemRuntimeProvider) Handles(intrinsic string) bool {
	return stdlib.IsResourceAcquisition(intrinsic)
}

func (*filesystemRuntimeProvider) Configure([]*ir.Program) error { return nil }

func (*filesystemRuntimeProvider) Close() error { return nil }

func (*filesystemRuntimeProvider) Call(_ *Evaluator, invocation runtimeInvocation) (Value, error) {
	return Value{}, fmt.Errorf("filesystem intrinsic %s requires a structured block", invocation.Name)
}

func (*filesystemRuntimeProvider) Block(evaluator *Evaluator, invocation runtimeBlockInvocation) (Value, error) {
	if invocation.Name == "trb.std.dir.open" {
		return evaluator.anchoredDirectoryBlock(invocation)
	}
	var anchor *os.Root
	if invocation.Name == "trb.std.dir.open_file" {
		if len(invocation.Arguments) == 0 {
			return Value{}, errors.New("Dir.open_file requires a receiver")
		}
		var ok bool
		anchor, ok = invocation.Arguments[0].Value.Data.(*os.Root)
		if !ok {
			return Value{}, errors.New("Dir.open_file receiver is not an open directory")
		}
		invocation.Arguments = invocation.Arguments[1:]
	}
	if len(invocation.Arguments) < 1 {
		return Value{}, errors.New("File.open requires a path")
	}
	path, ok := invocation.Arguments[0].Value.Data.(string)
	if !ok {
		return Value{}, errors.New("File.open path must be String")
	}
	resource := &scopedFile{path: path, rooted: anchor != nil}
	operation := "open"
	if anchor != nil {
		operation = "open_file"
	}
	fail := func(cause error, kind string) (Value, error) {
		return resource.failure(evaluator, invocation.Type, operation, cause, kind)
	}
	modeName := "Read"
	if len(invocation.Arguments) > 1 {
		mode, ok := invocation.Arguments[1].Value.Data.(*enumValue)
		if !ok {
			return Value{}, errors.New("File.open mode must be FileMode")
		}
		modeName = mode.Name
	}
	flags := os.O_RDONLY
	switch modeName {
	case "Read":
	case "Write":
		flags = os.O_WRONLY | os.O_CREATE
	case "CreateNew":
		flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	default:
		return Value{}, fmt.Errorf("unknown FileMode %s", modeName)
	}
	if path == "" || strings.IndexByte(path, 0) >= 0 {
		return fail(errors.New("path must be nonempty and contain no NUL"), "InvalidPath")
	}
	acquisitionFlags, supported := regularFileOpenFlags()
	if !supported {
		return fail(errors.New("regular-file acquisition is unavailable on this host"), "Other")
	}
	flags |= acquisitionFlags
	open := os.OpenFile
	if anchor != nil {
		open = anchor.OpenFile
		if modeName == "Write" {
			flags &^= os.O_CREATE
		}
	}
	file, err := open(path, flags, 0o644)
	if anchor != nil && modeName == "Write" && errors.Is(err, os.ErrNotExist) {
		file, err = open(path, flags|os.O_CREATE|os.O_EXCL, 0o644)
	}
	if err != nil {
		return fail(err, "")
	}
	resource.File = file
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return fail(err, "")
	}
	if !info.Mode().IsRegular() {
		return fail(errors.New("opened handle is not a regular file"), "Other")
	}
	if modeName == "Write" {
		if err := file.Truncate(0); err != nil {
			return fail(err, "")
		}
	}
	value, evaluateErr := invocation.Evaluate([]Value{{Type: stdlib.FileResourceType(), Data: resource}})
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
		return resource.failure(evaluator, invocation.Type, "close", closeErr, "")
	}
	return evaluator.filesystemOK(invocation.Type, value)
}

func filesystemErrorKind(cause error) string {
	switch {
	case errors.Is(cause, os.ErrNotExist):
		return "NotFound"
	case errors.Is(cause, os.ErrPermission):
		return "PermissionDenied"
	case errors.Is(cause, os.ErrExist):
		return "AlreadyExists"
	default:
		return "Other"
	}
}

func boundedDirectoryEntries(directory *os.File, maximum int64) (entries []string, err error) {
	for {
		count := int(min(int64(128), maximum-int64(len(entries))+1))
		batch, readErr := directory.Readdirnames(count)
		if readErr != nil && readErr != io.EOF {
			return nil, readErr
		}
		entries = append(entries, batch...)
		if int64(len(entries)) > maximum || readErr == io.EOF {
			return entries, nil
		}
	}
}

func (e *Evaluator) filesystemChildren(typ types.Type, path string, maximum int64) (result Value, runtimeErr error) {
	directory, openErr := os.Open(path)
	if openErr != nil {
		return e.filesystemErr(typ, "children", path, openErr)
	}
	defer func() {
		closeErr := directory.Close()
		// A failed listing wins over a close error; close errors replace only Ok.
		if status, ok := result.Data.(*enumValue); runtimeErr == nil && ok && status.Name == "Ok" && closeErr != nil {
			result, runtimeErr = e.filesystemErr(typ, "children", path, closeErr)
		}
	}()
	source, err := boundedDirectoryEntries(directory, maximum)
	if err != nil {
		return e.filesystemErr(typ, "children", path, err)
	}
	if int64(len(source)) > maximum {
		return e.filesystemErrKind(typ, "children", path, errors.New("directory exceeds max_entries"), "TooLarge")
	}
	sort.SliceStable(source, func(left, right int) bool { return source[left] < source[right] })
	entryDefinition, ok := e.definitions[symbolKey("trb/std/dir/index", "DirEntry")].(*recordDefinition)
	if !ok {
		return Value{}, errors.New("filesystem directory entries are not loaded")
	}
	kindDefinition, ok := e.definitions[symbolKey("trb/std/dir/index", "DirEntryKind")].(*enumDefinition)
	if !ok {
		return Value{}, errors.New("filesystem directory entry kinds are not loaded")
	}
	entryType := types.FromName("DirEntry")
	if len(typ.Args) == 2 && typ.Args[0].Kind == types.Array && len(typ.Args[0].Args) == 1 {
		entryType = typ.Args[0].Args[0]
	}
	kindType := types.Type{Kind: types.Named, Name: "DirEntryKind", Declaration: kindDefinition.Node.Declaration}
	items := make([]Value, 0, len(source))
	for _, sourceEntry := range source {
		if !utf8.ValidString(sourceEntry) {
			return e.filesystemErrKind(typ, "children", path, errors.New("directory entry name is not valid UTF-8"), "UnsupportedName")
		}
	}
	for _, sourceEntry := range source {
		childPath := directoryChildPath(path, sourceEntry)
		info, infoErr := os.Lstat(childPath)
		if infoErr != nil {
			return e.filesystemErr(typ, "children", childPath, infoErr)
		}
		kind := "Other"
		if info.Mode().IsRegular() {
			kind = "File"
		} else if info.IsDir() {
			kind = "Directory"
		}
		items = append(items, Value{Type: entryType, Data: &recordInstance{Definition: entryDefinition, Fields: map[string]Value{
			"name": {Type: types.FromName("String"), Data: sourceEntry},
			"path": {Type: stdlib.PathType(), Data: childPath},
			"kind": {Type: kindType, Data: &enumValue{Definition: kindDefinition, Name: kind, Payload: map[string]Value{}}},
		}}})
	}
	arrayType := types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{entryType}}
	return e.filesystemOK(typ, Value{Type: arrayType, Data: &arrayValue{Items: items}})

}
