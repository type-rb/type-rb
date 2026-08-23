package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/declarationadapterhost"
	"github.com/type-rb/type-rb/internal/declarationadaptertooling"
	"github.com/type-rb/type-rb/internal/nativepackage"
	packageManager "github.com/type-rb/type-rb/internal/packages"
	"github.com/type-rb/type-rb/internal/project"
)

func TestAdapterTestRunsDeclaredConformancePhases(t *testing.T) {
	root, configPath := writeAdapterTestFixture(t, []string{"go", "version"})
	report, stdout, stderr := runAdapterTestReport(t, []string{"adapter", "test", "--format", "json", root}, 0)
	if stderr != "" {
		t.Fatalf("successful adapter test wrote diagnostics: %s", stderr)
	}
	if report.ProtocolVersion != declarationadaptertooling.ProtocolVersion || report.CompilerVersion != Version {
		t.Fatalf("unexpected report version: %#v", report)
	}
	if report.Package == nil || report.Package.Name != "github.com/acme/ui-types" {
		t.Fatalf("unexpected package: %#v", report.Package)
	}
	if len(report.Tests) != 1 {
		t.Fatalf("unexpected tests: %#v", report.Tests)
	}
	test := report.Tests[0]
	if test.Mode != "typescript" || test.ConfigPath != configPath || !test.Passed || strings.Join(test.Command, " ") != "go version" {
		t.Fatalf("unexpected adapter test: %#v", test)
	}
	if got := adapterTestPhaseStatuses(test); got != "adapter_check=passed,build=passed,native_check=passed" {
		t.Fatalf("unexpected phases: %s", got)
	}
	if report.Summary.Tests != 1 || report.Summary.PassedTests != 1 || report.Summary.FailedTests != 0 || report.Summary.Errors != 0 || len(report.Diagnostics) != 0 {
		t.Fatalf("unexpected summary: %#v", report)
	}
	second, secondStdout, secondStderr := runAdapterTestReport(t, []string{"adapter", "test", "--format", "json", root}, 0)
	if secondStderr != "" || secondStdout != stdout || second.Summary != report.Summary {
		t.Fatalf("adapter test report changed without input changes:\n%s\n%s\n%s", stdout, secondStdout, secondStderr)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(configPath), "build", "main.ts")); err != nil {
		t.Fatalf("conformance project was not built: %v", err)
	}
}

func TestAdapterTestReportsNativeCommandFailureWithoutCorruptingJSON(t *testing.T) {
	root, _ := writeAdapterTestFixture(t, []string{"go", "tool", "type-rb-missing-tool"})
	report, _, stderr := runAdapterTestReport(t, []string{"adapter", "test", "--format", "json", root}, 1)
	if len(report.Tests) != 1 || report.Tests[0].Passed {
		t.Fatalf("native command failure passed: %#v", report.Tests)
	}
	if got := adapterTestPhaseStatuses(report.Tests[0]); got != "adapter_check=passed,build=passed,native_check=failed" {
		t.Fatalf("unexpected phases: %s", got)
	}
	if report.Summary.FailedTests != 1 || report.Summary.Errors != 1 || len(report.Diagnostics) != 1 || !strings.Contains(report.Diagnostics[0].Message, "native check failed") {
		t.Fatalf("unexpected failure report: %#v", report)
	}
	if stderr == "" {
		t.Fatal("native command diagnostic output was suppressed")
	}
}

func TestAdapterTestRequiresInstalledSelfReferencingProject(t *testing.T) {
	root, configPath := writeAdapterTestFixture(t, []string{"go", "version"})
	if err := os.Remove(filepath.Join(filepath.Dir(configPath), "trb.lock")); err != nil {
		t.Fatal(err)
	}
	report, _, _ := runAdapterTestReport(t, []string{"adapter", "test", "--format", "json", root}, 1)
	if len(report.Tests) != 1 {
		t.Fatalf("unexpected phases: %#v", report.Tests)
	}
	if got := adapterTestPhaseStatuses(report.Tests[0]); got != "adapter_check=passed,build=failed,native_check=not_run" {
		t.Fatalf("unexpected phases: %s", got)
	}
	if len(report.Diagnostics) != 1 || !strings.Contains(report.Diagnostics[0].Message, "run trb install") {
		t.Fatalf("unexpected install diagnostic: %#v", report.Diagnostics)
	}
}

func TestAdapterTestRejectsProjectThatDoesNotInstallCurrentPackage(t *testing.T) {
	root, configPath := writeAdapterTestFixture(t, []string{"go", "version"})
	config, err := project.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	config.Packages = map[string]project.PackageRequirement{}
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	report, _, _ := runAdapterTestReport(t, []string{"adapter", "test", "--format", "json", root}, 1)
	if len(report.Diagnostics) != 1 || !strings.Contains(report.Diagnostics[0].Message, "must install package github.com/acme/ui-types from this package root") {
		t.Fatalf("unexpected self-reference diagnostic: %#v", report.Diagnostics)
	}
}

func TestAdapterTestRejectsConformanceModeMismatch(t *testing.T) {
	root, configPath := writeAdapterTestFixture(t, []string{"go", "version"})
	config, err := project.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	config.Mode = "go"
	config.Go = &project.GoConfig{Module: "example.com/conformance", Version: project.DefaultGoVersion, RootPackage: "main"}
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	report, _, _ := runAdapterTestReport(t, []string{"adapter", "test", "--format", "json", root}, 1)
	if len(report.Diagnostics) != 1 || !strings.Contains(report.Diagnostics[0].Message, "project selects mode go") {
		t.Fatalf("unexpected mode diagnostic: %#v", report.Diagnostics)
	}
}

func TestAdapterTestRequiresDeclaredConformanceProject(t *testing.T) {
	root, _ := writeAdapterCheckPackage(t, "typescript", "ui", validAdapterCheckCatalog())
	report, _, _ := runAdapterTestReport(t, []string{"adapter", "test", "--format", "json", root}, 1)
	if report.Package == nil || len(report.Tests) != 0 || len(report.Diagnostics) != 1 || !strings.Contains(report.Diagnostics[0].Message, "declares no adapter tests") {
		t.Fatalf("unexpected missing-test report: %#v", report)
	}
}

func TestAdapterTestHasConciseHumanOutput(t *testing.T) {
	root, _ := writeAdapterTestFixture(t, []string{"go", "version"})
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"adapter", "test", root}); status != 0 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if stdout.String() != "tested 1 declaration adapter(s) for package github.com/acme/ui-types\n" || stderr.Len() != 0 {
		t.Fatalf("unexpected human output: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func writeAdapterTestFixture(t *testing.T, nativeCommand []string) (string, string) {
	t.Helper()
	root := t.TempDir()
	conformanceRoot := filepath.Join(root, "conformance")
	for _, directory := range []string{filepath.Join(root, "src"), filepath.Join(conformanceRoot, "src")} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "src", "index.trb"), []byte("# Declarations only.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	adapterPath := filepath.Join(root, "declarations.json")
	adapterData, err := json.MarshalIndent(validAdapterCheckCatalog(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(adapterPath, append(adapterData, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	config := project.New(conformanceRoot, "typescript")
	config.SourceDir = "src"
	config.OutDir = "build"
	config.PackageManagement = project.ExternalPackages
	config.TypeScript.Runtime = project.TypeScriptRuntimeBrowser
	config.Packages["acme/ui-types"] = project.PackageRequirement{Path: ".."}
	copyFiles := false
	config.CopyFiles = &copyFiles
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(`import { render } from ui

def main()
	puts(render())
	return
end
`), 0o644); err != nil {
		t.Fatal(err)
	}

	manifest := packageManager.TypeRBManifest{
		FormatVersion: 1,
		Name:          "github.com/acme/ui-types",
		Version:       "0.1.0",
		Modes:         []string{"typescript"},
		NativeDependencies: map[string]map[string]string{
			"typescript": {"ui": "1.0.0"},
		},
		DeclarationAdapters: map[string]string{"typescript": "declarations.json"},
		AdapterTests: map[string]packageManager.AdapterTest{
			"typescript": {Config: filepath.ToSlash(filepath.Join("conformance", project.ConfigName)), Command: nativeCommand},
		},
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, packageManager.TypeRBManifestName), append(manifestData, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, err := packageManager.ResolveTypeRBPackages(config, packageManager.TypeRBResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	catalog := nativepackage.Empty(resolved.NativeDependencies)
	if err := nativepackage.ApplyDeclarationAdapterFiles(catalog, []declarationadapterhost.Source{{
		Package: manifest.Name, Mode: "typescript", Path: adapterPath, Dependencies: manifest.NativeDependenciesFor("typescript"),
	}}); err != nil {
		t.Fatal(err)
	}
	if err := nativepackage.Write(config.Root, catalog); err != nil {
		t.Fatal(err)
	}
	return root, config.Path
}

func runAdapterTestReport(t *testing.T, args []string, wantStatus int) (declarationadaptertooling.TestReport, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run(args); status != wantStatus {
		t.Fatalf("status=%d, want %d; stdout=%s stderr=%s", status, wantStatus, stdout.String(), stderr.String())
	}
	var report declarationadaptertooling.TestReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("invalid adapter test JSON: %v\n%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	return report, stdout.String(), stderr.String()
}

func adapterTestPhaseStatuses(test declarationadaptertooling.Test) string {
	values := make([]string, 0, len(test.Phases))
	for _, phase := range test.Phases {
		values = append(values, phase.Name+"="+phase.Status)
	}
	return strings.Join(values, ",")
}
