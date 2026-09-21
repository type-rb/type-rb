package compiler

import (
	"strings"
	"testing"
)

func TestInterfacesCannotBeConstructedAcrossModes(t *testing.T) {
	cases := []struct {
		name, declarations, expression string
	}{
		{"direct", "interface Named\nname(): String\nend\n", "Named.new()"},
		{"generic", "interface Named<T>\nname(): T\nend\n", "Named<String>.new()"},
		{"nested", "module Contract\ninterface Named\nname(): String\nend\nend\n", "Contract::Named.new()"},
		{"alias", "interface Named\nname(): String\nend\nalias Label = Named\n", "Label.new()"},
		{"generic-alias", "interface Named<T>\nname(): T\nend\nalias Label<T> = Named<T>\n", "Label<String>.new()"},
		{"member-reference", "interface Named\nname(): String\nend\n", "Named.new"},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, tc := range cases {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				source := tc.declarations + "def main()\n" + tc.expression + "\nend\n"
				if _, err := Compile("interface_construction.trb", []byte(source), mode); err == nil || !strings.Contains(err.Error(), "cannot construct interface") && !strings.Contains(err.Error(), "not a generic declaration") {
					t.Fatalf("expected interface construction diagnostic, got %v", err)
				}
				unit := SourceUnit{Filename: "session.trb", ModulePath: "session", Source: []byte(tc.declarations + tc.expression + "\n")}
				if _, err := CompileProject([]SourceUnit{unit}, Options{Mode: mode, InteractiveModule: "session"}); err == nil || !strings.Contains(err.Error(), "cannot construct interface") && !strings.Contains(err.Error(), "not a generic declaration") {
					t.Fatalf("interactive compilation accepted interface construction: %v", err)
				}
			})
		}
	}
}

func TestImportedInterfacesCannotBeConstructedAcrossModes(t *testing.T) {
	contract := SourceUnit{Filename: "contracts.trb", ModulePath: "contracts", Source: []byte(`interface Named<T>
  name(): T
end
alias Label = Named<String>
module Nested
  interface View
    name(): String
  end
end
`)}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, tc := range []struct{ name, imports, expression string }{
			{"renamed", "import { Named as Contract } from contracts", "Contract<String>.new()"},
			{"alias", "import { Label } from contracts", "Label.new()"},
			{"local-alias", "import { Named as Contract } from contracts\nalias Local = Contract<String>", "Local.new()"},
			{"namespace", "import { Nested } from contracts", "Nested::View.new()"},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				consumer := SourceUnit{Filename: "main.trb", ModulePath: "main", Source: []byte(tc.imports + "\nclass Named\nend\ndef main()\n" + tc.expression + "\nend\n")}
				if _, err := CompileProject([]SourceUnit{contract, consumer}, Options{Mode: mode, GoModule: "example.com/interfaces", RubyLoader: "require_relative"}); err == nil || !strings.Contains(err.Error(), "cannot construct interface") && !strings.Contains(err.Error(), "not a generic declaration") {
					t.Fatalf("expected imported interface construction diagnostic, got %v", err)
				}
			})
		}
	}
}

func TestInterfaceConstructionCheckPreservesNominalClassIdentity(t *testing.T) {
	contract := SourceUnit{Filename: "contracts.trb", ModulePath: "contracts", Source: []byte("interface Named\nname(): String\nend\n")}
	consumer := SourceUnit{Filename: "main.trb", ModulePath: "main", Source: []byte(`import { Named as Contract } from contracts
class Named
  def name(): String
    return "held"
  end
end
def display(value: Contract): String
  return value.name()
end
def main()
  puts(Named.new().name())
end
`)}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			if _, err := CompileProject([]SourceUnit{contract, consumer}, Options{Mode: mode, GoModule: "example.com/interfaces", RubyLoader: "require_relative"}); err != nil {
				t.Fatalf("class construction or interface dispatch was rejected: %v", err)
			}
		})
	}
}

func TestInterfaceDeclarationsAreNotRuntimeValues(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, source := range []string{
			"interface Named\nname(): String\nend\ndef main()\nvalue := Named\nvalue.name()\nend\n",
			"interface Named<T>\nname(): T\nend\ndef main()\nvalue := Named<String>\nvalue.name()\nend\n",
		} {
			t.Run(mode, func(t *testing.T) {
				if _, err := Compile("interface_value.trb", []byte(source), mode); err == nil {
					t.Fatalf("expected interface declaration value diagnostic, got %v", err)
				}
			})
		}
	}
}
