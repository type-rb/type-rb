package nativepackage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteAndLoadNativePackageIndex(t *testing.T) {
	root := t.TempDir()
	catalog := &Catalog{
		TypeScriptVersion: "7.0.0",
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
	if loaded.TypeScriptVersion != "7.0.0" || !loaded.Owns("@scope/ui/button") {
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
