package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func TestCheckAndBuildDiagnoseNestedGoImportOfRunnableRoot(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.OutDir = "build"
	config.Go.Module = "example.com/root-import"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(config.SourcePath(), "routes"), 0o755); err != nil {
		t.Fatal(err)
	}
	mainSource := `OIDC_ISSUER := "https://identity.example.com/"

def main()
	return
end
`
	routeSource := `import { OIDC_ISSUER } from main

def issuer(): String
	return OIDC_ISSUER
end
`
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(mainSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "routes", "admin.trb"), []byte(routeSource), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, commandName := range []string{"check", "build"} {
		t.Run(commandName, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{commandName, "--config", config.Path}); status != 1 {
				t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			if stdout.Len() != 0 || !strings.Contains(stderr.String(), "error[TRB5000]") || !strings.Contains(stderr.String(), "cannot import runnable entrypoint module main in Go mode") {
				t.Fatalf("unexpected diagnostic stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
	if _, err := os.Stat(config.OutputPath()); !os.IsNotExist(err) {
		t.Fatalf("failed build wrote generated output: %v", err)
	}
}
