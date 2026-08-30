package compiler

import (
	"os/exec"
	"strings"
	"testing"
)

const constructorEffectPrelude = `def map_values(values: Array<Integer>): Array<Integer>
	return values.concurrent_map do |value|
		value
	end
end
`

func TestEffectfulConstructorParameterDefaultRunsInSynchronousBackends(t *testing.T) {
	source := []byte(constructorEffectPrelude + `
class Box
	@values: Array<Integer>

	def initialize(values: Array<Integer> = map_values([7]))
		@values = values
		return
	end

	def first(): Integer
		return @values[0]
	end
end

def main()
	puts(Box.new().first().to_s())
	return
end
`)
	assertConstructorEffectProgramOutput(t, source, "7")
}

func TestEffectfulConstructorBodyRunsInSynchronousBackends(t *testing.T) {
	source := []byte(constructorEffectPrelude + `
class Box
	@values: Array<Integer>

	def initialize(values: Array<Integer>)
		@values = map_values(values)
		return
	end

	def first(): Integer
		return @values[0]
	end
end

def main()
	puts(Box.new([8]).first().to_s())
	return
end
`)
	assertConstructorEffectProgramOutput(t, source, "8")
}

func TestImportedClassConstructorUsesItsInitializerEffectABI(t *testing.T) {
	model := SourceUnit{
		Filename: "box.trb", ModulePath: "models/box", Package: "models",
		Source: []byte(constructorEffectPrelude + `
class Box
	@values: Array<Integer>

	def initialize(values: Array<Integer> = map_values([7]))
		@values = values
		return
	end
end
`),
	}
	main := SourceUnit{
		Filename: "main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import { Box } from models/box

def main()
	Box.new()
	return
end
`),
	}
	wants := map[string]string{
		"go":   "NewBox(__trbScope)",
		"ruby": "Box.new(__trb_scope)",
	}
	for _, mode := range []string{"go", "ruby"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := CompileProject([]SourceUnit{model, main}, Options{
				Mode: mode, GoModule: "example.com/constructor-effect", RubyLoader: "require_relative",
				SourceRoot: "/project", ProjectRoot: "/project",
			})
			if err != nil {
				t.Fatal(err)
			}
			output := string(artifactForModule(artifacts, "main").Output)
			if !strings.Contains(output, wants[mode]) {
				t.Fatalf("generated %s imported constructor call is missing %q:\n%s", mode, wants[mode], output)
			}
		})
	}
}

func TestEffectfulParentConstructorDoesNotChangeChildConstructorABI(t *testing.T) {
	source := []byte(constructorEffectPrelude + `
class EffectParent
	@parent_values: Array<Integer>

	def initialize(values: Array<Integer> = map_values([1]))
		@parent_values = values
		return
	end
end

class PureChild < EffectParent
	@values: Array<Integer>

	def initialize(values: Array<Integer> = [2])
		@values = values
		return
	end

	def first(): Integer
		return @values[0]
	end
end

def main()
	puts(PureChild.new().first().to_s())
	return
end
`)
	assertConstructorEffectProgramOutput(t, source, "2")
}

func TestTypeScriptRejectsEffectfulConstructorBody(t *testing.T) {
	_, err := Compile("constructor_effect.trb", []byte(constructorEffectPrelude+`
class Box
	@values: Array<Integer>

	def initialize(values: Array<Integer>)
		@values = map_values(values)
		return
	end
end
`), "typescript")
	want := "TypeScript class initializer Box#initialize cannot use an operation that may suspend"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("Compile() error=%v, want %q", err, want)
	}
}

func TestEffectfulClassFieldDefaultRunsInSynchronousBackends(t *testing.T) {
	for _, test := range []struct {
		name        string
		initializer string
	}{
		{name: "explicit initializer", initializer: "\n\tdef initialize()\n\t\treturn\n\tend\n"},
		{name: "implicit initializer"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := []byte(constructorEffectPrelude + `
class Box
	@values: Array<Integer> := map_values([10])
` + test.initializer + `
	def first(): Integer
		return @values[0]
	end
end

def main()
	puts(Box.new().first().to_s())
	return
end
`)
			assertConstructorEffectProgramOutput(t, source, "10")
		})
	}
}

func TestClassFieldDefaultsUseTheConstructorExecutionScope(t *testing.T) {
	tests := []struct {
		name        string
		initializer string
		rubyHeader  string
	}{
		{name: "explicit initializer", initializer: "\n\tdef initialize()\n\t\treturn\n\tend\n", rubyHeader: "def initialize(__trb_scope)"},
		{name: "implicit initializer", rubyHeader: "def initialize(__trb_scope, ...)"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := []byte(constructorEffectPrelude + `
class Box
	@values: Array<Integer> := map_values([10])
` + test.initializer + `end

def make_box(): Box
	return Box.new()
end
`)
			wants := map[string][]string{
				"go": {
					"func NewBox(__trbScope trbcontext.Context) *Box",
					"self.TrbFieldValues = MapValues(__trbScope, trbArrayReference_",
					"func MakeBox(__trbScope trbcontext.Context) *Box",
					"return NewBox(__trbScope)",
				},
				"ruby": {
					test.rubyHeader,
					"@values = map_values(__trb_scope, [10])",
					"def make_box(__trb_scope)",
					"return Box.new(__trb_scope)",
				},
			}
			for _, mode := range []string{"go", "ruby"} {
				t.Run(mode, func(t *testing.T) {
					artifact, err := Compile("constructor_field_effect.trb", source, mode)
					if err != nil {
						t.Fatal(err)
					}
					output := string(artifact.Output)
					for _, want := range wants[mode] {
						if !strings.Contains(output, want) {
							t.Fatalf("generated %s constructor is missing %q:\n%s", mode, want, output)
						}
					}
				})
			}
		})
	}
}

func TestImportedClassFieldDefaultUsesTheConstructorExecutionScope(t *testing.T) {
	model := SourceUnit{
		Filename: "box.trb", ModulePath: "models/box", Package: "models",
		Source: []byte(constructorEffectPrelude + `
class Box
	@values: Array<Integer> := map_values([10])
end
`),
	}
	main := SourceUnit{
		Filename: "main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import { Box } from models/box

def make_box(): Box
	return Box.new()
end
`),
	}
	wants := map[string][]string{
		"go": {
			"func MakeBox(__trbScope trbcontext.Context) *models.Box",
			"return models.NewBox(__trbScope)",
		},
		"ruby": {
			"def make_box(__trb_scope)",
			"return Box.new(__trb_scope)",
		},
	}
	for _, mode := range []string{"go", "ruby"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := CompileProject([]SourceUnit{model, main}, Options{
				Mode: mode, GoModule: "example.com/constructor-field-effect", RubyLoader: "require_relative",
				SourceRoot: "/project", ProjectRoot: "/project",
			})
			if err != nil {
				t.Fatal(err)
			}
			output := string(artifactForModule(artifacts, "main").Output)
			for _, want := range wants[mode] {
				if !strings.Contains(output, want) {
					t.Fatalf("generated %s imported constructor is missing %q:\n%s", mode, want, output)
				}
			}
		})
	}
}

func assertConstructorEffectProgramOutput(t *testing.T, source []byte, want string) {
	t.Helper()
	for _, mode := range []string{"go", "ruby"} {
		t.Run(mode, func(t *testing.T) {
			if mode == "ruby" {
				if _, err := exec.LookPath("ruby"); err != nil {
					t.Skip("ruby is not installed")
				}
			}
			artifact, err := Compile("constructor_effect.trb", source, mode)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(runConcurrentProbeArtifact(t, mode, artifact.Output)); got != want {
				t.Fatalf("unexpected constructor effect output for %s: got %q, want %q", mode, got, want)
			}
		})
	}
}
