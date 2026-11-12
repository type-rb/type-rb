package ruby

// The helper replaces itself with the requested executable. Calling execv,
// rather than Ruby's spawn/exec, prevents libc's ENOEXEC shell fallback. The
// close-on-exec pipe separates launch failures from a child's exit status 127.
const rubyProcessRuntime = `
module TrbProcessRuntime
  HELPER = <<'TRB_PROCESS_HELPER'
require "json"
error_pipe = IO.new(3, "w")
begin
  gem "fiddle", "1.1.8"
  require "fiddle"
  payload = JSON.parse(STDIN.read)
  ENV.replace(payload.fetch("environment"))
  command = payload.fetch("command")
  arguments = payload.fetch("arguments")
  error_pipe.close_on_exec = true
  [STDIN, STDOUT, STDERR].each { |stream| stream.close_on_exec = false }
  Signal.trap("PIPE", "DEFAULT")
  execute = Fiddle::Function.new(Fiddle::Handle::DEFAULT["execv"], [Fiddle::TYPE_VOIDP, Fiddle::TYPE_VOIDP], Fiddle::TYPE_INT)
  strings = [command, *arguments].map { |value| Fiddle::Pointer[value + "\0"] }
  argv = Fiddle::Pointer[(strings.map(&:to_i) + [0]).pack("J*")]
  paths = command.include?("/") ? [command] : ENV.fetch("PATH", "").split(":", -1).map { |directory| File.join(directory.empty? ? "." : directory, command) }
  failure = Errno::ENOENT::Errno
  error_pipe.write("R")
  error_pipe.flush
  paths.each do |path|
    execute.call(path, argv)
    code = Fiddle.last_error
    if code == Errno::EACCES::Errno
      failure = code
    elsif code != Errno::ENOENT::Errno && code != Errno::ENOTDIR::Errno
      raise SystemCallError.new(command, code)
    end
  end
  raise SystemCallError.new(command, failure)
rescue Exception => error
  error_pipe.write("\nE" + error.message.encode(Encoding::UTF_8, invalid: :replace, undef: :replace)[0, 500])
  error_pipe.flush
  exit! 127
end
TRB_PROCESS_HELPER

  def self.capture(command, arguments)
    require "open3"
    require "json"
    require "rbconfig"
    raise ArgumentError, "command is empty" if command.empty?
    raise ArgumentError, "command or argument contains NUL" if [command, *arguments].any? { |value| value.include?("\0") }
    reader, writer = IO.pipe
    begin
      payload = JSON.generate({command: command, arguments: arguments, environment: ENV.to_h})
      output = Open3.capture3({"RUBYOPT" => nil}, RbConfig.ruby, "-e", HELPER, 3 => writer, stdin_data: payload, binmode: true)
      writer.close
      launch = reader.read
      raise IOError, (launch.include?("\nE") ? launch.split("\nE", 2).last : "process launcher failed") unless launch == "R"
      output
    ensure
      writer.close unless writer.closed?
      reader.close
    end
  end
end
`
