package compiler

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

const relativePathTestHelpers = `import { Path, RelativePath, RelativePathError } from trb/std/path
import trb/std/result

def error_name(error: RelativePathError): String
	case error
	when RelativePathError::Empty
		return "Empty"
	when RelativePathError::EmptyComponent
		return "EmptyComponent"
	when RelativePathError::DotComponent
		return "DotComponent"
	when RelativePathError::InvalidCharacter
		return "InvalidCharacter"
	when RelativePathError::TrailingDotOrSpace
		return "TrailingDotOrSpace"
	when RelativePathError::ReservedName
		return "ReservedName"
	when RelativePathError::MultipleComponents
		return "MultipleComponents"
	end
end

def describe(source: String): String
	case RelativePath.parse(source)
	when Result::Ok(path)
		return "ok:" + path.to_s()
	when Result::Err(error)
		return error_name(error)
	end
end
`

func TestRelativePathValidationAcrossBackends(t *testing.T) {
	valid := []string{
		"a", "a/b.txt", ".config", "..name", "dir/.hidden", "a..b",
		" leading/name", "CONsole", "CONIN", "COM10", "COM⁴", "cоn", "conın$",
		"日本語/😀.md", "e\u0301/é", "a\u0085", "a\u007f", "a/ b", ".CON",
		strings.Repeat("a/", 256) + "leaf",
	}
	invalid := []struct{ input, kind string }{
		{"", "Empty"}, {"/a", "EmptyComponent"}, {"a/", "EmptyComponent"}, {"a//b", "EmptyComponent"},
		{".", "DotComponent"}, {"..", "DotComponent"}, {"a/../b", "DotComponent"}, {"a/./b", "DotComponent"},
		{"C:/a", "InvalidCharacter"}, {`a\b`, "InvalidCharacter"}, {`\\server\share`, "InvalidCharacter"},
		{"a:b", "InvalidCharacter"}, {"a?b", "InvalidCharacter"}, {"a*b", "InvalidCharacter"},
		{"a<b", "InvalidCharacter"}, {"a>b", "InvalidCharacter"}, {"a|b", "InvalidCharacter"}, {"a\"b", "InvalidCharacter"},
		{"a.", "TrailingDotOrSpace"}, {"a ", "TrailingDotOrSpace"}, {"a /b", "TrailingDotOrSpace"},
		{"CON", "ReservedName"}, {"con.txt", "ReservedName"}, {"PrN", "ReservedName"}, {"aux.tar.gz", "ReservedName"},
		{"NUL", "ReservedName"}, {"CONIN$", "ReservedName"}, {"conout$.txt", "ReservedName"},
		{"CON .txt", "ReservedName"}, {"con..txt", "ReservedName"}, {"dir/lpt¹.log", "ReservedName"},
		{"CON" + strings.Repeat(" ", 1024) + ".txt", "ReservedName"},
	}
	for point := 0; point <= 31; point++ {
		invalid = append(invalid, struct{ input, kind string }{"a" + string(rune(point)) + "b", "InvalidCharacter"})
	}
	for _, prefix := range []string{"COM", "lpt"} {
		for _, suffix := range []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "¹", "²", "³"} {
			invalid = append(invalid, struct{ input, kind string }{prefix + suffix + ".txt", "ReservedName"})
		}
	}
	var source strings.Builder
	source.WriteString(relativePathTestHelpers)
	source.WriteString("def main()\nputs(Path.new(\"\").to_s() == \"\")\n")
	want := []string{"true"}
	for _, input := range valid {
		literal, _ := json.Marshal(input)
		source.WriteString("puts(describe(" + string(literal) + "))\n")
		want = append(want, "ok:"+input)
	}
	for _, fixture := range invalid {
		literal, _ := json.Marshal(fixture.input)
		source.WriteString("puts(describe(" + string(literal) + "))\n")
		want = append(want, fixture.kind)
	}
	source.WriteString("end\n")
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts := compilePathSource(t, mode, source.String(), "")
			t.Run("runtime", func(t *testing.T) {
				requireEffectRuntime(t, mode)
				if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, "example.com/paths")); got != strings.Join(want, "\n") {
					t.Fatalf("path validation:\n%s\nwant:\n%s", got, strings.Join(want, "\n"))
				}
			})
			if mode == "typescript" {
				t.Run("typecheck", func(t *testing.T) { checkTypeScriptArtifacts(t, artifacts, "path_validation") })
			}
		})
	}
}

func TestPathValueCompositionAcrossBackends(t *testing.T) {
	source := relativePathTestHelpers + `def parent(present: Boolean): Path?
	puts("parent")
	if present
		return Path.new("//root/link/../")
	end
	return nil
end
def child(path: RelativePath): RelativePath
	puts("child")
	return path
end
def main()
	base := RelativePath.parse("日本語") catch |_error|
		return
	end
	nested := RelativePath.parse("docs/a.md") catch |_error|
		return
	end
	joined := base.join(nested)
	puts(joined.to_s())
	puts(joined.parent()&.to_s())
	puts(base.parent() == nil)
	case base.child("docs/a.md")
	when Result::Ok(_path)
		puts("unexpected")
	when Result::Err(error)
		puts(error_name(error))
	end
	leaf := base.child("leaf.md") catch |_error|
		return
	end
	puts(leaf.to_s())
	actual_parent := leaf.parent()
	if actual_parent != nil
		puts(actual_parent == base)
	else
		puts(false)
	end
	puts(parent(false)&.join(child(nested)) == nil)
	present := parent(true)&.join(child(nested))
	if present != nil
		puts(present.to_s())
	end
	puts(Path.new("../raw//").to_s())
end
`
	hostJoined := "//root/link/../docs/a.md"
	if runtime.GOOS == "windows" {
		hostJoined = `//root/link/../docs\a.md`
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts := compilePathSource(t, mode, source, "")
			t.Run("runtime", func(t *testing.T) {
				requireEffectRuntime(t, mode)
				want := "日本語/docs/a.md\n日本語/docs\ntrue\nMultipleComponents\n日本語/leaf.md\ntrue\nparent\ntrue\nparent\nchild\n" + hostJoined + "\n../raw//"
				if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, "example.com/paths")); got != want {
					t.Fatalf("path composition: %q, want %q", got, want)
				}
			})
			if mode == "typescript" {
				t.Run("typecheck", func(t *testing.T) { checkTypeScriptArtifacts(t, artifacts, "path_composition") })
			}
		})
	}
}

func TestPathValueDiagnosticsAcrossBackends(t *testing.T) {
	for _, fixture := range []struct{ name, source, diagnostic string }{
		{"closed construction", "import { RelativePath } from trb/std/path\ndef main()\nx := RelativePath.new(\"../x\")\nputs(x.to_s())\nend", "raw constructor"},
		{"unvalidated child", "import trb/std/path\ndef main()\nputs(Path.new(\"a\").join(\"b\").to_s())\nend", "RelativePath"},
		{"missing child", "import trb/std/path\ndef main()\nputs(Path.new(\"a\").join().to_s())\nend", "argument"},
		{"extra child", "import { Path, RelativePath } from trb/std/path\ndef main()\nchild := RelativePath.parse(\"a\") catch |_error|\nreturn\nend\nputs(Path.new(\"a\").join(child, child).to_s())\nend", "argument"},
		{"unrelated nominal receiver", "import { RelativePath } from trb/std/path\nnewtype Path = String\ndef main()\nchild := RelativePath.parse(\"a\") catch |_error|\nreturn\nend\nputs(Path.new(\"a\").join(child))\nend", "join"},
		{"no static mirror", "import trb/std/path\ndef main()\nputs(Path.join(\"a\", \"b\"))\nend", "join"},
		{"private validation helper", "import { RelativePath } from trb/std/path\ndef main()\nputs(RelativePath._reserved_name(\"CON\"))\nend", "private"},
		{"no implicit inbound", "import { RelativePath } from trb/std/path\nimport trb/std/json\ndef main()\nvalue := JSON.decode<RelativePath>(\"x\")\nputs(value)\nend", "cannot construct closed newtype"},
		{"must use", "import { RelativePath } from trb/std/path\ndef main()\nRelativePath.parse(\"a\")\nend", "Result"},
	} {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			t.Run(fixture.name+"/"+mode, func(t *testing.T) {
				_, err := CompileProject([]SourceUnit{{Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: []byte(fixture.source)}}, Options{Mode: mode, GoModule: "example.com/paths", RubyLoader: "require_relative"})
				if err == nil || !strings.Contains(err.Error(), fixture.diagnostic) {
					t.Fatalf("wanted %q, got %v", fixture.diagnostic, err)
				}
			})
		}
	}
}

func TestPathValuesRetainIdentityAcrossModuleAndAliasBoundaries(t *testing.T) {
	const definitions = `import { Path as HostPath, RelativePath } from trb/std/path

def make_root(): HostPath
	return HostPath.new("root")
end
def make_child(): RelativePath?
	path := RelativePath.parse("docs/a.md") catch |_error|
		return nil
	end
	return path
end
def render(path: HostPath): String
	return path.to_s()
end
`
	const main = `import { make_root, make_child, render } from paths
newtype Path = String

def main()
	parent := make_root()
	child := make_child()
	if child != nil
		puts(parent.join(child).to_s())
	end
	puts(render(parent))
	puts(Path.new("local").value())
end
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := CompileProject([]SourceUnit{
				{Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: []byte(main)},
				{Filename: "/project/paths/index.trb", ModulePath: "paths/index", Package: "paths", Source: []byte(definitions)},
			}, Options{Mode: mode, GoModule: "example.com/paths", RubyLoader: "require_relative", SourceRoot: "/project", ProjectRoot: "/project"})
			if err != nil {
				t.Fatal(err)
			}
			t.Run("runtime", func(t *testing.T) {
				requireEffectRuntime(t, mode)
				want := "root/docs/a.md\nroot\nlocal"
				if runtime.GOOS == "windows" {
					want = "root\\docs\\a.md\nroot\nlocal"
				}
				if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, "example.com/paths")); got != want {
					t.Fatalf("inferred path result = %q, want %q", got, want)
				}
			})
			if mode == "typescript" {
				t.Run("typecheck", func(t *testing.T) { checkTypeScriptArtifacts(t, artifacts, "inferred_paths") })
			}
		})
	}
}

func TestPathValuesUseExplicitInputConversionAndOutboundProjection(t *testing.T) {
	const source = `import { RelativePath } from trb/std/path
import trb/std/json

record Input
	path: String
end

def main()
	input := JSON.decode<Input>("{\"path\":\"reports/a.md\"}") catch |_error|
		return
	end
	path := RelativePath.parse(input.path) catch |_error|
		return
	end
	encoded := JSON.encode(path) catch |_error|
		return
	end
	puts(encoded)
	round_trip := RelativePath.parse(path.to_s()) catch |_error|
		return
	end
	puts(round_trip == path)
end
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts := compilePathSource(t, mode, source, "")
			t.Run("runtime", func(t *testing.T) {
				requireEffectRuntime(t, mode)
				if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, "example.com/paths")); got != "\"reports/a.md\"\ntrue" {
					t.Fatal(got)
				}
			})
			if mode == "typescript" {
				t.Run("typecheck", func(t *testing.T) { checkTypeScriptArtifacts(t, artifacts, "explicit_path_input") })
			}
		})
	}
}

func TestPathValueIdentitySurvivesIncrementalRecheck(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			analyzer := NewAnalyzer()
			units := []SourceUnit{
				{Filename: "/project/paths/index.trb", ModulePath: "paths/index", Package: "paths", Source: []byte("import { Path as HostPath } from trb/std/path\ndef root(): HostPath\nreturn HostPath.new(\"root\")\nend\n")},
				{Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: []byte("import { root } from paths\ndef main()\nputs(root().to_s())\nend\n")},
			}
			options := Options{Mode: mode, GoModule: "example.com/paths", RubyLoader: "require_relative", ProjectRoot: "/project", SourceRoot: "/project"}
			if _, err := analyzer.AnalyzeProject(units, options); err != nil {
				t.Fatal(err)
			}
			units[1].Source = []byte("import { root } from paths\ndef main()\npath := root()\nputs(path.to_s())\nend\n")
			if _, err := analyzer.AnalyzeProject(units, options); err != nil {
				t.Fatalf("incremental recheck: %v", err)
			}
		})
	}
}

func TestRelativePathAvailableInBrowserWithoutHostOperations(t *testing.T) {
	source := relativePathTestHelpers + "def main()\nputs(describe(\"docs/a\"))\nputs(Path.new(\"anything\").to_s())\nend\n"
	artifacts := compilePathSource(t, "typescript", source, "browser")
	t.Run("typecheck", func(t *testing.T) { checkTypeScriptArtifacts(t, artifacts, "browser_paths") })
	for _, artifact := range artifacts {
		if strings.Contains(string(artifact.Output), "globalThis as { process") {
			t.Fatal("pure path module contains host path operations")
		}
	}
	source = "import { Path, RelativePath } from trb/std/path\ndef main()\nrelative := RelativePath.parse(\"a\") catch |_error|\nreturn\nend\nputs(Path.new(\"b\").join(relative).to_s())\nend\n"
	_, err := CompileProject([]SourceUnit{{Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: []byte(source)}}, Options{Mode: "typescript", TypeScriptRuntime: "browser"})
	if err == nil || !strings.Contains(err.Error(), "Path#join requires a Node or Bun host") {
		t.Fatalf("browser host-operation diagnostic = %v", err)
	}
}

func compilePathSource(t *testing.T, mode, source, runtime string) []*Artifact {
	t.Helper()
	artifacts, err := CompileProject([]SourceUnit{{Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: []byte(source)}}, Options{
		Mode: mode, TypeScriptRuntime: runtime, GoModule: "example.com/paths", RubyLoader: "require_relative", SourceRoot: "/project", ProjectRoot: "/project",
	})
	if err != nil {
		t.Fatal(err)
	}
	return artifacts
}
