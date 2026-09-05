package ruby

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
)

func rubyFilesystemError(operation, path, message, kind string) string {
	return "FileSystemError.new(operation: " + strconv.Quote(operation) + ", target: FileSystemTarget::Host.new(" + path + "), message: " + message + ", kind: FileSystemErrorKind::" + kind + ")"
}

func rubyFilesystemNativeError(operation, path, cause string) string {
	kind := "(" + cause + ".is_a?(Errno::ENOENT) ? FileSystemErrorKind::NotFound : ((" + cause + ".is_a?(Errno::EACCES) || " + cause + ".is_a?(Errno::EPERM)) ? FileSystemErrorKind::PermissionDenied : (" + cause + ".is_a?(Errno::EEXIST) ? FileSystemErrorKind::AlreadyExists : FileSystemErrorKind::Other)))"
	return "FileSystemError.new(operation: " + strconv.Quote(operation) + ", target: FileSystemTarget::Host.new(" + path + "), message: " + cause + ".message, kind: " + kind + ")"
}

func rubyFilesystemCreateAll(argument string) string {
	invalid := rubyFilesystemError("create_all", "path", strconv.Quote("path must be nonempty and contain no NUL"), "InvalidPath")
	return "->(path) { return Result::Err.new(" + invalid + ") if path.empty? || path.include?(\"\\0\"); begin; require \"fileutils\"; ::FileUtils.mkdir_p(path); Result::Ok.new(Unit.new); rescue StandardError => error; Result::Err.new(" + rubyFilesystemNativeError("create_all", "path", "error") + "); end }.call(" + argument + ")"
}

func rubyFilesystemChildren(arguments []string) string {
	invalid := func(kind, message string) string {
		return "Result::Err.new(" + rubyFilesystemError("children", "path", strconv.Quote(message), kind) + ")"
	}
	return strings.NewReplacer(
		"$invalidLimit", invalid("InvalidLimit", "max_entries must be non-negative"),
		"$invalidPath", invalid("InvalidPath", "path must be nonempty and contain no NUL"),
		"$tooLarge", invalid("TooLarge", "directory exceeds max_entries"),
		"$invalidName", invalid("UnsupportedName", "directory entry name is not valid UTF-8"),
		"$failure", "Result::Err.new("+rubyFilesystemNativeError("children", "active_path", "error")+")",
		"$closeFailure", "Result::Err.new("+rubyFilesystemNativeError("children", "path", "close_error")+")",
	).Replace(`->(path, maximum) {
		return $invalidLimit if maximum < 0
		return $invalidPath if path.empty? || path.include?("\0")
		active_path = path; directory = nil; completed = false; result = nil
		begin
			directory = ::Dir.open(path, encoding: Encoding::BINARY); entries = []
			directory.each_child do |raw_name|
				return $tooLarge if entries.length == maximum
				name = raw_name.dup.force_encoding(Encoding::UTF_8)
				return $invalidName unless name.valid_encoding?
				native_separator = ::File::ALT_SEPARATOR.nil? ? ::File::SEPARATOR : ::File::ALT_SEPARATOR
				drive_relative_root = !::File::ALT_SEPARATOR.nil? && path.match?(/\A[A-Za-z]:\z/)
				separator = path.end_with?(::File::SEPARATOR) || (!::File::ALT_SEPARATOR.nil? && path.end_with?(::File::ALT_SEPARATOR)) || drive_relative_root ? "" : native_separator
				child_path = path + separator + name; active_path = child_path
				info = ::File.lstat(child_path); active_path = path
				kind = info.file? ? DirEntryKind::File : (info.directory? ? DirEntryKind::Directory : DirEntryKind::Other)
				entries << DirEntry.new(name: name, path: child_path, kind: kind)
			end
			result = Result::Ok.new(entries.sort_by { |entry| entry.name.b }); completed = true
		rescue StandardError => error
			result = $failure
		ensure
			begin; directory.close unless directory.nil?; rescue StandardError => close_error; result = $closeFailure if completed; end
		end
		result
	}.call(` + arguments[0] + ", " + arguments[1] + ")")
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
	flags := "__trb_file_flags" + id
	openError := "__trb_file_open_error" + id
	closeError := "__trb_file_close_error" + id
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
	invalidPath := rubyFilesystemError("open", path, strconv.Quote("path must be nonempty and contain no NUL"), "InvalidPath")
	g.line("return Result::Err.new("+invalidPath+") if "+path+".empty? || "+path+".include?(\"\\0\")", "")
	g.line(flags+" = "+mode+" == FileMode::Read ? \"rb\" : ("+mode+" == FileMode::Write ? \"wb\" : \"wbx\")", "")
	g.line("begin", "")
	g.indent++
	g.line(handle+" = ::File.open("+path+", "+flags+", 0o644)", "")
	g.indent--
	g.line("rescue Errno::EEXIST => "+openError, "")
	g.indent++
	g.line("return Result::Err.new("+rubyFilesystemError("open", path, openError+".message", "AlreadyExists")+")", "")
	g.indent--
	g.line("rescue StandardError => "+openError, "")
	g.indent++
	g.line("return Result::Err.new("+rubyFilesystemNativeError("open", path, openError)+")", "")
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
	g.line("rescue StandardError => "+closeError, "")
	g.indent++
	g.line(result+" = Result::Err.new("+rubyFilesystemNativeError("close", path, closeError)+") if "+completed, "")
	g.indent--
	g.line("end", "")
	g.indent--
	g.line("end", "")
	g.line(result, "")
	g.indent--
	g.line("end.call", "")
	g.ormAssignStructuredResult(raw, block.Result, block.CaptureEffect, block.PropagateSuccess)
}
