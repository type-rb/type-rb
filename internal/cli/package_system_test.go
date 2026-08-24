package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/codegen"
	"github.com/type-rb/type-rb/internal/nativepackage"
	"github.com/type-rb/type-rb/internal/packageextension"
	packageManager "github.com/type-rb/type-rb/internal/packages"
	"github.com/type-rb/type-rb/internal/project"
	"github.com/type-rb/type-rb/internal/runtimeadapterhost"
)

func TestBuildLoadsAWSS3PackageRuntimeAdapterAcrossBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			workspace := t.TempDir()
			packageRoot := filepath.Join(workspace, "aws-s3")
			if err := os.MkdirAll(filepath.Join(packageRoot, "src"), 0o755); err != nil {
				t.Fatal(err)
			}
			dependency, targetModule, targetSymbol := cliRuntimeTarget(mode)
			dependencyVersion := "1.0.0"
			if mode == "go" {
				dependencyVersion = "v1.0.0"
			}
			manifest := packageManager.TypeRBManifest{
				FormatVersion: 1, Name: "github.com/acme/aws-s3", Version: "0.1.0", SourceDir: "src", Modes: []string{mode},
				NativeDependencies:  map[string]map[string]string{mode: {dependency: dependencyVersion}},
				DeclarationAdapters: map[string]string{mode: "declarations.json"},
				RuntimeAdapters:     map[string]string{mode: "runtime.json"},
			}
			writeCLIPackageManifest(t, packageRoot, manifest)
			packageSource := `import { head_object } from github.com/acme/aws-s3/native

def head_object_wire(input: String): String
	return head_object(input)
end
`
			if err := os.WriteFile(filepath.Join(packageRoot, "src", "index.trb"), []byte(packageSource), 0o644); err != nil {
				t.Fatal(err)
			}
			declarations := packageextension.DeclarationAdapterCatalog{
				ProtocolVersion: packageextension.DeclarationAdapterProtocolVersion,
				Modules: map[string]packageextension.DeclarationAdapterModule{
					"github.com/acme/aws-s3/native": {Exports: map[string]packageextension.DeclarationAdapterExport{
						"head_object": {
							Kind: "function", Type: packageextension.DeclarationAdapterType{Kind: "string", Name: "String"},
							Parameters: []packageextension.DeclarationAdapterType{{Kind: "string", Name: "String"}}, Required: 1,
						},
					}},
				},
			}
			writeCLIJSONFile(t, filepath.Join(packageRoot, "declarations.json"), declarations)
			runtimeProtocol := packageextension.NativeRuntimeAdapterCatalog{
				ProtocolVersion: packageextension.NativeRuntimeAdapterProtocolVersion,
				Bindings: map[string]packageextension.NativeRuntimeAdapterBinding{
					"github.com/acme/aws-s3/native#head_object": {
						Dependency: dependency, Module: targetModule, Symbol: targetSymbol, CallConvention: "function",
						MaySuspend: true, PropagatesExecutionScope: true,
					},
				},
			}
			writeCLIJSONFile(t, filepath.Join(packageRoot, "runtime.json"), runtimeProtocol)

			appRoot := filepath.Join(workspace, "app")
			if err := os.MkdirAll(filepath.Join(appRoot, "src"), 0o755); err != nil {
				t.Fatal(err)
			}
			config := project.New(appRoot, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/aws-s3-app"
			}
			if config.TypeScript != nil {
				config.TypeScript.Runtime = project.TypeScriptRuntimeBun
			}
			config.Packages["acme/aws-s3"] = project.PackageRequirement{Path: "../aws-s3"}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(appRoot, "src", "main.trb"), []byte("import { head_object_wire } from acme/aws-s3\n\ndef main()\n\tputs(head_object_wire(\"{\\\"bucket\\\":\\\"demo\\\",\\\"key\\\":\\\"object\\\"}\"))\n\treturn\nend\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			resolved, err := packageManager.ResolveTypeRBPackages(config, packageManager.TypeRBResolveOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if mode == "typescript" {
				declarationSources := declarationAdapterSources(resolved)
				runtimeAdapters, err := runtimeadapterhost.Load(runtimeAdapterSources(resolved))
				if err != nil {
					t.Fatal(err)
				}
				catalog := nativepackage.Empty(resolved.NativeDependencies)
				if err := nativepackage.ApplyDeclarationAdapterFilesWithRuntime(catalog, declarationSources, runtimeAdapters); err != nil {
					t.Fatal(err)
				}
				if err := nativepackage.Write(config.Root, catalog); err != nil {
					t.Fatal(err)
				}
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"build", "--config", config.Path}); status != 0 {
				t.Fatalf("status=%d stderr=%s", status, stderr.String())
			}
			packageOutput := filepath.Join(appRoot, "build", "github.com", "acme", "aws-s3", "index"+codegen.Extension(mode))
			generated, err := os.ReadFile(packageOutput)
			if err != nil {
				t.Fatal(err)
			}
			for _, expected := range cliRuntimeGeneratedMarkers(mode, targetModule, targetSymbol) {
				if !strings.Contains(string(generated), expected) {
					t.Fatalf("generated %s package runtime is missing %q:\n%s", mode, expected, generated)
				}
			}
		})
	}
}

func cliRuntimeTarget(mode string) (dependency, module, symbol string) {
	switch mode {
	case "go":
		return "github.com/acme/aws-s3-wire", "github.com/acme/aws-s3-wire/s3", "HeadObject"
	case "ruby":
		return "acme-aws-s3-wire", "acme/aws_s3_wire", "Acme::AwsS3Wire.head_object"
	default:
		return "@acme/aws-s3-wire", "@acme/aws-s3-wire/s3", "headObject"
	}
}

func cliRuntimeGeneratedMarkers(mode, module, symbol string) []string {
	switch mode {
	case "go":
		return []string{`"` + module + `"`, symbol + "(__trbScope, input)"}
	case "ruby":
		return []string{`require "` + module + `"`, symbol + "(__trb_scope, input)"}
	default:
		return []string{`from "` + module + `"`, "await __trb_runtime_"}
	}
}

func writeCLIJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildRunsRubyPackageWithFixedDeclarationProvider(t *testing.T) {
	if _, err := exec.LookPath("ruby"); err != nil {
		t.Skip("ruby is not installed")
	}
	workspace := t.TempDir()
	packageRoot := filepath.Join(workspace, "pagy")
	if err := os.MkdirAll(filepath.Join(packageRoot, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeCLIPackageManifest(t, packageRoot, packageManager.TypeRBManifest{
		FormatVersion: 1,
		Name:          "github.com/acme/pagy",
		Version:       "0.1.0",
		SourceDir:     "src",
		Modes:         []string{"ruby"},
		NativeDependencies: map[string]map[string]string{
			"ruby": {"pagy": "43.6.1"},
		},
		DeclarationProviders: map[string]string{"ruby": "declarations.json"},
	})
	if err := os.WriteFile(filepath.Join(packageRoot, "src", "index.trb"), []byte("import trb/platform/ruby/native\n\nrequire \"pagy\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeCLIJSONFile(t, filepath.Join(packageRoot, "declarations.json"), pagyDeclarationCatalog())

	appRoot := filepath.Join(workspace, "app")
	if err := os.MkdirAll(filepath.Join(appRoot, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := project.New(appRoot, "ruby")
	config.SourceDir = "src"
	config.Packages["acme/pagy"] = project.PackageRequirement{Path: "../pagy"}
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	application := `import acme/pagy
import trb/platform/ruby/native

class PageExample
	include Pagy::Method

	def first_page(): String
		page_result := pagy(:offset, ["first", "second"], limit: 1)
		pagination := page_result[0]
		records := page_result[1]
		puts(pagination.page)
		return records[0]
	end
end

def main()
	puts(PageExample.new().first_page())
	return
end
`
	if err := os.WriteFile(filepath.Join(appRoot, "src", "main.trb"), []byte(application), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := packageManager.ResolveTypeRBPackages(config, packageManager.TypeRBResolveOptions{}); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"build", "--config", config.Path}); status != 0 {
		t.Fatalf("build status=%d stderr=%s", status, stderr.String())
	}
	packageOutput, err := os.ReadFile(filepath.Join(appRoot, "build", "github.com", "acme", "pagy", "index.rb"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(packageOutput), `require "pagy"`) {
		t.Fatalf("package root did not load its native dependency:\n%s", packageOutput)
	}
	mainOutput, err := os.ReadFile(filepath.Join(appRoot, "build", "main.rb"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainOutput), `require_relative "./github.com/acme/pagy/index"`) {
		t.Fatalf("application did not load the declaration provider package:\n%s", mainOutput)
	}
	gemfile, err := os.ReadFile(filepath.Join(appRoot, "Gemfile"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gemfile), `gem "pagy", "43.6.1"`) {
		t.Fatalf("package native dependency is missing from Gemfile:\n%s", gemfile)
	}

	fakeLibrary := filepath.Join(workspace, "fake-ruby-library")
	if err := os.MkdirAll(fakeLibrary, 0o755); err != nil {
		t.Fatal(err)
	}
	fakePagy := `class Pagy
  class Offset
    attr_reader :page

    def initialize(page)
      @page = page
    end
  end

  module Method
    def pagy(_kind, collection, limit:)
      [Pagy::Offset.new(1), collection.first(limit)]
    end
  end
end
`
	if err := os.WriteFile(filepath.Join(fakeLibrary, "pagy.rb"), []byte(fakePagy), 0o644); err != nil {
		t.Fatal(err)
	}
	run := exec.Command("ruby", "-I", fakeLibrary, filepath.Join(appRoot, "build", "main.rb"))
	run.Dir = appRoot
	generatedOutput, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("generated Ruby failed: %v\n%s", err, generatedOutput)
	}
	if string(generatedOutput) != "1\nfirst\n" {
		t.Fatalf("unexpected generated Ruby output: %q", generatedOutput)
	}
}

func pagyDeclarationCatalog() packageextension.DeclarationCatalog {
	typeT := packageextension.Type{Kind: "named", Name: "T"}
	stringType := packageextension.Type{Kind: "string", Name: "String"}
	integerType := packageextension.Type{Kind: "int", Name: "Integer"}
	arrayT := packageextension.Type{Kind: "array", Name: "Array", Arguments: []packageextension.Type{typeT}}
	returnType := packageextension.Type{
		Kind: "named", Name: "Tuple",
		Arguments: []packageextension.Type{{Kind: "named", Name: "Pagy::Offset"}, arrayT},
	}
	return packageextension.DeclarationCatalog{
		ProtocolVersion: packageextension.DeclarationProtocolVersion,
		Provider:        "github.com/acme/pagy",
		Types: []packageextension.DeclaredType{{
			Name: "Pagy::Offset",
			InstanceMembers: []packageextension.DeclaredMember{{
				Name: "page", Kind: "property", Return: integerType,
			}},
		}},
		Modules: []packageextension.DeclaredModule{{
			Name: "Pagy::Method",
			InstanceMembers: []packageextension.DeclaredMember{{
				Name: "pagy", Kind: "method", TypeParameters: []string{"T"}, Return: returnType,
				Parameters: []packageextension.DeclaredParameter{
					{Name: "paginator", Type: stringType, LiteralValues: []string{"offset"}},
					{Name: "collection", Type: arrayT},
					{Name: "limit", Type: integerType, Keyword: true, Optional: true},
				},
			}},
		}},
	}
}

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

func TestUpdateSelectsDirectTypeRBPackage(t *testing.T) {
	workspace := t.TempDir()
	contractsRoot := filepath.Join(workspace, "contracts")
	writeCLIPackageFixture(t, contractsRoot, nil)
	otherRoot := filepath.Join(workspace, "other")
	writeCLIRemotePackageFixture(t, otherRoot, packageManager.TypeRBManifest{
		FormatVersion: 1, Name: "github.com/acme/other", Version: "0.1.0", SourceDir: "src",
	}, "record Other\nend\n")
	appRoot := filepath.Join(workspace, "app")
	config := project.New(appRoot, "ruby")
	config.Packages["acme/contracts"] = project.PackageRequirement{Path: "../contracts"}
	config.Packages["acme/other"] = project.PackageRequirement{Path: "../other"}
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := packageManager.ResolveTypeRBPackages(config, packageManager.TypeRBResolveOptions{}); err != nil {
		t.Fatal(err)
	}
	writeCLIPackageManifest(t, contractsRoot, packageManager.TypeRBManifest{
		FormatVersion: 1, Name: "github.com/acme/contracts", Version: "0.2.0", SourceDir: "src",
		Packages: map[string]project.PackageRequirement{"acme/shared": {Path: "shared"}},
	})
	t.Chdir(appRoot)

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"update", "acme/contracts"}); status != 0 {
		t.Fatalf("update status=%d stderr=%s", status, stderr.String())
	}
	lock, err := packageManager.ReadTypeRBLock(packageManager.TypeRBLockPath(config))
	if err != nil {
		t.Fatal(err)
	}
	if got := lock.Packages["github.com/acme/contracts"].Version; got != "0.2.0" {
		t.Fatalf("selected package version=%q", got)
	}
	if got := lock.Packages["github.com/acme/other"].Version; got != "0.1.0" {
		t.Fatalf("unselected package version=%q", got)
	}
	if !strings.Contains(stdout.String(), "updated selected package graph(s): acme/contracts") {
		t.Fatalf("unexpected update output: %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if status := command.Run([]string{"update", "acme/missing"}); status == 0 || !strings.Contains(stderr.String(), "not a direct project dependency") {
		t.Fatalf("missing update status=%d stderr=%s", status, stderr.String())
	}
}

func TestInstallLoadsExplicitConfig(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "ruby")
	config.Path = filepath.Join(root, "trbconfig.ruby.jsonc")
	config.PackageManagement = project.ExternalPackages
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	workingDirectory := t.TempDir()
	t.Chdir(workingDirectory)

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"install", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	if stdout.String() != "native package management is external\n" {
		t.Fatalf("unexpected install output: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("install reported errors: %s", stderr.String())
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

func TestRunLockedGitTypeRBPackagesAcrossBackends(t *testing.T) {
	workspace := t.TempDir()
	sharedRoot := filepath.Join(workspace, "shared")
	writeCLIRemotePackageFixture(t, sharedRoot, packageManager.TypeRBManifest{
		FormatVersion: 1, Name: "github.com/acme/shared", Version: "1.0.0", SourceDir: "src",
	}, "record Label\n\tvalue: String\nend\n\ndef shared_label(): Label\n\treturn Label.new(value: \"shared\")\nend\n")
	commitAndTagCLIPackageFixture(t, sharedRoot, "v1.0.0")

	contractsRoot := filepath.Join(workspace, "contracts")
	writeCLIRemotePackageFixture(t, contractsRoot, packageManager.TypeRBManifest{
		FormatVersion: 1, Name: "github.com/acme/contracts", Version: "1.1.0", SourceDir: "src",
		Packages: map[string]project.PackageRequirement{
			"acme/shared": {Source: "file://" + sharedRoot, Version: "v1.0.0"},
		},
	}, "import { Label, shared_label } from acme/shared\n\nrecord Message\n\tlabel: Label\nend\n\ndef default_message(): Message\n\treturn Message.new(label: shared_label())\nend\n")
	commitAndTagCLIPackageFixture(t, contractsRoot, "v1.1.0")

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			if mode == "ruby" {
				if _, err := exec.LookPath("ruby"); err != nil {
					t.Skip("ruby is not installed")
				}
			}
			if mode == "typescript" {
				if _, err := exec.LookPath("node"); err != nil {
					t.Skip("node is not installed")
				}
			}
			appRoot := filepath.Join(workspace, "apps", mode)
			if err := os.MkdirAll(appRoot, 0o755); err != nil {
				t.Fatal(err)
			}
			config := project.New(appRoot, mode)
			if config.Go != nil {
				config.Go.Module = "example.com/package-run"
			}
			config.Packages["acme/contracts"] = project.PackageRequirement{Source: "file://" + contractsRoot, Version: "latest"}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(appRoot, "main.trb"), []byte("import { default_message } from acme/contracts\n\ndef main()\n\tputs(default_message().label.value)\n\treturn\nend\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := packageManager.ResolveTypeRBPackages(config, packageManager.TypeRBResolveOptions{}); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"build", "--config", config.Path}); status != 0 {
				t.Fatalf("build status=%d stderr=%s", status, stderr.String())
			}
			packageOutput := filepath.Join(config.OutputPath(), "github.com", "acme", "contracts", "index"+codegen.Extension(mode))
			if _, err := os.Stat(packageOutput); err != nil {
				t.Fatalf("remote package output is missing: %v", err)
			}
			stdout.Reset()
			stderr.Reset()
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
				t.Fatalf("status=%d stderr=%s", status, stderr.String())
			}
			if stdout.String() != "shared\n" {
				t.Fatalf("unexpected package run output: %q", stdout.String())
			}
		})
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

func writeCLIRemotePackageFixture(t *testing.T, root string, manifest packageManager.TypeRBManifest, source string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeCLIPackageManifest(t, root, manifest)
	if err := os.WriteFile(filepath.Join(root, "src", "index.trb"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commitAndTagCLIPackageFixture(t *testing.T, root, tag string) {
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
