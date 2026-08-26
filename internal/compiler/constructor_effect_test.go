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
