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
