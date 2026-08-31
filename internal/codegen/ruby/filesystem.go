package ruby

import (
	"strconv"

	"github.com/type-rb/type-rb/internal/ir"
)

func rubyFilesystemError(operation, path, message, kind string) string {
	return "FileSystemError.new(operation: " + strconv.Quote(operation) + ", path: " + path + ", message: " + message + ", kind: FileSystemErrorKind::" + kind + ")"
}

func (g *generator) filesystemStructuredBlock(block *ir.StructuredBlock) {
	if block == nil || block.Result == nil || len(block.Call.Arguments) < 1 {
		return
	}
	g.temporary++
	id := strconv.Itoa(g.temporary)
	raw := "__trb_file_effect" + id
	path := "__trb_file_path" + id
	mode := "__trb_file_mode" + id
	handle := "__trb_file_handle" + id
	result := "__trb_file_result" + id
	completed := "__trb_file_completed" + id
	g.line(raw+" = -> do", block.TrailingComment)
	g.indent++
	g.line(path+" = "+g.expr(block.Call.Arguments[0].Value), "")
	modeValue := "FileMode::Read"
	if len(block.Call.Arguments) > 1 {
		modeValue = g.expr(block.Call.Arguments[1].Value)
	}
	g.line(mode+" = "+modeValue, "")
	g.line("flags = "+mode+" == FileMode::Read ? \"rb\" : ("+mode+" == FileMode::Write ? \"wb\" : \"wbx\")", "")
	g.line("begin", "")
	g.indent++
	g.line(handle+" = ::File.open("+path+", flags, 0o644)", "")
	g.indent--
	g.line("rescue Errno::EEXIST => error", "")
	g.indent++
	g.line("return Result::Err.new("+rubyFilesystemError("open", path, "error.message", "AlreadyExists")+")", "")
	g.indent--
	g.line("rescue StandardError => error", "")
	g.indent++
	g.line("return Result::Err.new("+rubyFilesystemError("open", path, "error.message", "Other")+")", "")
	g.indent--
	g.line("end", "")
	g.line(completed+" = false", "")
	g.line(result+" = nil", "")
	g.line("begin", "")
	g.indent++
	if len(block.Bindings) > 0 && block.Bindings[0].Name != "_" {
		g.line(block.Bindings[0].Name+" = "+handle, "")
	}
	g.statements(block.Body)
	g.line(result+" = Result::Ok.new("+g.expr(block.Value)+")", "")
	g.line(completed+" = true", "")
	g.indent--
	g.line("ensure", "")
	g.indent++
	g.line("begin", "")
	g.indent++
	g.line(handle+".close", "")
	g.indent--
	g.line("rescue StandardError => error", "")
	g.indent++
	g.line(result+" = Result::Err.new("+rubyFilesystemError("close", path, "error.message", "Other")+") if "+completed, "")
	g.indent--
	g.line("end", "")
	g.indent--
	g.line("end", "")
	g.line(result, "")
	g.indent--
	g.line("end.call", "")
	g.ormAssignStructuredResult(raw, block.Result, block.CaptureEffect, block.PropagateSuccess)
}
