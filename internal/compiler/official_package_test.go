package compiler

import (
	"strings"
	"testing"
)

func TestOfficialPackageSourceCompilesAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { Body, Header, Headers } from trb/http
import { Response } from trb/web

def response(): Response
	return Response.new(
		status: 200,
		headers: Headers.new([Header.new(name: "content-type", value: "application/json")]),
		body: Body.new("{}".to_bytes()),
	)
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/official-package", RubyLoader: "require_relative", ProjectRoot: "/project"})
			if err != nil {
				t.Fatal(err)
			}
			var packageArtifact *Artifact
			for _, artifact := range artifacts {
				if artifact.IR.ModulePath == "trb/web/index" {
					packageArtifact = artifact
					break
				}
			}
			if packageArtifact == nil || !packageArtifact.Official {
				t.Fatal("official trb/web artifact was not generated")
			}
			if !strings.Contains(string(packageArtifact.Output), "Response") {
				t.Fatalf("generated package does not declare Response:\n%s", packageArtifact.Output)
			}
		})
	}
}

func TestOfficialWebResponseHeadersRejectNonStringValues(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { Body, Headers } from trb/http
import { Response } from trb/web

def response(): Response
	base := Response.new(status: 204, headers: Headers.new(), body: Body.empty())
	return base.with_header("x-count", 1)
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			_, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/official-package", RubyLoader: "require_relative", ProjectRoot: "/project"})
			if err == nil || !strings.Contains(err.Error(), "argument 2 to with_header() has type Integer, expected String") {
				t.Fatalf("unexpected diagnostic: %v", err)
			}
		})
	}
}

func TestOfficialWebStrictHeaderLookupRejectsNonStringNames(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { HeaderValueError } from trb/http
import { Request } from trb/web
import { Result } from trb/std/result

def invalid(request: Request): Result<String, HeaderValueError>
	return request.header_value(1)
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			_, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/official-package", RubyLoader: "require_relative", ProjectRoot: "/project"})
			if err == nil || !strings.Contains(err.Error(), "argument 1 to header_value() has type Integer, expected String") {
				t.Fatalf("unexpected diagnostic: %v", err)
			}
		})
	}
}

func TestOfficialWebResponseCookiesRejectInvalidAttributePayloads(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { ResponseCookieAttribute } from trb/web

def invalid(): ResponseCookieAttribute
	return ResponseCookieAttribute::MaxAge("one hour")
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			_, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/official-package", RubyLoader: "require_relative", ProjectRoot: "/project"})
			if err == nil || !strings.Contains(err.Error(), "enum payload argument 1 has type String, expected Integer") {
				t.Fatalf("unexpected diagnostic: %v", err)
			}
		})
	}
}

func TestOfficialWebQueryHelpersRejectNonStringNames(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { Request, QueryValueError } from trb/web
import { Result } from trb/std/result

def invalid(request: Request): Result<String, QueryValueError>
	return request.query_value(1)
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			_, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/official-package", RubyLoader: "require_relative", ProjectRoot: "/project"})
			if err == nil || !strings.Contains(err.Error(), "argument 1 to query_value() has type Integer, expected String") {
				t.Fatalf("unexpected diagnostic: %v", err)
			}
		})
	}
}

func TestOfficialWebTypedParameterBindingRejectsUnsupportedShapes(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "non-record root",
			source: `import { ParameterError, Request } from trb/web
import { Result } from trb/std/result

def invalid(request: Request): Result<String, ParameterError>
	return request.query<String>()
end
`,
			want: "web parameter binding type String must be a non-nullable record",
		},
		{
			name: "nested query record",
			source: `import { ParameterError, Request } from trb/web
import { Result } from trb/std/result

record Filter
	start: Integer
end

record Query
	filter: Filter
end

def invalid(request: Request): Result<Query, ParameterError>
	return request.query<Query>()
end
`,
			want: "web parameter field filter has unsupported type Filter",
		},
		{
			name: "array path field",
			source: `import { Context, ParameterError } from trb/web
import { Result } from trb/std/result

record Params
	id: Array<Integer>
end

def invalid(context: Context): Result<Params, ParameterError>
	return context.params<Params>()
end
`,
			want: "path parameter field id cannot be an Array",
		},
	}
	for _, test := range tests {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			t.Run(test.name+"/"+mode, func(t *testing.T) {
				source := SourceUnit{Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: []byte(test.source)}
				_, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/official-package", RubyLoader: "require_relative", ProjectRoot: "/project"})
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("unexpected diagnostic: %v", err)
				}
			})
		}
	}
}

func TestOfficialWebTypedParameterBindingAcceptsTransparentScalarAliases(t *testing.T) {
	source := SourceUnit{
		Filename: "/project/main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import { ParameterError, Request } from trb/web
import { Result } from trb/std/result

type InsurerId = Integer

record Query
	insurer_id: InsurerId
end

def read(request: Request): Result<Query, ParameterError>
	return request.query<Query>()
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			if _, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/official-package", RubyLoader: "require_relative", ProjectRoot: "/project"}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOfficialWebTypedContextKeysCompileAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { Context, ContextKey, ContextValueError } from trb/web
import { Result } from trb/std/result

record User
	name: String
end

CURRENT_USER := ContextKey<User>.new("current_user")

def current_user(context: Context): Result<User, ContextValueError>
	updated := context.with(CURRENT_USER, User.new(name: "Ada"))
	return updated.fetch(CURRENT_USER)
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/typed-context-key", RubyLoader: "require_relative", ProjectRoot: "/project"})
			if err != nil {
				t.Fatal(err)
			}
			artifact := artifactForModule(artifacts, "main")
			if artifact == nil || !strings.Contains(string(artifact.Output), "ContextValueError") {
				t.Fatalf("%s output is missing typed context lookup:\n%s", mode, artifact.Output)
			}
		})
	}
}

func TestOfficialWebEndpointInputBindingCompilesAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { Context, EndpointInputError } from trb/web
import { Result } from trb/std/result

record Params
	id: Integer
end

record Query
	page: Integer?
	tag: Array<String>
end

record Payload
	title: String
end

record CreateTodoInput
	params: Params
	query: Query
	body: Payload
end

def bind(context: Context): Result<CreateTodoInput, EndpointInputError>
	return context.bind<CreateTodoInput>()
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/endpoint-input", RubyLoader: "require_relative", ProjectRoot: "/project"})
			if err != nil {
				t.Fatal(err)
			}
			artifact := artifactForModule(artifacts, "main")
			if artifact == nil || !strings.Contains(string(artifact.Output), "EndpointInputError") {
				t.Fatalf("%s output is missing endpoint input binding:\n%s", mode, artifact.Output)
			}
		})
	}
}

func TestOfficialWebEndpointInputBindingSupportsEverySourceCombinationAcrossBackends(t *testing.T) {
	contracts := []struct {
		name   string
		fields string
	}{
		{name: "params", fields: "\tparams: Params\n"},
		{name: "query", fields: "\tquery: Query\n"},
		{name: "body", fields: "\tbody: Payload\n"},
		{name: "scalar body", fields: "\tbody: String\n"},
		{name: "params and query", fields: "\tparams: Params\n\tquery: Query\n"},
		{name: "params and body", fields: "\tparams: Params\n\tbody: Payload\n"},
		{name: "query and body", fields: "\tquery: Query\n\tbody: Payload\n"},
		// Declaration order does not change the canonical params, query, body
		// binding order.
		{name: "all sources in reverse order", fields: "\tbody: Payload\n\tquery: Query\n\tparams: Params\n"},
	}

	for _, contract := range contracts {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			t.Run(contract.name+"/"+mode, func(t *testing.T) {
				source := SourceUnit{
					Filename:   "/project/main.trb",
					ModulePath: "main",
					Package:    "main",
					Source: []byte(`import { Context, EndpointInputError } from trb/web
import { Result } from trb/std/result

record Params
	id: Integer
end

record Query
	page: Integer?
end

record Payload
	title: String
end

record Input
` + contract.fields + `end

def bind(context: Context): Result<Input, EndpointInputError>
	return context.bind<Input>()
end
`),
				}
				if _, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/endpoint-input", RubyLoader: "require_relative", ProjectRoot: "/project"}); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestOfficialWebEndpointInputBindingRejectsUnsupportedContracts(t *testing.T) {
	tests := []struct {
		name         string
		declarations string
		target       string
		want         string
	}{
		{name: "non-record input", target: "String", want: "endpoint input type String must be a non-nullable record"},
		{name: "nullable input", declarations: "record Input\n\tbody: String\nend", target: "Input?", want: "endpoint input type Input? must be a non-nullable record"},
		{name: "empty", declarations: "record Input\nend", target: "Input", want: "endpoint input record Input must declare at least one of params, query, or body"},
		{name: "unknown field", declarations: "record Input\n\theaders: String\nend", target: "Input", want: `endpoint input record Input has unsupported field "headers"`},
		{name: "non-record query", declarations: "record Input\n\tquery: String\nend", target: "Input", want: "web parameter binding type String must be a non-nullable record"},
		{name: "nullable params", declarations: "record Params\n\tid: Integer\nend\n\nrecord Input\n\tparams: Params?\nend", target: "Input", want: "web parameter binding type Params? must be a non-nullable record"},
	}
	for _, test := range tests {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			t.Run(test.name+"/"+mode, func(t *testing.T) {
				source := SourceUnit{Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: []byte(`import { Context } from trb/web

` + test.declarations + `

def invalid(context: Context)
	context.bind<` + test.target + `>()
	return
end
`)}
				_, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/endpoint-input", RubyLoader: "require_relative", ProjectRoot: "/project"})
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("unexpected diagnostic: %v", err)
				}
			})
		}
	}
}

func TestOfficialWebTypedContextKeysRejectMismatchedValues(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { Context, ContextKey } from trb/web

record User
	name: String
end

CURRENT_USER := ContextKey<User>.new("current_user")

def invalid(context: Context): Context
	return context.with(CURRENT_USER, "Ada")
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			_, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/typed-context-key", RubyLoader: "require_relative", ProjectRoot: "/project"})
			if err == nil || !strings.Contains(err.Error(), "argument 2 to with() has type String, expected User") {
				t.Fatalf("unexpected diagnostic: %v", err)
			}
		})
	}
}

func TestOfficialWebResponseBuildersRejectInvalidValues(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { Response, text } from trb/web

def invalid(): Response
	return text(1)
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			_, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/official-package", RubyLoader: "require_relative", ProjectRoot: "/project"})
			if err == nil || !strings.Contains(err.Error(), "argument 1 to text() has type Integer, expected String") {
				t.Fatalf("unexpected diagnostic: %v", err)
			}
		})
	}
}

func TestOfficialSecureHeadersRejectInvalidHeaderValues(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { SecureHeadersOptions } from trb/web/middleware/secure_headers

def invalid(): SecureHeadersOptions
	return SecureHeadersOptions.new(headers: {"x-example" => 1})
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			_, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/official-package", RubyLoader: "require_relative", ProjectRoot: "/project"})
			if err == nil || !strings.Contains(err.Error(), "record field headers has type Hash<String, Integer>, expected Hash<String, String>") {
				t.Fatalf("unexpected diagnostic: %v", err)
			}
		})
	}
}

func TestOfficialCORSRejectsInvalidOriginOptions(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { CORSOptions, PreflightMaxAge } from trb/web/middleware/cors

def invalid(): CORSOptions
	return CORSOptions.new(
		allow_origins: [1],
		allow_methods: ["GET"],
		allow_headers: [],
		expose_headers: [],
		credentials: false,
		max_age: PreflightMaxAge::Disabled,
	)
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			_, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/official-package", RubyLoader: "require_relative", ProjectRoot: "/project"})
			if err == nil || !strings.Contains(err.Error(), "record field allow_origins has type Array<Integer>, expected Array<String>") {
				t.Fatalf("unexpected diagnostic: %v", err)
			}
		})
	}
}

func TestOfficialRequestIDRejectsInvalidLengthOptions(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { RequestIDOptions } from trb/web/middleware/request_id

def invalid(): RequestIDOptions
	return RequestIDOptions.new(header_name: "x-request-id", limit_length: "long")
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			_, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/official-package", RubyLoader: "require_relative", ProjectRoot: "/project"})
			if err == nil || !strings.Contains(err.Error(), "record field limit_length has type String, expected Integer") {
				t.Fatalf("unexpected diagnostic: %v", err)
			}
		})
	}
}

func TestOfficialWebMiddlewareStackCompilesAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { Body, Headers } from trb/http
import { Context, Next, Response } from trb/web
import { Middleware, compose } from trb/web/middleware
import trb/web/middleware/cors
import trb/web/middleware/compression
import trb/web/middleware/logger
import trb/web/middleware/request_id
import trb/web/middleware/secure_headers

class Terminal implements Next
	def call(_context: Context): Response
		return Response.new(status: 204, headers: Headers.new(), body: Body.empty())
	end
end

def stack(): Array<Middleware>
	return [
		request_id.middleware(),
		logger.middleware(),
		compression.middleware(),
		secure_headers.middleware(),
		cors.middleware(),
	]
end

def dispatch(context: Context): Response
	return compose(context, Terminal.new(), stack())
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/middleware-stack", RubyLoader: "require_relative", ProjectRoot: "/project"})
			if err != nil {
				t.Fatal(err)
			}
			if artifactForModule(artifacts, "trb/web/middleware/index") == nil {
				t.Fatal("middleware runtime artifact was not generated")
			}
		})
	}
}

func TestUserSourceCannotClaimOfficialPackageNamespace(t *testing.T) {
	_, err := CompileProject([]SourceUnit{{
		Filename:   "/project/trb/web/index.trb",
		ModulePath: "trb/web/index",
		Package:    "web",
		Source:     []byte("record Response; body: String; end\n"),
	}}, Options{Mode: "go", GoModule: "example.com/spoof"})
	if err == nil || !strings.Contains(err.Error(), "module path trb/web/index is reserved") {
		t.Fatalf("expected reserved namespace error, got %v", err)
	}
}
