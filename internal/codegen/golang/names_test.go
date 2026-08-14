package golang

import (
	"strings"
	"testing"
)

func TestAnalyzeGoBindingNamesRenamesOnlyImportCollisions(t *testing.T) {
	names := analyzeGoBindingNames(
		map[string]bool{"message": true, "strings": true},
		map[string]string{"strings": ""},
	)
	if names["message"] != "" {
		t.Fatalf("ordinary binding was renamed to %q", names["message"])
	}
	if target := names["strings"]; !strings.HasPrefix(target, "__trbBinding_") {
		t.Fatalf("colliding binding target=%q", target)
	}
}

func TestAnalyzeGoBindingNamesUsesExplicitImportAliases(t *testing.T) {
	names := analyzeGoBindingNames(
		map[string]bool{"http": true, "nethttp": true, "sqlite": true},
		map[string]string{
			"net/http":           "nethttp",
			"modernc.org/sqlite": "_",
		},
	)
	if names["http"] != "" {
		t.Fatalf("unused package base renamed binding to %q", names["http"])
	}
	if names["nethttp"] == "" {
		t.Fatal("explicit import alias did not reserve its generated identifier")
	}
	if names["sqlite"] != "" {
		t.Fatalf("blank import renamed binding to %q", names["sqlite"])
	}
}
