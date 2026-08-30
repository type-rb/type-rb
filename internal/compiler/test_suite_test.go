package compiler

import (
	"strings"
	"testing"
)

func TestCompilePortableTestSuite(t *testing.T) {
	const testSource = `import { describe, expect, test } from trb/std/test

describe("Calculator") do
	test("adds numbers") do
		expect(1 + 2).to_equal(3)
	end
end
`
	const runnerSource = `import { finish } from trb/std/test
import { trb_test_register_sample } from calculator_test

def main()
	trb_test_register_sample()
	finish()
	return
end
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			rootPackage := ""
			if mode == "go" {
				rootPackage = "main"
			}
			artifacts, err := CompileProject([]SourceUnit{
				{Filename: "/project/src/calculator_test.trb", Source: []byte(testSource), ModulePath: "calculator_test", Package: rootPackage, TestRegistration: "trb_test_register_sample"},
				{Filename: "/project/src/__trb_test_main.trb", Source: []byte(runnerSource), ModulePath: "__trb_test_main", Package: rootPackage, CompilerOwned: true},
			}, Options{Mode: mode, GoModule: "example.com/test", SourceRoot: "/project/src", ProjectRoot: "/project", TypeScriptRuntime: "bun"})
			if err != nil {
				t.Fatal(err)
			}
			joined := ""
			for _, artifact := range artifacts {
				joined += string(artifact.Output)
			}
			for _, expected := range []string{"Calculator", "adds numbers"} {
				if !strings.Contains(joined, expected) {
					t.Fatalf("generated %s test suite is missing %q:\n%s", mode, expected, joined)
				}
			}
		})
	}
}

func TestTypeScriptWebTestModuleOwnsItsDispatcher(t *testing.T) {
	const routeSource = `import { Context, Response } from trb/web

def get(_context: Context): Response
	return Response.text("ok")
end
`
	const testSource = `import { Body, Headers, HttpMethod } from trb/http
import { Request } from trb/web
import { dispatch } from trb/web/testing
import { describe, expect, test } from trb/std/test

describe("Web") do
	test("dispatches a route") do
		response := dispatch(Request.new(method: HttpMethod.get(), path: "/health", query_string: "", headers: Headers.new(), body: Body.empty()))
		expect(response.status).to_equal(200)
	end
end
`
	const runnerSource = `import { finish } from trb/std/test
import { trb_test_register_web } from web_test

def main()
	trb_test_register_web()
	finish()
	return
end
`
	artifacts, err := CompileProject([]SourceUnit{
		{Filename: "/project/src/routes/health.trb", Source: []byte(routeSource), ModulePath: "routes/health"},
		{Filename: "/project/src/web_test.trb", Source: []byte(testSource), ModulePath: "web_test", TestRegistration: "trb_test_register_web"},
		{Filename: "/project/src/__trb_test_main.trb", Source: []byte(runnerSource), ModulePath: "trb_test_main", CompilerOwned: true},
	}, Options{Mode: "typescript", SourceRoot: "/project/src", ProjectRoot: "/project", TypeScriptRuntime: "bun"})
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifactForModule(artifacts, "web_test").Output)
	if !strings.Contains(output, "async function trb_web_dispatch") {
		t.Fatalf("TypeScript web test module does not own a dispatcher:\n%s", output)
	}
	if strings.Contains(output, `from "node:http"`) {
		t.Fatalf("TypeScript web test module emitted the server host:\n%s", output)
	}
}

func TestTestRunnerProtocolIsNotAUserImport(t *testing.T) {
	_, err := Compile("example_test.trb", []byte("import { finish } from trb/std/test\n"), "go")
	if err == nil || !strings.Contains(err.Error(), "finish is internal to the TypeRB compiler") {
		t.Fatalf("unexpected finish import result: %v", err)
	}
}

func TestResultExpectationRequiresAStandardResultAcrossBackends(t *testing.T) {
	const source = `import { expect_ok } from trb/std/test

def unwrap(): Integer
	return expect_ok(1)
end
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			_, err := Compile("result_expectation.trb", []byte(source), mode)
			if err == nil || !strings.Contains(err.Error(), "argument 1 to expect_ok() has type Integer, expected Result<T, E>") {
				t.Fatalf("unexpected Result expectation diagnostic: %v", err)
			}
		})
	}
}

func TestPortableTestBlocksRejectEscapingControlAcrossBackends(t *testing.T) {
	tests := []struct {
		keyword string
		source  string
	}{
		{
			keyword: "return",
			source: `import { describe, test } from trb/std/test

describe("suite") do
	test("case") do
		return
	end
end
`,
		},
		{
			keyword: "break",
			source: `import { describe, test } from trb/std/test

describe("suite") do
	test("case") do
		break
	end
end
`,
		},
		{
			keyword: "next",
			source: `import { describe, test } from trb/std/test

describe("suite") do
	test("case") do
		next
	end
end
`,
		},
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			t.Run(mode+"/"+test.keyword, func(t *testing.T) {
				_, err := CompileProject([]SourceUnit{{
					Filename: "/project/src/control_test.trb", Source: []byte(test.source), ModulePath: "control_test",
				}}, Options{Mode: mode, GoModule: "example.com/test-control", SourceRoot: "/project/src", ProjectRoot: "/project", TypeScriptRuntime: "bun"})
				want := test.keyword + " cannot cross the test() block boundary"
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("expected %q, got %v", want, err)
				}
				compileError, ok := err.(*CompileError)
				if !ok {
					t.Fatalf("error type=%T, want *CompileError", err)
				}
				found := false
				for _, item := range compileError.Diagnostics {
					if item.Message != want {
						continue
					}
					if got := test.source[item.Span.Start.Offset:item.Span.End.Offset]; got != test.keyword {
						t.Fatalf("diagnostic span=%q, want %q", got, test.keyword)
					}
					found = true
				}
				if !found {
					t.Fatalf("missing structured diagnostic %q in %#v", want, compileError.Diagnostics)
				}
			})
		}
	}
}

func TestPortableTestBlocksAllowLocallyOwnedControlAcrossBackends(t *testing.T) {
	const testSource = `import { describe, expect, test } from trb/std/test

describe("Control") do
	test("keeps local owners") do
		while true
			break
		end
		[1].each do |_value|
			next
		end
		callback := fn()
			return
		end
		callback()
		expect(true).to_be_true()
	end
end
`
	const runnerSource = `import { finish } from trb/std/test
import { trb_test_register_control } from control_test

def main()
	trb_test_register_control()
	finish()
	return
end
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			rootPackage := ""
			if mode == "go" {
				rootPackage = "main"
			}
			_, err := CompileProject([]SourceUnit{
				{Filename: "/project/src/control_test.trb", Source: []byte(testSource), ModulePath: "control_test", Package: rootPackage, TestRegistration: "trb_test_register_control"},
				{Filename: "/project/src/__trb_test_main.trb", Source: []byte(runnerSource), ModulePath: "__trb_test_main", Package: rootPackage, CompilerOwned: true},
			}, Options{Mode: mode, GoModule: "example.com/test-control", SourceRoot: "/project/src", ProjectRoot: "/project", TypeScriptRuntime: "bun"})
			if err != nil {
				t.Fatalf("%s rejected locally owned control: %v", mode, err)
			}
		})
	}
}
