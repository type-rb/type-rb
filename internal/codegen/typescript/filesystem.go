package typescript

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/stdlib"
	"github.com/type-rb/type-rb/internal/types"
)

func (g *generator) filesystemErrorValue(errorType, operation, path, message, kind string) string {
	return "({ operation: " + strconv.Quote(operation) + ", target: " + g.filesystemRuntimeName(stdlib.FileSystemTargetType()) + ".Host(" + path + "), message: " + message + ", kind: " + g.filesystemRuntimeName(stdlib.FileSystemErrorKindType()) + "." + kind + " } satisfies " + errorType + ")"
}

func (g *generator) filesystemNativeError(operation, path, cause string) string {
	kind := g.filesystemRuntimeName(stdlib.FileSystemErrorKindType())
	value := g.filesystemErrorValue(g.tsType(stdlib.FileSystemErrorType()), operation, path, "message", "Other")
	value = strings.Replace(value, "kind: "+kind+".Other", "kind", 1)
	return "(() => { const code = (" + cause + " as any)?.code; const message = " + cause + " instanceof Error ? " + cause + ".message : String(" + cause + "); const kind = code === \"ENOENT\" ? " + kind + ".NotFound : code === \"EACCES\" || code === \"EPERM\" ? " + kind + ".PermissionDenied : code === \"EEXIST\" ? " + kind + ".AlreadyExists : " + kind + ".Other; return " + value + "; })()"

}

func (g *generator) filesystemCreateAll(call *ir.Call, arguments []string) string {
	success, failure := call.ExprType().Args[0], call.ExprType().Args[1]
	err := func(value string) string { return g.filesystemResultErr(success, failure, value) }
	return strings.NewReplacer(
		"$result", g.tsType(call.ExprType()),
		"$invalid", err(g.filesystemErrorValue(g.tsType(failure), "create_all", "path", strconv.Quote("path must be nonempty and contain no NUL"), "InvalidPath")),
		"$failure", err(g.filesystemNativeError("create_all", "path", "cause")),
		"$ok", g.filesystemResultOK(success, failure, "({} satisfies "+g.tsType(success)+")"),
	).Replace(`((path: string): $result => {
		if (path === "" || path.includes("\0")) return $invalid;
		try {
			const fs = (globalThis as any).process?.getBuiltinModule?.("fs");
			if (fs === undefined) throw new Error("filesystem is unavailable");
			// Check existing directories first, including Windows volume roots.
			try { if (fs.statSync(path).isDirectory()) return $ok; } catch (cause) { if ((cause as any)?.code !== "ENOENT") throw cause; }
			fs.mkdirSync(path, {recursive: true}); return $ok;
		} catch (cause) { return $failure; }
	})(` + arguments[0] + ")")
}

func (g *generator) filesystemChildren(call *ir.Call, arguments []string) string {
	success, failure := call.ExprType().Args[0], call.ExprType().Args[1]
	err := func(value string) string { return g.filesystemResultErr(success, failure, value) }
	inputError := func(kind, message string) string {
		return err(g.filesystemErrorValue(g.tsType(failure), "children", "path", strconv.Quote(message), kind))
	}
	return strings.NewReplacer(
		"$result", g.tsType(call.ExprType()), "$entry", g.tsType(success.Args[0]),
		"$invalidLimit", inputError("InvalidLimit", "max_entries must be non-negative"),
		"$invalidPath", inputError("InvalidPath", "path must be nonempty and contain no NUL"),
		"$tooLarge", inputError("TooLarge", "directory exceeds max_entries"),
		"$invalidName", inputError("UnsupportedName", "directory entry name is not valid UTF-8"),
		"$failure", err(g.filesystemNativeError("children", "activePath", "cause")),
		"$closeFailure", err(g.filesystemNativeError("children", "path", "cause")),
		"$kind", g.filesystemRuntimeName(stdlib.DirEntryKindType()),
		"$ok", g.filesystemResultOK(success, failure, "entries"),
	).Replace(`((path: string, maximum: number): $result => {
		if (maximum < 0) return $invalidLimit;
		if (path === "" || path.includes("\0")) return $invalidPath;
		let activePath = path; let directory: any; let completed = false; let result: $result;
		try {
			const host = (globalThis as any).process; const fs = host?.getBuiltinModule?.("fs"); const pathModule = host?.getBuiltinModule?.("path");
			if (fs === undefined || pathModule === undefined) throw new Error("filesystem is unavailable");
			directory = fs.opendirSync(path, {encoding: "buffer", bufferSize: 1});
			const decoder = new TextDecoder("utf-8", {fatal: true, ignoreBOM: true}); const entries: Array<$entry> = [];
			for (;;) {
				const source = directory.readSync(); if (source === null) break;
				if (entries.length === maximum) return $tooLarge;
				// Bun yields the encoded name directly; Node wraps it in a Dirent.
				const rawName = source instanceof Uint8Array ? source : source.name;
				if (!(rawName instanceof Uint8Array)) return $invalidName;
				let name: string; try { name = decoder.decode(rawName); } catch { return $invalidName; }
				const driveRelativeRoot = pathModule.sep === "\\" && /^[A-Za-z]:$/.test(path);
				const separator = path.endsWith(pathModule.sep) || (pathModule.sep === "\\" && path.endsWith("/")) || driveRelativeRoot ? "" : pathModule.sep;
				const childPath = path + separator + name; activePath = childPath; const info = fs.lstatSync(childPath); activePath = path;
				entries.push({name, path: childPath, kind: info.isFile() ? $kind.File : info.isDirectory() ? $kind.Directory : $kind.Other});
			}
			const encoder = new TextEncoder(); entries.sort((left, right) => { const a = encoder.encode(left.name); const b = encoder.encode(right.name); for (let i = 0; i < Math.min(a.length, b.length); i++) { if (a[i] !== b[i]) return a[i]! - b[i]!; } return a.length - b.length; });
			result = $ok; completed = true;
		} catch (cause) { result = $failure; }
		finally { if (directory !== undefined) { try { directory.closeSync(); } catch (cause) { if (completed) result = $closeFailure; } } }
		return result!;
	})(` + arguments[0] + ", " + arguments[1] + ")")
}

func (g *generator) filesystemResultOK(valueType, errorType types.Type, value string) string {
	return g.filesystemResultName() + ".Ok<" + g.tsType(valueType) + ", " + g.tsType(errorType) + ">(" + value + ")"
}

func (g *generator) filesystemResultErr(valueType, errorType types.Type, value string) string {
	return g.filesystemResultName() + ".Err<" + g.tsType(valueType) + ", " + g.tsType(errorType) + ">(" + value + ")"
}

func (g *generator) filesystemRuntimeName(typ types.Type) string {
	if name := g.declarationNames[typ.Declaration]; name != "" {
		return name
	}
	return g.runtimeName(typ.Name)
}

func (g *generator) filesystemResultName() string {
	return g.filesystemRuntimeName(stdlib.ResultType(types.Type{}, types.Type{}))
}

func (g *generator) filesystemStructuredBlock(block *ir.StructuredBlock) {
	if block == nil || block.Result == nil || len(block.Call.Arguments) < 1 {
		return
	}
	g.temporary++
	id := strconv.Itoa(g.temporary)
	raw := "__trbFileEffect" + id
	path := "__trbFilePath" + id
	mode := "__trbFileMode" + id
	handle := "__trbFileHandle" + id
	filesystem := "__trbFileSystem" + id
	flags := "__trbFileFlags" + id
	openError := "__trbFileOpenError" + id
	openMessage := "__trbFileOpenMessage" + id
	closeCaught := "__trbFileCloseError" + id
	closeMessage := "__trbFileCloseMessage" + id
	result := "__trbFileResult" + id
	completed := "__trbFileCompleted" + id
	successType := block.EffectSuccess
	if successType.Kind == "" || successType.Kind == types.Void {
		successType = types.FromName("Unit")
	}
	errorType := block.Fails
	errorTypeName := g.tsType(errorType)
	rawType := block.Call.ExprType()
	resultType := g.tsType(rawType)
	suspends := g.suspension != nil && g.suspension.StructuredBlocks[block]
	invocation := "(() => {"
	if suspends {
		invocation = "await (async (): Promise<" + resultType + "> => {"
	}
	g.line("const " + raw + ": " + resultType + " = " + invocation)
	g.indent++
	g.line("const " + path + ": string = " + g.expr(block.Call.Arguments[0].Value) + ";")
	modeName := g.filesystemRuntimeName(stdlib.FileModeType())
	modeValue := modeName + ".Read"
	if len(block.Call.Arguments) > 1 {
		modeValue = g.expr(block.Call.Arguments[1].Value)
	}
	g.line("const " + mode + " = " + modeValue + ";")
	invalidPath := g.filesystemErrorValue(errorTypeName, "open", path, strconv.Quote("path must be nonempty and contain no NUL"), "InvalidPath")
	g.line("if (" + path + " === \"\" || " + path + ".includes(\"\\0\")) return " + g.filesystemResultErr(successType, errorType, invalidPath) + ";")
	g.line(`const ` + filesystem + ` = (globalThis as any).process?.getBuiltinModule?.("fs");`)
	unavailable := g.filesystemErrorValue(errorTypeName, "open", path, strconv.Quote("filesystem is unavailable"), "Other")
	g.line("if (" + filesystem + " === undefined) return " + g.filesystemResultErr(successType, errorType, unavailable) + ";")
	hostUnavailable := g.filesystemErrorValue(errorTypeName, "open", path, strconv.Quote("regular-file acquisition is unavailable on this host"), "Other")
	g.line("if (![\"linux\", \"darwin\"].includes((globalThis as any).process?.platform) || !" + filesystem + ".constants.O_NONBLOCK || !" + filesystem + ".constants.O_NOCTTY) return " + g.filesystemResultErr(successType, errorType, hostUnavailable) + ";")
	constants := filesystem + ".constants."
	g.line("const " + flags + " = (" + mode + " === " + modeName + ".Read ? " + constants + "O_RDONLY : " + constants + "O_WRONLY | " + constants + "O_CREAT | (" + mode + " === " + modeName + ".CreateNew ? " + constants + "O_EXCL : 0)) | " + constants + "O_NONBLOCK | " + constants + "O_NOCTTY;")
	openOther := g.filesystemNativeError("open", path, openError)
	openExists := g.filesystemErrorValue(errorTypeName, "open", path, openMessage, "AlreadyExists")
	g.line("let " + handle + ": number;")
	g.line("try { " + handle + " = " + filesystem + ".openSync(" + path + ", " + flags + ", 0o644); } catch (" + openError + ") { const " + openMessage + " = " + openError + " instanceof Error ? " + openError + ".message : String(" + openError + "); return " + g.filesystemResultErr(successType, errorType, "("+openError+" as any)?.code === \"EEXIST\" ? "+openExists+" : "+openOther) + "; }")
	g.line("let " + result + ": " + resultType + " | undefined;")
	g.line("let " + completed + " = false;")
	g.line("try {")
	g.indent++
	notRegular := g.filesystemErrorValue(errorTypeName, "open", path, strconv.Quote("opened handle is not a regular file"), "Other")
	g.line("try {")
	g.indent++
	g.line("if (!" + filesystem + ".fstatSync(" + handle + ").isFile()) return " + g.filesystemResultErr(successType, errorType, notRegular) + ";")
	g.line("if (" + mode + " === " + modeName + ".Write) " + filesystem + ".ftruncateSync(" + handle + ", 0);")
	g.indent--
	g.line("} catch (" + openError + ") { return " + g.filesystemResultErr(successType, errorType, openOther) + "; }")
	if len(block.Bindings) > 0 && block.Bindings[0].Name != "_" {
		binding := block.Bindings[0].Name
		g.line("const " + binding + " = { fd: " + handle + ", path: " + path + " };")
		if namedUnusedBinding(binding) {
			g.line("void " + binding + ";")
		}
	}
	g.statements(block.Body)
	value := "({} satisfies Unit)"
	if block.Value != nil {
		value = g.expr(block.Value)
	}
	g.line(result + " = " + g.filesystemResultOK(successType, errorType, value) + ";")
	g.line(completed + " = true;")
	g.indent--
	g.line("} finally {")
	g.indent++
	closeError := g.filesystemNativeError("close", path, closeCaught)
	g.line("try { " + filesystem + ".closeSync(" + handle + "); } catch (" + closeCaught + ") { if (" + completed + ") { const " + closeMessage + " = " + closeCaught + " instanceof Error ? " + closeCaught + ".message : String(" + closeCaught + "); " + result + " = " + g.filesystemResultErr(successType, errorType, closeError) + "; } }")
	g.indent--
	g.line("}")
	g.line("return " + result + "!;")
	g.indent--
	g.line("})();")

	if block.Result.Return {
		g.line("return " + raw + ";")
		return
	}
	if block.CaptureEffect {
		if block.Result.Variable != nil {
			keyword := "const"
			if block.Result.Variable.Mutable {
				keyword = "let"
			}
			g.line(keyword + " " + block.Result.Variable.Name + ": " + g.tsType(block.Result.Type) + " = " + raw + ";")
		} else if block.Result.Target != nil {
			g.line(g.assignmentTarget(block.Result.Target) + " = " + raw + ";")
		}
		return
	}
	outerSuccess := block.PropagateSuccess
	if outerSuccess.Kind == "" {
		outerSuccess = successType
	}
	g.line("if (" + raw + ".kind === \"Err\") return " + g.filesystemResultErr(outerSuccess, errorType, raw+".error") + ";")
	if block.Result.Variable != nil {
		keyword := "const"
		if block.Result.Variable.Mutable {
			keyword = "let"
		}
		g.line(keyword + " " + block.Result.Variable.Name + ": " + g.tsType(block.Result.Type) + " = " + raw + ".value;")
	} else if block.Result.Target != nil {
		g.line(g.assignmentTarget(block.Result.Target) + " = " + raw + ".value;")
	}
}
