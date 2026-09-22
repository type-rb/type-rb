package compiler

import "testing"

func TestRubyKeywordBindingsRunAcrossBackends(t *testing.T) {
	source := []byte(`enum Choice
	Value(next: Integer)
	Named(*, next: Integer)
end

record Box
	next: Integer
end

class Counter
	@value: Integer

	def initialize(next: Integer)
		@value = next
		return
	end
end

def combine(next: Integer, *, value: Integer = next + 1): Integer
	return next + value
end

def labeled(*, next: Integer = 4, value: Integer = next + 2): Integer
	return next + value
end

def main()
	binding := 0
	puts(combine(2))
	puts(labeled())
	puts(labeled(next: 3))
	callback := fn(next: Integer): Integer
		return next + 1
	end
	puts(callback(5))
	mut total := 0
	[1, 2].each do |next|
		total += next
	end
	puts(total + binding)
	box := Box.new(next: 7)
	puts(box.next)
	choice := Choice::Value(8)
	case choice
	when Choice::Value(next)
		puts(next)
	when Choice::Named(next: label)
		puts(label)
	end
	puts(Counter.new(6).value)
	named := Choice::Named(next: 9)
	case named
	when Choice::Value(next)
		puts(next)
	when Choice::Named(next: label)
		puts(label)
	end
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "ruby_keyword_bindings.trb", source, "5\n10\n8\n6\n3\n7\n8\n6\n9")
		})
	}
}
