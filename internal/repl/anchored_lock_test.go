package repl

import (
	"errors"
	"os"
	"strconv"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/nativefs"
	"github.com/type-rb/type-rb/internal/stdlib"
	"github.com/type-rb/type-rb/internal/types"
)

func TestEvaluateAnchoredLock(t *testing.T) {
	source := `import trb/std/dir
import trb/std/path
import { RelativePath } from trb/std/path
import trb/std/result

def run(directory: Path): String
	case RelativePath.parse("guard")
	when Result::Err(_error)
		return "invalid"
	when Result::Ok(path)
		result := Dir.open(directory) do |root|
			text := try root.try_lock<String>(path) do
				"locked"
			end
			text
		end
		case result
		when Result::Ok(value)
			return value
		when Result::Err(error)
			return error.message
		end
	end
end
run(Path.new(` + strconv.Quote(t.TempDir()) + `))
`
	if got := evaluateDirBoundarySource(t, "go", source); got != `"locked"` {
		t.Fatalf("got %q", got)
	}
}

func TestAnchoredLockCleanupAndBodyFailure(t *testing.T) {
	for _, scenario := range []string{"success", "body-error", "runtime-fault"} {
		t.Run(scenario, func(t *testing.T) {
			unit := compiler.SourceUnit{Filename: "main.trb", ModulePath: "main", Source: []byte("import trb/std/dir\nimport { FileSystemError } from trb/std/errors\nimport trb/std/result\n")}
			artifacts, err := compiler.CompileProject([]compiler.SourceUnit{unit}, compiler.Options{Mode: "go", AllowUnusedImports: true})
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
			root, err := os.OpenRoot(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			resultType := stdlib.ResultType(types.FromName("String"), stdlib.FileSystemErrorType())
			fault := errors.New("injected runtime fault")
			returned, entered := false, false
			value, err := e.anchoredLockBlock(runtimeBlockInvocation{
				Name: "trb.std.dir.try_lock", Type: resultType,
				Arguments:    []evaluatedArgument{{Value: Value{Type: stdlib.DirResourceType(), Data: root}}, {Value: Value{Type: stdlib.RelativePathType(), Data: "guard"}}},
				BodyReturned: func() bool { return returned },
				Evaluate: func(bindings []Value) (Value, error) {
					entered = true
					if len(bindings) != 0 {
						t.Fatal("public lock handle was exposed")
					}
					file, err := nativefs.TryLock(root, "guard")
					if file != nil {
						_ = file.Close()
						t.Fatal("lock was not held")
					}
					if !errors.Is(err, nativefs.ErrBusy) {
						t.Fatalf("reentry: %v", err)
					}
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
			if !entered {
				t.Fatal("body was not entered")
			}
			file, acquireErr := nativefs.TryLock(root, "guard")
			if acquireErr != nil {
				t.Fatalf("scope retained lock: %v", acquireErr)
			}
			_ = file.Close()
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
			want := "Ok"
			if scenario == "body-error" {
				want = "Err"
			}
			if result.Name != want {
				t.Fatalf("got %s, want %s", result.Name, want)
			}
		})
	}
}
