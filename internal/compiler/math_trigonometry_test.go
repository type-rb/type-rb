package compiler

import (
	"os"
	"strings"
	"testing"
)

func TestPortableTrigonometryAcrossBackends(t *testing.T) {
	source, err := os.ReadFile("testdata/math_trigonometry.trb")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/math_trigonometry.out")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "math_trigonometry.trb", source, strings.TrimSpace(string(want)))
		})
	}
}

func TestPortableTrigonometryRejectsInvalidCalls(t *testing.T) {
	for _, expression := range []string{`Math.sin("0")`, `Math.cos(nil)`, `Math.pow(2, true)`, `Math.pow(2)`, `Math.sin(0, 1)`} {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			source := "import trb/std/math\ndef main()\nputs(" + expression + ")\nend\n"
			if _, err := Compile("invalid_math.trb", []byte(source), mode); err == nil {
				t.Fatalf("%s accepted %s", mode, expression)
			}
		}
	}
}
