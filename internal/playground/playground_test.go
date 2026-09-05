package playground

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestHandlerServesPlaygroundAndConfiguration(t *testing.T) {
	handler := Handler(Options{Mode: "ruby", Page: "play", Version: "1.2.3-test"})

	page := httptest.NewRecorder()
	handler.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/play/", nil))
	if page.Code != http.StatusOK {
		t.Fatalf("GET /play/ status=%d body=%s", page.Code, page.Body.String())
	}
	if !strings.Contains(page.Body.String(), `data-page-link="play">Playground</a>`) || !strings.Contains(page.Body.String(), `id="editor"`) {
		t.Fatalf("playground markup is incomplete:\n%s", page.Body.String())
	}
	if !strings.Contains(page.Body.String(), "scratch.trb") || strings.Contains(page.Body.String(), ">main.trb<") {
		t.Fatalf("playground should identify browser input as scratch source:\n%s", page.Body.String())
	}
	if !strings.Contains(page.Body.String(), `href="../" aria-label="TypeRB home"`) {
		t.Fatalf("playground logo does not link to the site homepage:\n%s", page.Body.String())
	}
	if !strings.Contains(page.Body.String(), `href="https://github.com/type-rb/type-rb"`) {
		t.Fatalf("playground does not link to the GitHub repository:\n%s", page.Body.String())
	}
	if !strings.Contains(page.Body.String(), `name="color-scheme" content="light"`) || !strings.Contains(page.Body.String(), `href="../docs/">Docs</a>`) {
		t.Fatalf("playground does not share the light website shell and navigation:\n%s", page.Body.String())
	}
	if policy := page.Header().Get("Content-Security-Policy"); !strings.Contains(policy, "connect-src 'self'") {
		t.Fatalf("missing local-only content security policy: %q", policy)
	}

	script := httptest.NewRecorder()
	handler.ServeHTTP(script, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if script.Code != http.StatusOK || !strings.Contains(script.Body.String(), "scratch.trb") {
		t.Fatalf("playground diagnostics do not use the scratch filename: status=%d\n%s", script.Code, script.Body.String())
	}
	if strings.Contains(script.Body.String(), `replaceAll("\t", "    ")`) {
		t.Fatalf("playground diagnostics still render tabs at width four:\n%s", script.Body.String())
	}

	config := httptest.NewRecorder()
	handler.ServeHTTP(config, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	var payload Config
	if err := json.Unmarshal(config.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Transport != "http" || payload.Mode != "ruby" || payload.Page != "play" || payload.Version != "1.2.3-test" {
		t.Fatalf("unexpected config: %#v", payload)
	}
}

func TestRunUsesCompilerTypedIRAndReturnsGeneratedTarget(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		handler := Handler(Options{Mode: mode})
		result := post(t, handler, "/api/run", Request{Source: "value := 1 + 2\nputs(value)\n", Mode: mode})
		if !result.OK || result.Output != "3\n" {
			t.Fatalf("%s run failed: %#v", mode, result)
		}
		if strings.TrimSpace(result.Generated) == "" {
			t.Fatalf("%s did not return generated target source", mode)
		}
	}
}

func TestRunReturnsSourceDiagnostics(t *testing.T) {
	result := post(t, Handler(Options{Mode: "go"}), "/api/run", Request{
		Source: "name := \"Ada\"\nname = 1\n", Mode: "go",
	})
	if result.OK || len(result.Diagnostics) == 0 {
		t.Fatalf("expected a diagnostic: %#v", result)
	}
	first := result.Diagnostics[0]
	if first.Line != 2 || first.Column == 0 || first.EndLine != 2 || first.EndColumn <= first.Column || !strings.Contains(first.Message, "immutable") {
		t.Fatalf("unexpected diagnostic: %#v", first)
	}
}

func TestRunReportsMalformedPortableExpressionsWithoutLeakingNativeASTTerms(t *testing.T) {
	result := post(t, Handler(Options{Mode: "go"}), "/api/run", Request{
		Source: "value := `native command`\n", Mode: "go",
	})
	if result.OK || len(result.Diagnostics) == 0 {
		t.Fatalf("expected a diagnostic: %#v", result)
	}
	first := result.Diagnostics[0]
	if first.Message != "unsupported expression syntax in portable TypeRB" || strings.Contains(first.Message, "Ruby-native") {
		t.Fatalf("unexpected diagnostic: %#v", first)
	}
}

func TestFormatUsesCommentPreservingFormatter(t *testing.T) {
	result := post(t, Handler(Options{Mode: "go"}), "/api/format", Request{
		Source: "def greet(name: String) # keep\n puts(name)\n return\nend\n", Mode: "go",
	})
	if !result.OK {
		t.Fatalf("format failed: %#v", result)
	}
	want := "def greet(name: String) # keep\n\tputs(name)\n\treturn\nend\n"
	if result.Formatted != want {
		t.Fatalf("unexpected format\nwant:\n%s\ngot:\n%s", want, result.Formatted)
	}
}

func TestRunRejectsHostAndPlatformPackages(t *testing.T) {
	for _, source := range []string{
		"import trb/std/path\nimport trb/std/file\nimport { FileSystemError } from trb/std/errors\nimport { Result } from trb/std/result\n\ndef probe(): Result<Bytes, FileSystemError>\n\treturn File.open(Path.new(\".\")) do |file|\n\t\ttry file.read(max_bytes: 1)\n\tend\nend\n",
		"import trb/std/path\nimport trb/std/dir\nimport { DirEntry } from trb/std/dir\nimport { FileSystemError } from trb/std/errors\nimport { Result } from trb/std/result\n\ndef probe(): Result<Array<DirEntry<Path>>, FileSystemError>\n\treturn Dir.children(Path.new(\".\"), max_entries: 1)\nend\n",
		"import trb/std/process\nProcess.argv()\n",
		"import trb/platform/go/http\nHTTP.router()\n",
	} {
		result := post(t, Handler(Options{Mode: "go"}), "/api/run", Request{Source: source, Mode: "go"})
		if result.OK || len(result.Diagnostics) == 0 || !strings.Contains(result.Diagnostics[0].Message, "not available in the browser playground") {
			t.Fatalf("host capability was not rejected for %q: %#v", source, result)
		}
	}
}

func TestRunStopsCanceledEvaluation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	cancel()
	result := Run(ctx, "while true\nend\n", "go")
	if result.OK || len(result.Diagnostics) == 0 || !strings.Contains(result.Diagnostics[0].Message, "evaluation exceeded") {
		t.Fatalf("canceled evaluation was not stopped: %#v", result)
	}
}

func TestAPIRejectsCrossOriginRequests(t *testing.T) {
	requestBody := httptest.NewRequest(http.MethodPost, "/api/run", strings.NewReader(`{"source":"puts(1)","mode":"go"}`))
	requestBody.Header.Set("Content-Type", "application/json")
	requestBody.Header.Set("Origin", "https://example.com")
	requestBody.Host = "127.0.0.1:3000"
	response := httptest.NewRecorder()
	Handler(Options{Mode: "go"}).ServeHTTP(response, requestBody)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestTourCatalogHasStableUniqueLessons(t *testing.T) {
	lessons := Tour()
	if len(lessons) != 12 {
		t.Fatalf("tour has %d lessons, want 12", len(lessons))
	}
	seen := map[string]bool{}
	chapters := []string{}
	for _, lesson := range lessons {
		if lesson.ID == "" || lesson.Chapter == "" || lesson.Title == "" || lesson.Source == "" || lesson.Expected == "" {
			t.Fatalf("incomplete lesson: %#v", lesson)
		}
		if seen[lesson.ID] {
			t.Fatalf("duplicate lesson id %q", lesson.ID)
		}
		seen[lesson.ID] = true
		if len(chapters) == 0 || chapters[len(chapters)-1] != lesson.Chapter {
			chapters = append(chapters, lesson.Chapter)
		}
	}
	wantChapters := []string{"Start", "Write programs", "Model data and errors", "Portability"}
	if !slices.Equal(chapters, wantChapters) {
		t.Fatalf("tour chapters=%v, want %v", chapters, wantChapters)
	}
}

func TestValidateTourAcrossModes(t *testing.T) {
	if testing.Short() {
		t.Skip("tour validation is a checkpoint test")
	}
	count, err := validateTour(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := len(Tour()) * 3; count != want {
		t.Fatalf("checked %d executions, want %d", count, want)
	}
}

func TestExportStaticBuildsHostIndependentSite(t *testing.T) {
	output := t.TempDir()
	if err := ExportStatic(StaticOptions{OutputDir: output, Version: "1.2.3-test"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"runtime.json",
		"tour.json",
		"play/index.html",
		"tour/index.html",
		"type-rb/index.html",
		"type-rb/play/index.html",
		"type-rb/tour/index.html",
		"assets/app.css",
		"assets/app.js",
		"assets/playground-worker.js",
	} {
		if _, err := os.Stat(filepath.Join(output, name)); err != nil {
			t.Fatalf("missing static file %s: %v", name, err)
		}
	}

	data, err := os.ReadFile(filepath.Join(output, "play/index.html"))
	if err != nil {
		t.Fatal(err)
	}
	markup := string(data)
	if !strings.Contains(markup, `href="../assets/app.css"`) || strings.Contains(markup, `href="/assets/`) {
		t.Fatalf("static markup is not base-path independent:\n%s", markup)
	}

	data, err = os.ReadFile(filepath.Join(output, "runtime.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if config.Transport != "wasm" || config.Mode != "go" || config.Version != "1.2.3-test" {
		t.Fatalf("unexpected static config: %#v", config)
	}

	for path, destination := range map[string]string{
		"type-rb/index.html":      "/",
		"type-rb/play/index.html": "/play/",
		"type-rb/tour/index.html": "/tour/",
	} {
		data, err := os.ReadFile(filepath.Join(output, path))
		if err != nil {
			t.Fatal(err)
		}
		redirect := string(data)
		if !strings.Contains(redirect, `http-equiv="refresh"`) || !strings.Contains(redirect, `url=`+destination) {
			t.Fatalf("legacy redirect %s does not target %s:\n%s", path, destination, redirect)
		}
	}
}

func post(t *testing.T, handler http.Handler, path string, input Request) Response {
	t.Helper()
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(data)))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("POST %s status=%d body=%s", path, recorder.Code, recorder.Body.String())
	}
	var result Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}
