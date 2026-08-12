package compiler

import (
	"strings"
	"testing"
)

func TestCompilePortableBearerAuthentication(t *testing.T) {
	source := SourceUnit{
		Filename:   "auth.trb",
		ModulePath: "app/auth",
		Source: []byte(`import { OidcAuthError, OidcBearerOptions, OidcPrincipal } from trb/auth/oidc
import { Context, Next, Response } from trb/web
import { authenticate, principal } from trb/web/auth/bearer

AUTH := OidcBearerOptions.new(
	issuer: "https://issuer.example/",
	audience: "api",
	jwks_uri: "https://issuer.example/.well-known/jwks.json",
	roles_claim: "roles",
)

def current_principal(context: Context): OidcPrincipal fails OidcAuthError
	return principal(context, AUTH)
end

def protect(context: Context, next_handler: Next): Response
	return authenticate(context, next_handler, AUTH)
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
			var output string
			for _, artifact := range artifacts {
				if artifact.Filename == source.Filename {
					output = string(artifact.Output)
				}
			}
			for _, expected := range map[string][]string{
				"go":         {"trbOidcVerifyBearer", "rsa.VerifyPKCS1v15", "Status: 401"},
				"ruby":       {"trb_oidc_verify_bearer", "public_key.verify", "status: 401"},
				"typescript": {"trbOidcVerifyBearer", "crypto.subtle.verify", "status: 401"},
			}[mode] {
				if !strings.Contains(output, expected) {
					t.Fatalf("generated %s bearer auth is missing %q:\n%s", mode, expected, output)
				}
			}
		})
	}
}

func TestCompileGoOidcRuntimeOncePerPackage(t *testing.T) {
	shared := `import { OidcAuthError, OidcBearerOptions, OidcPrincipal } from trb/auth/oidc
import { Context } from trb/web
import { principal } from trb/web/auth/bearer

AUTH := OidcBearerOptions.new(
	issuer: "https://issuer.example/",
	audience: "api",
	jwks_uri: "https://issuer.example/jwks",
	roles_claim: "roles",
)

def current_principal(context: Context): OidcPrincipal fails OidcAuthError
	return principal(context, AUTH)
end
`
	units := []SourceUnit{
		{Filename: "first.trb", ModulePath: "app/first", Source: []byte(shared)},
		{Filename: "second.trb", ModulePath: "app/second", Source: []byte(shared)},
	}
	artifacts, err := CompileProject(units, Options{Mode: "go", GoModule: "example.test/auth"})
	if err != nil {
		t.Fatal(err)
	}
	runtimeCount := 0
	callCount := 0
	for _, artifact := range artifacts {
		output := string(artifact.Output)
		runtimeCount += strings.Count(output, "type trbOidcPrincipalData struct")
		callCount += strings.Count(output, "trbOidcVerifyBearer(")
	}
	if runtimeCount != 1 {
		t.Fatalf("expected one OIDC runtime per Go package, got %d", runtimeCount)
	}
	if callCount < 3 {
		t.Fatalf("expected both modules and the runtime to reference bearer verification, got %d", callCount)
	}
}

func TestCompileReactOidcAndAuthenticatedTransport(t *testing.T) {
	source := SourceUnit{
		Filename:   "app.trb",
		ModulePath: "app/main",
		Source: []byte(`import { OidcBrowserOptions } from trb/auth/oidc
import { FetchError } from trb/platform/typescript/browser
import { get_json } from trb/platform/typescript/browser/bearer
import { ReactNode } from trb/platform/typescript/react
import { provider, use_oidc } from trb/platform/typescript/react/oidc

record Todo
	id: Integer
end

AUTH := OidcBrowserOptions.new(
	authority: "https://issuer.example/",
	client_id: "browser",
	redirect_uri: "http://localhost:5173/callback",
	post_logout_redirect_uri: "http://localhost:5173/",
	scope: "openid profile email",
	audience: "api",
	roles_claim: "roles",
)

def Page(): ReactNode
	auth := use_oidc()
	return <p>Signed in: {auth.authenticated}</p>
end

def load_todos(access_token: String): Array<Todo> fails FetchError
	return get_json<Array<Todo>>("/api/todos", access_token)
end

def App(): ReactNode
	return provider(AUTH, <Page />)
end
`),
	}
	artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: "typescript", TypeScriptRuntime: "browser"})
	if err != nil {
		t.Fatal(err)
	}
	var output string
	for _, artifact := range artifacts {
		if artifact.Filename == source.Filename {
			output = string(artifact.Output)
		}
	}
	for _, expected := range []string{
		`from "react-oidc-context"`, "automaticSilentRenew: true", "useTrbOidc()",
		"const auth: Readonly<{ loading: boolean; authenticated: boolean; principal:",
		`"Authorization": "Bearer " + access_token`, "await fetch(\"/api/todos\"",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated React OIDC app is missing %q:\n%s", expected, output)
		}
	}
}

func TestCompileBrowserSessionTransport(t *testing.T) {
	source := SourceUnit{
		Filename:   "session_client.trb",
		ModulePath: "app/session_client",
		Source: []byte(`import { FetchError } from trb/platform/typescript/browser
import trb/platform/typescript/browser/session

record Todo
	id: Integer
end

def load_todos(): Array<Todo> fails FetchError
	return session.get_json<Array<Todo>>("/api/todos")
end

def save_todo(todo: Todo): Todo fails FetchError
	return session.post_json<Todo>("/api/todos", todo)
end
`),
	}
	artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: "typescript", TypeScriptRuntime: "browser"})
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifacts[0].Output)
	for _, expected := range []string{`credentials: "same-origin"`, `document.cookie.split`, `"X-CSRF-Token": decodeURIComponent(__trbCsrf)`, `await fetch("/api/todos"`} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated session transport is missing %q:\n%s", expected, output)
		}
	}
}

func TestCompilePortableOidcSessionAuthentication(t *testing.T) {
	source := SourceUnit{
		Filename:   "session.trb",
		ModulePath: "app/session",
		Source: []byte(`import { OidcAuthError, OidcPrincipal, OidcSessionOptions } from trb/auth/oidc
import { Context, Next, Response } from trb/web
import { authenticate, complete_login, end_session, principal, start_login } from trb/web/auth/session

AUTH := OidcSessionOptions.new(
	issuer: "https://issuer.example/",
	client_id: "server",
	client_secret: "secret",
	authorization_endpoint: "https://issuer.example/authorize",
	token_endpoint: "https://issuer.example/token",
	jwks_uri: "https://issuer.example/jwks",
	redirect_uri: "https://app.example/auth/callback",
	post_logout_redirect_uri: "https://app.example/",
	end_session_endpoint: "https://issuer.example/logout",
	scope: "openid profile email",
	audience: "api",
	roles_claim: "roles",
	cookie_name: "trb_session",
	cookie_secret: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	secure: true,
)

def login(context: Context): Response
	return start_login(context, AUTH, "/")
end

def callback(context: Context): Response
	return complete_login(context, AUTH)
end

def current_principal(context: Context): OidcPrincipal fails OidcAuthError
	return principal(context, AUTH)
end

def protect(context: Context, next_handler: Next): Response
	return authenticate(context, next_handler, AUTH)
end

def logout(context: Context): Response
	return end_session(context, AUTH)
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
			var output string
			for _, artifact := range artifacts {
				if artifact.Filename == source.Filename {
					output = string(artifact.Output)
				}
			}
			expected := map[string][]string{
				"go":         {"trbOidcStartLogin", "cipher.NewGCM", "trbOidcCsrfValid"},
				"ruby":       {"trb_oidc_start_login", "trb_oidc_csrf_valid"},
				"typescript": {"trbOidcStartLogin", "trbOidcCsrfValid"},
			}[mode]
			for _, item := range expected {
				if !strings.Contains(output, item) {
					t.Fatalf("generated %s session auth is missing %q:\n%s", mode, item, output)
				}
			}
		})
	}
}
