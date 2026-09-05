package compiler

import (
	"strings"
	"testing"
)

func TestPathTypedFilesystemSignaturesAndTargets(t *testing.T) {
	source := scopedFilesystemSource + `
def make_tree(path: Path): Result<Unit, FileSystemError>
	return Dir.create_all(path)
end
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: []byte(source)}}, Options{Mode: mode, GoModule: "example.com/filesystem-values"})
			if err != nil {
				t.Fatal(err)
			}
			if mode == "typescript" {
				checkTypeScriptArtifacts(t, artifacts, "path_typed_filesystem")
			}
		})
	}
}

func TestFilesystemRejectsUntypedPathsAndMissingBounds(t *testing.T) {
	for _, test := range []struct{ source, want string }{
		{`import trb/std/dir
def main()
	value := Dir.children(".", max_entries: 1)
	puts(value)
end`, "expected Path"},
		{`import trb/std/dir
def main()
	value := Dir.create_all(".")
	puts(value)
end`, "expected Path"},
		{`import trb/std/dir
import trb/std/path
def main()
	value := Dir.children(Path.new("."))
	puts(value)
end`, "max_entries"},
	} {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			_, err := Compile("main.trb", []byte(test.source), mode)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("%s: got %v, want %q", mode, err, test.want)
			}
		}
	}
}

func TestHostFilesystemUnavailableInBrowser(t *testing.T) {
	for name, source := range map[string]string{
		"create_all": "import trb/std/dir\nimport trb/std/path\ndef main()\nvalue := Dir.create_all(Path.new(\".\"))\nputs(value)\nend\n",
		"children":   "import trb/std/dir\nimport trb/std/path\ndef main()\nvalue := Dir.children(Path.new(\".\"), max_entries: 1)\nputs(value)\nend\n",
		"open":       "import trb/std/file\nimport trb/std/path\ndef main()\nvalue := File.open(Path.new(\".\")) do |_file|\n1\nend\nputs(value)\nend\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := CompileProject([]SourceUnit{{Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: []byte(source)}}, Options{Mode: "typescript", TypeScriptRuntime: "browser"})
			if err == nil || !strings.Contains(err.Error(), "host filesystem operation requires a Node or Bun host") {
				t.Fatalf("got %v", err)
			}
		})
	}
}
