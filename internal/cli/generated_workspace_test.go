package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/type-rb/type-rb/internal/project"
)

func TestGeneratedWorkspaceCleansAndRetainsTargetSource(t *testing.T) {
	root := t.TempDir()
	workspace, err := createGeneratedWorkspace(root, "run")
	if err != nil {
		t.Fatal(err)
	}
	expectedBase := filepath.Join(root, ".trb", "run") + string(filepath.Separator)
	if !strings.HasPrefix(workspace.Path(), expectedBase) {
		t.Fatalf("workspace %s is outside %s", workspace.Path(), expectedBase)
	}
	if err := os.WriteFile(filepath.Join(workspace.Path(), "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	removed, err := reapGeneratedWorkspaces(root, "run", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 0 {
		t.Fatalf("active workspace was reaped: %v", removed)
	}
	workspacePath := workspace.Path()
	if retained, err := workspace.Close(); err != nil || retained != "" {
		t.Fatalf("close retained=%q err=%v", retained, err)
	}
	if _, err := os.Stat(workspacePath); !os.IsNotExist(err) {
		t.Fatalf("workspace remained after close: %v", err)
	}

	kept, err := createGeneratedWorkspace(root, "run")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kept.Path(), "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	kept.Keep()
	retainedPath, err := kept.Close()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(retainedPath) != filepath.Join(root, ".trb", "generated") {
		t.Fatalf("unexpected retained path %s", retainedPath)
	}
	if _, err := os.Stat(filepath.Join(retainedPath, "main.go")); err != nil {
		t.Fatalf("retained source is unavailable: %v", err)
	}

	next, err := createGeneratedWorkspace(root, "run")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := next.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(retainedPath); err != nil {
		t.Fatalf("automatic orphan recovery removed retained source: %v", err)
	}
}

func TestGeneratedWorkspaceReapsReleasedLease(t *testing.T) {
	root := t.TempDir()
	workspace, err := createGeneratedWorkspace(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	workspacePath := workspace.Path()
	if err := workspace.releaseLease(); err != nil {
		t.Fatal(err)
	}
	workspace.closed = true // Simulate a process that exited without removing its workspace.

	removed, err := reapGeneratedWorkspaces(root, "test", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != workspacePath {
		t.Fatalf("unexpected recovered workspaces: %v", removed)
	}
	if _, err := os.Stat(workspacePath); !os.IsNotExist(err) {
		t.Fatalf("orphan workspace remained: %v", err)
	}
}

func TestCleanRemovesOnlyRequestedCompilerState(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.OutDir = "build"
	config.Go.Module = "example.com/type-rb/clean"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(root, "build", "main.go"),
		filepath.Join(root, ".trb", "packages", "checksum", "src", "index.trb"),
		filepath.Join(root, ".trb", "native-types.json"),
		filepath.Join(root, ".trb", "generated", "saved", "main.go"),
		filepath.Join(root, ".trb", "repl_history"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("state\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	orphan, err := createGeneratedWorkspace(root, "run")
	if err != nil {
		t.Fatal(err)
	}
	orphanPath := orphan.Path()
	if err := orphan.releaseLease(); err != nil {
		t.Fatal(err)
	}
	orphan.closed = true

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"clean", "--config", config.Path, "--build", "--cache", "--generated"}); status != 0 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	for _, path := range []string{
		filepath.Join(root, "build"),
		filepath.Join(root, ".trb", "packages"),
		filepath.Join(root, ".trb", "native-types.json"),
		filepath.Join(root, ".trb", "generated"),
		orphanPath,
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("cleaned path remained %s: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".trb", "repl_history")); err != nil {
		t.Fatalf("clean removed REPL history: %v", err)
	}
	for _, expected := range []string{".trb/run/", ".trb/generated", ".trb/packages", ".trb/native-types.json", "build"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("clean output does not mention %q: %s", expected, stdout.String())
		}
	}
}

func TestCleanSkipsActiveGeneratedWorkspace(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.Go.Module = "example.com/type-rb/active-clean"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	workspace, err := createGeneratedWorkspace(root, "run")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = workspace.Close() }()

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"clean", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(workspace.Path()); err != nil {
		t.Fatalf("clean removed active workspace: %v", err)
	}
}

func TestCopyProjectFilesSkipsLegacyGeneratedWorkspaces(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "build")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"trb-run-123", "trb-test-456"} {
		path := filepath.Join(root, name, "generated.go")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("generated\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := copyProjectFiles(root, output); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(output, "README.md")); err != nil {
		t.Fatalf("ordinary project file was not copied: %v", err)
	}
	for _, name := range []string{"trb-run-123", "trb-test-456"} {
		if _, err := os.Stat(filepath.Join(output, name)); !os.IsNotExist(err) {
			t.Fatalf("legacy generated workspace %s was copied: %v", name, err)
		}
	}
}

func TestRunKeepGeneratedRetainsExactTargetTree(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go is unavailable: %v", err)
	}
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/keep-generated"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/type-rb/keep-generated\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte("def main()\n\tputs(\"kept\")\n\treturn\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"run", "--config", config.Path, "--keep-generated"}); status != 0 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "kept\n") || !strings.Contains(stdout.String(), "generated files kept at .trb/generated/") {
		t.Fatalf("unexpected run output: %s", stdout.String())
	}
	retained, err := os.ReadDir(filepath.Join(root, ".trb", "generated"))
	if err != nil || len(retained) != 1 {
		t.Fatalf("retained entries=%v err=%v", retained, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".trb", "generated", retained[0].Name(), "main.go")); err != nil {
		t.Fatalf("generated Go entrypoint is unavailable: %v", err)
	}
	runEntries, err := os.ReadDir(filepath.Join(root, ".trb", "run"))
	if err != nil || len(runEntries) != 0 {
		t.Fatalf("run workspace leaked: entries=%v err=%v", runEntries, err)
	}
}

func TestRunCleansGeneratedWorkspaceAcrossBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			required := map[string]string{"go": "go", "ruby": "ruby", "typescript": "bun"}[mode]
			if _, err := exec.LookPath(required); err != nil {
				t.Skipf("%s is unavailable: %v", required, err)
			}
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if mode == "go" {
				config.Go.Module = "example.com/type-rb/workspace-cleanup"
			}
			if mode == "typescript" {
				config.TypeScript.Runtime = project.TypeScriptRuntimeBun
				config.TypeScript.PackageManager = "bun"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
				t.Fatal(err)
			}
			if mode == "go" {
				module := "module example.com/type-rb/workspace-cleanup\n\ngo 1.26\n"
				if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(module), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			source := "def main()\n\tputs(\"workspace-cleanup\")\n\treturn\nend\n"
			if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
				t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			if stdout.String() != "workspace-cleanup\n" {
				t.Fatalf("unexpected run output %q", stdout.String())
			}
			entries, err := os.ReadDir(filepath.Join(root, ".trb", "run"))
			if err != nil || len(entries) != 0 {
				t.Fatalf("%s run workspace leaked: entries=%v err=%v", mode, entries, err)
			}
		})
	}
}

func TestCleanRemovesUnknownOldWorkspaceButNotNewWorkspace(t *testing.T) {
	root := t.TempDir()
	base := generatedWorkspaceBase(root, "run")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(base, "123-old")
	newPath := filepath.Join(base, "456-new")
	for _, path := range []string{oldPath, newPath} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	oldTime := time.Now().Add(-2 * workspaceCreationGrace)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	removed, err := reapGeneratedWorkspaces(root, "run", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != oldPath {
		t.Fatalf("unexpected unknown workspace cleanup: %v", removed)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("new workspace was removed during its creation grace: %v", err)
	}
}

func TestGeneratedWorkspaceCreationRecoversUnknownOldWorkspace(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(generatedWorkspaceBase(root, "run"), "123-incomplete")
	if err := os.MkdirAll(oldPath, 0o755); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-2 * workspaceCreationGrace)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	workspace, err := createGeneratedWorkspace(root, "run")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = workspace.Close() }()
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("incomplete orphan remained after the next run: %v", err)
	}
}
