package playground

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	if !strings.Contains(page.Body.String(), "TypeRB Playground") || !strings.Contains(page.Body.String(), `id="editor"`) {
		t.Fatalf("playground markup is incomplete:\n%s", page.Body.String())
	}
	if policy := page.Header().Get("Content-Security-Policy"); !strings.Contains(policy, "connect-src 'self'") {
		t.Fatalf("missing local-only content security policy: %q", policy)
	}

	config := httptest.NewRecorder()
	handler.ServeHTTP(config, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	var payload configResponse
	if err := json.Unmarshal(config.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Mode != "ruby" || payload.Page != "play" || payload.Version != "1.2.3-test" {
		t.Fatalf("unexpected config: %#v", payload)
	}
}

func TestRunUsesCompilerTypedIRAndReturnsGeneratedTarget(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		handler := Handler(Options{Mode: mode})
		result := post(t, handler, "/api/run", request{Source: "value := 1 + 2\nputs(value)\n", Mode: mode})
		if !result.OK || result.Output != "3\n" {
			t.Fatalf("%s run failed: %#v", mode, result)
		}
		if strings.TrimSpace(result.Generated) == "" {
			t.Fatalf("%s did not return generated target source", mode)
		}
	}
}

func TestRunReturnsSourceDiagnostics(t *testing.T) {
	result := post(t, Handler(Options{Mode: "go"}), "/api/run", request{
		Source: "name := \"Ada\"\nname = 1\n", Mode: "go",
	})
	if result.OK || len(result.Diagnostics) == 0 {
		t.Fatalf("expected a diagnostic: %#v", result)
	}
	first := result.Diagnostics[0]
	if first.Line != 2 || first.Column == 0 || !strings.Contains(first.Message, "immutable") {
		t.Fatalf("unexpected diagnostic: %#v", first)
	}
}

func TestFormatUsesCommentPreservingFormatter(t *testing.T) {
	result := post(t, Handler(Options{Mode: "go"}), "/api/format", request{
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
		"import trb/std/filesystem\nfilesystem.exists(\".\")\n",
		"import trb/std/process\nprocess.argv()\n",
		"import trb/platform/go/http\n",
	} {
		result := post(t, Handler(Options{Mode: "go"}), "/api/run", request{Source: source, Mode: "go"})
		if result.OK || len(result.Diagnostics) == 0 || !strings.Contains(result.Diagnostics[0].Message, "not available in the browser playground") {
			t.Fatalf("host capability was not rejected for %q: %#v", source, result)
		}
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
	if len(lessons) < 6 {
		t.Fatalf("tour is too short: %d lessons", len(lessons))
	}
	seen := map[string]bool{}
	for _, lesson := range lessons {
		if lesson.ID == "" || lesson.Title == "" || lesson.Source == "" || lesson.Expected == "" {
			t.Fatalf("incomplete lesson: %#v", lesson)
		}
		if seen[lesson.ID] {
			t.Fatalf("duplicate lesson id %q", lesson.ID)
		}
		seen[lesson.ID] = true
	}
}

func TestValidateTourAcrossModes(t *testing.T) {
	if testing.Short() {
		t.Skip("tour validation is a checkpoint test")
	}
	count, err := ValidateTour(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := len(Tour()) * 3; count != want {
		t.Fatalf("checked %d executions, want %d", count, want)
	}
}

func post(t *testing.T, handler http.Handler, path string, input request) response {
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
	var result response
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}
