package packages

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func TestResolveLocalTypeRBPackageWritesDeterministicLock(t *testing.T) {
	workspace := t.TempDir()
	appRoot := filepath.Join(workspace, "app")
	packageRoot := filepath.Join(workspace, "contracts")
	writeTestPackage(t, packageRoot, TypeRBManifest{
		Name: "github.com/acme/contracts", Version: "0.1.0", SourceDir: "src",
		NativeDependencies: map[string]map[string]string{"go": {"golang.org/x/text": "v0.29.0"}},
	}, "record Message\n\ttext: String\nend\n")
	config := project.New(appRoot, "go")
	config.Go.Module = "example.com/app"
	config.Packages["acme/contracts"] = project.PackageRequirement{Path: "../contracts"}

	resolved, err := ResolveTypeRBPackages(config, TypeRBResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Aliases["acme/contracts"] != "github.com/acme/contracts" {
		t.Fatalf("unexpected alias map: %#v", resolved.Aliases)
	}
	if resolved.NativeDependencies["golang.org/x/text"] != "v0.29.0" {
		t.Fatalf("unexpected native dependencies: %#v", resolved.NativeDependencies)
	}
	first, err := os.ReadFile(TypeRBLockPath(config))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveTypeRBPackages(config, TypeRBResolveOptions{}); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(TypeRBLockPath(config))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("lock is not deterministic:\n%s\n%s", first, second)
	}
	if strings.Contains(string(first), packageRoot) {
		t.Fatalf("lock contains a machine-specific absolute path:\n%s", first)
	}
	if err := os.WriteFile(filepath.Join(packageRoot, "src", "index.trb"), []byte("record Changed\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTypeRBPackages(config); err != nil {
		t.Fatalf("local source edit required a relock: %v", err)
	}
	writeTestPackage(t, packageRoot, TypeRBManifest{
		Name: "github.com/acme/contracts", Version: "0.1.0", SourceDir: "src", Modes: []string{"go"},
	}, "record Changed\nend\n")
	if _, err := LoadTypeRBPackages(config); err == nil || !strings.Contains(err.Error(), "manifest changed") {
		t.Fatalf("local manifest drift was accepted: %v", err)
	}
	if _, err := ResolveTypeRBPackages(config, TypeRBResolveOptions{Frozen: true}); err == nil || !strings.Contains(err.Error(), "manifest changed") {
		t.Fatalf("frozen install accepted local manifest drift: %v", err)
	}
	if _, err := ResolveTypeRBPackages(config, TypeRBResolveOptions{}); err != nil {
		t.Fatalf("ordinary install did not refresh local manifest drift: %v", err)
	}
	if _, err := LoadTypeRBPackages(config); err != nil {
		t.Fatalf("refreshed local manifest lock did not load: %v", err)
	}
}

func TestResolveTypeRBPackageExposesDeclarationAdapter(t *testing.T) {
	workspace := t.TempDir()
	packageRoot := filepath.Join(workspace, "ui-types")
	writeTestPackage(t, packageRoot, TypeRBManifest{
		Name:    "github.com/acme/ui-types",
		Version: "0.1.0",
		Modes:   []string{"typescript"},
		NativeDependencies: map[string]map[string]string{
			"typescript": {"@acme/ui": "1.0.0"},
		},
		DeclarationAdapters: map[string]string{"typescript": "declarations.json"},
	}, "")
	adapterPath := filepath.Join(packageRoot, "declarations.json")
	if err := os.WriteFile(adapterPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	config := project.New(filepath.Join(workspace, "app"), "typescript")
	config.Packages["acme/ui-types"] = project.PackageRequirement{Path: "../ui-types"}
	resolved, err := ResolveTypeRBPackages(config, TypeRBResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.DeclarationAdapters) != 1 {
		t.Fatalf("unexpected declaration adapters: %#v", resolved.DeclarationAdapters)
	}
	adapter := resolved.DeclarationAdapters[0]
	if adapter.Package != "github.com/acme/ui-types" || adapter.Mode != "typescript" || adapter.Path != adapterPath {
		t.Fatalf("unexpected declaration adapter: %#v", adapter)
	}
	if adapter.Dependencies["@acme/ui"] != "1.0.0" {
		t.Fatalf("declaration adapter lost package-owned dependencies: %#v", adapter.Dependencies)
	}
}

func TestTypeRBPackageDeclarationAdapterDiagnosesLegacyAndUnavailableModes(t *testing.T) {
	workspace := t.TempDir()
	legacyRoot := filepath.Join(workspace, "legacy")
	writeTestPackage(t, legacyRoot, TypeRBManifest{
		Name: "github.com/acme/legacy", Version: "0.1.0", Modes: []string{"typescript"},
		LegacyNativeTypeProviders: map[string]string{"typescript": "native-types.json"},
	}, "")
	if _, err := ReadTypeRBManifest(legacyRoot); err == nil || !strings.Contains(err.Error(), "nativeTypeProviders has been replaced by declarationAdapters") {
		t.Fatalf("expected native type provider migration diagnostic, got %v", err)
	}

	rubyRoot := filepath.Join(workspace, "ruby-adapter")
	writeTestPackage(t, rubyRoot, TypeRBManifest{
		Name: "github.com/acme/ruby-adapter", Version: "0.1.0", Modes: []string{"ruby"},
		NativeDependencies:  map[string]map[string]string{"ruby": {"pagy": "9.4.0"}},
		DeclarationAdapters: map[string]string{"ruby": "declarations.json"},
	}, "")
	if err := os.WriteFile(filepath.Join(rubyRoot, "declarations.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	config := project.New(filepath.Join(workspace, "app"), "ruby")
	config.Packages["acme/ruby-adapter"] = project.PackageRequirement{Path: "../ruby-adapter"}
	if _, err := ResolveTypeRBPackages(config, TypeRBResolveOptions{}); err == nil || !strings.Contains(err.Error(), "requires runtimeAdapters.ruby") {
		t.Fatalf("expected unavailable Ruby adapter diagnostic, got %v", err)
	}
}

func TestResolveTypeRBPackageExposesNativeRuntimeAdapter(t *testing.T) {
	workspace := t.TempDir()
	packageRoot := filepath.Join(workspace, "aws-s3")
	writeTestPackage(t, packageRoot, TypeRBManifest{
		Name: "github.com/acme/aws-s3", Version: "0.1.0", Modes: []string{"ruby"},
		NativeDependencies:  map[string]map[string]string{"ruby": {"acme-aws-s3-wire": "0.1.0"}},
		DeclarationAdapters: map[string]string{"ruby": "declarations.json"},
		RuntimeAdapters:     map[string]string{"ruby": "runtime.json"},
	}, "")
	declarations := filepath.Join(packageRoot, "declarations.json")
	runtime := filepath.Join(packageRoot, "runtime.json")
	if err := os.WriteFile(declarations, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runtime, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	config := project.New(filepath.Join(workspace, "app"), "ruby")
	config.Packages["acme/aws-s3"] = project.PackageRequirement{Path: "../aws-s3"}
	resolved, err := ResolveTypeRBPackages(config, TypeRBResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.DeclarationAdapters) != 1 || len(resolved.RuntimeAdapters) != 1 {
		t.Fatalf("unexpected adapter sources: declarations=%#v runtime=%#v", resolved.DeclarationAdapters, resolved.RuntimeAdapters)
	}
	adapter := resolved.RuntimeAdapters[0]
	if adapter.Package != "github.com/acme/aws-s3" || adapter.Mode != "ruby" || adapter.Path != runtime || adapter.Dependencies["acme-aws-s3-wire"] != "0.1.0" {
		t.Fatalf("unexpected runtime adapter: %#v", adapter)
	}
}

func TestTypeRBManifestAcceptsExplicitContainedAdapterTest(t *testing.T) {
	root := t.TempDir()
	for _, directory := range []string{"src", "conformance"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "declarations.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "conformance", project.ConfigName), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{
		"formatVersion": 1,
		"name": "github.com/acme/ui-types",
		"version": "0.1.0",
		"modes": ["typescript"],
		"nativeDependencies": {"typescript": {"ui": "1.0.0"}},
		"declarationAdapters": {"typescript": "declarations.json"},
		"adapterTests": {
			"typescript": {
				"config": "conformance/trbconfig.jsonc",
				"command": ["bun", "run", "check"]
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(root, TypeRBManifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadTypeRBManifest(root); err != nil {
		t.Fatalf("explicit adapter test was rejected: %v", err)
	}
}

func TestTypeRBManifestRejectsUnsafeAdapterTestDeclarations(t *testing.T) {
	tests := []struct {
		name       string
		definition AdapterTest
		adapters   map[string]string
		want       string
	}{
		{name: "escaping config", definition: AdapterTest{Config: "../trbconfig.jsonc", Command: []string{"bun", "run", "check"}}, adapters: map[string]string{"typescript": "declarations.json"}, want: "config must stay below"},
		{name: "missing executable", definition: AdapterTest{Config: "conformance/trbconfig.jsonc"}, adapters: map[string]string{"typescript": "declarations.json"}, want: "command must contain an executable"},
		{name: "absolute executable", definition: AdapterTest{Config: "conformance/trbconfig.jsonc", Command: []string{"/usr/bin/env"}}, adapters: map[string]string{"typescript": "declarations.json"}, want: "must not be absolute"},
		{name: "newline argument", definition: AdapterTest{Config: "conformance/trbconfig.jsonc", Command: []string{"bun", "bad\nargument"}}, adapters: map[string]string{"typescript": "declarations.json"}, want: "must not contain NUL or newlines"},
		{name: "missing adapter", definition: AdapterTest{Config: "conformance/trbconfig.jsonc", Command: []string{"bun", "run", "check"}}, adapters: map[string]string{}, want: "requires declarationAdapters.typescript"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeTestPackage(t, root, TypeRBManifest{
				Name: "github.com/acme/ui-types", Version: "0.1.0", Modes: []string{"typescript"},
				NativeDependencies:  map[string]map[string]string{"typescript": {"ui": "1.0.0"}},
				DeclarationAdapters: test.adapters,
				AdapterTests:        map[string]AdapterTest{"typescript": test.definition},
			}, "")
			if err := os.WriteFile(filepath.Join(root, "declarations.json"), []byte("{}\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(root, "conformance"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "conformance", project.ConfigName), []byte("{}\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := ReadTypeRBManifest(root); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q, got %v", test.want, err)
			}
		})
	}
}

func TestResolveGitTypeRBPackagesLocksTransitiveContentAndSupportsOfflineUse(t *testing.T) {
	workspace := t.TempDir()
	baseRepository := filepath.Join(workspace, "base-repository")
	writeTestPackage(t, baseRepository, TypeRBManifest{
		Name: "github.com/acme/base", Version: "1.0.0", SourceDir: "src",
	}, "record Label\n\tvalue: String\nend\n")
	commitAndTag(t, baseRepository, "v1.0.0")

	contractRepository := filepath.Join(workspace, "contract-repository")
	writeTestPackage(t, contractRepository, TypeRBManifest{
		Name: "github.com/acme/contracts", Version: "1.1.0", SourceDir: "src",
		Packages: map[string]project.PackageRequirement{
			"acme/base": {Source: "file://" + baseRepository, Version: "v1.0.0"},
		},
	}, "import { Label } from acme/base\n\nrecord Message\n\tlabel: Label\nend\n")
	commitAndTag(t, contractRepository, "v1.1.0")

	appRoot := filepath.Join(workspace, "app")
	config := project.New(appRoot, "typescript")
	config.Packages["acme/contracts"] = project.PackageRequirement{Source: "file://" + contractRepository, Version: "latest"}
	resolved, err := ResolveTypeRBPackages(config, TypeRBResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Packages) != 2 {
		t.Fatalf("expected two packages, got %#v", resolved.Packages)
	}
	if got := resolved.Lock.Imports["acme/contracts"]; got != "github.com/acme/contracts" {
		t.Fatalf("unexpected root import: %q", got)
	}
	if got := resolved.Lock.Packages["github.com/acme/contracts"].Dependencies["acme/base"]; got != "github.com/acme/base" {
		t.Fatalf("unexpected transitive dependency: %q", got)
	}
	if _, exposed := resolved.Lock.Imports["acme/base"]; exposed {
		t.Fatalf("transitive alias leaked into application imports: %#v", resolved.Lock.Imports)
	}
	for name, locked := range resolved.Lock.Packages {
		if locked.Revision == "" || !strings.HasPrefix(locked.Checksum, typeRBChecksumPrefix) {
			t.Fatalf("package %s was not pinned: %#v", name, locked)
		}
	}
	if _, err := ResolveTypeRBPackages(config, TypeRBResolveOptions{Frozen: true, Offline: true}); err != nil {
		t.Fatalf("locked offline install failed: %v", err)
	}

	contract := resolved.Lock.Packages["github.com/acme/contracts"]
	cacheName, err := checksumCacheName(contract.Checksum)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appRoot, ".trb", "packages", cacheName, "src", "index.trb"), []byte("record Tampered\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTypeRBPackages(config); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("tampered package was accepted: %v", err)
	}
}

func TestFrozenTypeRBInstallRejectsConfigurationDrift(t *testing.T) {
	workspace := t.TempDir()
	packageRoot := filepath.Join(workspace, "package")
	writeTestPackage(t, packageRoot, TypeRBManifest{Name: "github.com/acme/contracts", Version: "0.1.0", SourceDir: "src"}, "record Message\nend\n")
	config := project.New(filepath.Join(workspace, "app"), "ruby")
	config.Packages["acme/contracts"] = project.PackageRequirement{Path: "../package"}
	if _, err := ResolveTypeRBPackages(config, TypeRBResolveOptions{}); err != nil {
		t.Fatal(err)
	}
	config.Packages["acme/other"] = project.PackageRequirement{Path: "../other"}
	if _, err := ResolveTypeRBPackages(config, TypeRBResolveOptions{Frozen: true}); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("frozen install accepted configuration drift: %v", err)
	}
}

func TestOfflineTypeRBInstallAllowsLocalPackagesAndEmptyProjects(t *testing.T) {
	workspace := t.TempDir()
	packageRoot := filepath.Join(workspace, "package")
	writeTestPackage(t, packageRoot, TypeRBManifest{Name: "github.com/acme/contracts", Version: "0.1.0", SourceDir: "src"}, "record Message\nend\n")
	config := project.New(filepath.Join(workspace, "app"), "ruby")
	config.Packages["acme/contracts"] = project.PackageRequirement{Path: "../package"}
	if _, err := ResolveTypeRBPackages(config, TypeRBResolveOptions{Offline: true}); err != nil {
		t.Fatalf("offline local package resolution failed: %v", err)
	}

	empty := project.New(filepath.Join(workspace, "empty"), "typescript")
	if _, err := ResolveTypeRBPackages(empty, TypeRBResolveOptions{Frozen: true, Offline: true}); err != nil {
		t.Fatalf("empty frozen offline install failed: %v", err)
	}
}

func TestTypeRBPackageDependencyCycleIsRejected(t *testing.T) {
	workspace := t.TempDir()
	packageRoot := filepath.Join(workspace, "package")
	writeTestPackage(t, packageRoot, TypeRBManifest{
		Name: "github.com/acme/cycle", Version: "0.1.0", SourceDir: "src",
		Packages: map[string]project.PackageRequirement{"self": {Path: "."}},
	}, "record Cyclic\nend\n")
	config := project.New(filepath.Join(workspace, "app"), "go")
	config.Go.Module = "example.com/app"
	config.Packages["acme/cycle"] = project.PackageRequirement{Path: "../package"}
	if _, err := ResolveTypeRBPackages(config, TypeRBResolveOptions{}); err == nil || !strings.Contains(err.Error(), "dependency cycle") {
		t.Fatalf("package dependency cycle was accepted: %v", err)
	}
}

func TestTypeRBManifestRejectsTrailingJSON(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	data := `{"formatVersion":1,"name":"github.com/acme/package","version":"1.0.0"} {}`
	if err := os.WriteFile(filepath.Join(root, TypeRBManifestName), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadTypeRBManifest(root); err == nil || !strings.Contains(err.Error(), "trailing JSON") {
		t.Fatalf("manifest trailing content was accepted: %v", err)
	}
}

func TestTypeRBPackageSourceRejectsEmbeddedCredentials(t *testing.T) {
	workspace := t.TempDir()
	config := project.New(filepath.Join(workspace, "app"), "ruby")
	config.Packages["acme/private"] = project.PackageRequirement{
		Source: "https://user:secret@example.com/acme/private.git", Version: "v1.0.0",
	}
	if _, err := ResolveTypeRBPackages(config, TypeRBResolveOptions{}); err == nil || !strings.Contains(err.Error(), "must not embed credentials") {
		t.Fatalf("embedded Git credentials were accepted: %v", err)
	}
}

func writeTestPackage(t *testing.T, root string, manifest TypeRBManifest, source string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest.FormatVersion = 1
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, TypeRBManifestName), append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "index.trb"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commitAndTag(t *testing.T, root, tag string) {
	t.Helper()
	for _, arguments := range [][]string{
		{"init", "--quiet"},
		{"config", "user.email", "packages@example.com"},
		{"config", "user.name", "TypeRB package test"},
		{"add", "."},
		{"commit", "--quiet", "-m", "Initial package"},
		{"tag", tag},
	} {
		command := exec.Command("git", arguments...)
		command.Dir = root
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(arguments, " "), err, output)
		}
	}
}
