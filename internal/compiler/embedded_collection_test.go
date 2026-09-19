package compiler

import (
	"strings"
	"testing"
)

func TestEmbeddedCollectionBlockDiagnosticsAcrossModes(t *testing.T) {
	tests := []struct {
		name, expression, want string
	}{
		{"predicate", "[1].select { |value| value }[0]", "select block result must be Boolean, got Integer"},
		{"accumulator", "[1].reduce(0) do |sum, value|\n(sum + value).to_s()\nend", "reduce block result is String, expected Integer"},
		{"escaping return", "[1].map do |value|\nif value > 0\nreturn value\nend\nvalue\nend[0]", "return is not supported inside value-producing collection transformations"},
		{"duplicate parameter", "[1].reduce(0) { |value, value| value }", "block parameter value is duplicated"},
		{"missing result", "[1].map { |value| copy := value }[0]", "map block must end with a result expression"},
		{"bad header", "[1].map do |1|\n1\nend[0]", "iteration block parameters must be identifiers"},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			t.Run(mode+"/"+test.name, func(t *testing.T) {
				source := "def main()\nputs(" + test.expression + ")\nend\n"
				_, err := Compile("embedded_collection.trb", []byte(source), mode)
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("expected %q, got %v", test.want, err)
				}
			})
		}
	}
}
