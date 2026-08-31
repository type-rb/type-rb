package typescript

import (
	"strconv"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/stdlib"
	"github.com/type-rb/type-rb/internal/types"
)

func (g *generator) filesystemErrorValue(errorType, operation, path, message, kind string) string {
	return "({ operation: " + strconv.Quote(operation) + ", path: " + path + ", message: " + message + ", kind: " + g.filesystemRuntimeName(stdlib.FileSystemErrorKindType()) + "." + kind + " } satisfies " + errorType + ")"
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
	g.line("const " + path + " = " + g.expr(block.Call.Arguments[0].Value) + ";")
	modeName := g.filesystemRuntimeName(stdlib.FileModeType())
	modeValue := modeName + ".Read"
	if len(block.Call.Arguments) > 1 {
		modeValue = g.expr(block.Call.Arguments[1].Value)
	}
	g.line("const " + mode + " = " + modeValue + ";")
	g.line(`const ` + filesystem + ` = (globalThis as any).process?.getBuiltinModule?.("fs");`)
	unavailable := g.filesystemErrorValue(errorTypeName, "open", path, strconv.Quote("filesystem is unavailable"), "Other")
	g.line("if (" + filesystem + " === undefined) return " + g.filesystemResultErr(successType, errorType, unavailable) + ";")
	g.line("const " + flags + " = " + mode + " === " + modeName + ".Read ? \"r\" : " + mode + " === " + modeName + ".Write ? \"w\" : \"wx\";")
	openOther := g.filesystemErrorValue(errorTypeName, "open", path, openMessage, "Other")
	openExists := g.filesystemErrorValue(errorTypeName, "open", path, openMessage, "AlreadyExists")
	g.line("let " + handle + ": number;")
	g.line("try { " + handle + " = " + filesystem + ".openSync(" + path + ", " + flags + ", 0o644); } catch (" + openError + ") { const " + openMessage + " = " + openError + " instanceof Error ? " + openError + ".message : String(" + openError + "); return " + g.filesystemResultErr(successType, errorType, "("+openError+" as any)?.code === \"EEXIST\" ? "+openExists+" : "+openOther) + "; }")
	g.line("let " + result + ": " + resultType + " | undefined;")
	g.line("let " + completed + " = false;")
	g.line("try {")
	g.indent++
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
	closeError := g.filesystemErrorValue(errorTypeName, "close", path, closeMessage, "Other")
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
