package compiler

import (
	"strings"
	"testing"
)

func TestEnumSelfCallUsesExactEffectOwnerAcrossBackends(t *testing.T) {
	source := []byte(`enum Status
	Ready

	def values(input: Array<Integer>): Array<Integer>
		return input.concurrent_map do |value|
			value
		end
	end

	def forwarded(input: Array<Integer>): Array<Integer>
		return self.values(input)
	end
end

def main()
	status := Status::Ready
	puts(status.forwarded([7])[0].to_s())
	return
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "enum_effect_owner.trb", source, "7")
		})
	}
}

func TestNestedEnumCallKeepsQualifiedEffectOwnerAcrossBackends(t *testing.T) {
	source := []byte(`module Services
	enum Status
		Ready

		def values(input: Array<Integer>): Array<Integer>
			return input.concurrent_map do |value|
				value
			end
		end

		def forwarded(input: Array<Integer>): Array<Integer>
			return self.values(input)
		end
	end
end

def main()
	status := Services::Status::Ready
	puts(status.forwarded([8])[0].to_s())
	return
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "nested_enum_effect_owner.trb", source, "8")
		})
	}
}

func TestNestedRawEnumUsesQualifiedOwnerAcrossBackends(t *testing.T) {
	source := []byte(`import { Result } from trb/std/result

module Services
	enum Status
		Ready = "READY"
	end
end

def main()
	parsed := Services::Status.from_raw("READY")
	case parsed
	when Result::Ok(value)
		puts(value.raw_value())
	when Result::Err(error)
		puts(error.message)
	end
	return
end
`)
	wants := map[string]string{
		"go":         "StatusReady",
		"ruby":       "Result::Ok.new(Services::Status::Ready)",
		"typescript": "Result.Ok<Status, EnumValueError>(Services.Status.Ready)",
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifact, err := Compile("nested_raw_enum.trb", source, mode)
			if err != nil {
				t.Fatal(err)
			}
			if output := string(artifact.Output); !strings.Contains(output, wants[mode]) {
				t.Fatalf("generated %s raw enum conversion is missing qualified owner %q:\n%s", mode, wants[mode], output)
			}
		})
	}
}

func TestQualifiedNestedRecordDefaultsUseHelpersAcrossBackends(t *testing.T) {
	source := []byte(`def map_values(values: Array<Integer>): Array<Integer>
	return values.concurrent_map do |value|
		value
	end
end

module Services
	record Config
		values: Array<Integer> = map_values([31])
	end
end

def main()
	defaulted := Services::Config.new()
	provided := Services::Config.new(values: [32])
	puts(defaulted.values[0].to_s())
	puts(provided.values[0].to_s())
	return
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "nested_record_defaults.trb", source, "31\n32")
		})
	}
}

func TestQualifiedGenericNestedRecordConstructionAcrossBackends(t *testing.T) {
	source := []byte(`module Services
	record Box<T>
		value: T
	end

	record Empty
	end
end

def main()
	box := Services::Box<Integer>.new(value: 17)
	_empty := Services::Empty.new()
	puts(box.value.to_s())
	return
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "nested_generic_record.trb", source, "17")
		})
	}
}

func TestNamespaceImportedRecordDefaultsUseHelpersAcrossBackends(t *testing.T) {
	model := SourceUnit{
		Filename: "models/config.trb", ModulePath: "models/config", Package: "models",
		Source: []byte(`def map_values(values: Array<Integer>): Array<Integer>
	return values.concurrent_map do |value|
		value
	end
end

record Config
	values: Array<Integer> = map_values([41])
end

record Required
	value: Integer
end
`),
	}
	main := SourceUnit{
		Filename: "main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import models/config as configs

def main()
	puts(configs::Config.new().values[0].to_s())
	puts(configs::Required.new(value: 42).value.to_s())
	return
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireEffectRuntime(t, mode)
			artifacts, err := CompileProject([]SourceUnit{model, main}, Options{
				Mode: mode, GoModule: "example.com/namespace-record-default", RubyLoader: "require_relative",
				TypeScriptRuntime: "bun", SourceRoot: "/project", ProjectRoot: "/project",
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := runEffectProject(t, mode, artifacts, "example.com/namespace-record-default"); got != "41\n42\n" {
				t.Fatalf("unexpected namespace-imported record output for %s: got %q, want %q", mode, got, "41\n42\n")
			}
		})
	}
}
