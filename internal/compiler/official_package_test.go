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
