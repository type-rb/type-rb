package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocumentationOnlyExportKeepsDocsWithoutBrowserExecution(t *testing.T) {
	out := t.TempDir()
	if err := Export(Options{OutputDir: out, DocsDir: "../../docs", Version: "0.5.0", DocsOnly: true}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateInternalLinks(out); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"play/index.html", "tour/index.html", "type-rb/play/index.html", "type-rb/tour/index.html"} {
		data, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if !strings.Contains(text, "temporarily paused") || !strings.Contains(text, "Transpile and Format") || strings.Contains(text, "<script") {
			t.Fatalf("wrong unavailable page: %s", name)
		}
	}
	legacy, err := os.ReadFile(filepath.Join(out, "type-rb/index.html"))
	if err != nil || !strings.Contains(string(legacy), "url=/") {
		t.Fatal("legacy home must reach documentation", err)
	}
	for _, name := range []string{"assets/trb.wasm", "assets/wasm_exec.js", "assets/playground-worker.js", "assets/app.js", "runtime.json", "tour.json"} {
		if _, err := os.Stat(filepath.Join(out, name)); !os.IsNotExist(err) {
			t.Fatalf("browser asset exported: %s (%v)", name, err)
		}
	}
	for _, name := range []string{"index.html", "docs/index.html", "docs/native-transition/index.html", "assets/docs.js"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDocumentationOnlyRefusesStaleInteractiveAssets(t *testing.T) {
	for _, name := range []string{"assets/trb.wasm", "assets/wasm_exec.js", "assets/playground-worker.js", "assets/app.js", "assets/app.css", "runtime.json", "tour.json"} {
		t.Run(name, func(t *testing.T) {
			out := t.TempDir()
			p := filepath.Join(out, name)
			if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte("retained"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := Export(Options{OutputDir: out, DocsDir: "../../docs", DocsOnly: true}); err == nil {
				t.Fatal("stale runtime accepted")
			}
			data, err := os.ReadFile(p)
			if err != nil || string(data) != "retained" {
				t.Fatal("existing data changed")
			}
		})
	}
}
