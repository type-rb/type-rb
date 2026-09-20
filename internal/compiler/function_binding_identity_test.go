package compiler

import "testing"

func TestNamedFunctionIdentitySurvivesLaterBindings(t *testing.T) {
	source := []byte(`def read(): Integer
  return 1
end
def before(): Integer
  return read()
end
def generic<T>(_value: T): Integer
  return read()
end
def defaulted(value: Integer = read()): Integer
  return value
end
saved := read
closure := fn(): Integer; return read(); end
read := fn(): Integer; return 2; end
def after(): Integer
  return read()
end
def main()
  puts(saved())
  puts(before())
  puts(generic<Integer>(3))
  puts(defaulted())
  puts(closure())
  puts(read())
  puts(after())
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "function_bindings.trb", source, "1\n1\n1\n1\n1\n2\n2")
		})
	}
}
