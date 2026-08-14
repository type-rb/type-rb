package sourcemap

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/token"
)

func TestRecorderMapsGeneratedUnicodePositionsToSource(t *testing.T) {
	output := "const greeting = \"こんにちは\";\nreturn greeting;\n"
	start := strings.Index(output, "return")
	recorder := NewRecorder("src/main.trb")
	recorder.Record(start, len(output), token.Span{
		Start: token.Position{Offset: 25, Line: 3, Column: 2},
		End:   token.Position{Offset: 40, Line: 3, Column: 17},
	})
	mapping := recorder.Build(output)
	location, found := mapping.SourceAt(PositionAt(output, start+2))
	if !found || location.Path != "src/main.trb" || location.Span.Start.Line != 3 {
		t.Fatalf("unexpected source location: %#v, found=%t", location, found)
	}
	if got := PositionAt(output, start); got.Line != 2 || got.Column != 1 {
		t.Fatalf("unexpected generated position: %#v", got)
	}
}

func TestExtractMarkersPreservesNestedSourceMappings(t *testing.T) {
	outer := Location{Path: "src/main.trb", Span: token.Span{Start: token.Position{Line: 1, Column: 1}, End: token.Position{Line: 5, Column: 4}}}
	inner := Location{Path: "src/main.trb", Span: token.Span{Start: token.Position{Line: 3, Column: 2}, End: token.Position{Line: 3, Column: 11}}}
	marked := StartMarker(0) + "\nfunc main() {\n\t" + StartMarker(1) + "\n\treturn\n\t" + EndMarker(1) + "\n}\n" + EndMarker(0) + "\n"
	output, mapping := ExtractMarkers(marked, map[int]Location{0: outer, 1: inner})
	if strings.Contains(output, markerPrefix) {
		t.Fatalf("source marker leaked into output:\n%s", output)
	}
	position := PositionAt(output, strings.Index(output, "return"))
	location, found := mapping.SourceAt(position)
	if !found || location.Span.Start.Line != 3 {
		t.Fatalf("nested mapping was not preferred: %#v, found=%t", location, found)
	}
}

func TestExtractMarkersKeepsUnknownMarkerShapedComments(t *testing.T) {
	comment := StartMarker(99) + "\n"
	output, mapping := ExtractMarkers(comment, map[int]Location{})
	if output != comment || len(mapping.Mappings) != 0 {
		t.Fatalf("unknown marker-shaped comment changed: output=%q map=%#v", output, mapping)
	}
}

func TestGoLineDirectivesExposeTypeRBSourceLocations(t *testing.T) {
	output := "package main\n\nfunc main() {\n\tanswer := 42\n\tprintln(answer)\n}\n"
	start := strings.Index(output, "\tanswer")
	end := strings.Index(output, "\tprintln") + len("\tprintln(answer)\n")
	recorder := NewRecorder("/workspace/src/main.trb")
	recorder.Record(start, end, token.Span{
		Start: token.Position{Line: 7, Column: 2},
		End:   token.Position{Line: 8, Column: 14},
	})
	generated := WithGoLineDirectives(output, "/tmp/generated/main.go", recorder.Build(output))
	for _, want := range []string{
		"//line /workspace/src/main.trb:7:2\n\tanswer := 42",
		"//line /workspace/src/main.trb:8:1\n\tprintln(answer)",
		"//line /tmp/generated/main.go:6:1\n}",
	} {
		if !strings.Contains(generated, want) {
			t.Fatalf("generated output does not contain %q:\n%s", want, generated)
		}
	}
}
