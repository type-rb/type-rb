package compiler

import (
	"strings"
	"testing"
)

func importedIdentityAliasUnits() []SourceUnit {
	return []SourceUnit{
		{Filename: "models/aliases.trb", ModulePath: "models/aliases", Package: "models", Source: []byte(`alias Identity<T> = T
alias Values<T> = Array<Identity<T>>
def keep<T>(value: T): Identity<T>
 return value
end
`)},
		{Filename: "collections/aliases.trb", ModulePath: "collections/aliases", Package: "collections", Source: []byte("import { Identity as Item } from models/aliases\nalias Identity<T> = Array<Item<T>>\n")},
		{Filename: "main.trb", ModulePath: "main", Package: "main", Source: []byte(`import { Identity as Same, Values, keep } from models/aliases
import { Identity as List } from collections/aliases
record Packet
 value: String
end
def main()
 values: Values<String> := ["held"]
 value: Same<String> := values[0]
 numbers: List<Integer> := [2]
 item := keep<Packet>(Packet.new(value: value))
 puts(item.value)
 puts(numbers[0])
end
`)},
	}
}

func TestImportedAliasDoesNotPublishHiddenNamesOrChangeArguments(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range []struct{ imports, body, message string }{
			{"import { keep } from models/aliases", "item: Identity<String> := keep<String>(\"held\")\nputs(item)", "type Identity is not declared or imported"},
			{"import { Identity as List } from collections/aliases", "item: List<Integer> := [\"wrong\"]\nputs(item)", "cannot assign Array<String> to Array<Integer>"},
		} {
			units := importedIdentityAliasUnits()[:2]
			units = append(units, SourceUnit{Filename: "main.trb", ModulePath: "main", Package: "main", Source: []byte(test.imports + "\ndef main()\n" + test.body + "\nend\n")})
			_, err := CompileProject(units, Options{Mode: mode})
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("%s: got %v, want %q", mode, err, test.message)
			}
		}
	}
}

func TestImportedAliasDeclarationIdentityAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireEffectRuntime(t, mode)
			options := Options{Mode: mode, GoModule: "example.com/alias-identity", RubyLoader: "require_relative", SourceRoot: "/project", ProjectRoot: "/project"}
			// Rebuild the catalog with both source orders. Independent declarations
			// must not depend on map traversal or the order of compilation units.
			for iteration := 0; iteration < 12; iteration++ {
				units := importedIdentityAliasUnits()
				if iteration%2 != 0 {
					units[0], units[1] = units[1], units[0]
				}
				artifacts, err := CompileProject(units, options)
				if err != nil {
					t.Fatalf("iteration %d: %v", iteration, err)
				}
				if iteration == 0 {
					if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, options.GoModule)); got != "held\n2" {
						t.Fatalf("alias output: %q", got)
					}
				}
			}
		})
	}
}

func TestInferredAliasRecordFieldAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			units := importedIdentityAliasUnits()[:1]
			units = append(units, SourceUnit{Filename: "main.trb", ModulePath: "main", Package: "main", Source: []byte(`import { keep } from models/aliases
record Packet
 value: String
end
def main()
 item := keep<Packet>(Packet.new(value: "held"))
 puts(item.value)
end
`)})
			requireEffectRuntime(t, mode)
			options := Options{Mode: mode, GoModule: "example.com/inferred-alias", RubyLoader: "require_relative", SourceRoot: "/project", ProjectRoot: "/project"}
			artifacts, err := CompileProject(units, options)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, options.GoModule)); got != "held" {
				t.Fatalf("inferred record output: %q", got)
			}
		})
	}
}
