package compiler

import (
	"os"
	"strings"
	"testing"
)

func TestNestedFunctionAnnotationsRunAcrossBackends(t *testing.T) {
	source, err := os.ReadFile("testdata/nested_function_annotations.trb")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "nested_function_annotations.trb", source, "called\n5\n9\n13\n17\ncalled")
		})
	}
}

func TestNestedFunctionAnnotationsRejectVoidValuesAcrossModes(t *testing.T) {
	for _, annotation := range []string{
		"Array<Void>", "Hash<String, Void>", "Array<(Void) -> Integer>",
		"Array<() -> Array<Void>>", "Array<() -> Void?>",
	} {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			t.Run(mode+"/"+annotation, func(t *testing.T) {
				source := "def consume(value: " + annotation + ")\nend\n"
				_, err := Compile("invalid_function_annotation.trb", []byte(source), mode)
				if err == nil || !strings.Contains(err.Error(), "Void may be used only as the return type of a function type") {
					t.Fatalf("expected Void position diagnostic, got %v", err)
				}
			})
		}
	}
}
