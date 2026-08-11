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
