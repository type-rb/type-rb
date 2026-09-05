package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func cliLockSource(directory string) string {
	return `import trb/std/path
import { RelativePath } from trb/std/path
import trb/std/dir
import { FileSystemError, FileSystemErrorKind } from trb/std/errors
import trb/std/result

def reenter(root: Dir, path: RelativePath): String
	result := root.try_lock(path) do
		"unexpected"
	end
	case result
	when Result::Ok(value)
		return value
	when Result::Err(error)
		case error.kind
		when FileSystemErrorKind::Busy
			return "busy"
		else
			return error.message
		end
	end
end

def protect(root: Dir, path: RelativePath): Result<String, FileSystemError>
	return root.try_lock(path) do
		reenter(root, path)
	end
end

def main()
	case RelativePath.parse("guard")
	when Result::Err(_error)
		puts("invalid")
	when Result::Ok(path)
		result := Dir.open(Path.new(` + strconv.Quote(directory) + `)) do |root|
			first := try protect(root, path)
			second := try protect(root, path)
			first + ":" + second
		end
		case result
		when Result::Ok(value)
			puts(value)
		when Result::Err(error)
			puts(error.message)
		end
	end
end
`
}

func TestAnchoredLockStandaloneRunAndBuild(t *testing.T) {
	t.Setenv("CGO_ENABLED", "0")
	root := t.TempDir()
	entry := filepath.Join(root, "guard.trb")
	if err := os.WriteFile(entry, []byte(cliLockSource(root)), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	cli := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := cli.Run([]string{"run", entry}); status != 0 || stdout.String() != "busy:busy\n" {
		t.Fatalf("run: %d\n%s\n%s", status, &stdout, &stderr)
	}
	stdout.Reset()
	stderr.Reset()
	output := filepath.Join(root, "guard-bin")
	if status := cli.Run([]string{"build", "--compile", "--outfile", output, entry}); status != 0 {
		t.Fatalf("build: %d\n%s\n%s", status, &stdout, &stderr)
	}
	data, err := exec.Command(output).CombinedOutput()
	if err != nil || string(data) != "busy:busy\n" {
		t.Fatalf("executable: %v\n%s", err, data)
	}
}

func TestAnchoredLockBuildTreeIncludesNativeSupport(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/guard"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.SourcePath(), 0o700); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(config.SourcePath(), "main.trb")
	if err := os.WriteFile(entry, []byte(cliLockSource(root)), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	cli := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := cli.Run([]string{"build", "--config", filepath.Join(root, project.ConfigName)}); status != 0 {
		t.Fatalf("build: %d\n%s\n%s", status, &stdout, &stderr)
	}
	command := exec.Command("go", "run", "-mod=mod", ".")
	command.Dir = config.OutputPath()
	data, err := command.CombinedOutput()
	if err != nil || string(data) != "busy:busy\n" {
		t.Fatalf("tree: %v\n%s", err, data)
	}
	stdout.Reset()
	stderr.Reset()
	if status := cli.Run([]string{"build", "--config", filepath.Join(root, project.ConfigName), "--stdout", entry}); status == 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "native support files") {
		t.Fatalf("stdout: %d\n%s\n%s", status, &stdout, &stderr)
	}
}
