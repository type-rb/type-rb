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
		Source: []byte(`import { Response } from trb/web

def response(): Response
	return Response.new(
		status: 200,
		headers: {"content-type" => ["application/json"]},
		body: "{}".to_bytes(),
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
		Source: []byte(`import { Response, with_header } from trb/web

def response(): Response
	base := Response.new(status: 204, headers: {}, body: "".to_bytes())
	return with_header(base, "x-count", 1)
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			_, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/official-package", RubyLoader: "require_relative", ProjectRoot: "/project"})
			if err == nil || !strings.Contains(err.Error(), "argument 3 to with_header() has type Integer, expected String") {
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
		Source: []byte(`import { HeaderValueError, Request, header_value } from trb/web
import { Result } from trb/std/result

def invalid(request: Request): Result<String, HeaderValueError>
	return header_value(request, 1)
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			_, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/official-package", RubyLoader: "require_relative", ProjectRoot: "/project"})
			if err == nil || !strings.Contains(err.Error(), "argument 2 to header_value() has type Integer, expected String") {
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
		Source: []byte(`import { Request, QueryValueError, query_value } from trb/web
import { Result } from trb/std/result

def invalid(request: Request): Result<String, QueryValueError>
	return query_value(request, 1)
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			_, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/official-package", RubyLoader: "require_relative", ProjectRoot: "/project"})
			if err == nil || !strings.Contains(err.Error(), "argument 2 to query_value() has type Integer, expected String") {
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
		Source: []byte(`import { Options } from trb/web/middleware/secure_headers

def invalid(): Options
	return Options.new(headers: {"x-example" => 1})
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
		Source: []byte(`import { Options, PreflightMaxAge } from trb/web/middleware/cors

def invalid(): Options
	return Options.new(
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
		Source: []byte(`import { Options } from trb/web/middleware/request_id

def invalid(): Options
	return Options.new(header_name: "x-request-id", limit_length: "long")
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
