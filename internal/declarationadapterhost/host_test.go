package declarationadapterhost

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadUsesStrictVersionedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "declarations.json")
	data := `{"protocolVersion":1,"modules":{"ui":{"exports":{"Button":{"kind":"component","type":{"kind":"named","name":"ReactNode"}}}}}}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	catalog, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Modules["ui"].Exports["Button"].Kind != "component" {
		t.Fatalf("unexpected declaration adapter catalog: %#v", catalog)
	}

	if err := os.WriteFile(path, []byte(data+` {}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err == nil || !strings.Contains(err.Error(), "trailing JSON content") {
		t.Fatalf("expected trailing content diagnostic, got %v", err)
	}

	unknown := `{"protocolVersion":1,"modules":{},"compilerExtension":{}}`
	if err := os.WriteFile(path, []byte(unknown), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err == nil || !strings.Contains(err.Error(), `unknown field "compilerExtension"`) {
		t.Fatalf("expected strict unknown-field diagnostic, got %v", err)
	}
}

func TestReadDiagnosesLegacyNativeTypeProvider(t *testing.T) {
	path := filepath.Join(t.TempDir(), "native-types.json")
	legacy := `{"formatVersion":2,"modules":{}}`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Read(path)
	if err == nil || !strings.Contains(err.Error(), "formatVersion 2") || !strings.Contains(err.Error(), "declarationAdapters") || !strings.Contains(err.Error(), "protocolVersion 1") || !strings.Contains(err.Error(), "run trb install") {
		t.Fatalf("expected declaration adapter migration diagnostic, got %v", err)
	}
}

func TestChecksumsIncludeModeIdentity(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "declarations.json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	checksums, err := Checksums([]Source{{Package: "github.com/acme/ui", Mode: "typescript", Path: path}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(checksums["github.com/acme/ui#typescript"], "sha256:") {
		t.Fatalf("unexpected declaration adapter checksums: %#v", checksums)
	}
}

func TestChecksumsRejectsIncompleteSourceIdentity(t *testing.T) {
	if _, err := Checksums([]Source{{Package: "provider"}}); err == nil || !strings.Contains(err.Error(), "requires a package, mode, and path") {
		t.Fatalf("expected incomplete source diagnostic, got %v", err)
	}
}
