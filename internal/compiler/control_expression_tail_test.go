package compiler

import (
	"strings"
	"testing"
)

func TestControlExpressionTailIsStillChecked(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, expression := range []string{
			"(if true\n1\nelse\n2\nend]",
			"pair(if true\n1\nelse\n2\nend, \"wrong\")",
		} {
			source := "def pair(left: Integer, right: Integer): Integer\nreturn left + right\nend\ndef main()\nchosen := " + expression + "\nputs(chosen)\nend\n"
			_, err := Compile("control_tail.trb", []byte(source), mode)
			if err == nil {
				t.Fatalf("%s accepted an invalid expression tail: %s", mode, expression)
			}
			if strings.Contains(expression, "wrong") && !strings.Contains(err.Error(), "has type String, expected Integer") {
				t.Fatalf("%s did not check the remaining argument: %v", mode, err)
			}
		}
	}
}
