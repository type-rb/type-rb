package compiler

import (
	"strings"
	"testing"
)

func TestClassAliasConstructionRunsAcrossBackends(t *testing.T) {
	source := []byte(`class Box
	@value: Integer

	def initialize(value: Integer)
		@value = value
		return
	end

	def self.constant(): Integer
		return 11
	end
end

alias Label = Box

def main()
	puts(Label.new(3).value)
	puts(Label.constant())
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "class_alias_construction.trb", source, "3\n11")
		})
	}
}

func TestGenericClassAliasConstructionRunsAcrossBackends(t *testing.T) {
	source := []byte(`class Box<T>
	@value: T

	def initialize(value: T)
		@value = value
		return
	end
end

alias Wrapped<T> = Box<T>
alias NumberBox = Wrapped<Integer>

def main()
	puts(Wrapped<Integer>.new(7).value)
	puts(NumberBox.new(8).value)
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "generic_class_alias_construction.trb", source, "7\n8")
		})
	}
}

func TestImportedClassAliasConstructionRunsAcrossBackends(t *testing.T) {
	units := []SourceUnit{
		{Filename: "models/box.trb", ModulePath: "models/box", Package: "models", Source: []byte(`class Box<T>
	@value: T

	def initialize(value: T)
		@value = value
		return
	end

end

alias Wrapped<T> = Box<T>

class Marker
	def self.constant(): Integer
		return 12
	end
end

alias Tag = Marker
`)},
		{Filename: "main.trb", ModulePath: "main", Package: "main", Source: []byte(`import { Wrapped as ImportedBox, Tag } from models/box

def main()
	puts(ImportedBox<Integer>.new(9).value)
	puts(Tag.constant())
end
`)},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireEffectRuntime(t, mode)
			options := Options{Mode: mode, GoModule: "example.com/class-alias", RubyLoader: "require_relative", SourceRoot: "/project", ProjectRoot: "/project"}
			artifacts, err := CompileProject(units, options)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, options.GoModule)); got != "9\n12" {
				t.Fatalf("imported class alias output = %q", got)
			}
		})
	}
}

func TestNamespaceClassAliasConstructionRunsAcrossBackends(t *testing.T) {
	source := []byte(`module Models
	class Box
		@value: Integer

		def initialize(value: Integer)
			@value = value
			return
		end
	end

	alias Wrapped = Box
end

def main()
	puts(Models::Wrapped.new(13).value)
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "namespace_class_alias.trb", source, "13")
		})
	}
}

func TestClassAliasConstructionChecksArgumentsAcrossBackends(t *testing.T) {
	prelude := `class Box
	def initialize(value: Integer)
		return
	end
end
alias Wrapped = Box
def main()
`
	for _, test := range []struct{ name, expression, diagnostic string }{
		{"wrong type", `Wrapped.new("wrong")`, "has type String, expected Integer"},
		{"missing argument", `Wrapped.new()`, "missing required argument 1"},
		{"extra argument", `Wrapped.new(1, 2)`, "does not accept this positional argument"},
	} {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			t.Run(test.name+"/"+mode, func(t *testing.T) {
				unit := SourceUnit{Filename: "main.trb", ModulePath: "main", Package: "main", Source: []byte(prelude + "\t" + test.expression + "\nend\n")}
				_, err := CompileProject([]SourceUnit{unit}, Options{Mode: mode})
				if err == nil || !strings.Contains(err.Error(), test.diagnostic) {
					t.Fatalf("CompileProject() = %v, want %q", err, test.diagnostic)
				}
			})
		}
	}
}

func TestGenericClassAliasConstructionChecksSubstitutedArguments(t *testing.T) {
	source := []byte(`class Box<T>
	def initialize(value: T)
		return
	end
end
alias Wrapped<T> = Box<T>
def main()
	Wrapped<Integer>.new("wrong")
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			_, err := Compile("generic_class_alias_arguments.trb", source, mode)
			if err == nil || !strings.Contains(err.Error(), "has type String, expected Integer") {
				t.Fatalf("Compile() = %v, want substituted constructor argument diagnostic", err)
			}
		})
	}
}
