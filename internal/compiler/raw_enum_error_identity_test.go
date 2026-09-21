package compiler

import (
	"strings"
	"testing"
)

func TestRawEnumConversionKeepsStandardErrorIdentity(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, tc := range []struct {
			name, declarations, value string
			files                     []SourceUnit
		}{
			{name: "local", declarations: "record EnumValueError\nflag: Boolean\nend\n", value: "EnumValueError.new(flag: true)"},
			{name: "imported", declarations: "import { Shadow as EnumValueError } from shadow\n", value: "EnumValueError.new(flag: true)", files: []SourceUnit{{Filename: "/project/shadow/index.trb", ModulePath: "shadow/index", Package: "shadow", Source: []byte("record Shadow\nflag: Boolean\nend\n")}}},
			{name: "nested", declarations: "module Domain\nrecord EnumValueError\nflag: Boolean\nend\nend\n", value: "Domain::EnumValueError.new(flag: true)"},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				requireEffectRuntime(t, mode)
				source := tc.declarations + `enum Status
Ready = "ready"
end
def main()
value := ` + tc.value + `
case Status.from_raw("missing")
when Result::Ok(_status)
puts("unexpected")
when Result::Err(error)
puts(error.message)
end
case Status.from_raw("ready")
when Result::Ok(status)
puts(status.raw_value())
when Result::Err(error)
puts(error.message)
end
puts(value.flag)
end
`
				options := Options{Mode: mode, GoModule: "example.com/enum-errors", RubyLoader: "require_relative", ProjectRoot: "/project", SourceRoot: "/project"}
				units := append([]SourceUnit{{Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: []byte(source)}}, tc.files...)
				artifacts, err := CompileProject(units, options)
				if err != nil {
					t.Fatal(err)
				}
				if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, options.GoModule)); got != "unknown raw value for Status\nready\ntrue" {
					t.Fatalf("got %q", got)
				}
			})
		}
	}
}

func TestImportedRawEnumConversionPreservesStandardAliases(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireEffectRuntime(t, mode)
			units := []SourceUnit{
				{Filename: "/project/model/index.trb", ModulePath: "model/index", Package: "model", Source: []byte(`module Codes
enum State
Ready = "ready"
end
end
`)},
				{Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: []byte(`import { Codes } from model
import { Result as Outcome } from trb/std/result
import { EnumValueError as RawError } from trb/std/errors
record EnumValueError
flag: Boolean = true
end
record Result
flag: Boolean = false
end
def parse(text: String): Outcome<Codes::State, RawError>
value := try Codes::State.from_raw(text)
return Outcome<Codes::State, RawError>::Ok(value)
end
def main()
value := EnumValueError.new()
result := Result.new()
state := parse("missing") catch |error|
puts(error.value)
Codes::State::Ready
end
puts(state.raw_value())
puts(value.flag)
puts(result.flag)
end
`)},
			}
			options := Options{Mode: mode, GoModule: "example.com/enum-errors", RubyLoader: "require_relative", ProjectRoot: "/project", SourceRoot: "/project"}
			artifacts, err := CompileProject(units, options)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, options.GoModule)); got != "missing\nready\ntrue\nfalse" {
				t.Fatalf("got %q", got)
			}
		})
	}
}

func TestRawEnumConversionRejectsUnrelatedAuthoredErrorType(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			_, err := CompileProject([]SourceUnit{{Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: []byte(`import trb/std/result
record EnumValueError
flag: Boolean
end
enum Status
Ready = "ready"
end
def parse(): Result<Status, EnumValueError>
return Status.from_raw("ready")
end
def main()
end
`)}}, Options{Mode: mode, ProjectRoot: "/project", SourceRoot: "/project"})
			if err == nil || !strings.Contains(err.Error(), "expected Result<Status, EnumValueError>") {
				t.Fatalf("expected a nominal error-type mismatch, got %v", err)
			}
		})
	}
}

func TestCollidingRecordNamesKeepConstructorsAndDefaults(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireEffectRuntime(t, mode)
			units := []SourceUnit{
				{Filename: "/project/model/index.trb", ModulePath: "model/index", Package: "model", Source: []byte(`record Entry<T>
value: T
count: Integer = 2
end
def make(): Entry<String>
return Entry<String>.new(value: "imported")
end
`)},
				{Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: []byte(`import { Entry as Item, make } from model
record Entry
flag: Boolean = true
end
def main()
local := Entry.new()
imported: Item<String> := make()
direct := Item<Integer>.new(value: 7, count: 3)
puts(local.flag)
puts(imported.value)
puts(imported.count)
puts(direct.value)
puts(direct.count)
end
`)},
			}
			options := Options{Mode: mode, GoModule: "example.com/record-names", RubyLoader: "require_relative", ProjectRoot: "/project", SourceRoot: "/project"}
			artifacts, err := CompileProject(units, options)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, options.GoModule)); got != "true\nimported\n2\n7\n3" {
				t.Fatalf("got %q", got)
			}
		})
	}
}
