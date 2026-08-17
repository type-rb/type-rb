package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLoadFileRootSourceGraphSnapshotsCanonicalImportClosure(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"main.trb": `import "./exact.trb"
import "./directory"
import "./excluded_test"
import "./missing"
import trb/std/io
`,
		"exact.trb":             "import \"./nested/transitive\"\n",
		"nested/transitive.trb": "import \"./main\"\n",
		"directory/index.trb":   "def indexed(): Integer\n\treturn 1\nend\n",
		"exact/index.trb":       "this file must lose to exact.trb\n",
		"excluded_test.trb":     "def excluded(): Integer\n\treturn 1\nend\n",
		"unrelated.trb":         "this sibling must not be loaded\n",
	}
	for relative, source := range files {
		filename := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	reads := map[string]int{}
	readFile := func(filename string) ([]byte, error) {
		reads[filename]++
		return os.ReadFile(filename)
	}
	entry := filepath.Join(root, "main.trb")
	graph, err := loadFileRootSourceGraph(entry, readFile)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(root, "directory", "index.trb"),
		filepath.Join(root, "exact.trb"),
		filepath.Join(root, "main.trb"),
		filepath.Join(root, "nested", "transitive.trb"),
	}
	got := make([]string, 0, len(graph.Sources))
	for _, source := range graph.Sources {
		got = append(got, source.Filename)
		if reads[source.Filename] != 1 {
			t.Fatalf("source %s was read %d times", source.Filename, reads[source.Filename])
		}
	}
	if graph.Root != root || graph.Entry != entry || !slices.Equal(got, want) {
		t.Fatalf("unexpected graph: root=%s entry=%s sources=%v", graph.Root, graph.Entry, got)
	}
	if reads[filepath.Join(root, "exact", "index.trb")] != 0 || reads[filepath.Join(root, "unrelated.trb")] != 0 {
		t.Fatalf("unrelated or shadowed sources were read: %v", reads)
	}
	if reads[filepath.Join(root, "excluded_test.trb")] != 0 {
		t.Fatalf("production graph read an imported test file: %v", reads)
	}
	if reads[filepath.Join(root, "missing.trb")] != 1 || reads[filepath.Join(root, "missing", "index.trb")] != 1 {
		t.Fatalf("missing candidates were not resolved deterministically: %v", reads)
	}
	for filename, count := range reads {
		if count != 1 {
			t.Fatalf("candidate %s was read %d times", filename, count)
		}
	}
}

func TestLoadFileRootSourceGraphUsesReaderSnapshot(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "main.trb")
	overlayHelper := filepath.Join(root, "overlay.trb")
	diskHelper := filepath.Join(root, "disk.trb")
	if err := os.WriteFile(entry, []byte("import disk\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(diskHelper, []byte("def disk(): Integer\n\treturn 1\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	overlays := map[string][]byte{
		entry:         []byte("import overlay\n"),
		overlayHelper: []byte("def overlay(): Integer\n\treturn 2\nend\n"),
	}
	readFile := func(filename string) ([]byte, error) {
		if source, ok := overlays[filename]; ok {
			return source, nil
		}
		return os.ReadFile(filename)
	}

	graph, err := loadFileRootSourceGraph(entry, readFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Sources) != 2 || graph.Sources[0].Filename != entry || graph.Sources[1].Filename != overlayHelper {
		t.Fatalf("graph did not follow overlay imports: %+v", graph.Sources)
	}
	overlays[entry][0] = 'X'
	if string(graph.Sources[0].Source) != "import overlay\n" {
		t.Fatalf("graph retained mutable reader bytes: %q", graph.Sources[0].Source)
	}
}

func TestRunConfigFreeFileRootImportClosureAcrossBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			required := mode
			if mode == "typescript" {
				required = "node"
			}
			if _, err := exec.LookPath(required); err != nil {
				t.Skipf("%s is not installed", required)
			}

			root := t.TempDir()
			sources := map[string]string{
				"main.trb": `import { exact_value } from "./exact.trb"
import { indexed_value } from "./directory"

def main()
	puts(exact_value())
	puts(indexed_value())
	return
end
`,
				"exact.trb": `import { transitive_value } from "./nested/transitive"

def exact_value(): String
	return transitive_value()
end
`,
				"nested/transitive.trb": `def transitive_value(): String
	return "exact"
end
`,
				"directory/index.trb": `def indexed_value(): String
	return "index"
end
`,
				"exact/index.trb": "def broken(\n",
				"unrelated.trb":   "def broken(\n",
			}
			for relative, source := range sources {
				filename := filepath.Join(root, filepath.FromSlash(relative))
				if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filename, []byte(source), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			args := []string{"run", "--mode", mode}
			if mode == "typescript" {
				args = append(args, "--runtime", "node")
			}
			args = append(args, filepath.Join(root, "main.trb"))
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run(args); status != 0 {
				t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			if stdout.String() != "exact\nindex\n" || stderr.Len() != 0 {
				t.Fatalf("unexpected output stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunConfigFreeRequiresMainInSelectedEntry(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "entry.trb")
	helper := filepath.Join(root, "helper.trb")
	if err := os.WriteFile(entry, []byte("import { value } from helper\n\ndef entry_value(): Integer\n\treturn value()\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(helper, []byte("def value(): Integer\n\treturn 1\nend\n\ndef main()\n\treturn\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"run", entry}); status != 1 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "standalone file has no top-level main()") {
		t.Fatalf("unexpected diagnostic: %s", stderr.String())
	}
}

func TestRunConfigFreeDoesNotCompileImportedTestFiles(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "main.trb")
	testHelper := filepath.Join(root, "helper_test.trb")
	if err := os.WriteFile(entry, []byte("import { helper } from helper_test\n\ndef main()\n\tputs(helper())\n\treturn\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(testHelper, []byte("def helper(): Integer\n\treturn 1\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--mode", mode, entry}); status != 1 {
				t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), "cannot resolve project import helper_test") {
				t.Fatalf("unexpected diagnostic: %s", stderr.String())
			}
		})
	}
}

func TestRunConfigFreeReportsStableGraphDiagnostics(t *testing.T) {
	for _, test := range []struct {
		name  string
		files map[string]string
		want  string
	}{
		{
			name: "missing import",
			files: map[string]string{
				"main.trb": "import { missing_value } from missing\n\ndef main()\n\tputs(missing_value())\n\treturn\nend\n",
			},
			want: "cannot resolve project import missing",
		},
		{
			name: "import cycle",
			files: map[string]string{
				"main.trb": "import { value } from a\n\ndef start(): Integer\n\treturn value()\nend\n\ndef main()\n\tputs(start())\n\treturn\nend\n",
				"a.trb":    "import { start } from main\n\ndef value(): Integer\n\treturn start()\nend\n",
			},
			want: "import cycle:",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			for relative, source := range test.files {
				if err := os.WriteFile(filepath.Join(root, relative), []byte(source), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			entry := filepath.Join(root, "main.trb")
			var previous string
			for attempt := 0; attempt < 2; attempt++ {
				var stdout, stderr bytes.Buffer
				command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
				if status := command.Run([]string{"run", entry}); status != 1 {
					t.Fatalf("attempt=%d status=%d stdout=%s stderr=%s", attempt, status, stdout.String(), stderr.String())
				}
				if !strings.Contains(stderr.String(), test.want) {
					t.Fatalf("attempt=%d unexpected diagnostic: %s", attempt, stderr.String())
				}
				if attempt > 0 && stderr.String() != previous {
					t.Fatalf("diagnostic changed between identical snapshots\nfirst:\n%s\nsecond:\n%s", previous, stderr.String())
				}
				previous = stderr.String()
			}
		})
	}
}

func TestLoadFileRootSourceGraphReportsEntryReadError(t *testing.T) {
	entry := filepath.Join(t.TempDir(), "missing.trb")
	_, err := loadFileRootSourceGraph(entry, os.ReadFile)
	if err == nil || !errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), entry) {
		t.Fatalf("unexpected error: %v", err)
	}
}
