package compiler

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/nativepackage"
)

const nativeNullishDeclarations = `
export function missing(): string | undefined;
export function empty(): string | undefined;
export function zero(): number | undefined;
export function falsy(): boolean | undefined;
export function absent(): undefined;
export function nullValue(): null;
export function either(flag: boolean): string | null | undefined;
export function describe(value: string | undefined): string;
export function onlyUndefined(value: undefined): string;
export function nullable(value: string | null): string;
export function both(value: string | null | undefined): string;
export function optional(value?: string): string;
export function numberInput(value: number | undefined): string;
export function mark(label: string): string | undefined;
export function pair(left: string | undefined, right: string | undefined): string;
export function arrayValue(): string[] | undefined;
export function sameArray(value: string[] | undefined): boolean;
export function invoke(callback: (value: string | null) => string): string;
export function nested(): (string | undefined)[];
export function rest(...values: (string | undefined)[]): void;
export function callbackInput(callback: (value: string | undefined) => string): void;
export function callbackOutput(): () => string | undefined;
`

const nativeNullishImplementation = `
const saved = ["identity"];
export function missing() { return undefined; }
export function empty() { return ""; }
export function zero() { return 0; }
export function falsy() { return false; }
export function absent() { return undefined; }
export function nullValue() { return null; }
export function either(flag) { return flag ? null : undefined; }
export function describe(value) { return value === undefined ? "undefined" : String(value); }
export function onlyUndefined(value) { return describe(value); }
export function nullable(value) { return value === null ? "null" : String(value); }
export function both(value) { return value === null ? "null" : describe(value); }
export function optional(value) { return arguments.length + ":" + describe(value); }
export function numberInput(value) { return describe(value); }
export function mark(label) { console.log(label); return undefined; }
export function pair(left, right) { return describe(left) + ":" + describe(right); }
export function arrayValue() { return saved; }
export function sameArray(value) { return value === saved; }
export function invoke(callback) { return callback(null); }
`

const nativeNullishSource = `import { absent, arrayValue, both, describe as format, either, empty, falsy, invoke, mark, missing, nullable, nullValue, numberInput, onlyUndefined, optional, pair, sameArray, zero } from "native-values"

def read(): String?
	return missing()
end

def main()
	puts(read() == nil)
	puts(either(true) == nil)
	puts(either(false) == nil)
	puts(absent() == nil)
	puts(nullValue() == nil)
	puts(format(nil))
	puts(format("value"))
	puts(format(read()))
	puts(onlyUndefined(absent()))
	puts(onlyUndefined(nil))
	puts(nullable(nil))
	puts(nullable(missing()))
	puts(both(nil))
	puts(optional())
	puts(optional(nil))
	puts(optional("value"))
	puts(numberInput(42))
	puts(numberInput(nil))
	puts(format(empty()) == "")
	puts(numberInput(zero()) == "0")
	false_value := falsy()
	if false_value == nil
		puts(false)
	else
		puts(false_value == false)
	end
	puts(pair(mark("left"), mark("right")))
	puts(sameArray(arrayValue()))
	wrapper := fn(value: String?): String
		return format(value)
	end
	puts(invoke(wrapper))
	return
end
`

func nativeNullishCatalog() *nativepackage.Catalog {
	stringType := nativepackage.Type{Kind: "string", Name: "String"}
	nilString := func(representation string) nativepackage.Type {
		return nativepackage.Type{Kind: "string", Name: "String", Nullable: true, NativeNil: representation}
	}
	undefined := nativepackage.Type{Kind: "nil", Name: "Nil", NativeNil: "undefined"}
	array := nativepackage.Type{Kind: "array", Name: "Array", Args: []nativepackage.Type{stringType}, Nullable: true, NativeNil: "undefined"}
	boolean := nativepackage.Type{Kind: "bool", Name: "Boolean"}
	number := nativepackage.Type{Kind: "float", Name: "Float", Nullable: true, NativeNil: "undefined"}
	function := func(result nativepackage.Type, parameters ...nativepackage.Type) nativepackage.Export {
		return nativepackage.Export{Kind: "function", Type: result, Parameters: parameters, Required: len(parameters)}
	}
	exports := map[string]nativepackage.Export{
		"missing": function(nilString("undefined")), "empty": function(nilString("undefined")),
		"zero": function(number), "falsy": function(nativepackage.Type{Kind: "bool", Name: "Boolean", Nullable: true, NativeNil: "undefined"}),
		"absent": function(undefined), "nullValue": function(nativepackage.Type{Kind: "nil", Name: "Nil", NativeNil: "null"}),
		"either":   function(nilString("null_or_undefined"), boolean),
		"describe": function(stringType, nilString("undefined")), "onlyUndefined": function(stringType, undefined),
		"nullable": function(stringType, nilString("null")), "both": function(stringType, nilString("null_or_undefined")),
		"optional": function(stringType, nilString("undefined")), "numberInput": function(stringType, number),
		"mark": function(nilString("undefined"), stringType), "pair": function(stringType, nilString("undefined"), nilString("undefined")),
		"arrayValue": function(array), "sameArray": function(boolean, array),
		"invoke": function(stringType, nativepackage.Type{Kind: "function", Name: "Function", Args: []nativepackage.Type{nilString("null"), stringType}}),
	}
	optional := exports["optional"]
	optional.Required = 0
	exports["optional"] = optional
	return &nativepackage.Catalog{FormatVersion: nativepackage.FormatVersion, Dependencies: map[string]string{"native-values": "1.0.0"}, Modules: map[string]nativepackage.Module{"native-values": {Exports: exports}}}
}

func writeNativeNullishFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeCompilerRuntimeFile(t, filepath.Join(root, "package.json"), []byte(`{"type":"module"}`))
	module := filepath.Join(root, "node_modules", "native-values")
	writeCompilerRuntimeFile(t, filepath.Join(module, "package.json"), []byte(`{"name":"native-values","version":"1.0.0","type":"module","types":"./index.d.ts","exports":{"types":"./index.d.ts","default":"./index.js"}}`))
	writeCompilerRuntimeFile(t, filepath.Join(module, "index.d.ts"), []byte(nativeNullishDeclarations))
	writeCompilerRuntimeFile(t, filepath.Join(module, "index.js"), []byte(nativeNullishImplementation))
	return root
}

func checkNativeNullishCalls(t *testing.T, root string, catalog *nativepackage.Catalog) {
	t.Helper()
	artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Source: []byte(nativeNullishSource)}}, Options{
		Mode: "typescript", TypeScriptRuntime: "bun", NativePackages: catalog,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range artifacts {
		writeCompilerRuntimeFile(t, filepath.Join(root, artifact.IR.ModulePath+".ts"), artifact.Output)
	}
	t.Run("strict TypeScript", func(t *testing.T) {
		tsc, err := exec.LookPath("tsc")
		if err != nil {
			t.Skip("tsc is not installed")
		}
		command := exec.Command(tsc, "--strict", "--noEmit", "--target", "ES2024", "--module", "NodeNext", "main.ts")
		command.Dir = root
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("native nullish calls fail strict TypeScript: %v\n%s", err, output)
		}
	})
	t.Run("runtime", func(t *testing.T) {
		if _, err := exec.LookPath("bun"); err != nil {
			t.Skip("bun is not installed")
		}
		command := exec.Command("bun", "main.ts")
		command.Dir = root
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("native nullish calls failed: %v\n%s", err, output)
		}
		want := "true\ntrue\ntrue\ntrue\ntrue\nundefined\nvalue\nundefined\nundefined\nundefined\nnull\nnull\nnull\n0:undefined\n1:undefined\n1:value\n42\nundefined\ntrue\ntrue\ntrue\nleft\nright\nundefined:undefined\ntrue\nundefined\n"
		if string(output) != want {
			t.Fatalf("native boundary changed values, omission, identity, or evaluation order:\n%s\nwant:\n%s", output, want)
		}
	})
}

func TestNativeNullishCalls(t *testing.T) {
	checkNativeNullishCalls(t, writeNativeNullishFixture(t), nativeNullishCatalog())
}

func TestNativeNullishFunctionValuesRequireExplicitWrapper(t *testing.T) {
	for _, expression := range []string{"format", "invoke(format)"} {
		source := "import { describe as format, invoke } from \"native-values\"\n\ndef main()\n\tvalue := " + expression + "\n\treturn\nend\n"
		_, err := CompileWithOptions("main.trb", []byte(source), Options{Mode: "typescript", TypeScriptRuntime: "bun", NativePackages: nativeNullishCatalog()})
		if err == nil || !strings.Contains(err.Error(), "must be called directly; wrap the call in a typed fn") {
			t.Fatalf("expected explicit native wrapper diagnostic for %s, got %v", expression, err)
		}
	}
}

// This integration runs with TypeScript 6 in the native-package workflow. The
// ordinary compiler suite also checks lowering with its target tsc version.
func TestNativeNullishIndexAndCalls(t *testing.T) {
	tsc, err := exec.LookPath("tsc")
	if err != nil {
		t.Skip("tsc is not installed")
	}
	version, err := exec.Command(tsc, "--version").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(version), "Version 6.") {
		t.Skip("native indexing requires TypeScript 6")
	}
	tsc, err = filepath.EvalSymlinks(tsc)
	if err != nil {
		t.Fatal(err)
	}
	root := writeNativeNullishFixture(t)
	if err := os.Symlink(filepath.Dir(filepath.Dir(tsc)), filepath.Join(root, "node_modules", "typescript")); err != nil {
		t.Fatal(err)
	}
	catalog, err := nativepackage.Generate(root, "npm", map[string]string{"native-values": "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	module := catalog.Modules["native-values"]
	for name, want := range map[string]string{"nested": "undefined in an Array element", "rest": "undefined in an Array element", "callbackInput": "undefined in a callback parameter", "callbackOutput": "undefined in a callback return"} {
		if _, accepted := module.Exports[name]; accepted || !strings.Contains(module.Unsupported[name], want) {
			t.Fatalf("expected %s to reject %q, got %#v", name, want, module)
		}
	}
	if err := nativepackage.Write(root, catalog); err != nil {
		t.Fatal(err)
	}
	catalog, err = nativepackage.Load(root, catalog.Dependencies)
	if err != nil {
		t.Fatal(err)
	}
	checkNativeNullishCalls(t, root, catalog)
}
