package nativepackage

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/types"
)

func TestNativeTypePreservesFunctionEffects(t *testing.T) {
	failure := types.FromName("RequestError")
	semantic := types.FunctionWithEffect([]types.Type{types.FromName("String")}, types.FromName("Integer"), failure)
	wire := FromSemantic(semantic)
	if wire.Fails == nil || wire.Fails.Name != "RequestError" {
		t.Fatalf("wire function lost its failure type: %#v", wire)
	}
	if restored := wire.Semantic(); !types.Equivalent(restored, semantic) {
		t.Fatalf("function effect round trip changed %s to %s", semantic, restored)
	}
}

func TestWriteAndLoadNativePackageIndex(t *testing.T) {
	root := t.TempDir()
	catalog := &Catalog{
		TypeScriptVersion: "6.0.3",
		Dependencies:      map[string]string{"@scope/ui": "1.2.3"},
		Modules: map[string]Module{
			"@scope/ui": {Exports: map[string]Export{"Button": {Kind: "component", Type: Type{Kind: "named", Name: "ReactNode"}}}},
		},
	}
	if err := Write(root, catalog); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(root, map[string]string{"@scope/ui": "1.2.3"})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.TypeScriptVersion != "6.0.3" || !loaded.Owns("@scope/ui/button") {
		t.Fatalf("unexpected loaded catalog: %#v", loaded)
	}
	if _, ok := loaded.Modules["@scope/ui"].Exports["Button"]; !ok {
		t.Fatalf("native export was not preserved: %#v", loaded.Modules)
	}
}

func TestLoadMarksChangedNativeDependenciesStale(t *testing.T) {
	root := t.TempDir()
	if err := Write(root, &Catalog{Dependencies: map[string]string{"ui": "1.0.0"}, Modules: map[string]Module{"ui": {Exports: map[string]Export{}}}}); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(root, map[string]string{"ui": "2.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(loaded.UnavailableReason, "stale") || len(loaded.Modules) != 0 {
		t.Fatalf("changed dependencies did not invalidate the index: %#v", loaded)
	}
}

func TestLoadRejectsTrailingNativeIndexContent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(IndexPath(root)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(IndexPath(root), []byte(`{"formatVersion":1,"dependencies":{"ui":"1"},"modules":{}} {}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root, map[string]string{"ui": "1"}); err == nil || !strings.Contains(err.Error(), "trailing JSON content") {
		t.Fatalf("expected trailing content diagnostic, got %v", err)
	}
}

func TestGenerateModulesRejectsUnsupportedTypeScriptMajor(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required for the TypeScript indexer diagnostic")
	}
	root := t.TempDir()
	typeScriptRoot := filepath.Join(root, "node_modules", "typescript")
	if err := os.MkdirAll(typeScriptRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	packageJSON := `{"name":"typescript","version":"7.0.2","main":"index.cjs"}`
	if err := os.WriteFile(filepath.Join(typeScriptRoot, "package.json"), []byte(packageJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(typeScriptRoot, "index.cjs"), []byte(`module.exports = { version: "7.0.2" };`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := GenerateModules(root, "npm", map[string]string{"ui": "1.0.0"}, []string{"ui"})
	if err == nil || !strings.Contains(err.Error(), `supports TypeScript 6.x; found 7.0.2`) || !strings.Contains(err.Error(), `"^6.0.0"`) {
		t.Fatalf("unexpected TypeScript compatibility diagnostic: %v", err)
	}
}

func TestTypeScriptIndexerUsesSyntaxNodesForSyntheticSymbols(t *testing.T) {
	if strings.Contains(typeScriptIndexer, "declaration || containingFile") {
		t.Fatal("native TypeScript indexer passes a file path where the compiler API requires a syntax node")
	}
	if !strings.Contains(typeScriptIndexer, "fallbackNode: source") || !strings.Contains(typeScriptIndexer, "declaration || state.fallbackNode") {
		t.Fatal("native TypeScript indexer does not retain a source-file fallback for synthetic symbols")
	}
}

func TestTypeScriptIndexerRecognizesTheDOMFileBoundary(t *testing.T) {
	for _, want := range []string{
		`name === "File" && sourceLooksDOM(symbol)`,
		`return wire("named", "File")`,
	} {
		if !strings.Contains(typeScriptIndexer, want) {
			t.Fatalf("native TypeScript indexer is missing the DOM File boundary %q", want)
		}
	}
}
