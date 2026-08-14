package diagnostic

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/token"
)

func TestNormalizePreservesSpecificCodesAndFillsNestedPaths(t *testing.T) {
	span := token.Span{Start: token.Position{Offset: 3, Line: 2, Column: 1}, End: token.Position{Offset: 7, Line: 2, Column: 5}}
	items := Normalize([]Diagnostic{{
		Code: DuplicateBinding, Severity: Error, Message: "duplicate", Span: span,
		Related: []RelatedInformation{{Message: "first declaration", Location: Location{Span: span}}},
		Fixes:   []Fix{{Message: "remove it", Edits: []TextEdit{{Location: Location{Span: span}}}}},
	}}, "src/main.trb", TypeError)

	if items[0].Code != DuplicateBinding || items[0].Path != "src/main.trb" {
		t.Fatalf("unexpected normalized diagnostic: %#v", items[0])
	}
	if items[0].Related[0].Location.Path != "src/main.trb" || items[0].Fixes[0].Edits[0].Location.Path != "src/main.trb" {
		t.Fatalf("nested paths were not normalized: %#v", items[0])
	}
}

func TestJSONReportHasStableSchemaAndSummary(t *testing.T) {
	span := token.Span{Start: token.Position{Offset: 3, Line: 2, Column: 1}, End: token.Position{Offset: 7, Line: 2, Column: 5}}
	report := NewJSONReport([]Diagnostic{{
		Code: UnusedBinding, Severity: Error, Message: "unused", Path: "src/main.trb", Span: span,
		Fixes: []Fix{{Message: "remove import", Edits: []TextEdit{{Location: Location{Path: "src/main.trb", Span: span}, Replacement: ""}}}},
	}})
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, fragment := range []string{`"schemaVersion":1`, `"code":"TRB3002"`, `"line":2`, `"errors":1`, `"replacement":""`} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("JSON report is missing %s: %s", fragment, text)
		}
	}
}
