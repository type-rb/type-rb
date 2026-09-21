package compiler

import (
	"strings"
	"testing"
)

func TestNamespaceBindingsKeepDistinctSharedStorage(t *testing.T) {
	source := []byte(`module Counter
  mut value := 1
  def self.bump(): Integer
    value += 1
    return value
  end
  def self.shadow(): Integer
    saved := fn(): Integer; return value; end
    value := 20
    return saved() + value
  end
  module Inner
    mut value := 10
    def self.bump(): Integer
      value += 1
      return value
    end
  end
end
module Other
  value := 30
  def self.read(): Integer
    return value
  end
end
module Counter
  def self.read(): Integer
    return value
  end
end
def main()
  puts(Counter.bump())
  puts(Counter::Inner.bump())
  puts(Other.read())
  puts(Counter.shadow())
  puts(Counter.read())
  puts(Counter.bump())
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "namespace_bindings.trb", source, "2\n11\n30\n22\n2\n3")
		})
	}
}

func TestNamespaceMutableBindingsInvalidateNullableFacts(t *testing.T) {
	for _, body := range []string{
		"if value != nil\nState.clear()\nputs(value + 1)\nend\n",
		"call := fn(); State.clear(); end\nif value != nil\ncall()\nputs(value + 1)\nend\n",
	} {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			source := []byte("module State\nmut value: Integer? := 1\ndef self.clear()\nvalue = nil\nend\ndef self.run()\n" + body + "end\nend\ndef main()\nState.run()\nend\n")
			if _, err := Compile("namespace_nullable.trb", source, mode); err == nil || !strings.Contains(err.Error(), "operator + does not support Integer? and Integer") {
				t.Fatalf("%s should reject stale namespace proof: %v", mode, err)
			}
		}
	}
}

func TestNamespaceBindingsKeepSourceModuleIdentity(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireEffectRuntime(t, mode)
			units := []SourceUnit{
				{Filename: "/project/left.trb", ModulePath: "left", Source: []byte("module Counter\nmut value := 1\ndef self.bump(): Integer\nvalue += 1\nreturn value\nend\nend\n")},
				{Filename: "/project/right.trb", ModulePath: "right", Source: []byte("module Counter\nmut value := 10\ndef self.bump(): Integer\nvalue += 1\nreturn value\nend\nend\n")},
				{Filename: "/project/main.trb", ModulePath: "main", Source: []byte("import { Counter as Left } from left\nimport { Counter as Right } from right\ndef main()\nputs(Left.bump())\nputs(Right.bump())\nputs(Left.bump())\nend\n")},
			}
			options := Options{Mode: mode, GoModule: "example.com/namespaces", RubyLoader: "require_relative", ProjectRoot: "/project", SourceRoot: "/project"}
			artifacts, err := CompileProject(units, options)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, options.GoModule)); got != "2\n11\n3" {
				t.Fatalf("namespace module identity: %q", got)
			}
		})
	}
}

func TestNamespaceNullableBindingsPermitFreshGuardsAndLocalShadowing(t *testing.T) {
	source := []byte(`module State
  mut value: Integer? := 1
  fixed: Integer? := 2
  def self.clear()
    value = nil
  end
  def self.local(): Integer
    mut value: Integer? := 3
    if value != nil
      State.clear()
      return value + 1
    end
    return 0
  end
  def self.read(): Integer
    State.clear()
    if value != nil
      return value
    end
    if fixed != nil
      State.clear()
      return fixed
    end
    return 0
  end
end
def main()
  puts(State.local())
  puts(State.read())
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "namespace_fresh_guards.trb", source, "4\n2")
		})
	}
}
