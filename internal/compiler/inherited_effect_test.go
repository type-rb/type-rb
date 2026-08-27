package compiler

import (
	"os/exec"
	"strings"
	"testing"
)

const inheritedEffectPrelude = `def map_values(values: Array<Integer>): Array<Integer>
	return values.concurrent_map do |value|
		value
	end
end

record Config
	values: Array<Integer>
	mapped: Array<Integer> = map_values(values)
end
`

func TestInheritedEffectfulParameterDefaultRunsAcrossBackends(t *testing.T) {
	source := []byte(inheritedEffectPrelude + `
class Base
	def load(config: Config = Config.new(values: [7])): Array<Integer>
		return config.mapped
	end
end

class Child < Base
end

def main()
	puts(Child.new().load()[0].to_s())
	return
end
`)
	assertInheritedEffectProgramOutput(t, source, "7")
}

func TestOverriddenParameterDefaultsShareEffectABIAcrossBackends(t *testing.T) {
	source := []byte(inheritedEffectPrelude + `
PURE_CONFIG := Config.new(values: [3], mapped: [3])

class Base
	def load(config: Config = PURE_CONFIG): Array<Integer>
		return config.mapped
	end
end

class Child < Base
	def load(config: Config = Config.new(values: [9])): Array<Integer>
		return config.mapped
	end
end

def main()
	puts(Base.new().load()[0].to_s())
	puts(Child.new().load()[0].to_s())
	return
end
`)
	assertInheritedEffectProgramOutput(t, source, "3\n9")
}

func TestImportedInheritedEffectfulParameterDefaultUsesSharedABI(t *testing.T) {
	model := SourceUnit{
		Filename: "base.trb", ModulePath: "models/base", Package: "models",
		Source: []byte(inheritedEffectPrelude + `
class Base
	def load(config: Config = Config.new(values: [7])): Array<Integer>
		return config.mapped
	end
end
`),
	}
	main := SourceUnit{
		Filename: "main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import { Base } from models/base

class Child < Base
end

def main()
	puts(Child.new().load()[0].to_s())
	return
end
`),
	}
	wants := map[string]string{
		"go":         "NewChild().Load(__trbScope)",
		"ruby":       "Child.new().load(__trb_scope)",
		"typescript": "await new Child().load(__trbScope, [])",
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := CompileProject([]SourceUnit{model, main}, Options{
				Mode: mode, GoModule: "example.com/inherited-effect", RubyLoader: "require_relative",
				SourceRoot: "/project", ProjectRoot: "/project",
			})
			if err != nil {
				t.Fatal(err)
			}
			output := string(artifactForModule(artifacts, "main").Output)
			if !strings.Contains(output, wants[mode]) {
				t.Fatalf("generated %s imported inherited call is missing %q:\n%s", mode, wants[mode], output)
			}
		})
	}
}

func assertInheritedEffectProgramOutput(t *testing.T, source []byte, want string) {
	t.Helper()
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			if mode == "ruby" {
				if _, err := exec.LookPath("ruby"); err != nil {
					t.Skip("ruby is not installed")
				}
			}
			if mode == "typescript" {
				if _, err := exec.LookPath("bun"); err != nil {
					t.Skip("bun is not installed")
				}
			}
			artifact, err := Compile("inherited_effect.trb", source, mode)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(runConcurrentProbeArtifact(t, mode, artifact.Output)); got != want {
				t.Fatalf("unexpected inherited effect output for %s: got %q, want %q", mode, got, want)
			}
		})
	}
}
