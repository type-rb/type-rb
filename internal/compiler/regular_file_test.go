package compiler

import (
	"strings"
	"testing"
)

func TestRegularFileValidationPrecedesTruncationAndBody(t *testing.T) {
	source := `import trb/std/path
import trb/std/file
import { FileMode } from trb/std/file
import { FileSystemError } from trb/std/errors
import trb/std/result
def opened(path: Path): Result<String, FileSystemError>
	return File.open(path, mode: FileMode::Write) do |_file|
		"validated-body"
	end
end
`
	for _, test := range []struct {
		mode      string
		ordered   []string
		forbidden []string
	}{
		{"go", []string{"regular-file acquisition is unavailable", "syscall.O_NONBLOCK | syscall.O_NOCTTY", "os.OpenFile(", "defer func()", ".Stat()", ".Mode().IsRegular()", ".Truncate(0)", "validated-body"}, []string{"os.O_TRUNC", "os.Stat("}},
		{"ruby", []string{"regular-file acquisition is unavailable", "::File::NONBLOCK | ::File::NOCTTY", "::File.open(", ".stat.file?", ".truncate(0)", "validated-body"}, []string{"::File::TRUNC", "::File.stat("}},
		{"typescript", []string{"regular-file acquisition is unavailable", ".constants.O_NONBLOCK |", ".openSync(", ".fstatSync(", ".isFile()", ".ftruncateSync(", "validated-body"}, []string{".constants.O_TRUNC", ".statSync("}},
	} {
		t.Run(test.mode, func(t *testing.T) {
			artifact, err := Compile("main.trb", []byte(source), test.mode)
			if err != nil {
				t.Fatal(err)
			}
			output := string(artifact.Output)
			remaining := output
			for _, fragment := range test.ordered {
				index := strings.Index(remaining, fragment)
				if index < 0 {
					t.Fatalf("missing or out-of-order acquisition step %q:\n%s", fragment, output)
				}
				remaining = remaining[index+len(fragment):]
			}
			for _, fragment := range test.forbidden {
				if strings.Contains(output, fragment) {
					t.Errorf("unsafe pre-validation operation %q", fragment)
				}
			}
		})
	}
}
