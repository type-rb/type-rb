package compiler

import (
	"strings"
	"testing"
)

func TestRunnableMainRequiresExactSignatureAcrossBackends(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "parameter",
			source: `def main(value: Integer)
	puts(value)
	return
end
`,
		},
		{
			name: "return type",
			source: `def main(): Integer
	return 1
end
`,
		},
		{
			name: "type parameter",
			source: `def main<T>()
	return
end
`,
		},
		{
			name: "class function",
			source: `def self.main()
	return
end
`,
		},
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			t.Run(mode+"/"+test.name, func(t *testing.T) {
				_, err := Compile("main.trb", []byte(test.source), mode)
				if err == nil || !strings.Contains(err.Error(), "runnable main must have signature def main()") {
					t.Fatalf("expected exact runnable main diagnostic, got %v", err)
				}
			})
		}
	}
}

func TestRubyNativeNestedMainIsNotTheRunnableEntrypoint(t *testing.T) {
	source := []byte(`import trb/platform/ruby/native

begin
	def main(value: Integer)
		puts(value)
		return
	end
end
`)
	if _, err := Compile("library.trb", source, "ruby"); err != nil {
		t.Fatalf("rejected non-entrypoint native main: %v", err)
	}
}

func TestNonEntrypointMainMethodKeepsOrdinarySignatureRulesAcrossBackends(t *testing.T) {
	source := []byte(`class Runner
	def main(value: Integer): Integer
		return value
	end
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			if _, err := Compile("runner.trb", source, mode); err != nil {
				t.Fatalf("%s rejected a non-entrypoint main method: %v", mode, err)
			}
		})
	}
}
