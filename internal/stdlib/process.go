package stdlib

func processSource() string {
	return `import trb/std/result
import trb/internal/process as native_process

module Process
	record Output
		status: Integer
		stdout: String
		stderr: String
		success: Boolean
	end

	record Error
		operation: String
		command: String
		message: String
	end
end

def argv(): Array<String>
	return native_process.argv()
end

def environment(name: String): String?
	return native_process.environment(name)
end

def working_directory(): Result<String, Process::Error>
	return native_process.working_directory()
end

def run(command: String, args: Array<String>): Result<Process::Output, Process::Error>
	return native_process.run(command, args)
end
`
}
