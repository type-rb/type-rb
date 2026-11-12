package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
	"golang.org/x/mod/modfile"
)

func standaloneOIDCFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	entry := filepath.Join(root, "main.trb")
	files := map[string]string{
		"main.trb": "import { probe } from provider\ndef main()\n\tputs(probe())\nend\n",
		"provider.trb": `import { OidcBearerOptions } from trb/auth/oidc
import { Body, Headers, HttpMethod } from trb/http
import { Request } from trb/web
import trb/web/auth/bearer
import trb/std/result

def probe(): String
	request := Request.new(method: HttpMethod.get(), path: "/", query_string: "", headers: Headers.new(), body: Body.empty())
	options := OidcBearerOptions.new(issuer: "https://provider.example.invalid", audience: "api", jwks_uri: nil, roles_claim: "roles", clock_skew_seconds: 0)
	case Bearer.verify(request, options)
	when Result::Err(error)
		return error.message
	when Result::Ok(_principal)
		return "unexpected"
	end
end
`,
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root, entry
}

func TestStandaloneGoModulePreservesOfficialDependencyVersions(t *testing.T) {
	root, entry := standaloneOIDCFixture(t)
	graph, err := loadFileRootSourceGraph(entry, os.ReadFile)
	if err != nil {
		t.Fatal(err)
	}
	config, standalone, err := loadCommandConfig("build", "", root, entry, "go", "")
	if err != nil || !standalone {
		t.Fatalf("load standalone config: %v, standalone=%v", err, standalone)
	}
	// Dependencies must describe the same source snapshot as compilation.
	if err := os.Remove(filepath.Join(root, "provider.trb")); err != nil {
		t.Fatal(err)
	}
	compiled, err := compileProjectSources(config, graph.Sources)
	if err != nil {
		t.Fatal(err)
	}
	output := t.TempDir()
	if err := writeStandaloneGoModule(config, output, compiled, graph.Sources); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(output, "go.mod")
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	module, err := modfile.Parse(filename, data, nil)
	if err != nil {
		t.Fatal(err)
	}
	requirements := map[string]string{}
	for _, requirement := range module.Require {
		requirements[requirement.Mod.Path] = requirement.Mod.Version
	}
	for name, want := range map[string]string{"golang.org/x/net": "v0.62.0", "golang.org/x/text": "v0.43.0"} {
		if requirements[name] != want {
			t.Errorf("%s: got %q, want %q before Go resolves dependencies\n%s", name, requirements[name], want, data)
		}
	}
}

func TestStandaloneGoOIDCRunAndBuild(t *testing.T) {
	root, entry := standaloneOIDCFixture(t)
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	const want = "Authorization header is required\n"
	if status := command.Run([]string{"run", entry}); status != 0 || stdout.String() != want {
		t.Fatalf("run: status=%d stdout=%q stderr=%s", status, stdout.String(), stderr.String())
	}
	if strings.Contains(stderr.String(), "finding module for package") {
		t.Fatalf("run performed an unversioned dependency lookup: %s", stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	output := filepath.Join(root, "oidc-app")
	if status := command.Run([]string{"build", "--compile", "--outfile", output, entry}); status != 0 {
		t.Fatalf("build: status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if strings.Contains(stderr.String(), "finding module for package") {
		t.Fatalf("build performed an unversioned dependency lookup: %s", stderr.String())
	}
	observed, err := exec.Command(output).CombinedOutput()
	if err != nil || string(observed) != want {
		t.Fatalf("execute: err=%v stdout=%q", err, observed)
	}
	for _, name := range []string{"go.mod", "go.sum", project.ConfigName} {
		if _, err := os.Stat(filepath.Join(root, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("standalone command wrote %s into the source directory: %v", name, err)
		}
	}
}
