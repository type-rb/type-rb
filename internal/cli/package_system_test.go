package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/codegen"
	packageManager "github.com/type-rb/type-rb/internal/packages"
	"github.com/type-rb/type-rb/internal/project"
)

func TestBuildCompilesLockedTypeRBPackageAcrossBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			workspace := t.TempDir()
			packageRoot := filepath.Join(workspace, "contracts")
			nativeName, nativeVersion := packageNativeFixture(mode)
			writeCLIPackageFixture(t, packageRoot, map[string]map[string]string{
				mode: {nativeName: nativeVersion},
			})

			appRoot := filepath.Join(workspace, "app")
			if err := os.MkdirAll(filepath.Join(appRoot, "src"), 0o755); err != nil {
				t.Fatal(err)
			}
			config := project.New(appRoot, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/package-app"
			}
			config.Packages["acme/contracts"] = project.PackageRequirement{Path: "../contracts"}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if _, err := packageManager.ResolveTypeRBPackages(config, packageManager.TypeRBResolveOptions{}); err != nil {
				t.Fatal(err)
			}
			main := `import { Message, default_text } from acme/contracts

def main()
	message := Message.new(text: default_text())
	puts(message.text)
	return
end
`
			if err := os.WriteFile(filepath.Join(appRoot, "src", "main.trb"), []byte(main), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"build", "--config", config.Path}); status != 0 {
				t.Fatalf("status=%d stderr=%s", status, stderr.String())
			}
			packageOutput := filepath.Join(appRoot, "build", "github.com", "acme", "contracts", "index"+codegen.Extension(mode))
			if _, err := os.Stat(packageOutput); err != nil {
				t.Fatalf("external package output is missing: %v", err)
			}
			sharedOutput := filepath.Join(appRoot, "build", "github.com", "acme", "shared", "index"+codegen.Extension(mode))
			if _, err := os.Stat(sharedOutput); err != nil {
				t.Fatalf("transitive package output is missing: %v", err)
			}
			mainOutput, err := os.ReadFile(filepath.Join(appRoot, "build", "main"+codegen.Extension(mode)))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(mainOutput), "contracts") {
				t.Fatalf("generated application did not reference the canonical package:\n%s", mainOutput)
			}
			manifest := targetManifestPath(config)
			contents, err := os.ReadFile(manifest)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(contents), nativeName) || !strings.Contains(string(contents), nativeVersion) {
				t.Fatalf("package native dependency is missing from %s:\n%s", manifest, contents)
			}
		})
	}
}

func TestAddAndRemoveLocalTypeRBPackage(t *testing.T) {
	workspace := t.TempDir()
	packageRoot := filepath.Join(workspace, "contracts")
	writeCLIPackageFixture(t, packageRoot, nil)
	appRoot := filepath.Join(workspace, "app")
	config := project.New(appRoot, "ruby")
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	t.Chdir(appRoot)

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"add", "--path", "../contracts", "acme/contracts"}); status != 0 {
		t.Fatalf("add status=%d stderr=%s", status, stderr.String())
	}
	loaded, err := project.Load(config.Path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Packages["acme/contracts"].Path != "../contracts" {
		t.Fatalf("package was not saved: %#v", loaded.Packages)
	}
	if _, err := os.Stat(packageManager.TypeRBLockPath(loaded)); err != nil {
		t.Fatalf("lock was not created: %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	if status := command.Run([]string{"remove", "acme/contracts"}); status != 0 {
		t.Fatalf("remove status=%d stderr=%s", status, stderr.String())
	}
	loaded, err = project.Load(config.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Packages) != 0 {
		t.Fatalf("package was not removed: %#v", loaded.Packages)
	}
}

func TestProjectWalkSkipsTypeRBPackageCache(t *testing.T) {
	root := t.TempDir()
	mainPath := filepath.Join(root, "main.trb")
	cacheSource := filepath.Join(root, ".trb", "packages", "checksum", "src", "index.trb")
	if err := os.MkdirAll(filepath.Dir(cacheSource), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainPath, []byte("def main()\n\treturn\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cacheSource, []byte("record Cached\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := collectTRB([]string{root}, filepath.Join(root, "build"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != mainPath {
		t.Fatalf("compiler state leaked into project sources: %#v", files)
	}

	output := filepath.Join(root, "build")
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyProjectFiles(root, output); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(output, ".trb")); !os.IsNotExist(err) {
		t.Fatalf("compiler state was copied into build output: %v", err)
	}
}

func TestReplLoadsLockedTypeRBPackages(t *testing.T) {
	workspace := t.TempDir()
	packageRoot := filepath.Join(workspace, "contracts")
	writeCLIPackageFixture(t, packageRoot, nil)
	appRoot := filepath.Join(workspace, "app")
	if err := os.MkdirAll(filepath.Join(appRoot, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := project.New(appRoot, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/package-repl"
	config.Packages["acme/contracts"] = project.PackageRequirement{Path: "../contracts"}
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := packageManager.ResolveTypeRBPackages(config, packageManager.TypeRBResolveOptions{}); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{
		Stdin:  strings.NewReader("default_text()\nMessage.new(text: default_text()).text\n:quit\n"),
		Stdout: &stdout,
		Stderr: &stderr,
	}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	if strings.Count(stdout.String(), `"shared" : String`) != 2 {
		t.Fatalf("package declarations were not evaluated in the REPL:\n%s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("package REPL reported errors: %s", stderr.String())
	}
}

func writeCLIPackageFixture(t *testing.T, root string, native map[string]map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	sharedRoot := filepath.Join(root, "shared")
	if err := os.MkdirAll(filepath.Join(sharedRoot, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := packageManager.TypeRBManifest{
		FormatVersion: 1, Name: "github.com/acme/contracts", Version: "0.1.0", SourceDir: "src", NativeDependencies: native,
		Packages: map[string]project.PackageRequirement{"acme/shared": {Path: "shared"}},
	}
	writeCLIPackageManifest(t, root, manifest)
	writeCLIPackageManifest(t, sharedRoot, packageManager.TypeRBManifest{
		FormatVersion: 1, Name: "github.com/acme/shared", Version: "0.1.0", SourceDir: "src",
	})
	source := `import { shared_text } from acme/shared

record Message
	text: String
end

def default_text(): String
	return shared_text()
end
`
	if err := os.WriteFile(filepath.Join(root, "src", "index.trb"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sharedRoot, "src", "index.trb"), []byte("def shared_text(): String\n\treturn \"shared\"\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeCLIPackageManifest(t *testing.T, root string, manifest packageManager.TypeRBManifest) {
	t.Helper()
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, packageManager.TypeRBManifestName), append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func packageNativeFixture(mode string) (string, string) {
	switch mode {
	case "go":
		return "example.com/native/package", "v1.2.3"
	case "ruby":
		return "native-package", "1.2.3"
	default:
		return "@acme/native-package", "1.2.3"
	}
}

func targetManifestPath(config *project.Config) string {
	switch config.Mode {
	case "go":
		return filepath.Join(config.Root, "go.mod")
	case "ruby":
		return filepath.Join(config.Root, "Gemfile")
	default:
		return filepath.Join(config.Root, "package.json")
	}
}
