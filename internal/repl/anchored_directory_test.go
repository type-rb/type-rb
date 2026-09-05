package repl

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/stdlib"
	"github.com/type-rb/type-rb/internal/types"
)

func TestEvaluateAnchoredDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "sample"), []byte("anchored"), 0o600); err != nil {
		t.Fatal(err)
	}
	source := `import trb/std/path
import { RelativePath } from trb/std/path
import trb/std/dir
import trb/std/file
import { FileSystemError } from trb/std/errors
import trb/std/result

def read(file: File): Result<String, FileSystemError>
	return file.read_text(max_bytes: 100)
end

def run(path: Path): String
	case RelativePath.parse("sample")
	when Result::Err(_error)
		return "parse-failed"
	when Result::Ok(child)
		result := Dir.open<String>(path) do |root|
			entries := try root.children(max_entries: 20)
			text := try root.open_file<String>(child) do |file|
				try read(file)
			end
			entries.size().to_s() + ":" + text
		end
		case result
		when Result::Ok(text)
			return text
		when Result::Err(error)
			return error.message
		end
	end
end

run(Path.new(` + strconv.Quote(root) + `))
`
	if got := evaluateDirBoundarySource(t, "go", source); got != `"1:anchored"` {
		t.Fatalf("got %s", got)
	}
}

func TestAnchoredDirectoryCleanupAndBodyFailure(t *testing.T) {
	for _, scenario := range []string{"success", "body-error", "runtime-fault"} {
		t.Run(scenario, func(t *testing.T) {
			source := compiler.SourceUnit{Filename: "main.trb", ModulePath: "main", Source: []byte("import trb/std/dir\nimport { FileSystemError } from trb/std/errors\nimport trb/std/result\n")}
			artifacts, err := compiler.CompileProject([]compiler.SourceUnit{source}, compiler.Options{Mode: "go", AllowUnusedImports: true})
			if err != nil {
				t.Fatal(err)
			}
			programs := make([]*ir.Program, 0, len(artifacts))
			for _, artifact := range artifacts {
				programs = append(programs, artifact.IR)
			}
			e := NewEvaluator(nil, "go")
			defer e.Close()
			if err := e.LoadProject(programs, "main"); err != nil {
				t.Fatal(err)
			}
			resultType := stdlib.ResultType(types.FromName("String"), stdlib.FileSystemErrorType())
			var acquired *os.Root
			fault := errors.New("injected runtime fault")
			returned := false
			value, err := e.anchoredDirectoryBlock(runtimeBlockInvocation{
				Name: "trb.std.dir.open", Type: resultType,
				Arguments:    []evaluatedArgument{{Value: Value{Type: stdlib.PathType(), Data: t.TempDir()}}},
				BodyReturned: func() bool { return returned },
				Evaluate: func(bindings []Value) (Value, error) {
					acquired = bindings[0].Data.(*os.Root)
					if scenario == "runtime-fault" {
						return Value{}, fault
					}
					if scenario == "body-error" {
						returned = true
						return e.filesystemDomainError(resultType, "children", "", errors.New("body failure"), "InvalidLimit", true, false)
					}
					return Value{Type: types.FromName("String"), Data: "body success"}, nil
				},
			})
			if acquired == nil {
				t.Fatal("body was not entered")
			}
			if handle, err := acquired.Open("."); err == nil {
				handle.Close()
				t.Fatal("anchor survived its acquisition scope")
			}
			if scenario == "runtime-fault" {
				if !errors.Is(err, fault) {
					t.Fatalf("lost fault: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			result := value.Data.(*enumValue)
			if scenario == "success" {
				if result.Name != "Ok" {
					t.Fatalf("expected Ok: %#v", result)
				}
				return
			}
			if result.Name != "Err" {
				t.Fatalf("expected Err: %#v", result)
			}
			failure := result.Payload["error"].Data.(*recordInstance)
			want := "children"
			if got := failure.Fields["operation"].Data; got != want {
				t.Fatalf("operation=%v, want %s", got, want)
			}
		})
	}
}
