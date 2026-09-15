package compiler

import "testing"

func TestDiscardedValueExpressionsExecuteAcrossBackends(t *testing.T) {
	source := []byte(`record Box
value: Integer
end
def number(label: String): Integer
puts(label)
return 2
end
def flag(label: String): Boolean
puts(label)
return true
end
def main()
r := (2...5)
r
r.each do |value|
puts(value)
end
1
"text"
true
nil
Box.new(value: number("record"))
[number("array")]
{1 => number("hash")}
number("left") + number("right")
false && flag("unreachable")
true && flag("reachable")
number("call")
puts("done")
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "main.trb", source,
				"2\n3\n4\nrecord\narray\nhash\nleft\nright\nreachable\ncall\ndone")
		})
	}
}
