package stdlib

func processSource() string {
	return `import trb/std/result
import trb/internal/process as native_process

record ProcessResult
	status: Integer
	stdout: String
	stderr: String
	success: Boolean
end

record ProcessError
	operation: String
	command: String
	message: String
end

def argv(): Array<String>
	return native_process.argv()
end

def environment(name: String): String?
	return native_process.environment(name)
end

def working_directory(): Result<String, ProcessError>
	return native_process.working_directory()
end

def run(command: String, args: Array<String>): Result<ProcessResult, ProcessError>
	return native_process.run(command, args)
end
`
}
