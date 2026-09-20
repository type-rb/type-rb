package compiler

import (
	"strings"
	"testing"
)

func TestLibraryFunctionResultsKeepDeclarationArgumentAndMutationChecks(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, row := range []struct{ name, expression, want string }{
			{"first_arguments", "callbacks.first(1)", "argument"},
			{"last_arguments", "callbacks.last(1)", "argument"},
			{"readonly_pop", "callbacks.pop()", "mutable"},
			{"readonly_shift", "callbacks.shift()", "mutable"},
		} {
			t.Run(mode+"/"+row.name, func(t *testing.T) {
				source := "def main()\nread := fn(value: Integer): Integer\nreturn value\nend\ncallbacks := [read]\n" + row.expression + "\nend\n"
				_, err := CompileProject([]SourceUnit{{Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: []byte(source)}}, Options{Mode: mode, GoModule: "example.com/callable-results"})
				if err == nil || !strings.Contains(err.Error(), row.want) {
					t.Fatalf("expected %s rejection, got %v", row.want, err)
				}
			})
		}
	}
}
