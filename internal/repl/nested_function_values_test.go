package repl

import "testing"

func TestCompleteNestedFunctionValues(t *testing.T) {
	for _, source := range []string{
		"invoke(fn(): Integer; return 1; end)",
		"callbacks := [fn(): Integer; return 1; end, fn(): Integer; return 2; end]",
		"def main()\ninvoke(fn(): Integer\nif true\nreturn 1\nend\nreturn 0\nend)\nend",
		"value.fn()",
		"class Named\ndef fn(): Integer\nreturn 4\nend\nend",
		"interface Named\nfn(): Integer\nend",
		"table := {fn: :fn}",
	} {
		if !Complete(source) {
			t.Errorf("complete function expression rejected: %s", source)
		}
	}
	for _, source := range []string{
		"def main()\ninvoke(fn(): Integer; return 1; end)",
		"invoke(fn(): Integer\nif true\nreturn 1\nend",
		"callbacks := [fn(): Integer; return 1; end, fn(): Integer",
	} {
		if Complete(source) {
			t.Errorf("incomplete function expression accepted: %s", source)
		}
	}
}
