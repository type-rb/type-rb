package releasecontract

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/parser"
)

func TestDiscoveryIncludesPortablePlatformReceiverAndCommandContracts(t *testing.T) {
	surfaces, err := Discover("../..")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Surface{}
	for _, surface := range surfaces {
		if strings.HasPrefix(surface.ID, "package:trb/internal/") {
			t.Errorf("private package exposed as public: %s", surface.ID)
		}
		byID[surface.ID] = surface
	}
	for _, id := range []string{"package:trb/std/path", "package:trb/platform/typescript/dom", "package:trb/web", "builtin:String", "syntax:ClassStatement", "command:web openapi", "command:install", "flags:runFmt", "lsp:textDocument/rename", "delivery:packaging/container/Dockerfile", "tests:internal/repl", "kinds:internal/types/types.go"} {
		if _, ok := byID[id]; !ok {
			t.Errorf("missing existing public contract %s", id)
		}
	}
	path := strings.Join(byID["package:trb/std/path"].Members, "\n")
	for _, expected := range []string{"Path#join(", "private new RelativePath", "RelativePath::def self.parse", "RelativePath::def parent()"} {
		if !strings.Contains(path, expected) {
			t.Errorf("path declaration missing %q: %s", expected, path)
		}
	}
	if strings.Contains(path, "def self._component_error") {
		t.Error("private helper advertised as a public API")
	}
	dom := byID["package:trb/platform/typescript/dom"]
	if strings.Join(dom.Modes, ",") != "typescript" {
		t.Errorf("platform applicability lost: %v", dom.Modes)
	}
}

func TestSourceDiscoveryRetainsNominalPolicyAndSignatures(t *testing.T) {
	source := `newtype Checked = String do
  private new
  def self.parse(value: String): Checked
    return self.new(value)
  end
  def self._implementation(): Integer
    return 1
  end
end
record Box<T>
  value: T
end
enum Choice<T>
  Item(value: T)
end
`
	program, diagnostics := parser.Parse([]byte(source))
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	discovered, err := sourceMembers(program.Statements, "", source)
	if err != nil {
		t.Fatal(err)
	}
	members := strings.Join(discovered, "\n")
	for _, expected := range []string{"private new Checked", "Checked::def self.parse(value: String): Checked", "record Box<T>", "field Box<T>::value: T", "variant Choice<T>::Item(value: T)"} {
		if !strings.Contains(members, expected) {
			t.Errorf("missing %q in %s", expected, members)
		}
	}
	if strings.Contains(members, "_implementation") || strings.Contains(members, "return self.new") {
		t.Errorf("copied private implementation into API census: %s", members)
	}
}
