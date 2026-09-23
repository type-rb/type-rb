package compiler

import (
	"strings"
	"testing"
)

func TestUnionIndexRequiresNarrowingAcrossModes(t *testing.T) {
	tests := []struct {
		name, source string
	}{
		{
			name:   "declared union",
			source: "def main()\nvalue: Array<Integer> | Array<Float> := [2]\nputs(value[0])\nend\n",
		},
		{
			name:   "inferred Hash value union",
			source: "def main()\nmut values := {}\nif true\nvalues[1] = [2]\nelse\nvalues[1] = [4.5]\nend\nputs(values[1][0])\nend\n",
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			t.Run(mode+"/"+test.name, func(t *testing.T) {
				_, err := Compile("union_index.trb", []byte(test.source), mode)
				if err == nil || !strings.Contains(err.Error(), "cannot be indexed without narrowing") {
					t.Fatalf("expected union index diagnostic, got %v", err)
				}
			})
		}
		t.Run(mode+"/typed Hash value", func(t *testing.T) {
			source := "def main()\nvalues: Hash<Integer, Array<Integer>> := {1 => [2]}\nputs(values[1][0])\nend\n"
			if _, err := Compile("typed_hash_index.trb", []byte(source), mode); err != nil {
				t.Fatalf("typed nested indexing must remain available: %v", err)
			}
		})
	}
}
