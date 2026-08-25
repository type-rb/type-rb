package compiler

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/callsignature"
	"github.com/type-rb/type-rb/internal/ir"
)

func TestNamedOnlyParametersCompileAcrossModes(t *testing.T) {
	source := []byte(`def describe(count: Integer = 1, *, label: String = "item"): String
	return label + ": " + count.to_s()
end

def main()
	_value := describe(label: "selected")
	return
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifact, err := Compile("named_only.trb", source, mode)
			if err != nil {
				t.Fatal(err)
			}
			output := string(artifact.Output)
			switch mode {
			case "go":
				if !strings.Contains(output, `__trbOptional []any, __trbNamed map[string]any`) || !strings.Contains(output, `__trbValues["label"] = "selected"`) {
					t.Fatalf("generated Go lost the named-only ABI:\n%s", output)
				}
			case "ruby":
				if !strings.Contains(output, `def describe(count = 1, label: "item")`) || !strings.Contains(output, `describe(label: "selected")`) {
					t.Fatalf("generated Ruby lost keyword binding:\n%s", output)
				}
			case "typescript":
				if !strings.Contains(output, `__trbOptional: unknown[], __trbNamed: { label?: string }`) || !strings.Contains(output, `describe([], { label: "selected" })`) {
					t.Fatalf("generated TypeScript lost the named-only ABI:\n%s", output)
				}
			}
		})
	}
}

func TestNamedOnlySignaturesSurviveProjectImports(t *testing.T) {
	api := SourceUnit{Filename: "/project/src/app/api.trb", ModulePath: "app/api", Package: "api", Source: []byte(`def request(path: String, *, timeout: Integer, retries: Integer = 2): String
	return path + timeout.to_s() + retries.to_s()
end
`)}
	main := SourceUnit{Filename: "/project/src/main.trb", ModulePath: "main", Package: "main", Source: []byte(`import { request } from app/api
def main()
	_value := request("/", retries: 4, timeout: 10)
	return
end
`)}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{api, main}, Options{Mode: mode, GoModule: "example.com/named", ProjectRoot: "/project", SourceRoot: "/project/src", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		var call *ir.Call
		for _, artifact := range artifacts {
			if artifact.IR.ModulePath != "main" {
				continue
			}
			method := artifact.IR.Statements[len(artifact.IR.Statements)-1].(*ir.Method)
			call = method.Body[0].(*ir.Variable).Value.(*ir.Call)
		}
		if call == nil || len(call.CallSignature) != 3 || call.CallSignature[1].Kind != callsignature.NamedOnly || call.CallSignature[1].Label != "timeout" || call.CallSignature[2].Presence != callsignature.Omittable {
			t.Fatalf("%s: imported call signature was lost: %#v", mode, call)
		}
	}
}

func TestNamedOnlySignaturesSurviveImportedInterfacesAndOverrides(t *testing.T) {
	contracts := SourceUnit{
		Filename: "/project/src/app/contracts.trb", ModulePath: "app/contracts", Package: "contracts",
		Source: []byte(`interface Renderer
	render(*, prefix: String, value: String): String
end
class BaseClient
	def request(*, host: String, token: String): String
		return host + token
	end
end
`),
	}
	client := SourceUnit{
		Filename: "/project/src/app/client.trb", ModulePath: "app/client", Package: "client",
		Source: []byte(`import { BaseClient, Renderer } from app/contracts
class Client < BaseClient implements Renderer
	def request(*, token: String, host: String): String
		return host + token
	end
	def render(*, value: String, prefix: String): String
		return prefix + value
	end
end
`),
	}
	main := SourceUnit{
		Filename: "/project/src/main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import { Client } from app/client
def main()
	client := Client.new()
	puts(client.request(token: "token", host: "host"))
	puts(client.render(value: "value", prefix: "prefix"))
	return
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := CompileProject([]SourceUnit{contracts, client, main}, Options{Mode: mode, GoModule: "example.com/named", ProjectRoot: "/project", SourceRoot: "/project/src", RubyLoader: "require_relative"}); err != nil {
			t.Fatalf("%s: imported named-only contract was lost: %v", mode, err)
		}
	}
}

func TestRubyNativeKeywordCandidatesSurviveProjectImports(t *testing.T) {
	api := SourceUnit{
		Filename: "/project/src/app/api.trb", ModulePath: "app/api", Package: "api",
		Source: []byte(`import trb/platform/ruby/native
def configure(required:, raw: nil): String
	return "configured"
end
`),
	}
	main := SourceUnit{
		Filename: "/project/src/main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import { configure } from app/api
def main()
	puts(configure(raw: nil, required: true))
	return
end
`),
	}
	artifacts, err := CompileProject([]SourceUnit{api, main}, Options{Mode: "ruby", ProjectRoot: "/project", SourceRoot: "/project/src", RubyLoader: "require_relative"})
	if err != nil {
		t.Fatal(err)
	}
	generated := artifactForModule(artifacts, "main")
	if generated == nil || !strings.Contains(string(generated.Output), `configure(raw: nil, required: true)`) {
		t.Fatalf("Ruby-native named signature was not preserved across the project import: %#v", generated)
	}
}

func TestNamedOnlyArgumentDiagnostics(t *testing.T) {
	tests := []struct {
		name   string
		call   string
		wanted string
	}{
		{name: "unknown", call: `request(url, duration: 1)`, wanted: "has no named argument duration"},
		{name: "duplicate", call: `request(url, timeout: 1, timeout: 2)`, wanted: "receives argument timeout more than once"},
		{name: "missing", call: `request(url)`, wanted: "missing required argument timeout"},
		{name: "positional-only", call: `request(url: url, timeout: 1)`, wanted: "url is a positional-only parameter"},
		{name: "positional-after-named", call: `request(timeout: 1, url)`, wanted: "positional argument cannot follow a named argument"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := []byte(`def request(url: String, *, timeout: Integer): String
	return url
end
def invalid(url: String): String
	return ` + test.call + `
end
`)
			for _, mode := range []string{"go", "ruby", "typescript"} {
				if _, err := Compile("invalid_named_only.trb", source, mode); err == nil || !strings.Contains(err.Error(), test.wanted) {
					t.Fatalf("%s: expected %q, got %v", mode, test.wanted, err)
				}
			}
		})
	}
}

func TestNamedOnlySignaturesApplyToInterfacesAndOverrides(t *testing.T) {
	valid := []byte(`interface ClientContract
	request(*, host: String, token: String): String
end
class ContractClient implements ClientContract
	def request(*, token: String, host: String): String
		return host + token
	end
end
class BaseClient
	def request(*, host: String, token: String, timeout: Integer = 10, retries: Integer = 2): String
		return host + token + timeout.to_s() + retries.to_s()
	end
end
class Client < BaseClient
	def request(*, token: String, host: String, retries: Integer = 3, timeout: Integer = 30): String
		return host + token + timeout.to_s() + retries.to_s()
	end
end
`)
	// Named-only declaration order is not part of signature equivalence.
	if _, err := Compile("valid_override.trb", valid, "go"); err != nil {
		t.Fatalf("named-only order changed call-signature equivalence: %v", err)
	}

	invalid := []byte(`class BaseClient
	def request(*, timeout: Integer = 10): String
		return timeout.to_s()
	end
end
class Client < BaseClient
	def request(*, duration: Integer = 30): String
		return duration.to_s()
	end
end
`)
	if _, err := Compile("invalid_override.trb", invalid, "go"); err == nil || !strings.Contains(err.Error(), "does not match inherited method BaseClient.request") {
		t.Fatalf("expected inherited label mismatch, got %v", err)
	}

	presenceMismatch := []byte(`interface Connector
	connect(*, host: String): String
end
class DefaultConnector implements Connector
	def connect(*, host: String = "localhost"): String
		return host
	end
end
`)
	if _, err := Compile("invalid_presence.trb", presenceMismatch, "go"); err == nil || !strings.Contains(err.Error(), "does not match interface Connector") {
		t.Fatalf("expected exact required/omittable interface matching, got %v", err)
	}
}

func TestNamedOnlySyntaxBoundaries(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{source: "def old(label:: String): String\n\treturn label\nend\n", want: "typed keyword parameter syntax name:: Type was removed"},
		{source: "def rest(*values): String\n\treturn \"x\"\nend\n", want: "rest parameters are not supported"},
		{source: "def spread(values: Array<String>): String\n\treturn spread(*values)\nend\n", want: "argument splats are not supported"},
		{source: "interface Invalid\n\tread(*, timeout: Integer = 1): String\nend\n", want: "interface parameters cannot have defaults"},
		{source: "value := fn(*, label: String): String; return label; end\n", want: "fn parameters must be required positional parameters"},
		{source: "newtype UserID = Integer\nvalue := UserID.new(value: 1)\n", want: "UserID.new() has no named argument value"},
	}
	for _, test := range tests {
		if _, err := Compile("invalid_boundary.trb", []byte(test.source), "go"); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("expected %q, got %v", test.want, err)
		}
	}
}
