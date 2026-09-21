package compiler

import (
	"strings"
	"testing"
)

func TestNamespaceImportsPreserveNestedDeclarationIdentity(t *testing.T) {
	const library = `module Library
  BASE := SEED
  module Inner
    VALUE := SEED + 1
    def self.read(): Integer
      return VALUE
    end
  end
  record Entry
    value: Integer = BASE
  end
  enum Code
    Ready = SEED
  end
  alias CodeAlias = Code
  enum Packet
    Value(value: Integer)
  end
  class Item
    @value: Integer := BASE
  end
  def self._read(): Integer
    return BASE
  end
  def self.read(): Integer
    return Library._read() + Inner.read()
  end
end
module Library
  module Inner
    def self.again(): Integer
      return VALUE
    end
  end
  def self.again(): Integer
    return BASE
  end
  def self.decode(): Integer
    code := Code.from_raw(BASE) catch |_error|
      return -1
    end
    return code.raw_value()
  end
  def self.payload(): Integer
    packet := Packet::Value(BASE)
    return case packet
    when Packet::Value(value)
      value
    end
  end
  def self.aliased(): Integer
    return CodeAlias::Ready.raw_value()
  end
end
`
	const main = `import left/library as Left
import { Library as Right } from right/library
def main()
  puts(Left.read())
  puts(Right.read())
  puts(Left.again())
  puts(Right.again())
  puts(Left::Inner::VALUE)
  puts(Right::Inner.read())
  puts(Left::Entry.new().value)
  puts(Right::Entry.new().value)
  puts(Left::Code::Ready.raw_value())
  puts(Right::Code::Ready.raw_value())
  puts(Left::Item.new().value)
  puts(Right::Item.new().value)
  puts(Left::Inner.again())
  puts(Right::Inner.again())
  puts(Left.decode())
  puts(Right.decode())
  puts(Left.payload())
  puts(Right.payload())
  puts(Left.aliased())
  puts(Right.aliased())
end
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireEffectRuntime(t, mode)
			units := []SourceUnit{
				{Filename: "/project/left/library.trb", ModulePath: "left/library", Package: "left", Source: []byte(strings.ReplaceAll(library, "SEED", "2"))},
				{Filename: "/project/right/library.trb", ModulePath: "right/library", Package: "right", Source: []byte(strings.ReplaceAll(library, "SEED", "20"))},
				{Filename: "/project/main.trb", ModulePath: "main", Source: []byte(main)},
			}
			options := Options{Mode: mode, GoModule: "example.com/namespace-identity", RubyLoader: "require_relative", ProjectRoot: "/project", SourceRoot: "/project"}
			artifacts, err := CompileProject(units, options)
			if err != nil {
				t.Fatal(err)
			}
			want := "5\n41\n2\n20\n3\n21\n2\n20\n2\n20\n2\n20\n3\n21\n2\n20\n2\n20\n2\n20"
			if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, options.GoModule)); got != want {
				t.Fatalf("nested namespace identity: got %q, want %q", got, want)
			}
			if mode == "typescript" {
				checkTypeScriptArtifacts(t, artifacts, "namespace_identity")
			}
		})
	}
}

func TestRubyNamespaceDoesNotReopenAnotherSourceClass(t *testing.T) {
	requireEffectRuntime(t, "ruby")
	units := []SourceUnit{
		{Filename: "/project/namespace.trb", ModulePath: "namespace", Source: []byte("module Store\nVALUE := 4\ndef self.read(): Integer\nreturn VALUE\nend\nend\n")},
		{Filename: "/project/model.trb", ModulePath: "model", Source: []byte("class Store\n@value: Integer := 9\nend\n")},
		{Filename: "/project/main.trb", ModulePath: "main", Source: []byte("import { Store as Namespace } from namespace\nimport { Store as Model } from model\ndef main()\nputs(Namespace.read())\nputs(Model.new().value)\nend\n")},
	}
	options := Options{Mode: "ruby", RubyLoader: "require_relative", ProjectRoot: "/project", SourceRoot: "/project"}
	for _, reversed := range []bool{false, true} {
		if reversed {
			units[0], units[1] = units[1], units[0]
			units[2].Source = []byte("import { Store as Model } from model\nimport { Store as Namespace } from namespace\ndef main()\nputs(Namespace.read())\nputs(Model.new().value)\nend\n")
		}
		artifacts, err := CompileProject(units, options)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.TrimSpace(runEffectProject(t, "ruby", artifacts, "")); got != "4\n9" {
			t.Fatalf("namespace/class identity (reversed=%v): %q", reversed, got)
		}
	}
}
