package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

const arrayAssignmentProgram = `def grow(mut values: Array<Integer>): Integer
	mut count := 0
	while count < 64
		values.push(3)
		count += 1
	end
	return 9
end

def change(mut values: Array<Integer>): Integer
	values[1] = 20
	return grow(values)
end

def add_true(mut values: Array<Boolean>): Boolean
	values.push(true)
	return false
end

def add_false(mut values: Array<Boolean>): Boolean
	values.push(false)
	return true
end

def skipped(): Boolean
	puts("unexpected RHS")
	return true
end

def outer(): Integer
	puts("outer")
	return 0
end

def replace_inner(mut rows: Array<Array<Integer>>): Integer
	puts("inner")
	rows[0] = [7, 8]
	return -1
end

def prepend(mut values: Array<Integer>): Integer
	values.unshift(0)
	return 9
end

def rebuild(mut values: Array<Integer>): Integer
	values.pop()
	values.pop()
	values.push(7)
	values.push(8)
	return 9
end

def grow_strings(mut values: Array<String>): String
	mut count := 0
	while count < 64
		values.push("extra")
		count += 1
	end
	return "new"
end

def main()
	mut values := [1, 2]
	mut shared := values
	values[-1] = grow(shared)
	puts(values[1])
	puts(shared[1])
	puts(values.size())
	values = [1, 2]
	values[values.size() - 1] = grow(values)
	puts(values[1])
	values = [1, 2]
	values[-1] += grow(values)
	puts(values[1])
	values = [1, 2]
	values[-1] += change(values)
	puts(values[1])
	mut flags := [true, false]
	flags[-1] ||= add_true(flags)
	puts(flags[1])
	puts(flags[2])
	flags = [false, true]
	flags[-1] &&= add_false(flags)
	puts(flags[1])
	puts(flags[2])
	flags[0] &&= skipped()
	flags[1] ||= skipped()
	mut original := [1, 2]
	mut rows := [original]
	rows[outer()][replace_inner(rows)] = 9
	puts(original[1])
	puts(rows[0][1])
	values = [1, 2]
	values[-1] += prepend(values)
	puts(values[0])
	puts(values[1])
	puts(values[2])
	values = [1, 2]
	values[-1] += rebuild(values)
	puts(values[0])
	puts(values[1])
	mut strings := ["first", "old"]
	strings[-1] = grow_strings(strings)
	puts(strings[1])
	puts(strings.size())
	return
end
`

func runArrayAssignmentCase(t *testing.T, source, want, failure string) {
	t.Helper()
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, surface := range []string{"run", "repl"} {
			t.Run(mode+"/"+surface, func(t *testing.T) {
				if surface == "run" {
					executable := map[string]string{"go": "go", "ruby": "ruby", "typescript": "node"}[mode]
					if _, err := exec.LookPath(executable); err != nil {
						t.Skipf("%s is unavailable", executable)
					}
				}
				root := t.TempDir()
				config := project.New(root, mode)
				config.SourceDir = "src"
				if config.Go != nil {
					config.Go.Module = "example.com/array-assignment"
				}
				if err := config.Save(); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(config.SourcePath(), 0755); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(config.SourcePath(), "main.trb")
				if surface == "run" {
					if err := os.WriteFile(path, []byte(source), 0644); err != nil {
						t.Fatal(err)
					}
				}
				args := []string{surface, "--config", config.Path}
				input := ""
				if surface == "run" {
					args = append(args, path)
				} else {
					input = source + "\nmain()\n:quit\n"
				}
				var stdout, stderr bytes.Buffer
				command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
				status := command.Run(args)
				if stdout.String() != want {
					t.Fatalf("stdout=%q want=%q status=%d stderr=%s", stdout.String(), want, status, stderr.String())
				}
				if failure == "" {
					if status != 0 || stderr.Len() != 0 {
						t.Fatalf("status=%d stderr=%s", status, stderr.String())
					}
				} else if !strings.Contains(strings.ToLower(stderr.String()), failure) || (surface == "run" && status == 0) {
					t.Fatalf("expected %q failure, status=%d stderr=%s", failure, status, stderr.String())
				}
			})
		}
	}
}

func TestArrayAssignmentRetainsPositionAcrossAllExecutionPaths(t *testing.T) {
	runArrayAssignmentCase(t, arrayAssignmentProgram, "9\n9\n66\n9\n11\n11\nfalse\ntrue\ntrue\nfalse\nouter\ninner\n9\n8\n0\n11\n2\n7\n11\nnew\n66\n", "")
}

func TestArrayAssignmentChecksBoundsBeforeRHSAndBeforeStore(t *testing.T) {
	for _, test := range []struct{ name, index, operator, mutation, want string }{
		{"initial-positive", "2", "=", "values.push(3)", ""},
		{"initial-negative", "-3", "=", "values.push(3)", ""},
		{"shrink-set", "-1", "=", "values.pop()", "rhs\n"},
		{"shrink-add", "-1", "+=", "values.pop()", "rhs\n"},
		{"empty-set", "-1", "=", "values.pop()\n\tvalues.pop()", "rhs\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := "def change(mut values: Array<Integer>): Integer\n\t" + test.mutation + "\n\tputs(\"rhs\")\n\treturn 9\nend\n\ndef main()\n\tmut values := [1, 2]\n\tvalues[" + test.index + "] " + test.operator + " change(values)\n\tputs(\"unexpected store\")\nend\n"
			runArrayAssignmentCase(t, source, test.want, "out of bounds")
		})
	}
}

func TestArrayAssignmentPreservesRHSControlFlowAndTypedValues(t *testing.T) {
	source := `import { Result } from trb/std/result

def change(mut values: Array<Integer>, succeed: Boolean): Result<Integer, String>
	values.push(3)
	if succeed
		return Result<Integer, String>::Ok(9)
	end
	return Result<Integer, String>::Err("missing")
end

def assign_try(mut values: Array<Integer>, succeed: Boolean): Result<Integer, String>
	values[-1] += try change(values, succeed)
	return Result<Integer, String>::Ok(values[1])
end

def assign_return(mut values: Array<Integer>, leave: Boolean): Integer
	values[-1] = if leave
		values.push(3)
		return 7
	else
		9
	end
	return values[1]
end

def replace<T>(mut values: Array<T>, value: T)
	values[-1] = value
end

def main()
	mut values := [1, 2]
	case assign_try(values, false)
	when Result::Ok(value)
		puts(value)
	when Result::Err(error)
		puts(error)
	end
	puts(values[1])
	puts(values.size())
	values = [1, 2]
	case assign_try(values, true)
	when Result::Ok(value)
		puts(value)
	when Result::Err(error)
		puts(error)
	end
	values = [1, 2]
	values[-1] = change(values, false) catch |_error|
		5
	end
	puts(values[1])
	values = [1, 2]
	puts(assign_return(values, true))
	puts(values[1])
	puts(values.size())
	values = [1, 2]
	puts(assign_return(values, false))
	mut floats := [1.0, 2.0]
	floats[-1] += 1
	puts(floats[1] == 3.0)
	mut optional: Array<Integer?> := [1, nil]
	optional[-1] = 4
	optional[0] = nil
	puts(optional[0] == nil)
	value := optional[1]
	if value == nil
		puts(false)
	else
		puts(value == 4)
	end
	mut labels := ["a", "b"]
	replace<String>(labels, "c")
	puts(labels[1])
	mut results: Array<Result<Integer, String>> := [Result<Integer, String>::Ok(1)]
	results[0] = Result<Integer, String>::Err("stored")
	case results[0]
	when Result::Ok(value)
		puts(value)
	when Result::Err(error)
		puts(error)
	end
	return
end
`
	runArrayAssignmentCase(t, source, "missing\n2\n3\n11\n5\n7\n2\n3\n9\ntrue\ntrue\ntrue\nc\nstored\n", "")
}
