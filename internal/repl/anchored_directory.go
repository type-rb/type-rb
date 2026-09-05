package repl

import (
	"errors"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/type-rb/type-rb/internal/stdlib"
	"github.com/type-rb/type-rb/internal/types"
)

func (e *Evaluator) anchoredDirectoryBlock(invocation runtimeBlockInvocation) (Value, error) {
	if len(invocation.Arguments) != 1 {
		return Value{}, errors.New("Dir.open requires a path")
	}
	path, ok := invocation.Arguments[0].Value.Data.(string)
	if !ok {
		return Value{}, errors.New("Dir.open requires Path")
	}
	if path == "" || strings.IndexByte(path, 0) >= 0 {
		return e.filesystemErrKind(invocation.Type, "open", path, errors.New("path must be nonempty and contain no NUL"), "InvalidPath")
	}
	if _, supported := regularFileOpenFlags(); !supported {
		return e.filesystemErrKind(invocation.Type, "open", path, errors.New("anchored-directory acquisition is unavailable on this host"), "Other")
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return e.filesystemErr(invocation.Type, "open", path, err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = root.Close()
		}
	}()
	value, evaluateErr := invocation.Evaluate([]Value{{Type: stdlib.DirResourceType(), Data: root}})
	closeErr := root.Close()
	closed = true
	if evaluateErr != nil {
		return Value{}, evaluateErr
	}
	if invocation.BodyReturned != nil && invocation.BodyReturned() {
		value.Type = invocation.Type
		return value, nil
	}
	if closeErr != nil {
		return e.filesystemErr(invocation.Type, "close", path, closeErr)
	}
	return e.filesystemOK(invocation.Type, value)
}

func (e *Evaluator) anchoredDirectoryCall(name string, arguments []evaluatedArgument, typ types.Type) (Value, error) {
	if len(arguments) < 2 {
		return Value{}, errors.New("anchored directory operation is missing arguments")
	}
	root, ok := arguments[0].Value.Data.(*os.Root)
	if !ok {
		return Value{}, errors.New("directory receiver is not an open anchor")
	}
	if name == "trb.std.dir.root_create_all" {
		path, ok := arguments[1].Value.Data.(string)
		if !ok {
			return Value{}, errors.New("create_all requires RelativePath")
		}
		if err := root.MkdirAll(path, 0o777); err != nil {
			return e.filesystemDomainError(typ, "create_all", path, err, filesystemErrorKind(err), true, true)
		}
		unit, err := e.unitValue()
		if err != nil {
			return Value{}, err
		}
		return e.filesystemOK(typ, unit)
	}
	path := ""
	if len(arguments) == 3 && arguments[1].Value.Data != nil {
		path, ok = arguments[1].Value.Data.(string)
		if !ok {
			return Value{}, errors.New("children requires RelativePath or nil")
		}
	}
	maximum, ok := arguments[len(arguments)-1].Value.Data.(int64)
	if !ok {
		return Value{}, errors.New("children requires Integer max_entries")
	}
	return e.anchoredChildren(typ, root, path, maximum)
}

func (e *Evaluator) anchoredChildren(typ types.Type, root *os.Root, path string, maximum int64) (result Value, runtimeErr error) {
	fail := func(path string, cause error, kind string) (Value, error) {
		native := kind == ""
		if native {
			kind = filesystemErrorKind(cause)
		}
		return e.filesystemDomainError(typ, "children", path, cause, kind, true, native)
	}
	if maximum < 0 {
		return fail(path, errors.New("max_entries must be non-negative"), "InvalidLimit")
	}
	lookup := path
	if lookup == "" {
		lookup = "."
	}
	listing, err := root.OpenRoot(lookup)
	if err != nil {
		return fail(path, err, "")
	}
	closeResource := func(close func() error) {
		closeErr := close()
		if status, ok := result.Data.(*enumValue); runtimeErr == nil && ok && status.Name == "Ok" && closeErr != nil {
			result, runtimeErr = fail(path, closeErr, "")
		}
	}
	defer closeResource(listing.Close)
	directory, err := listing.Open(".")
	if err != nil {
		return fail(path, err, "")
	}
	defer closeResource(directory.Close)
	names, err := boundedDirectoryEntries(directory, maximum)
	if err != nil {
		return fail(path, err, "")
	}
	if int64(len(names)) > maximum {
		return fail(path, errors.New("directory exceeds max_entries"), "TooLarge")
	}
	sort.Strings(names)
	entryDefinition, ok := e.definitions[symbolKey("trb/std/dir/index", "DirEntry")].(*recordDefinition)
	if !ok {
		return Value{}, errors.New("directory entry definition is unavailable")
	}
	kindDefinition, ok := e.definitions[symbolKey("trb/std/dir/index", "DirEntryKind")].(*enumDefinition)
	if !ok {
		return Value{}, errors.New("directory entry kind definition is unavailable")
	}
	parse, ok := e.definitions[symbolKey("trb/std/path/index", "RelativePath#parse")].(*functionDefinition)
	if !ok {
		return Value{}, errors.New("relative path factory is unavailable")
	}
	entryType := typ.Args[0].Args[0]
	kindType := stdlib.DirEntryKindType()
	items := make([]Value, 0, len(names))
	for _, name := range names {
		if !utf8.ValidString(name) {
			return fail(path, errors.New("directory entry name is not a portable relative component"), "UnsupportedName")
		}
		parsed, err := e.call(&callable{Function: parse}, []evaluatedArgument{{Value: Value{Type: types.FromName("String"), Data: name}}})
		if err != nil {
			return Value{}, err
		}
		if status, ok := parsed.Data.(*enumValue); !ok || status.Name != "Ok" {
			return fail(path, errors.New("directory entry name is not a portable relative component"), "UnsupportedName")
		}
		child := name
		if path != "" {
			child = path + "/" + name
		}
		info, err := listing.Lstat(name)
		if err != nil {
			return fail(child, err, "")
		}
		kind := "Other"
		if info.Mode().IsRegular() {
			kind = "File"
		} else if info.IsDir() {
			kind = "Directory"
		}
		items = append(items, Value{Type: entryType, Data: &recordInstance{Definition: entryDefinition, Fields: map[string]Value{
			"name": {Type: types.FromName("String"), Data: name},
			"path": {Type: stdlib.RelativePathType(), Data: child},
			"kind": {Type: kindType, Data: &enumValue{Definition: kindDefinition, Name: kind, Payload: map[string]Value{}}},
		}}})
	}
	return e.filesystemOK(typ, Value{Type: typ.Args[0], Data: &arrayValue{Items: items}})
}
