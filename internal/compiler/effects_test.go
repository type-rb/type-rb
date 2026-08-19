package compiler

import (
	"strings"
	"testing"
)

func TestResultFunctionValuesLowerAcrossBackends(t *testing.T) {
	source := []byte(`import { Result } from trb/std/result

record AppError
	message: String
end

def read_number(): Result<Integer, AppError>
	return Result<Integer, AppError>::Ok(7)
end

def invoke<T, E>(callback: () -> Result<T, E>): Result<T, E>
	return callback()
end

def main()
	callback: () -> Result<Integer, AppError> := fn(): Result<Integer, AppError>
		return read_number()
	end
	puts(invoke<Integer, AppError>(callback))
	return
end
`)

	expected := map[string][]string{
		"go": {
			"callback func() __trb_result.Result[T, E]",
			"func() __trb_result.Result[int, AppError]",
		},
		"ruby": {
			"callback.call",
			"Result::Ok.new(7)",
		},
		"typescript": {
			"callback: () => Result<number, AppError> | Promise<Result<number, AppError>>",
			"(): Result<number, AppError> =>",
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifact, err := Compile("main.trb", source, mode)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range expected[mode] {
				if !strings.Contains(string(artifact.Output), want) {
					t.Fatalf("generated %s Result function is missing %q:\n%s", mode, want, artifact.Output)
				}
			}
		})
	}
}

func TestResultFunctionValuesRequireOrdinaryResultHandling(t *testing.T) {
	source := []byte(`import { Result } from trb/std/result

record AppError
end

def read_number(): Result<Integer, AppError>
	return Result<Integer, AppError>::Ok(7)
end

def invalid()
	callback: () -> Result<Integer, AppError> := fn(): Result<Integer, AppError>
		return read_number()
	end
	callback()
	return
end
`)
	if _, err := Compile("main.trb", source, "go"); err == nil || !strings.Contains(err.Error(), "Result value must be used") {
		t.Fatalf("missing Result function must-use diagnostic: %v", err)
	}

	unsafe := []byte(`import { Result } from trb/std/result

record AppError
end

pure: () -> Integer := fn(): Integer
	return 1
end
result_callback: () -> Result<Integer, AppError> := pure
`)
	if _, err := Compile("main.trb", unsafe, "go"); err == nil || !strings.Contains(err.Error(), "cannot assign () -> Integer to () -> Result<Integer, AppError>") {
		t.Fatalf("pure-to-Result assignment diagnostic: %v", err)
	}
}
