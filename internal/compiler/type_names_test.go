package compiler

import (
	"strings"
	"testing"
)

func TestTypeNamesRequireCanonicalSpellingAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range []struct {
			name   string
			type_  string
			wanted string
		}{
			{name: "short Integer alias", type_: "Int", wanted: "type name Int is not canonical; use Integer"},
			{name: "lowercase Integer alias", type_: "int", wanted: "type name int is not canonical; use Integer"},
			{name: "Boolean alias", type_: "bool", wanted: "type name bool is not canonical; use Boolean"},
			{name: "Hash alias", type_: "Map<String, Integer>", wanted: "type name Map is not canonical; use Hash"},
		} {
			t.Run(mode+"/"+test.name, func(t *testing.T) {
				source := []byte("record User\n\tid: " + test.type_ + "\nend\n")
				_, err := Compile("user.trb", source, mode)
				if err == nil || !strings.Contains(err.Error(), test.wanted) {
					t.Fatalf("Compile() error=%v, want %q", err, test.wanted)
				}
				compilation, ok := err.(*CompileError)
				if !ok || len(compilation.Diagnostics) != 1 || len(compilation.Diagnostics[0].Fixes) != 1 {
					t.Fatalf("Compile() diagnostics=%#v, want one canonical type fix", compilation)
				}
				fix := compilation.Diagnostics[0].Fixes[0]
				if len(fix.Edits) != 1 || fix.Edits[0].Replacement != strings.Fields(test.wanted)[len(strings.Fields(test.wanted))-1] {
					t.Fatalf("canonical type fix=%#v", fix)
				}
				edited := fix.Edits[0]
				if selected := string(source[edited.Location.Span.Start.Offset:edited.Location.Span.End.Offset]); selected != strings.Fields(test.wanted)[2] {
					t.Fatalf("canonical type fix selected %q", selected)
				}
			})
		}
	}
}

func TestUnknownTypeNamesAreRejectedAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			_, err := Compile("user.trb", []byte("record User\n\tid: MissingType\nend\n"), mode)
			if err == nil || !strings.Contains(err.Error(), "type MissingType is not declared or imported") {
				t.Fatalf("Compile() error=%v, want unknown type diagnostic", err)
			}
		})
	}
}

func TestTypeParametersRemainValidTypeNames(t *testing.T) {
	source := []byte(`record Pair<T, U>
	left: T
	right: U
end

def identity<T>(value: T): T
	return value
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("generic.trb", source, mode); err != nil {
			t.Fatalf("%s rejected scoped type parameters: %v", mode, err)
		}
	}
}
