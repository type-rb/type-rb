package compiler

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const anchoredLockSource = `import trb/std/path
import { RelativePath } from trb/std/path
import trb/std/dir
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result

def protected(root: Dir, path: RelativePath): Result<String, FileSystemError>
	return root.try_lock<String>(path) do
		"locked"
	end
end

def run(path: Path, child: RelativePath): Result<String, FileSystemError>
	return Dir.open(path) do |root|
		try protected(root, child)
	end
end
`

func TestAnchoredLockEmitsSharedSupport(t *testing.T) {
	artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: []byte(anchoredLockSource)}}, Options{Mode: "go", GoModule: "example.com/locking"})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, artifact := range artifacts {
		count += len(artifact.SupportFiles)
		if artifact.IR.ModulePath == "main" && !strings.Contains(string(artifact.Output), "__trb_nativefs.TryLock") {
			t.Fatalf("missing lock lowering:\n%s", artifact.Output)
		}
	}
	if count != 4 {
		t.Fatalf("got %d native support files", count)
	}
}

func TestAnchoredLockRuns(t *testing.T) {
	source := anchoredLockSource + `
def main()
	case RelativePath.parse("guard")
	when Result::Err(_error)
		puts("invalid")
	when Result::Ok(child)
		case run(Path.new(` + strconv.Quote(t.TempDir()) + `), child)
		when Result::Ok(value)
			puts(value)
		when Result::Err(error)
			puts(error.message)
		end
	end
end
`
	artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: []byte(source)}}, Options{Mode: "go", GoModule: "example.com/locking"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(runEffectProject(t, "go", artifacts, "example.com/locking")); got != "locked" {
		t.Fatalf("got %q", got)
	}
}

func TestAnchoredLockRequiresParameterlessSynchronousBody(t *testing.T) {
	for name, source := range map[string]string{
		"parameter":  strings.ReplaceAll(anchoredLockSource, "try_lock<String>(path) do", "try_lock<String>(path) do |lock|"),
		"suspension": strings.ReplaceAll(anchoredLockSource, "\"locked\"", "pause()") + "\ndef pause(): String\n\tvalues := [1].concurrent_map(limit: 1) do |value|\n\t\tvalue\n\tend\n\treturn values.size().to_s()\nend\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Source: []byte(source)}}, Options{Mode: "go"})
			if err == nil {
				t.Fatal("accepted invalid lock body")
			}
			if name == "suspension" && !strings.Contains(err.Error(), "suspend") && !strings.Contains(err.Error(), "synchronous") {
				t.Fatalf("wrong rejection: %v", err)
			}
		})
	}
	for _, mode := range []string{"ruby", "typescript"} {
		_, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Source: []byte(anchoredLockSource)}}, Options{Mode: mode})
		if err == nil || !strings.Contains(err.Error(), "anchored Dir operations require") {
			t.Fatalf("%s: %v", mode, err)
		}
	}
}

func TestAnchoredLockSuccessRetainsCleanupFailure(t *testing.T) {
	source := strings.ReplaceAll(anchoredLockSource, "{ FileSystemError }", "{ FileSystemError, FileSystemTarget }")
	source += `
def main()
	case RelativePath.parse("guard")
	when Result::Err(_error)
		puts("invalid")
	when Result::Ok(child)
		case run(Path.new(` + strconv.Quote(t.TempDir()) + `), child)
		when Result::Ok(value)
			puts(value)
		when Result::Err(error)
			case error.target
			when FileSystemTarget::Relative(path)
				puts(error.operation + ":" + path.to_s())
			else
				puts("wrong-target")
			end
		end
	end
end
`
	artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: []byte(source)}}, Options{Mode: "go", GoModule: "example.com/locking"})
	if err != nil {
		t.Fatal(err)
	}
	close := regexp.MustCompile(`if closeError := (__trbFileHandle[0-9]+)\.Close\(\);`)
	for _, artifact := range artifacts {
		if artifact.IR.ModulePath == "main" {
			// Report a native cleanup failure after actually releasing the
			// descriptor. No TypeRB code can inject this adapter behavior.
			artifact.Output = close.ReplaceAll(artifact.Output, []byte(`if closeError := func() error { _ = $1.Close(); return errors.New("injected cleanup failure") }();`))
		}
	}
	if got := strings.TrimSpace(runEffectProject(t, "go", artifacts, "example.com/locking")); got != "close:guard" {
		t.Fatalf("lost cleanup failure: %q", got)
	}
}
