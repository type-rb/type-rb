package compiler

import (
	"strings"
	"testing"
)

func TestClassAndInstanceMethodsDoNotShareEffectABI(t *testing.T) {
	source := []byte(`class Base
	def self.values(input: Array<Integer>): Array<Integer>
		return input.concurrent_map do |value|
			value
		end
	end
end

class Child < Base
	def values(input: Array<Integer>): Array<Integer>
		return input
	end
end

module Settings
	VALUES := Child.new().values([2])
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifact, err := Compile("member_effect_identity.trb", source, mode)
			if err != nil {
				t.Fatal(err)
			}
			output := string(artifact.Output)
			switch mode {
			case "go":
				if strings.Contains(output, "func (self *Child) Values(__trbScope") || strings.Contains(output, "NewChild().Values(trbcontext.Background()") {
					t.Fatalf("pure instance method received the class-method effect ABI:\n%s", output)
				}
			case "ruby":
				if strings.Contains(output, "def values(__trb_scope") || strings.Contains(output, "Child.new().values(TrbExecutionScope.root") {
					t.Fatalf("pure instance method received the class-method effect ABI:\n%s", output)
				}
			}
		})
	}
}

func TestPrivateClassesInDifferentModulesDoNotShareEffectABI(t *testing.T) {
	effectful := SourceUnit{
		Filename: "effectful.trb", ModulePath: "services/effectful",
		Source: []byte(`class _Box
	def values(input: Array<Integer>): Array<Integer>
		return input.concurrent_map do |value|
			value
		end
	end
end
`),
	}
	pure := SourceUnit{
		Filename: "pure.trb", ModulePath: "services/pure",
		Source: []byte(`class _Box
	def values(input: Array<Integer>): Array<Integer>
		return input
	end
end

module Settings
	VALUES := _Box.new().values([2])
end
`),
	}
	assertPureEffectIdentityModule(t, []SourceUnit{effectful, pure}, "services/pure")
}

func TestImportedSuperclassKeepsClassAndInstanceMethodEffectsSeparate(t *testing.T) {
	base := SourceUnit{
		Filename: "base.trb", ModulePath: "models/base",
		Source: []byte(`class Base
	def self.values(input: Array<Integer>): Array<Integer>
		return input.concurrent_map do |value|
			value
		end
	end
end
`),
	}
	child := SourceUnit{
		Filename: "child.trb", ModulePath: "models/child",
		Source: []byte(`import { Base } from models/base

class Child < Base
	def values(input: Array<Integer>): Array<Integer>
		return input
	end
end

module Settings
	VALUES := Child.new().values([2])
end
`),
	}
	assertPureEffectIdentityModule(t, []SourceUnit{base, child}, "models/child")
}

func TestPrivateInterfacesInDifferentModulesDoNotShareEffectABI(t *testing.T) {
	effectful := SourceUnit{
		Filename: "effectful.trb", ModulePath: "services/effectful",
		Source: []byte(`interface _Worker
	values(input: Array<Integer>): Array<Integer>
end

class EffectWorker implements _Worker
	def values(input: Array<Integer>): Array<Integer>
		return input.concurrent_map do |value|
			value
		end
	end
end
`),
	}
	pure := SourceUnit{
		Filename: "pure.trb", ModulePath: "services/pure",
		Source: []byte(`interface _Worker
	values(input: Array<Integer>): Array<Integer>
end

class PureWorker implements _Worker
	def values(input: Array<Integer>): Array<Integer>
		return input
	end
end

module Settings
	VALUES := PureWorker.new().values([2])
end
`),
	}
	assertPureEffectIdentityModule(t, []SourceUnit{effectful, pure}, "services/pure")
}

func TestImportedInterfaceSharesEffectABIWithItsImplementations(t *testing.T) {
	contract := SourceUnit{
		Filename: "worker.trb", ModulePath: "contracts/worker",
		Source: []byte(`interface Worker
	values(input: Array<Integer>): Array<Integer>
end
`),
	}
	effectful := SourceUnit{
		Filename: "effectful.trb", ModulePath: "services/effectful",
		Source: []byte(`import { Worker } from contracts/worker

class EffectWorker implements Worker
	def values(input: Array<Integer>): Array<Integer>
		return input.concurrent_map do |value|
			value
		end
	end
end
`),
	}
	pure := SourceUnit{
		Filename: "pure.trb", ModulePath: "services/pure",
		Source: []byte(`import { Worker } from contracts/worker

class PureWorker implements Worker
	def values(input: Array<Integer>): Array<Integer>
		return input
	end
end

def main()
	puts(PureWorker.new().values([2])[0].to_s())
	return
end
`),
	}
	wants := map[string]string{
		"go":         "func (self *PureWorker) Values(__trbScope trbcontext.Context",
		"ruby":       "def values(__trb_scope, input)",
		"typescript": "async values(__trbScope: AbortSignal | undefined",
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := CompileProject([]SourceUnit{contract, effectful, pure}, Options{
				Mode: mode, GoModule: "example.com/imported-interface-effect", RubyLoader: "require_relative",
				SourceRoot: "/project", ProjectRoot: "/project",
			})
			if err != nil {
				t.Fatal(err)
			}
			output := string(artifactForModule(artifacts, "services/pure").Output)
			if !strings.Contains(output, wants[mode]) {
				t.Fatalf("generated %s implementation is missing imported interface effect ABI %q:\n%s", mode, wants[mode], output)
			}
		})
	}
}

func assertPureEffectIdentityModule(t *testing.T, units []SourceUnit, module string) {
	t.Helper()
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := CompileProject(units, Options{
				Mode: mode, GoModule: "example.com/effect-identity", RubyLoader: "require_relative",
				SourceRoot: "/project", ProjectRoot: "/project",
			})
			if err != nil {
				t.Fatal(err)
			}
			output := string(artifactForModule(artifacts, module).Output)
			switch mode {
			case "go":
				if strings.Contains(output, "Values(__trbScope") || strings.Contains(output, ".Values(trbcontext.Background()") {
					t.Fatalf("pure Go module received another module's effect ABI:\n%s", output)
				}
			case "ruby":
				if strings.Contains(output, "def values(__trb_scope") || strings.Contains(output, ".values(TrbExecutionScope.root") {
					t.Fatalf("pure Ruby module received another module's effect ABI:\n%s", output)
				}
			}
		})
	}
}
