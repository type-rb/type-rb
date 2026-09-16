package compiler

import (
	"strings"
	"testing"
)

func TestImportedGenericEnumPatternsExecuteAcrossBackends(t *testing.T) {
	units := []SourceUnit{
		{Filename: "/project/model/item.trb", ModulePath: "model/item", Package: "model", Source: []byte(`enum Item<T>
Value(value: T)
Other(text: String)
end
`)},
		{Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: []byte(`import { Item as Choice } from model/item
def label(item: Choice<String>): String
return case item
when Choice::Value(value)
value
when Choice::Other(text)
text
end
end
def main()
puts(label(Choice<String>::Value("kept")))
puts(label(Choice<String>::Other("other")))
case Choice<Integer>::Value(7)
when Choice::Value(value)
puts(value + 1)
when Choice::Other(text)
puts(text)
end
end
`)},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireEffectRuntime(t, mode)
			artifacts, err := CompileProject(units, Options{Mode: mode, GoModule: "example.com/generic-enums", RubyLoader: "require_relative", ProjectRoot: "/project", SourceRoot: "/project"})
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, "example.com/generic-enums")); got != "kept\nother\n8" {
				t.Fatalf("got %q", got)
			}
		})
	}
}

func TestImportedGenericEnumPatternKeepsNominalIdentity(t *testing.T) {
	units := []SourceUnit{
		{Filename: "/project/left.trb", ModulePath: "left", Package: "left", Source: []byte("enum Item<T>\nValue(value: T)\nend\n")},
		{Filename: "/project/right.trb", ModulePath: "right", Package: "right", Source: []byte("enum Item<T>\nValue(value: T)\nend\n")},
		{Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: []byte(`import { Item as Left } from left
import { Item as Right } from right
def main()
case Left<Integer>::Value(1)
when Right::Value(value)
puts(value)
end
end
`)},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		_, err := CompileProject(units, Options{Mode: mode, GoModule: "example.com/generic-enums", ProjectRoot: "/project", SourceRoot: "/project"})
		if err == nil || (!strings.Contains(err.Error(), "when value must be a member of") && !strings.Contains(err.Error(), "expects 1 type argument(s), got 0")) {
			t.Fatalf("%s: expected nominal mismatch, got %v", mode, err)
		}
	}
}
