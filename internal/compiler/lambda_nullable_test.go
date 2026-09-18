package compiler

import (
	"strings"
	"testing"
)

func TestClosureRejectsInheritedMutableNullableFactsAcrossBackends(t *testing.T) {
	tests := map[string]string{
		"replacement after capture": `def sample()
  mut value: Integer? := 1
  if value != nil
    callback := fn(): Integer; return value; end
    value = nil
    puts(callback())
  end
end
`,
		"nested capture after inner guard": `def sample(mut value: Integer?)
  outer := fn(): () -> Integer
    if value != nil
      return fn(): Integer; return value; end
    end
    return fn(): Integer; return 0; end
  end
  callback := outer()
  value = nil
  puts(callback())
end
`,
		"readonly field on replaceable root": `record Box
  value: Integer?
end
def sample()
  mut box := Box.new(value: 1)
  if box.value != nil
    callback := fn(): Integer; return box.value; end
    box = Box.new(value: nil)
    puts(callback())
  end
end
`,
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			for _, mode := range []string{"go", "ruby", "typescript"} {
				_, err := Compile("lambda_nullable.trb", []byte(source), mode)
				if err == nil || !strings.Contains(err.Error(), "return type is Integer?") {
					t.Fatalf("%s: expected nullable return diagnostic, got %v", mode, err)
				}
			}
		})
	}
}

func TestClosureNullableGuardsUseInvocationBindingsAcrossBackends(t *testing.T) {
	source := []byte(`record Box
  value: Integer?
end
def main()
  mut value: Integer? := 1
  callback := fn(): Integer
    if value != nil
      return value
    end
    return 0
  end
  puts(callback())
  value = nil
  puts(callback())
  value = 3
  puts(callback())
  fixed: Integer? := 7
  if fixed != nil
    stable := fn(): Integer; return fixed; end
    puts(stable())
  end
  box := Box.new(value: 9)
  if box.value != nil
    stable_field := fn(): Integer; return box.value; end
    puts(stable_field())
  end
  shadow := fn(value: Integer): Integer; return value; end
  puts(shadow(11))
  puts(value)
  mut absent: Integer? := nil
  retained := fn(): Integer?; return absent; end
  absent = 4
  puts(retained())
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "lambda_nullable.trb", source, "1\n0\n3\n7\n9\n11\n3\n4")
		})
	}
}
