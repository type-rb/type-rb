package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func TestTypeScriptTestSuiteDispatchesWebRoutes(t *testing.T) {
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skipf("bun is unavailable: %v", err)
	}
	root := t.TempDir()
	config := project.New(root, "typescript")
	config.SourceDir = "src"
	config.TypeScript.Runtime = project.TypeScriptRuntimeBun
	config.TypeScript.PackageManager = "bun"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(config.SourcePath(), "routes"), 0o755); err != nil {
		t.Fatal(err)
	}
	routeSource := `import { Context, Response, text } from trb/web

def get(_context: Context): Response
	return text("ok")
end
`
	testSource := `import { Body, Headers, HttpMethod } from trb/http
import { Request } from trb/web
import { dispatch } from trb/web/testing
import { describe, expect, test } from trb/std/test

describe("Web") do
	test("dispatches a route") do
		response := dispatch(Request.new(method: HttpMethod.get(), path: "/health", query_string: "", headers: Headers.new(), body: Body.empty()))
		expect(response.status).to_equal(200)
		expect(response.body.to_s()).to_equal("ok")
	end
end
`
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "routes", "health.trb"), []byte(routeSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "web_test.trb"), []byte(testSource), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"test", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "PASS Web / dispatches a route") || !strings.Contains(stdout.String(), "1 test(s), 0 failure(s)") {
		t.Fatalf("unexpected test output:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
}

func TestGoTestSuiteDispatchesRouteBelowDynamicSegment(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go is unavailable: %v", err)
	}
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/nested-dynamic-route"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/type-rb/nested-dynamic-route\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	routeDirectory := filepath.Join(config.SourcePath(), "routes", "accounts", "[id]", "preferences")
	if err := os.MkdirAll(routeDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	routeSource := `import { Context, Response, text } from trb/web

def get(_context: Context): Response
	return text("preferences")
end
`
	testSource := `import { Body, Headers, HttpMethod } from trb/http
import { Request } from trb/web
import { dispatch } from trb/web/testing
import { describe, expect, test } from trb/std/test

describe("Nested dynamic route") do
	test("dispatches below a path parameter") do
		response := dispatch(Request.new(method: HttpMethod.get(), path: "/accounts/42/preferences", query_string: "", headers: Headers.new(), body: Body.empty()))
		expect(response.status).to_equal(200)
		expect(response.body.to_s()).to_equal("preferences")
	end
end
`
	if err := os.WriteFile(filepath.Join(routeDirectory, "index.trb"), []byte(routeSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "web_test.trb"), []byte(testSource), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"test", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "PASS Nested dynamic route / dispatches below a path parameter") || !strings.Contains(stdout.String(), "1 test(s), 0 failure(s)") {
		t.Fatalf("unexpected test output:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
}

func TestGoTestSuiteRunsBesideWebRoute(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go is unavailable: %v", err)
	}
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/colocated-web-test"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/type-rb/colocated-web-test\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	routeDirectory := filepath.Join(config.SourcePath(), "routes")
	if err := os.MkdirAll(routeDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	routeSource := `import { Context, Response, text } from trb/web

def health_response(): Response
	return text("ok")
end

def get(_context: Context): Response
	return health_response()
end
`
	testSource := `import { health_response } from routes/health
import { describe, expect, test } from trb/std/test

describe("Health") do
	test("returns the response") do
		response := health_response()
		expect(response.status).to_equal(200)
		expect(response.body.to_s()).to_equal("ok")
	end
end
`
	if err := os.WriteFile(filepath.Join(routeDirectory, "health.trb"), []byte(routeSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(routeDirectory, "health_test.trb"), []byte(testSource), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"test", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "PASS Health / returns the response") || !strings.Contains(stdout.String(), "1 test(s), 0 failure(s)") {
		t.Fatalf("unexpected test output:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
}
