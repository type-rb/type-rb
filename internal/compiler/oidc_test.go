package compiler

import (
	"strings"
	"testing"
)

func TestCompilePortableOidcBearerAuthentication(t *testing.T) {
	source := SourceUnit{
		Filename:   "auth.trb",
		ModulePath: "app/auth",
		Source: []byte(`import { OidcAuthError, OidcPrincipal, bearer_options } from trb/auth/oidc
import { Result } from trb/std/result
import { Context, Next, Response } from trb/web
import trb/web/auth/bearer

AUTH := bearer_options(
	issuer: "https://identity.example/",
	audience: "api",
)

def protect(context: Context, next_handler: Next): Response
	return Bearer.authenticate(context, next_handler, AUTH)
end

def current_principal(context: Context): Result<OidcPrincipal, OidcAuthError>
	return Bearer.principal(context)
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			options := Options{Mode: mode}
			if mode == "go" {
				options.GoModule = "example.test/auth"
			}
			if mode == "typescript" {
				options.TypeScriptRuntime = "bun"
			}
			artifacts, err := CompileProject([]SourceUnit{source}, options)
			if err != nil {
				t.Fatal(err)
			}
			var generated strings.Builder
			for _, artifact := range artifacts {
				generated.Write(artifact.Output)
			}
			for _, expected := range map[string][]string{
				"go":         {"trbOidcVerifyBearer", "trbOidcLoadProvider", "rsa.VerifyPKCS1v15"},
				"ruby":       {"TrbOidcRuntime.verify_bearer", "def load_provider", "rsa.verify"},
				"typescript": {"trb_oidc_verify_bearer", "trb_oidc_load_provider", "crypto.subtle.verify", "unauthorized: _unauthorized"},
			}[mode] {
				if !strings.Contains(generated.String(), expected) {
					t.Fatalf("generated %s OIDC bearer package is missing %q:\n%s", mode, expected, generated.String())
				}
			}
		})
	}
}
