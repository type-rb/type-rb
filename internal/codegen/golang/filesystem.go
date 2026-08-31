package golang

import (
	"strconv"

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
	return g.goType(errorType) + "{Operation: " + strconv.Quote(operation) + ", Path: " + path + ", Message: " + message + ", Kind: " + g.filesystemKind(kind) + "}"
}

func (g *generator) filesystemStructuredBlock(block *ir.StructuredBlock) {
	if block == nil || block.Result == nil || len(block.Call.Arguments) < 1 {
		return
	}
	g.requireImport("os", "")
	g.temporary++
	id := strconv.Itoa(g.temporary)
	raw := "__trbFileEffect" + id
	path := "__trbFilePath" + id
	mode := "__trbFileMode" + id
	handle := "__trbFileHandle" + id
	openError := "__trbFileOpenError" + id
	completed := "__trbFileCompleted" + id
	value := "__trbFileValue" + id
	successType := block.EffectSuccess
	if successType.Kind == "" || successType.Kind == types.Void {
		successType = types.FromName("Unit")
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
	g.line(path + " := " + g.expr(block.Call.Arguments[0].Value))
	modeValue := g.filesystemOpenMode("Read")
	if len(block.Call.Arguments) > 1 {
		modeValue = g.expr(block.Call.Arguments[1].Value)
	}
	g.line(mode + " := " + modeValue)
	g.line("flags := os.O_RDONLY")
	g.line("if " + mode + " == " + g.filesystemOpenMode("Write") + " { flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC }")
	g.line("if " + mode + " == " + g.filesystemOpenMode("CreateNew") + " { flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL }")
	g.line(handle + ", " + openError + " := os.OpenFile(" + path + ", flags, 0o644)")
	openFailure := g.filesystemError(errorType, "open", path, openError+".Error()", "Other")
	existsFailure := g.filesystemError(errorType, "open", path, openError+".Error()", "AlreadyExists")
	g.line("if " + openError + " != nil { if os.IsExist(" + openError + ") { return " + g.filesystemResultErr(successType, errorType, existsFailure) + " }; return " + g.filesystemResultErr(successType, errorType, openFailure) + " }")
	g.line(completed + " := false")
	closeFailure := g.filesystemError(errorType, "close", path, "closeError.Error()", "Other")
	g.line("defer func() { if closeError := " + handle + ".Close(); " + completed + " && closeError != nil { " + raw + " = " + g.filesystemResultErr(successType, errorType, closeFailure) + " } }()")
	if len(block.Bindings) > 0 && block.Bindings[0].Name != "_" {
		binding := g.bindingIdentifier(block.Bindings[0].Name)
		g.line(binding + " := " + handle)
		g.line("_ = " + binding)
	}
	g.statements(block.Body)
	g.line(value + " := " + g.expr(block.Value))
	g.line(completed + " = true")
	g.line("return " + g.filesystemResultOK(successType, errorType, value))
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
