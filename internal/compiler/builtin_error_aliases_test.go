package compiler

import (
	"strings"
	"testing"
)

func TestBuiltinErrorsPreserveImportedAliasesAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, tc := range []struct{ name, source, want string }{
			{"index", `import { Result as Outcome } from trb/std/result
import { IndexLookupError as MissingIndex } from trb/std/errors
def first(values: Array<String>): Outcome<String, MissingIndex>
  return values.try_fetch(0)
end
def main()
  value := first([]) catch |error|
    puts(error.index)
    error.message
  end
  puts(value)
end
`, "0\nArray index is out of bounds"},
			{"slice", `import { Result as Outcome } from trb/std/result
import { SliceRangeError as MissingSlice } from trb/std/errors
def slice(values: Array<Integer>): Outcome<Array<Integer>, MissingSlice>
  return values.try_slice(0..1)
end
def main()
  values := slice([1]) catch |error|
    puts(error.message)
    []
  end
  puts(values.size())
end
`, "Array slice range is out of bounds\n0"},
			{"key", `import { Result as Outcome } from trb/std/result
import { KeyLookupError as MissingKey } from trb/std/errors
def lookup(values: Hash<String, Integer>): Outcome<Integer, MissingKey>
  return values.try_fetch("missing")
end
def main()
  value := lookup({}) catch |error|
    puts(error.message)
    0
  end
  puts(value)
end
`, "Hash key is missing\n0"},
			{"number", `import { Result as Outcome } from trb/std/result
import { NumberParseError as ParseFailure, NumberParseErrorKind as FailureKind } from trb/std/errors
def parse(text: String): Outcome<Integer, ParseFailure>
  return text.try_to_i()
end
def main()
  value := parse("invalid") catch |error|
    case error.kind
    when FailureKind::InvalidFormat
      puts(error.message)
    when FailureKind::OutOfRange
      puts("range")
    end
    0
  end
  puts(value)
end
`, "invalid Integer\n0"},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				requireEffectRuntime(t, mode)
				options := Options{Mode: mode, GoModule: "example.com/error-aliases", RubyLoader: "require_relative", ProjectRoot: "/project", SourceRoot: "/project"}
				artifacts, err := CompileProject([]SourceUnit{{Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: []byte(tc.source)}}, options)
				if err != nil {
					t.Fatal(err)
				}
				if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, options.GoModule)); got != tc.want {
					t.Fatalf("got %q, want %q", got, tc.want)
				}
			})
		}
	}
}
