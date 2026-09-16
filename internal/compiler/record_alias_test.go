package compiler

import (
	"strings"
	"testing"
)

func TestRecordAliasConstructionAcrossModes(t *testing.T) {
	source := []byte(`record Box<T>
 value: T
 copy: T = value
end
alias Holder<T> = Box<T>
alias TextBox = Holder<String>
def main()
 box := Holder<Integer>.new(value: 7)
 puts(box.copy.to_s())
 puts(TextBox.new(value: "held").copy)
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifact, err := Compile("record_alias.trb", source, mode)
			if err != nil {
				t.Fatal(err)
			}
			construction := recordConstructionInMain(t, artifact)
			if construction.Declaration.Name != "Box" || len(construction.TypeArguments) != 1 || construction.TypeArguments[0].Name != "Integer" {
				t.Fatalf("alias target lost canonical identity: %#v", construction)
			}
			runEffectSource(t, mode, "record_alias.trb", source, "7\nheld")
		})
	}
}

func TestRecordAliasConstructionChecksFields(t *testing.T) {
	prefix := "record Box<T>\nvalue: T\nend\nalias TextBox = Box<String>\n"
	for _, test := range []struct{ body, message string }{
		{"TextBox.new()", "missing record field value"},
		{"TextBox.new(value: 7)", "expected String"},
		{"TextBox.new(extra: 7)", "has no field extra"},
		{"TextBox.new(7)", "keyword-only"},
		{"TextBox.new(value: \"held\") { |item| puts(item) }", "call blocks require"},
	} {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			_, err := Compile("invalid_alias.trb", []byte(prefix+"def main()\n"+test.body+"\nend\n"), mode)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("%s %s: error=%v, want %s", mode, test.body, err, test.message)
			}
		}
	}
}

func TestImportedRecordAliasConstructionAcrossModes(t *testing.T) {
	units := []SourceUnit{
		{Filename: "models/box.trb", ModulePath: "models/box", Package: "models", Source: []byte("record Box<T>\nvalue: T\ncopy: T = value\nend\nalias Holder<T> = Box<T>\n")},
		{Filename: "main.trb", ModulePath: "main", Package: "main", Source: []byte("import { Holder as Container } from models/box\ndef main()\nbox := Container<String>.new(value: \"held\")\nputs(box.copy)\nend\n")},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := CompileProject(units, Options{Mode: mode, GoModule: "example.com/aliases", RubyLoader: "require_relative", SourceRoot: "/project", ProjectRoot: "/project"})
			if err != nil {
				t.Fatal(err)
			}
			construction := recordConstructionInMain(t, artifactForModule(artifacts, "main"))
			if construction.Declaration.Name != "Box" || construction.Declaration.Module != "models/box" {
				t.Fatalf("alias target identity: %#v", construction)
			}
			requireEffectRuntime(t, mode)
			if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, "example.com/aliases")); got != "held" {
				t.Fatalf("output %q", got)
			}
		})
	}
}
