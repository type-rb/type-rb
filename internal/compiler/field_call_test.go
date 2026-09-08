package compiler

import (
	"strings"
	"testing"
)

func TestNonFunctionDataFieldCallsAreRejectedAcrossModes(t *testing.T) {
	cases := []struct{ name, declaration, receiver, call, want string }{
		{"record", "record Box\nvalue: Integer\nend\n", "Box", "box.value()", "field value of type Integer is not callable"},
		{"parenthesized", "record Box\nvalue: Integer\nend\n", "Box", "(box.value)()", "field value of type Integer is not callable"},
		{"array", "record Box\nitems: Array<Integer>\nend\n", "Box", "box.items()", "field items of type Array<Integer> is not callable"},
		{"generic", "record Box<T>\nvalue: T\nend\n", "Box<Integer>", "box.value()", "field value of type Integer is not callable"},
		{"class", "class Box\n@value: Integer := 1\nend\n", "Box", "box.value()", "field value of type Integer is not callable"},
	}
	for _, tc := range cases {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				source := tc.declaration + "def invoke(box: " + tc.receiver + ")\n" + tc.call + "\nend\n"
				_, err := Compile("field_call.trb", []byte(source), mode)
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("want %q, got %v", tc.want, err)
				}
			})
		}
	}
}

func TestImportedDataFieldCallsPreserveFunctionFieldsAcrossModes(t *testing.T) {
	model := SourceUnit{Filename: "model.trb", ModulePath: "model", Source: []byte(`record Box<T>
value: T
callback: () -> Integer
items: Array<Integer>
end
`)}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			invalid := SourceUnit{Filename: "main.trb", ModulePath: "main", Source: []byte("import { Box } from model\ndef invoke(box: Box<Integer>)\n(box.value)()\nend\n")}
			_, err := CompileProject([]SourceUnit{model, invalid}, Options{Mode: mode, SourceRoot: "/project", ProjectRoot: "/project"})
			if err == nil || !strings.Contains(err.Error(), "field value of type Integer is not callable") {
				t.Fatalf("non-function field accepted: %v", err)
			}
			valid := SourceUnit{Filename: "main.trb", ModulePath: "main", Source: []byte("import { Box } from model\ndef invoke(box: Box<Integer>): Integer\nputs(box.value.to_s())\nreturn (box.callback)() + box.items.size()\nend\n")}
			if _, err := CompileProject([]SourceUnit{model, valid}, Options{Mode: mode, SourceRoot: "/project", ProjectRoot: "/project"}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
