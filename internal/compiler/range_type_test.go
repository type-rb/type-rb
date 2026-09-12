package compiler

import (
	"strings"
	"testing"
)

func TestRangeElementMismatchesAreRejectedAcrossModes(t *testing.T) {
	cases := []struct {
		name   string
		source string
	}{
		{
			name: "binding",
			source: `def main()
	span: Range<String> := 1..2
	span.each { |value| puts(value.size()) }
end
`,
		},
		{
			name: "argument",
			source: `def first(span: Range<String>): String
	span.each { |value| return value }
	return "empty"
end

def main()
	puts(first(1..2))
end
`,
		},
		{
			name: "return",
			source: `def bounds(): Range<String>
	return 1..2
end
`,
		},
		{
			name: "reassignment",
			source: `def replace(mut span: Range<String>)
	span = 1..2
	puts(span)
end
`,
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range cases {
			t.Run(mode+"/"+test.name, func(t *testing.T) {
				_, err := Compile("range_types.trb", []byte(test.source), mode)
				if err == nil || !strings.Contains(err.Error(), "Range<String>") || !strings.Contains(err.Error(), "Range<Integer>") {
					t.Fatalf("expected incompatible Range element diagnostic, got %v", err)
				}
			})
		}
	}
}

func TestMatchingRangeElementsCompileAcrossModes(t *testing.T) {
	source := []byte(`def retained(span: Range<Integer>): Range<Integer>
	return span
end

def main()
	mut span: Range<Integer> := retained(-1..1)
	span = 2...4
	span.each { |value| puts(value) }
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			if _, err := Compile("range_types.trb", source, mode); err != nil {
				t.Fatal(err)
			}
		})
	}
}
