package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunResultTryAndCatchAcrossBackends(t *testing.T) {
	tests := []struct {
		name     string
		required string
		args     func(string) []string
	}{
		{name: "go", required: "go", args: func(filename string) []string { return []string{filename} }},
		{name: "ruby", required: "ruby", args: func(filename string) []string { return []string{"run", "--mode", "ruby", filename} }},
		{name: "typescript-node", required: "node", args: func(filename string) []string {
			return []string{"run", "--mode", "typescript", "--runtime", "node", filename}
		}},
		{name: "typescript-bun", required: "bun", args: func(filename string) []string {
			return []string{"run", "--mode", "typescript", "--runtime", "bun", filename}
		}},
	}

	source := `import { Result } from trb/std/result

enum AppError
	NotFound
end

alias AppResult<T> = Result<T, AppError>

class Probe
	@calls: Integer

	def initialize()
		@calls = 0
		return
	end

	def read(success: Boolean): AppResult<Integer>
		@calls += 1
		if success
			return AppResult<Integer>::Ok(7)
		end
		return AppResult<Integer>::Err(AppError::NotFound)
	end

	def calls(): Integer
		return @calls
	end
end

def source(success: Boolean): AppResult<Integer>
	if success
		return AppResult<Integer>::Ok(7)
	end
	return AppResult<Integer>::Err(AppError::NotFound)
end

def propagated(success: Boolean): AppResult<String>
	value := try source(success)
	return AppResult<String>::Ok("value=" + value.to_s())
end

def recovered(success: Boolean): Integer
	return source(success) catch |_error|
		41
	end
end

def returned(success: Boolean): String
	value := source(success) catch |_error|
		return "caught"
	end
	return value.to_s()
end

def lambda_propagated(success: Boolean): AppResult<String>
	callback := fn(): AppResult<String>
		value := try source(success)
		return AppResult<String>::Ok("lambda=" + value.to_s())
	end
	return callback()
end

def void_lambda_catch_return(): String
	callback := fn()
		_value := source(false) catch |_error|
			return
		end
		return
	end
	callback()
	return "outer"
end

def each_propagated(success: Boolean): AppResult<String>
	mut rendered := ""
	[true, success, true].each do |current|
		value := try source(current)
		rendered += value.to_s()
	end
	return AppResult<String>::Ok(rendered)
end

def transform_fallbacks(): Integer
	mapped := [true, false, true].map do |success|
		source(success) catch |_error|
			0
		end
	end
	selected := [true, false, true].select do |success|
		value := source(success) catch |_error|
			0
		end
		value > 0
	end
	total := [true, false, true].reduce(0) do |sum, success|
		value := source(success) catch |_error|
			1
		end
		sum + value
	end
	return mapped[0] + mapped[1] + mapped[2] + selected.size() + total
end

def catch_once(): Integer
	mut probe := Probe.new()
	value := probe.read(false) catch |_error|
		0
	end
	return value + probe.calls()
end

def try_once(): AppResult<Integer>
	mut probe := Probe.new()
	value := try probe.read(true)
	return AppResult<Integer>::Ok(value + probe.calls())
end

def integer_failure(): Result<Integer, Integer>
	return Result<Integer, Integer>::Err(7)
end

def widened_failure(): Result<String, Float>
	value := try integer_failure()
	return Result<String, Float>::Ok(value.to_s())
end

def widened_error_converted?(): Boolean
	case widened_failure()
	when Result::Ok(_value)
		return false
	when Result::Err(error)
		return error == 7.0
	end
end

def array_failure(): Result<Array<Integer>, String>
	return Result<Array<Integer>, String>::Err("empty")
end

def recovered_empty_array(): Array<Integer>
	return array_failure() catch |_error|
		[]
	end
end

def render(result: AppResult<String>): String
	case result
	when AppResult::Ok(value)
		return "ok:" + value
	when AppResult::Err(_error)
		return "err"
	end
end

def main()
	puts(render(propagated(true)))
	puts(render(propagated(false)))
	puts(recovered(true))
	puts(recovered(false))
	puts(returned(true))
	puts(returned(false))
	puts(render(lambda_propagated(true)))
	puts(render(lambda_propagated(false)))
	puts(void_lambda_catch_return())
	puts(render(each_propagated(true)))
	puts(render(each_propagated(false)))
	puts(transform_fallbacks())
	puts(catch_once())
	once := try_once() catch |_error|
		-1
	end
	puts(once)
	puts(widened_error_converted?())
	puts(recovered_empty_array().size())
	return
end
`
	want := "ok:value=7\nerr\n7\n41\n7\ncaught\nok:lambda=7\nerr\nouter\nok:777\nerr\n31\n1\n8\ntrue\n0\n"

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := exec.LookPath(test.required); err != nil {
				t.Skipf("%s is unavailable: %v", test.required, err)
			}
			root := t.TempDir()
			filename := filepath.Join(root, "result_flow.trb")
			if err := os.WriteFile(filename, []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run(test.args(filename)); status != 0 {
				t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			if got := stdout.String(); got != want || stderr.Len() != 0 {
				t.Fatalf("Result try/catch output=%q, want %q; stderr=%q", got, want, stderr.String())
			}
		})
	}
}
