package compiler

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
)

const typescriptResultFunctionSource = `import { HttpClient, RequestError, Response } from trb/platform/typescript/browser
import { Result } from trb/std/result

API := HttpClient.new("https://example.test")

type LoadResult<T> = Result<T, RequestError>
type Loader = () -> LoadResult<Response<String>>

def named_loader(): LoadResult<Response<String>>
	raw := try API.request("/named")
	return LoadResult<Response<String>>::Ok(raw.text())
end

def invoke(loader: Loader): LoadResult<Response<String>>
	return loader()
end

def recover_failed_load(): String
	response := API.request("/failed") catch |error|
		return "recovered:" + error.message
	end
	return response.text().body
end

def print_result(label: String, result: LoadResult<Response<String>>)
	case result
	when Result::Ok(response)
		puts(label + ":" + response.body)
	when Result::Err(error)
		puts(label + ":error:" + error.message)
	end
	return
end

def main()
	inline := fn(): LoadResult<Response<String>>
		raw := try API.request("/lambda")
		return LoadResult<Response<String>>::Ok(raw.text())
	end
	print_result("named", named_loader())
	print_result("lambda", inline())
	print_result("named-hof", invoke(named_loader))
	print_result("lambda-hof", invoke(inline))
	puts(recover_failed_load())
	return
end
`

func TestCompileTypeScriptResultFunctionValuesMaySuspendAcrossOrdinaryBoundaries(t *testing.T) {
	artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Source: []byte(typescriptResultFunctionSource)}}, Options{
		Mode: "typescript", TypeScriptRuntime: "browser",
	})
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifacts[0].Output)
	for _, expected := range []string{
		"export type LoadResult<T> = Result<T, __trb_browser.RequestError>;",
		"export type Loader = () => Result<__trb_browser.Response<string>, __trb_browser.RequestError> | Promise<Result<__trb_browser.Response<string>, __trb_browser.RequestError>>;",
		"export async function named_loader(__trbScope: AbortSignal | undefined): Promise<LoadResult<__trb_browser.Response<string>>>",
		"export async function invoke(loader: Loader): Promise<LoadResult<__trb_browser.Response<string>>>",
		"export async function recover_failed_load(__trbScope: AbortSignal | undefined): Promise<string>",
		"return (await loader());",
		"const inline: () => Promise<Result<__trb_browser.Response<string>, __trb_browser.RequestError>> = async (): Promise<LoadResult<__trb_browser.Response<string>>> =>",
		"await invoke(named_loader.bind(undefined, __trbScope))",
		"await invoke(inline)",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated TypeScript Result function boundary is missing %q:\n%s", expected, output)
		}
	}
}

func TestCompileTypeScriptSuspendingResultFunctionUsesImportedTransparentAlias(t *testing.T) {
	contracts := SourceUnit{Filename: "contracts.trb", ModulePath: "app/contracts", Source: []byte(`import { Result } from trb/std/result

type ExternalResult<T, E> = Result<T, E>
type ExternalLoader<T, E> = () -> ExternalResult<T, E>
`)}
	consumer := SourceUnit{Filename: "consumer.trb", ModulePath: "app/consumer", Source: []byte(`import { ExternalLoader, ExternalResult } from app/contracts
import { HttpClient, RequestError, Response } from trb/platform/typescript/browser

API := HttpClient.new("https://example.test")

def make_loader(): ExternalLoader<Response<String>, RequestError>
	loader := fn(): ExternalResult<Response<String>, RequestError>
		raw := try API.request("/external")
		return ExternalResult<Response<String>, RequestError>::Ok(raw.text())
	end
	return loader
end
`)}
	artifacts, err := CompileProject([]SourceUnit{contracts, consumer}, Options{Mode: "typescript", TypeScriptRuntime: "browser"})
	if err != nil {
		t.Fatal(err)
	}
	var consumerOutput string
	var hasImplicitResult bool
	for _, artifact := range artifacts {
		if artifact.IR.ModulePath != "app/consumer" {
			continue
		}
		consumerOutput = string(artifact.Output)
		for _, statement := range artifact.IR.Statements {
			if imported, ok := statement.(*ir.Import); ok && imported.Implicit && imported.Path == "trb/std/result/index" {
				hasImplicitResult = true
			}
		}
	}
	if !hasImplicitResult {
		t.Fatalf("transparent Result alias consumer is missing its compiler-owned runtime import")
	}
	for _, expected := range []string{
		"export function make_loader(__trbScope: AbortSignal | undefined): ExternalLoader<__trb_browser.Response<string>, __trb_browser.RequestError>",
		"const loader: () => Promise<Result<__trb_browser.Response<string>, __trb_browser.RequestError>> = async (): Promise<ExternalResult<__trb_browser.Response<string>, __trb_browser.RequestError>> =>",
	} {
		if !strings.Contains(consumerOutput, expected) {
			t.Fatalf("imported transparent Result alias output is missing %q:\n%s", expected, consumerOutput)
		}
	}
}

func TestGeneratedTypeScriptResultFunctionValuesRunAcrossNamedLambdaAndHOFBoundaries(t *testing.T) {
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("bun is not installed")
	}
	artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Source: []byte(typescriptResultFunctionSource)}}, Options{
		Mode: "typescript", TypeScriptRuntime: "bun",
	})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, artifact := range artifacts {
		path := filepath.Join(root, filepath.FromSlash(artifact.IR.ModulePath+".ts"))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, artifact.Output, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	harness := `globalThis.fetch = async (input) => {
  const path = new URL(String(input)).pathname;
  if (path === "/failed") throw new Error("offline");
  return new Response("body" + path, { status: 200, headers: { "content-type": "text/plain" } });
};
await import("./main.ts");
`
	if err := os.WriteFile(filepath.Join(root, "runner.ts"), []byte(harness), 0o644); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("bun", "run", "runner.ts")
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("generated TypeScript failed: %v\n%s", err, output)
	}
	lines := strings.Fields(string(output))
	want := "named:body/named\nlambda:body/lambda\nnamed-hof:body/named\nlambda-hof:body/lambda\nrecovered:offline"
	if got := strings.Join(lines, "\n"); got != want {
		t.Fatalf("ordinary Result function boundaries produced %q, want %q", got, want)
	}
}
