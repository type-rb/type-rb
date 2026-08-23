package nativepackage

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/types"
)

func TestNativeTypeRoundTripsFunctionTypes(t *testing.T) {
	semantic := types.FunctionOf([]types.Type{types.FromName("String")}, types.FromName("Integer"))
	wire := FromSemantic(semantic)
	if restored := wire.Semantic(); !types.Equivalent(restored, semantic) {
		t.Fatalf("function type round trip changed %s to %s", semantic, restored)
	}
}

func TestCatalogOwnsOnlyDependenciesAndRuntimeBackedSemanticModules(t *testing.T) {
	catalog := &Catalog{
		Dependencies: map[string]string{"native": "1.0.0"},
		Modules: map[string]Module{
			"semantic/direct": {Exports: map[string]Export{"invoke": {Kind: "function"}}},
			"semantic/runtime": {Exports: map[string]Export{
				"invoke": {Kind: "function", Runtime: &RuntimeBinding{Identity: "semantic/runtime#invoke"}},
			}},
		},
	}
	if !catalog.Owns("native/submodule") || !catalog.Owns("semantic/runtime") {
		t.Fatalf("expected dependency and runtime-backed module ownership")
	}
	if catalog.Owns("semantic/direct") {
		t.Fatalf("an unrelated catalog module must not grant native import ownership")
	}
}

func TestWriteAndLoadNativePackageIndex(t *testing.T) {
	root := t.TempDir()
	catalog := &Catalog{
		TypeScriptVersion: "6.0.3",
		Dependencies:      map[string]string{"@scope/ui": "1.2.3"},
		Modules: map[string]Module{
			"@scope/ui": {Exports: map[string]Export{
				"Button": {Kind: "component", Type: Type{Kind: "named", Name: "ReactNode"}},
				"ClientView": {
					Kind: "interface", Type: Type{Kind: "named", Name: "ClientView"},
					InstanceMembers: map[string]Export{
						"run": {Kind: "function", Type: Type{Kind: "string", Name: "String"}},
					},
				},
				"Client": {
					Kind: "class", Type: Type{Kind: "named", Name: "Client"},
					InstanceMembers: map[string]Export{
						"run": {Kind: "function", Type: Type{Kind: "string", Name: "String"}},
					},
					ClassMembers: map[string]Export{
						"create": {Kind: "function", Type: Type{Kind: "named", Name: "Client"}},
					},
				},
				"runQuery": {
					Kind: "function", Type: Type{Kind: "int", Name: "Integer"}, Required: 1,
					Parameters: []Type{{
						Kind: "function", Name: "Function", Args: []Type{{Kind: "int", Name: "Integer"}},
						ResultBridge: &ResultBridge{Kind: "result_to_promise_rejection", Error: Type{Kind: "string", Name: "String"}},
					}},
				},
				"loadToken": {
					Kind:         "function",
					Type:         Type{Kind: "named", Name: "Result", Args: []Type{{Kind: "string", Name: "String"}, {Kind: "string", Name: "String"}}},
					ResultBridge: &ResultBridge{Kind: "promise_rejection_to_result", Error: Type{Kind: "string", Name: "String"}},
				},
			}},
		},
	}
	if err := Write(root, catalog); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(IndexPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), `"formatVersion": 5`) || strings.Contains(string(written), `"fails"`) || strings.Contains(string(written), `"effectBridge"`) {
		t.Fatalf("native cache did not use the current schema:\n%s", written)
	}
	loaded, err := Load(root, map[string]string{"@scope/ui": "1.2.3"})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.FormatVersion != FormatVersion || loaded.TypeScriptVersion != "6.0.3" || !loaded.Owns("@scope/ui/button") {
		t.Fatalf("unexpected loaded catalog: %#v", loaded)
	}
	if _, ok := loaded.Modules["@scope/ui"].Exports["Button"]; !ok {
		t.Fatalf("native export was not preserved: %#v", loaded.Modules)
	}
	client := loaded.Modules["@scope/ui"].Exports["Client"]
	if client.InstanceMembers["run"].Type.Name != "String" || client.ClassMembers["create"].Type.Name != "Client" {
		t.Fatalf("native class member identity was not preserved: %#v", client)
	}
	clientView := loaded.Modules["@scope/ui"].Exports["ClientView"]
	if clientView.Kind != "interface" || clientView.InstanceMembers["run"].Type.Name != "String" {
		t.Fatalf("native interface member identity was not preserved: %#v", clientView)
	}
	bridge := loaded.Modules["@scope/ui"].Exports["runQuery"].Parameters[0].ResultBridge
	if bridge == nil || bridge.Kind != "result_to_promise_rejection" || bridge.Error.Name != "String" {
		t.Fatalf("native Result bridge was not preserved: %#v", bridge)
	}
	callBridge := loaded.Modules["@scope/ui"].Exports["loadToken"].ResultBridge
	if callBridge == nil || callBridge.Kind != "promise_rejection_to_result" || callBridge.Error.Name != "String" {
		t.Fatalf("native call Result bridge was not preserved: %#v", callBridge)
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
	data := fmt.Sprintf(`{"formatVersion":%d,"dependencies":{"ui":"1"},"modules":{}} {}`, FormatVersion)
	if err := os.WriteFile(IndexPath(root), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root, map[string]string{"ui": "1"}); err == nil || !strings.Contains(err.Error(), "trailing JSON content") {
		t.Fatalf("expected trailing content diagnostic, got %v", err)
	}
}

func TestLoadRejectsOlderNativeIndexBeforeLegacyFields(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(IndexPath(root)), 0o755); err != nil {
		t.Fatal(err)
	}
	data := `{"formatVersion":3,"dependencies":{"ui":"1"},"modules":{"ui":{"exports":{"run":{"kind":"function","type":{"kind":"function","name":"Function","args":[{"kind":"int","name":"Integer"}],"fails":{"kind":"string","name":"String"},"effectBridge":"promise_rejection"}}}}}}`
	if err := os.WriteFile(IndexPath(root), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(root, map[string]string{"ui": "1"})
	if err == nil || !strings.Contains(err.Error(), "formatVersion 3") || !strings.Contains(err.Error(), "expected 5") || !strings.Contains(err.Error(), "run trb install") {
		t.Fatalf("expected native cache migration diagnostic, got %v", err)
	}
	if strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("legacy cache fields hid the version migration diagnostic: %v", err)
	}
}

func TestLoadStrictlyRejectsLegacyFieldsInCurrentIndex(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(IndexPath(root)), 0o755); err != nil {
		t.Fatal(err)
	}
	data := fmt.Sprintf(`{"formatVersion":%d,"dependencies":{"ui":"1"},"modules":{"ui":{"exports":{"run":{"kind":"function","type":{"kind":"function","name":"Function","args":[{"kind":"int","name":"Integer"}],"fails":{"kind":"string","name":"String"}}}}}}}`, FormatVersion)
	if err := os.WriteFile(IndexPath(root), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root, map[string]string{"ui": "1"}); err == nil || !strings.Contains(err.Error(), `unknown field "fails"`) {
		t.Fatalf("expected strict legacy field diagnostic, got %v", err)
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
	if !strings.Contains(typeScriptIndexer, "checker.getTypeOfPropertyOfType(alternative, name)") {
		t.Fatal("native TypeScript indexer does not resolve declarationless object properties from their containing type")
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
