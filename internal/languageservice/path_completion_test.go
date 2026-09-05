package languageservice_test

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/languageservice"
)

func TestPathCompletionUsesNominalValueAndFactoryContracts(t *testing.T) {
	const source = `import { Path, RelativePath } from trb/std/path
path := Path.new("root")
def describe(relative: RelativePath): String
	return relative.to_s()
end
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			candidates := languageservice.StandardImportCandidates(mode)
			for _, expected := range []struct{ name, symbol string }{
				{"Path", ""}, {"RelativePath", "RelativePath"}, {"RelativePathError", "RelativePathError"},
			} {
				symbol, ok := filesystemCompletionSymbol(candidates.Symbols, expected.name)
				if !ok || symbol.Import == nil || symbol.Import.Path != "trb/std/path" || symbol.Import.Symbol != expected.symbol {
					t.Errorf("%s import candidate = %#v", expected.name, symbol)
				}
			}
			artifact := compile(t, mode, source)
			service := languageservice.New(mode)
			service.Update([]*ir.Program{artifact.IR}, "repl")
			for _, fixture := range []struct{ source, member string }{
				{"Path.ne", "new"}, {"RelativePath.pa", "parse"},
				{"path.jo", "join"}, {"path.to", "to_s"},
				{"def inspect(relative: RelativePath)\nrelative.jo", "join"},
				{"def inspect(relative: RelativePath)\nrelative.ch", "child"},
				{"def inspect(relative: RelativePath)\nrelative.pa", "parent"},
			} {
				items := service.Complete(fixture.source, len(fixture.source))
				item, ok := findCompletion(items, fixture.member)
				if !ok {
					t.Errorf("Complete(%q) omitted %s: %v", fixture.source, fixture.member, labels(items))
				} else if fixture.source == "path.jo" && !strings.Contains(item.Detail, "RelativePath") {
					t.Errorf("Path#join lost its validated argument: %#v", item)
				}
			}
			for _, fixture := range []struct{ source, member string }{
				{"RelativePath.ne", "new"}, {"RelativePath._c", "_component_error"},
				{"Path.jo", "join"}, {"path.si", "size"},
			} {
				if item, ok := findCompletion(service.Complete(fixture.source, len(fixture.source)), fixture.member); ok {
					t.Errorf("Complete(%q) advertised invalid member: %#v", fixture.source, item)
				}
			}
		})
	}
}
