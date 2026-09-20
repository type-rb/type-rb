package compiler

import (
	"strings"
	"testing"
)

func TestLibraryGenericBindingsRetainEqualityAndOrderingRequirements(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, row := range []struct{ name, source, want string }{
			{"equality", "def query<T>(items: Array<T>, item: T): Boolean\nreturn items.include?(item)\nend\n", "portable equality"},
			{"ordering", "def ordered<T>(items: Array<T>): Array<T>\nreturn items.sort()\nend\n", "portable natural order"},
			{"element_mismatch", "def copy<T>(items: Array<T>): Array<String>\nreturn items.dup()\nend\n", "Array<String>"},
		} {
			t.Run(mode+"/"+row.name, func(t *testing.T) {
				_, err := CompileProject([]SourceUnit{{Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: []byte(row.source)}}, Options{Mode: mode, GoModule: "example.com/library-generics"})
				if err == nil || !strings.Contains(err.Error(), row.want) {
					t.Fatalf("expected %s rejection, got %v", row.want, err)
				}
			})
		}
	}
}
