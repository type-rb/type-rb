package typescript

import (
	"strconv"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

func (g *generator) filesystemErrorValue(operation, path, message, kind string) string {
	errorType := g.tsType(types.FromName("FileSystem::Error"))
	return "({ operation: " + strconv.Quote(operation) + ", path: " + path + ", message: " + message + ", kind: " + g.runtimeName("FileSystem::ErrorKind") + "." + kind + " } satisfies " + errorType + ")"
}

func (g *generator) filesystemResultOK(valueType, errorType types.Type, value string) string {
	return g.runtimeName("Result") + ".Ok<" + g.tsType(valueType) + ", " + g.tsType(errorType) + ">(" + value + ")"
}

func (g *generator) filesystemResultErr(valueType, errorType types.Type, value string) string {
	return g.runtimeName("Result") + ".Err<" + g.tsType(valueType) + ", " + g.tsType(errorType) + ">(" + value + ")"
}

func (g *generator) filesystemStructuredBlock(block *ir.StructuredBlock) {
	if block == nil || block.Result == nil || len(block.Call.Arguments) < 2 {
		return
	}
	g.temporary++
	id := strconv.Itoa(g.temporary)
	raw := "__trbFileEffect" + id
	path := "__trbFilePath" + id
	mode := "__trbFileMode" + id
	handle := "__trbFileHandle" + id
	result := "__trbFileResult" + id
	completed := "__trbFileCompleted" + id
	successType := block.EffectSuccess
	if successType.Kind == "" || successType.Kind == types.Void {
		successType = types.FromName("Unit")
	}
	errorType := block.Fails
	rawType := block.Call.ExprType()
	resultType := g.tsType(rawType)
	g.line("const " + raw + ": " + resultType + " = (() => {")
	g.indent++
	g.line("const " + path + " = " + g.expr(block.Call.Arguments[0].Value) + ";")
	g.line(`const fs = (globalThis as any).process?.getBuiltinModule?.("fs");`)
	unavailable := g.filesystemErrorValue("open", path, strconv.Quote("filesystem is unavailable"), "Other")
	g.line("if (fs === undefined) return " + g.filesystemResultErr(successType, errorType, unavailable) + ";")
	g.line("const " + mode + " = " + g.expr(block.Call.Arguments[1].Value) + ";")
	g.line("const flags = " + mode + " === " + g.runtimeName("FileSystem::OpenMode") + ".Read ? \"r\" : " + mode + " === " + g.runtimeName("FileSystem::OpenMode") + ".Write ? \"w\" : \"wx\";")
	openOther := g.filesystemErrorValue("open", path, "message", "Other")
	openExists := g.filesystemErrorValue("open", path, "message", "AlreadyExists")
	g.line("let " + handle + ": number;")
	g.line("try { " + handle + " = fs.openSync(" + path + ", flags, 0o644); } catch (error) { const message = error instanceof Error ? error.message : String(error); return " + g.filesystemResultErr(successType, errorType, "(error as any)?.code === \"EEXIST\" ? "+openExists+" : "+openOther) + "; }")
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
	closeError := g.filesystemErrorValue("close", path, "message", "Other")
	g.line("try { fs.closeSync(" + handle + "); } catch (error) { if (" + completed + ") { const message = error instanceof Error ? error.message : String(error); " + result + " = " + g.filesystemResultErr(successType, errorType, closeError) + "; } }")
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
