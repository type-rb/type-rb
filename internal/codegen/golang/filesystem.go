package golang

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/stdlib"
	"github.com/type-rb/type-rb/internal/types"
)

func (g *generator) filesystemKind(name string) string {
	return g.filesystemEnumConstant(stdlib.FileSystemErrorKindType(), name)
}

func (g *generator) filesystemOpenMode(name string) string {
	return g.filesystemEnumConstant(stdlib.FileModeType(), name)
}

func (g *generator) filesystemDirectoryEntryKind(name string) string {
	return g.filesystemEnumConstant(stdlib.DirEntryKindType(), name)
}

func (g *generator) filesystemEnumConstant(enumType types.Type, name string) string {
	prefix := ""
	if alias := g.declarationAlias(enumType.Declaration); alias != "" {
		prefix = alias + "."
	}
	return prefix + goConstantIdentifier(enumType.Declaration.Name, name)
}

func (g *generator) filesystemResultAlias() string {
	alias := g.declarationAlias(stdlib.ResultType(types.Type{}, types.Type{}).Declaration)
	if alias == "" {
		return "__trb_result"
	}
	return alias
}

func (g *generator) filesystemResultOK(valueType, errorType types.Type, value string) string {
	alias := g.filesystemResultAlias()
	return alias + ".NewResultOk[" + g.goType(valueType) + ", " + g.goType(errorType) + "](" + value + ")"
}

func (g *generator) filesystemResultErr(valueType, errorType types.Type, value string) string {
	alias := g.filesystemResultAlias()
	return alias + ".NewResultErr[" + g.goType(valueType) + ", " + g.goType(errorType) + "](" + value + ")"
}

func (g *generator) filesystemError(errorType types.Type, operation, path, message, kind string) string {
	target := g.declarationAlias(stdlib.FileSystemTargetType().Declaration) + ".NewFileSystemTargetHost(" + path + ")"
	return g.goType(stdlib.FileSystemErrorType()) + "{Operation: " + strconv.Quote(operation) + ", Target: " + target + ", Message: " + message + ", Kind: " + g.filesystemKind(kind) + "}"
}

func (g *generator) filesystemNativeError(operation, path, cause string) string {
	g.requireImport("os", "")
	value := g.filesystemError(stdlib.FileSystemErrorType(), operation, path, cause+".Error()", "Other")
	return "func() " + g.goType(stdlib.FileSystemErrorType()) + " { failure := " + value + "; if os.IsNotExist(" + cause + ") { failure.Kind = " + g.filesystemKind("NotFound") + " } else if os.IsPermission(" + cause + ") { failure.Kind = " + g.filesystemKind("PermissionDenied") + " } else if os.IsExist(" + cause + ") { failure.Kind = " + g.filesystemKind("AlreadyExists") + " }; return failure }()"
}

// Preserve the acquisition domain even through source helper calls. Native
// errors can contain the absolute anchor name and must not cross this boundary.
func (g *generator) filesystemRootedError(value, path, rooted string, native bool) string {
	alias := g.declarationAlias(stdlib.FileSystemTargetType().Declaration)
	message := ""
	if native {
		message = `; failure.Message = "filesystem operation failed"`
	}
	return "func() " + g.goType(stdlib.FileSystemErrorType()) + " { failure := " + value + "; if " + rooted + " { if " + path + ` == "" { failure.Target = ` + alias + ".FileSystemTargetRoot } else { failure.Target = " + alias + ".NewFileSystemTargetRelative(" + path + ") }" + message + " }; return failure }()"
}

func (g *generator) filesystemResourceError(errorType types.Type, operation, path, message, kind, rooted string) string {
	return g.filesystemRootedError(g.filesystemError(errorType, operation, path, message, kind), path, rooted, false)
}

func (g *generator) filesystemResourceNativeError(operation, path, cause, rooted string) string {
	return g.filesystemRootedError(g.filesystemNativeError(operation, path, cause), path, rooted, true)
}

func (g *generator) filesystemCreateAll(call *ir.Call, arguments []string) string {
	g.requireImport("os", "")
	g.requireImport("strings", "")
	success, failure := call.ExprType().Args[0], call.ExprType().Args[1]
	err := func(value string) string { return g.filesystemResultErr(success, failure, value) }
	return "func(path string) " + g.goType(call.ExprType()) + " { if path == \"\" || strings.IndexByte(path, 0) >= 0 { return " + err(g.filesystemError(failure, "create_all", "path", strconv.Quote("path must be nonempty and contain no NUL"), "InvalidPath")) + " }; if cause := os.MkdirAll(path, 0o777); cause != nil { return " + err(g.filesystemNativeError("create_all", "path", "cause")) + " }; return " + g.filesystemResultOK(success, failure, g.goType(success)+"{}") + " }(" + arguments[0] + ")"
}

func (g *generator) filesystemChildren(call *ir.Call, arguments []string) string {
	for _, name := range []string{"os", "io", "strings", "slices", "path/filepath", "unicode/utf8"} {
		g.requireImport(name, "")
	}
	success, failure := call.ExprType().Args[0], call.ExprType().Args[1]
	entry := g.goType(success.Args[0])
	err := func(value string) string { return g.filesystemResultErr(success, failure, value) }
	inputError := func(kind, message string) string {
		return err(g.filesystemError(failure, "children", "path", strconv.Quote(message), kind))
	}
	return strings.NewReplacer(
		"$result", g.goType(call.ExprType()), "$entry", entry,
		"$invalidLimit", inputError("InvalidLimit", "max_entries must be non-negative"),
		"$invalidPath", inputError("InvalidPath", "path must be nonempty and contain no NUL"),
		"$tooLarge", inputError("TooLarge", "directory exceeds max_entries"),
		"$invalidName", inputError("UnsupportedName", "directory entry name is not valid UTF-8"),
		"$failure", err(g.filesystemNativeError("children", "path", "cause")),
		"$metadataFailure", err(g.filesystemNativeError("children", "childPath", "infoErr")),
		"$closeFailure", err(g.filesystemNativeError("children", "path", "closeError")),
		"$other", g.filesystemDirectoryEntryKind("Other"), "$file", g.filesystemDirectoryEntryKind("File"), "$directory", g.filesystemDirectoryEntryKind("Directory"),
		"$ok", g.filesystemResultOK(success, failure, g.arrayReference("entries")),
	).Replace(`func(path string, maximum int) (result $result) {
		if maximum < 0 { return $invalidLimit }
		if path == "" || strings.IndexByte(path, 0) >= 0 { return $invalidPath }
		directoryHandle, cause := os.Open(path); if cause != nil { return $failure }
		completed := false
		defer func() { if closeError := directoryHandle.Close(); completed && closeError != nil { result = $closeFailure } }()
		entries := make([]$entry, 0)
		for {
			// Request one beyond the remaining allowance, in bounded batches.
			count := min(128, maximum - len(entries) + 1)
			source, cause := directoryHandle.Readdirnames(count)
			if cause != nil && cause != io.EOF { return $failure }
			if len(source) > maximum - len(entries) { return $tooLarge }
			for _, name := range source {
				if !utf8.ValidString(name) { return $invalidName }
				childPath := path; volume := filepath.VolumeName(path)
				driveRelativeRoot := volume == path && len(volume) == 2 && volume[1] == ':'
				if !driveRelativeRoot && (len(childPath) == 0 || !os.IsPathSeparator(childPath[len(childPath)-1])) { childPath += string(os.PathSeparator) }
				childPath += name
				info, infoErr := os.Lstat(childPath); if infoErr != nil { return $metadataFailure }
				kind := $other; if info.Mode().IsRegular() { kind = $file } else if info.IsDir() { kind = $directory }
				entries = append(entries, $entry{Name: name, Path: childPath, Kind: kind})
			}
			if cause == io.EOF { break }
		}
		slices.SortStableFunc(entries, func(left, right $entry) int { return strings.Compare(left.Name, right.Name) })
		completed = true
		return $ok
	}(` + arguments[0] + ", " + arguments[1] + ")")
}

func (g *generator) filesystemStructuredBlock(block *ir.StructuredBlock) {
	if block == nil || block.Result == nil || len(block.Call.Arguments) < 1 {
		return
	}
	g.requireImport("os", "")
	g.requireImport("strings", "")
	g.requireImport("runtime", "")
	isDirectory := block.Intrinsic == "trb.std.dir.open"
	isLock := block.Intrinsic == "trb.std.dir.try_lock"
	if isLock {
		supportPath := "trb/runtime/nativefs"
		if g.goModule != "" {
			supportPath = strings.TrimSuffix(g.goModule, "/") + "/" + supportPath
		}
		g.requireImport(supportPath, "__trb_nativefs")
		g.requireImport("errors", "")
	}
	rooted := "false"
	if block.Intrinsic == "trb.std.dir.open_file" || isLock {
		rooted = "true"
	}
	if !isDirectory && !isLock {
		g.requireImport("syscall", "")
	}
	g.temporary++
	id := strconv.Itoa(g.temporary)
	raw := "__trbFileEffect" + id
	path := "__trbFilePath" + id
	mode := "__trbFileMode" + id
	handle := "__trbFileHandle" + id
	openError := "__trbFileOpenError" + id
	flags := "__trbFileFlags" + id
	completed := "__trbFileCompleted" + id
	value := "__trbFileValue" + id
	successType := block.EffectSuccess
	if successType.Kind == "" || successType.Kind == types.Void {
		successType = types.FromName("Unit")
	} else if successType.Kind == types.Never {
		successType = types.FromName("Any")
	}
	errorType := block.Fails
	rawType := block.Call.ExprType()
	target := ""
	if block.Result.Variable != nil {
		target = g.bindingIdentifier(block.Result.Variable.Name)
	} else if block.Result.Target != nil {
		target = g.assignmentTarget(block.Result.Target)
	}
	if target == "" && !block.Result.Return {
		return
	}

	sourceResult := !block.CaptureEffect
	prefix := raw + " := "
	if !sourceResult {
		if block.Result.Variable != nil {
			prefix = target + " := "
		} else if block.Result.Target != nil {
			prefix = target + " = "
		} else {
			prefix = "return "
		}
	}
	g.line(prefix + "func() (" + raw + " " + g.goType(rawType) + ") {")
	g.indent++
	anchor := "__trbDirAnchor" + id
	if rooted == "true" {
		callee := block.Call.Callee
		for {
			application, ok := callee.(*ir.TypeApply)
			if !ok {
				break
			}
			callee = application.Receiver
		}
		member := callee.(*ir.Member)
		g.line(anchor + " := " + g.expr(member.Receiver))
	}
	g.line(path + " := " + g.expr(block.Call.Arguments[0].Value))
	if !isDirectory && !isLock {
		modeValue := g.filesystemOpenMode("Read")
		if len(block.Call.Arguments) > 1 {
			modeValue = g.expr(block.Call.Arguments[1].Value)
		}
		g.line(mode + " := " + modeValue)
	}
	operation := "open"
	if rooted == "true" {
		operation = "open_file"
	}
	if isLock {
		operation = "try_lock"
	}
	invalidPath := g.filesystemResourceError(errorType, operation, path, strconv.Quote("path must be nonempty and contain no NUL"), "InvalidPath", rooted)
	g.line("if " + path + " == \"\" || strings.IndexByte(" + path + ", 0) >= 0 { return " + g.filesystemResultErr(successType, errorType, invalidPath) + " }")
	unavailableMessage := "regular-file acquisition is unavailable on this host"
	if isDirectory {
		unavailableMessage = "anchored-directory acquisition is unavailable on this host"
	}
	unavailableKind := "Other"
	if isLock {
		unavailableMessage = "cooperative locking is unavailable on this host"
		unavailableKind = "Unsupported"
	}
	unavailable := g.filesystemResourceError(errorType, operation, path, strconv.Quote(unavailableMessage), unavailableKind, rooted)
	g.line("if runtime.GOOS != \"linux\" && runtime.GOOS != \"darwin\" { return " + g.filesystemResultErr(successType, errorType, unavailable) + " }")
	if isLock {
		g.line(handle + ", " + openError + " := __trb_nativefs.TryLock(" + anchor + ", " + path + ")")
	} else if isDirectory {
		g.line(handle + ", " + openError + " := os.OpenRoot(" + path + ")")
	} else {
		g.line(flags + " := os.O_RDONLY")
		g.line("if " + mode + " == " + g.filesystemOpenMode("Write") + " { " + flags + " = os.O_WRONLY | os.O_CREATE }")
		g.line("if " + mode + " == " + g.filesystemOpenMode("CreateNew") + " { " + flags + " = os.O_WRONLY | os.O_CREATE | os.O_EXCL }")
		g.line(flags + " |= syscall.O_NONBLOCK | syscall.O_NOCTTY")
		open := "os.OpenFile"
		if rooted == "true" {
			open = anchor + ".OpenFile"
			// Open an existing leaf first. Creation is exclusive, so a dangling
			// symlink cannot be turned into a new target by O_CREATE traversal.
			g.line("if " + mode + " == " + g.filesystemOpenMode("Write") + " { " + flags + " &^= os.O_CREATE }")
		}
		g.line(handle + ", " + openError + " := " + open + "(" + path + ", " + flags + ", 0o644)")
		if rooted == "true" {
			g.line("if " + mode + " == " + g.filesystemOpenMode("Write") + " && os.IsNotExist(" + openError + ") { " + handle + ", " + openError + " = " + open + "(" + path + ", " + flags + " | os.O_CREATE | os.O_EXCL, 0o644) }")
		}
	}
	openFailure := g.filesystemResourceNativeError(operation, path, openError, rooted)
	if isLock {
		openFailure = "func() " + g.goType(errorType) + " { failure := " + openFailure + "; if errors.Is(" + openError + ", __trb_nativefs.ErrBusy) { failure.Kind = " + g.filesystemKind("Busy") + " } else if errors.Is(" + openError + ", __trb_nativefs.ErrUnsupported) { failure.Kind = " + g.filesystemKind("Unsupported") + " }; return failure }()"
	}
	g.line("if " + openError + " != nil { return " + g.filesystemResultErr(successType, errorType, openFailure) + " }")
	g.line(completed + " := false")
	closeFailure := g.filesystemResourceNativeError("close", path, "closeError", rooted)
	g.line("defer func() { if closeError := " + handle + ".Close(); " + completed + " && closeError != nil { " + raw + " = " + g.filesystemResultErr(successType, errorType, closeFailure) + " } }()")
	if !isDirectory && !isLock {
		info := "__trbFileInfo" + id
		g.line(info + ", " + openError + " := " + handle + ".Stat()")
		g.line("if " + openError + " != nil { return " + g.filesystemResultErr(successType, errorType, openFailure) + " }")
		notRegular := g.filesystemResourceError(errorType, operation, path, strconv.Quote("opened handle is not a regular file"), "Other", rooted)
		g.line("if !" + info + ".Mode().IsRegular() { return " + g.filesystemResultErr(successType, errorType, notRegular) + " }")
		g.line("if " + mode + " == " + g.filesystemOpenMode("Write") + " { if " + openError + " := " + handle + ".Truncate(0); " + openError + " != nil { return " + g.filesystemResultErr(successType, errorType, openFailure) + " } }")
	}
	if len(block.Bindings) > 0 && block.Bindings[0].Name != "_" {
		binding := g.bindingIdentifier(block.Bindings[0].Name)
		bound := handle
		if !isDirectory {
			bound = g.goType(stdlib.FileResourceType()) + "{File: " + handle + ", Path: " + path + ", Rooted: " + rooted + "}"
		}
		g.line(binding + " := " + bound)
		g.line("_ = " + binding)
	}
	g.statements(block.Body)
	if block.Value != nil {
		g.line(value + " := " + g.expr(block.Value))
		g.line(completed + " = true")
		g.line("return " + g.filesystemResultOK(successType, errorType, value))
	}
	g.indent--
	g.line("}()")

	if !sourceResult {
		if block.Result.Variable != nil && namedUnusedBinding(block.Result.Variable.Name) {
			g.line("_ = " + target)
		}
		return
	}
	outerSuccess := block.PropagateSuccess
	if outerSuccess.Kind == "" {
		outerSuccess = successType
	}
	resultAlias := g.filesystemResultAlias()
	g.line("if " + raw + ".Kind == " + resultAlias + ".ResultErrTag { return " + g.filesystemResultErr(outerSuccess, errorType, raw+".ErrError") + " }")
	if block.Result.Return {
		g.line("return " + raw)
	} else if block.Result.Variable != nil {
		g.line(target + " := " + raw + ".OkValue")
		if namedUnusedBinding(block.Result.Variable.Name) {
			g.line("_ = " + target)
		}
	} else {
		g.line(target + " = " + raw + ".OkValue")
	}
}
