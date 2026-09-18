package compiler

import (
	"strings"
	"testing"
)

func TestCallbackReplacementInvalidatesNullableProofsAcrossBackends(t *testing.T) {
	prefix := `mut value: Integer? := 1
reset := fn(): Boolean; value = nil; return true; end
if value != nil
`
	tests := map[string]string{
		"direct":             "reset()\nresult := value + 1\nputs(result)",
		"alias":              "other := reset\nother()\nresult := value + 1\nputs(result)",
		"stored":             "callbacks := [reset]\ncallbacks[0]()\nresult := value + 1\nputs(result)",
		"argument":           "invoke(reset)\nresult := value + 1\nputs(result)",
		"argument order":     "consume(reset(), value + 1)",
		"conditional join":   "if true; reset(); end\nresult := value + 1\nputs(result)",
		"case join":          "case 1\nwhen 1\nreset()\nelse\nfalse\nend\nresult := value + 1\nputs(result)",
		"short circuit join": "other: Integer? := 1\nchanged := (other != nil && reset())\nresult := value + 1\nputs(changed)\nputs(result)",
		"short circuit or":   "other: Integer? := 1\nchanged := (other == nil || reset())\nresult := value + 1\nputs(changed)\nputs(result)",
		"short circuit use":  "puts(reset() && value + 1 > 0)",
		"loop backedge":      "mut step := 0\nwhile step < 2\nresult := value + 1\nreset()\nputs(result)\nstep += 1\nend",
		"iteration backedge": "[1, 2].each do |step|\nresult := value + step\nreset()\nputs(result)\nend",
		"shadowed join":      "if true\nvalue := 42\nreset()\nputs(value)\nend\nresult := value + 1\nputs(result)",
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			source := `def invoke(callback: () -> Boolean); callback(); end
def consume(changed: Boolean, value: Integer); puts(changed); puts(value); end
def main()
` + prefix + body + "\nend\nend\n"
			assertNullableCallbackRejected(t, source)
		})
	}
	for name, source := range map[string]string{
		"callback created in repeated body": `def main()
mut value: Integer? := 1
if value != nil
mut step := 0
while step < 2
result := value + 1
reset := fn(); value = nil; end
reset()
puts(result)
step += 1
end
end
end`,
		"nested writer": `def main()
mut value: Integer? := 1
factory := fn(): () -> Void
return fn(); value = nil; end
end
if value != nil
reset := factory()
reset()
result := value + 1
puts(result)
end
end`,
		"writer from previous iteration after fresh guard": `def main()
mut value: Integer? := 1
noop := fn(): Boolean; return true; end
mut callbacks := [noop]
mut step := 0
while step < 2
if value != nil
callbacks[0]()
result := value + 1
callbacks[0] = fn(): Boolean; value = nil; return true; end
puts(result)
end
step += 1
end
end`,
		"readonly field of replaced root": `record Box
value: Integer?
end
def main()
mut box := Box.new(value: 1)
reset := fn(); box = Box.new(value: nil); end
if box.value != nil
reset()
result := box.value + 1
puts(result)
end
end`,
		"conditional new capture": `def main()
mut value: Integer? := 1
if value != nil
if true
reset := fn(); value = nil; end
reset()
end
result := value + 1
puts(result)
end
end`,
		"catch join": `import { Result } from trb/std/result
def main()
mut value: Integer? := 1
reset := fn(); value = nil; end
outcome := Result<Integer, String>::Err("missing")
if value != nil
recovered := outcome catch |_error|
reset()
0
end
result := value + 1
puts(recovered)
puts(result)
end
end`,
	} {
		t.Run(name, func(t *testing.T) { assertNullableCallbackRejected(t, source) })
	}
}

func assertNullableCallbackRejected(t *testing.T, source string) {
	t.Helper()
	for _, mode := range []string{"go", "ruby", "typescript"} {
		_, err := Compile("callback_nullable.trb", []byte(source), mode)
		if err == nil || !strings.Contains(err.Error(), "operator + does not support Integer? and Integer") {
			t.Fatalf("%s: expected stale nullable proof rejection, got %v", mode, err)
		}
	}
}

func TestCallbackNullableFreshGuardsAndStableBindingsAcrossBackends(t *testing.T) {
	source := []byte(`record Box
value: Integer?
end
def main()
mut value: Integer? := 1
reset := fn(); value = nil; end
reset()
if value != nil
puts(value + 1)
else
puts("absent")
end
restore := fn(); value = 4; end
restore()
if value != nil
puts(value + 1)
end
mut unchanged: Integer? := 6
read := fn(): Integer?
return unchanged
end
if unchanged != nil
read()
puts(unchanged + 1)
end
fixed: Integer? := 8
if fixed != nil
reset()
puts(fixed + 1)
end
box := Box.new(value: 10)
if box.value != nil
reset()
puts(box.value + 1)
end
mut outer: Integer? := 12
shadow := fn(mut outer: Integer?); outer = nil; puts(outer == nil); end
if outer != nil
shadow(outer)
puts(outer + 1)
end
mut deferred: Integer? := 14
if deferred != nil
clear := fn(); deferred = nil; end
before := deferred + 1
clear()
puts(before)
end
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "callback_nullable.trb", source, "absent\n5\n7\n9\n11\ntrue\n13\n15")
		})
	}
}

func TestCallbackNullableLocalShadowDoesNotInvalidateOuterBinding(t *testing.T) {
	source := []byte(`def main()
mut value: Integer? := 1
local := fn()
mut value: Integer? := 2
inner := fn(); value = nil; end
inner()
puts(value == nil)
end
if value != nil
local()
puts(value + 1)
end
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("callback_shadow.trb", source, mode); err != nil {
			t.Fatalf("%s: unrelated shadow invalidated the outer binding: %v", mode, err)
		}
	}
}
