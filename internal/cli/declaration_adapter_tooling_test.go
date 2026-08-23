package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/declarationadaptertooling"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/packageextension"
	packageManager "github.com/type-rb/type-rb/internal/packages"
)

func TestAdapterCheckEmitsDeterministicMachineReadableReport(t *testing.T) {
	root, adapterPath := writeAdapterCheckPackage(t, "typescript", "ui", validAdapterCheckCatalog())
	report, stdout := runAdapterCheckReport(t, []string{"adapter", "check", "--format", "json", root}, 0)
	if report.ProtocolVersion != declarationadaptertooling.ProtocolVersion || report.CompilerVersion != Version {
		t.Fatalf("unexpected report version: %#v", report)
	}
	if report.Package == nil || report.Package.Name != "github.com/acme/ui-types" || report.Package.Version != "0.1.0" {
		t.Fatalf("unexpected package identity: %#v", report.Package)
	}
	if len(report.Adapters) != 1 {
		t.Fatalf("unexpected adapter report: %#v", report.Adapters)
	}
	adapter := report.Adapters[0]
	if adapter.Mode != "typescript" || adapter.Path != adapterPath || !adapter.Valid || adapter.DeclarationProtocolVersion != packageextension.DeclarationAdapterProtocolVersion {
		t.Fatalf("unexpected checked adapter: %#v", adapter)
	}
	if adapter.Modules != 1 || adapter.Exports != 1 || adapter.SupportingRecords != 1 {
		t.Fatalf("unexpected adapter counts: %#v", adapter)
	}
	if report.Summary.Adapters != 1 || report.Summary.ValidAdapters != 1 || report.Summary.Modules != 1 || report.Summary.Exports != 1 || report.Summary.SupportingRecords != 1 || report.Summary.Errors != 0 {
		t.Fatalf("unexpected report summary: %#v", report.Summary)
	}
	second, secondStdout := runAdapterCheckReport(t, []string{"adapter", "check", "--format", "json", root}, 0)
	if secondStdout != stdout || second.Summary != report.Summary {
		t.Fatalf("adapter check output changed without an input change:\n%s\n%s", stdout, secondStdout)
	}
}

func TestAdapterCheckReportsManifestAndConsumerDiagnosticsAsJSON(t *testing.T) {
	t.Run("missing manifest", func(t *testing.T) {
		report, _ := runAdapterCheckReport(t, []string{"adapter", "check", "--format", "json", t.TempDir()}, 1)
		assertAdapterCheckDiagnostic(t, report, diagnostic.ProjectError, "trbpackage.json")
		if report.Package != nil || len(report.Adapters) != 0 {
			t.Fatalf("manifest failure exposed partial package data: %#v", report)
		}
	})

	t.Run("no adapters", func(t *testing.T) {
		root := writeAdapterCheckManifest(t, "typescript", "ui", "")
		report, _ := runAdapterCheckReport(t, []string{"adapter", "check", "--format", "json", root}, 1)
		assertAdapterCheckDiagnostic(t, report, diagnostic.ProjectIntegration, "declares no declaration adapters")
		if report.Package == nil || len(report.Adapters) != 0 {
			t.Fatalf("unexpected empty-adapter report: %#v", report)
		}
	})

	t.Run("malformed catalog", func(t *testing.T) {
		catalog := validAdapterCheckCatalog()
		exported := catalog.Modules["ui"].Exports["render"]
		exported.Type = packageextension.DeclarationAdapterType{Kind: "array", Name: "Array"}
		catalog.Modules["ui"].Exports["render"] = exported
		root, adapterPath := writeAdapterCheckPackage(t, "typescript", "ui", catalog)
		report, _ := runAdapterCheckReport(t, []string{"adapter", "check", "--format", "json", root}, 1)
		assertAdapterCheckDiagnostic(t, report, diagnostic.ProjectIntegration, "requires exactly one argument")
		if len(report.Adapters) != 1 || report.Adapters[0].Path != adapterPath || report.Adapters[0].Valid {
			t.Fatalf("unexpected invalid-adapter report: %#v", report.Adapters)
		}
	})

	t.Run("native ownership", func(t *testing.T) {
		root, _ := writeAdapterCheckPackage(t, "typescript", "ui", packageextension.DeclarationAdapterCatalog{
			ProtocolVersion: packageextension.DeclarationAdapterProtocolVersion,
			Modules: map[string]packageextension.DeclarationAdapterModule{
				"other": {Exports: map[string]packageextension.DeclarationAdapterExport{
					"render": {Kind: "function", Type: packageextension.DeclarationAdapterType{Kind: "string", Name: "String"}},
				}},
			},
		})
		report, _ := runAdapterCheckReport(t, []string{"adapter", "check", "--format", "json", root}, 1)
		assertAdapterCheckDiagnostic(t, report, diagnostic.ProjectIntegration, "without a matching TypeScript native dependency")
	})

	t.Run("unsupported ecosystem", func(t *testing.T) {
		root, _ := writeAdapterCheckPackage(t, "ruby", "pagy", packageextension.DeclarationAdapterCatalog{
			ProtocolVersion: packageextension.DeclarationAdapterProtocolVersion,
			Modules: map[string]packageextension.DeclarationAdapterModule{
				"pagy": {Exports: map[string]packageextension.DeclarationAdapterExport{
					"pages": {Kind: "function", Type: packageextension.DeclarationAdapterType{Kind: "int", Name: "Integer"}},
				}},
			},
		})
		report, _ := runAdapterCheckReport(t, []string{"adapter", "check", "--format", "json", root}, 1)
		assertAdapterCheckDiagnostic(t, report, diagnostic.ProjectIntegration, "provides only the TypeScript declaration adapter")
	})
}

func TestAdapterCheckHasConciseHumanOutput(t *testing.T) {
	root, _ := writeAdapterCheckPackage(t, "typescript", "ui", validAdapterCheckCatalog())
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"adapter", "check", root}); status != 0 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if stdout.String() != "checked 1 declaration adapter(s) for package github.com/acme/ui-types\n" || stderr.Len() != 0 {
		t.Fatalf("unexpected human output: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	catalog := validAdapterCheckCatalog()
	exported := catalog.Modules["ui"].Exports["render"]
	exported.Type = packageextension.DeclarationAdapterType{Kind: "function", Name: "Function"}
	catalog.Modules["ui"].Exports["render"] = exported
	invalidRoot, adapterPath := writeAdapterCheckPackage(t, "typescript", "ui", catalog)
	stdout.Reset()
	stderr.Reset()
	command = &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"adapter", "check", invalidRoot}); status != 1 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), adapterPath+": error[TRB4001]") || !strings.Contains(stderr.String(), "type kind function requires a return type") {
		t.Fatalf("unexpected human diagnostic: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func validAdapterCheckCatalog() packageextension.DeclarationAdapterCatalog {
	return packageextension.DeclarationAdapterCatalog{
		ProtocolVersion: packageextension.DeclarationAdapterProtocolVersion,
		Modules: map[string]packageextension.DeclarationAdapterModule{
			"ui": {
				Exports: map[string]packageextension.DeclarationAdapterExport{
					"render": {Kind: "function", Type: packageextension.DeclarationAdapterType{Kind: "string", Name: "String"}},
				},
				Records: map[string]packageextension.DeclarationAdapterExport{
					"RenderOptions": {Kind: "record", Type: packageextension.DeclarationAdapterType{Kind: "named", Name: "RenderOptions"}},
				},
			},
		},
	}
}

func writeAdapterCheckPackage(t *testing.T, mode, nativeDependency string, catalog packageextension.DeclarationAdapterCatalog) (string, string) {
	t.Helper()
	root := writeAdapterCheckManifest(t, mode, nativeDependency, "declarations.json")
	path := filepath.Join(root, "declarations.json")
	data, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, path
}

func writeAdapterCheckManifest(t *testing.T, mode, nativeDependency, adapter string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := packageManager.TypeRBManifest{
		FormatVersion: 1,
		Name:          "github.com/acme/ui-types",
		Version:       "0.1.0",
		Modes:         []string{mode},
		NativeDependencies: map[string]map[string]string{
			mode: {nativeDependency: "1.0.0"},
		},
	}
	if adapter != "" {
		manifest.DeclarationAdapters = map[string]string{mode: adapter}
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, packageManager.TypeRBManifestName), append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func runAdapterCheckReport(t *testing.T, args []string, wantStatus int) (declarationadaptertooling.Report, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run(args); status != wantStatus {
		t.Fatalf("status=%d, want %d; stdout=%s stderr=%s", status, wantStatus, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("adapter check JSON wrote to stderr: %s", stderr.String())
	}
	var report declarationadaptertooling.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("invalid adapter check JSON: %v\n%s", err, stdout.String())
	}
	return report, stdout.String()
}

func assertAdapterCheckDiagnostic(t *testing.T, report declarationadaptertooling.Report, code diagnostic.Code, message string) {
	t.Helper()
	if report.Summary.Errors != 1 || len(report.Diagnostics) != 1 || report.Diagnostics[0].Code != code || !strings.Contains(report.Diagnostics[0].Message, message) {
		t.Fatalf("expected %s diagnostic containing %q, got %#v", code, message, report)
	}
}
