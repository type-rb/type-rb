package compiler

import "testing"

func TestPartialExpressionsCannotSilentlyDiscardTheirSuffix(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, expression := range []string{"4.930380657631324e-32", "1.5 unexpected", "number() trailing"} {
			t.Run(mode+"/"+expression, func(t *testing.T) {
				source := "def number(): Float\nreturn 1.5\nend\ndef main()\nvalue := " + expression + "\nputs(value)\nend\n"
				if _, err := Compile("partial.trb", []byte(source), mode); err == nil {
					t.Fatalf("accepted an incomplete expression: %s", expression)
				}
			})
		}
	}
}
