package cliapp

import "testing"

func TestKebabUsesWordAndAcronymBoundaries(t *testing.T) {
	tests := map[string]string{
		"output_path": "output-path",
		"HTTPServer":  "http-server",
		"ServeHTTP":   "serve-http",
		"URL2Path":    "url2-path",
		"配信設定":        "配信設定",
	}
	for source, want := range tests {
		if got := kebab(source); got != want {
			t.Fatalf("kebab(%q) = %q, want %q", source, got, want)
		}
	}
}
