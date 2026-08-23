package compiler

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/nativepackage"
)

const nativeRuntimeAdapterSource = `import { invoke } from github.com/acme/runtime/native

def call_native(input: String): String
	return invoke(input)
end

def main()
	puts(call_native("payload"))
	return
end
`

func TestGeneratedNativeRuntimeAdapterCallsPackageShimAcrossBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			if mode == "ruby" {
				if _, err := exec.LookPath("ruby"); err != nil {
					t.Skip("ruby is not installed")
				}
			}
			if mode == "typescript" {
				if _, err := exec.LookPath("bun"); err != nil {
					t.Skip("bun is not installed")
				}
			}
			catalog := nativeRuntimeCompilerCatalog(mode)
			options := Options{Mode: mode, ModulePath: "main", NativePackages: catalog}
			if mode == "go" {
				options.Package = "main"
				options.GoModule = "example.com/runtime-app"
			}
			if mode == "typescript" {
				options.TypeScriptRuntime = "bun"
			}
			artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: options.Package, Source: []byte(nativeRuntimeAdapterSource)}}, options)
			if err != nil {
				t.Fatal(err)
			}
			if len(artifacts) != 1 {
				t.Fatalf("unexpected artifacts: %d", len(artifacts))
			}
			generated := string(artifacts[0].Output)
			for _, expected := range nativeRuntimeGeneratedMarkers(mode) {
				if !strings.Contains(generated, expected) {
					t.Fatalf("generated %s runtime adapter is missing %q:\n%s", mode, expected, generated)
				}
			}
			if unexpected := nativeRuntimeUnusedMarker(mode); strings.Contains(generated, unexpected) {
				t.Fatalf("generated %s runtime adapter imported unselected symbol %q:\n%s", mode, unexpected, generated)
			}
			root := t.TempDir()
			output := runNativeRuntimeArtifact(t, mode, root, artifacts[0].Output)
			if strings.TrimSpace(output) != "wire:payload" {
				t.Fatalf("unexpected %s runtime output: %q", mode, output)
			}
		})
	}
}

func TestGeneratedNativeRuntimeAdapterSupportsNamespaceImports(t *testing.T) {
	source := strings.Replace(nativeRuntimeAdapterSource,
		"import { invoke } from github.com/acme/runtime/native",
		"import github.com/acme/runtime/native as native_runtime", 1)
	source = strings.Replace(source, "return invoke(input)", "return native_runtime.invoke(input)", 1)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			options := Options{Mode: mode, ModulePath: "main", NativePackages: nativeRuntimeCompilerCatalog(mode)}
			if mode == "go" {
				options.Package = "main"
				options.GoModule = "example.com/runtime-app"
			}
			artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: options.Package, Source: []byte(source)}}, options)
			if err != nil {
				t.Fatal(err)
			}
			generated := string(artifacts[0].Output)
			for _, expected := range nativeRuntimeGeneratedMarkers(mode) {
				if !strings.Contains(generated, expected) {
					t.Fatalf("generated %s namespace runtime adapter is missing %q:\n%s", mode, expected, generated)
				}
			}
		})
	}
}

func nativeRuntimeCompilerCatalog(mode string) *nativepackage.Catalog {
	dependency, module, symbol := compilerRuntimeTarget(mode)
	stringType := nativepackage.Type{Kind: "string", Name: "String"}
	identity := "github.com/acme/runtime/native#invoke"
	return &nativepackage.Catalog{
		FormatVersion: nativepackage.FormatVersion,
		Dependencies:  map[string]string{dependency: "1.0.0"},
		Modules: map[string]nativepackage.Module{
			"github.com/acme/runtime/native": {Exports: map[string]nativepackage.Export{
				"invoke": {
					Kind: "function", Type: stringType, Parameters: []nativepackage.Type{stringType}, Required: 1,
					Runtime: &nativepackage.RuntimeBinding{
						Identity: identity, Dependency: dependency, Module: module, Symbol: symbol, CallConvention: "function",
						MaySuspend: true, PropagatesExecutionScope: true,
					},
				},
				"unused": {
					Kind: "function", Type: stringType, Parameters: []nativepackage.Type{stringType}, Required: 1,
					Runtime: &nativepackage.RuntimeBinding{
						Identity: "github.com/acme/runtime/native#unused", Dependency: dependency, Module: module,
						Symbol: nativeRuntimeUnusedMarker(mode), CallConvention: "function",
					},
				},
			}},
		},
	}
}

func compilerRuntimeTarget(mode string) (dependency, module, symbol string) {
	switch mode {
	case "go":
		return "example.com/runtime-wire", "example.com/runtime-wire", "Invoke"
	case "ruby":
		return "acme-runtime-wire", "acme/runtime_wire", "Acme::RuntimeWire.invoke"
	default:
		return "@acme/runtime-wire", "@acme/runtime-wire", "invoke"
	}
}

func nativeRuntimeGeneratedMarkers(mode string) []string {
	switch mode {
	case "go":
		return []string{`"example.com/runtime-wire"`, "func CallNative(__trbScope", "runtimeWire.Invoke(__trbScope, input)"}
	case "ruby":
		return []string{`require "acme/runtime_wire"`, "def call_native(__trb_scope, input)", "Acme::RuntimeWire.invoke(__trb_scope, input)"}
	default:
		return []string{`from "@acme/runtime-wire"`, "export async function call_native(__trbScope", "await __trb_runtime_"}
	}
}

func nativeRuntimeUnusedMarker(mode string) string {
	switch mode {
	case "go":
		return "UnusedInvoke"
	case "ruby":
		return "Acme::RuntimeWire.unused"
	default:
		return "unusedInvoke"
	}
}

func runNativeRuntimeArtifact(t *testing.T, mode, root string, source []byte) string {
	t.Helper()
	switch mode {
	case "go":
		writeCompilerRuntimeFile(t, filepath.Join(root, "main.go"), source)
		writeCompilerRuntimeFile(t, filepath.Join(root, "go.mod"), []byte("module example.com/runtime-app\n\ngo 1.27\n\nrequire example.com/runtime-wire v0.0.0\n\nreplace example.com/runtime-wire => ./runtime-wire\n"))
		writeCompilerRuntimeFile(t, filepath.Join(root, "runtime-wire", "go.mod"), []byte("module example.com/runtime-wire\n\ngo 1.27\n"))
		writeCompilerRuntimeFile(t, filepath.Join(root, "runtime-wire", "wire.go"), []byte("package runtimewire\n\nimport \"context\"\n\nfunc Invoke(scope context.Context, input string) string {\n\tif scope == nil { panic(\"missing scope\") }\n\treturn \"wire:\" + input\n}\n"))
		command := exec.Command("go", "run", ".")
		command.Dir = root
		command.Env = append(os.Environ(), "GOCACHE="+filepath.Join(root, "go-cache"))
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("generated Go runtime adapter failed: %v\n%s", err, output)
		}
		return string(output)
	case "ruby":
		writeCompilerRuntimeFile(t, filepath.Join(root, "main.rb"), source)
		nativeRoot := filepath.Join(root, "native")
		writeCompilerRuntimeFile(t, filepath.Join(nativeRoot, "acme", "runtime_wire.rb"), []byte("module Acme\n  module RuntimeWire\n    def self.invoke(scope, input)\n      scope.check!\n      \"wire:\" + input\n    end\n  end\nend\n"))
		command := exec.Command("ruby", "-I", nativeRoot, "main.rb")
		command.Dir = root
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("generated Ruby runtime adapter failed: %v\n%s", err, output)
		}
		return string(output)
	default:
		writeCompilerRuntimeFile(t, filepath.Join(root, "main.ts"), source)
		moduleRoot := filepath.Join(root, "node_modules", "@acme", "runtime-wire")
		writeCompilerRuntimeFile(t, filepath.Join(moduleRoot, "package.json"), []byte(`{"name":"@acme/runtime-wire","type":"module","exports":"./index.ts"}`))
		writeCompilerRuntimeFile(t, filepath.Join(moduleRoot, "index.ts"), []byte("export async function invoke(scope: AbortSignal | undefined, input: string): Promise<string> {\n  if (scope?.aborted) throw new Error(\"cancelled\");\n  return \"wire:\" + input;\n}\n"))
		command := exec.Command("bun", "run", "main.ts")
		command.Dir = root
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("generated TypeScript runtime adapter failed: %v\n%s", err, output)
		}
		return string(output)
	}
}

func writeCompilerRuntimeFile(t *testing.T, path string, contents []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}
}
