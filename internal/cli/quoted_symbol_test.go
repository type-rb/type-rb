package cli

import "testing"

func TestQuotedSymbolsAcrossTargetsAndREPL(t *testing.T) {
	source := `def main()
puts(:ready == "ready")
puts(:true == "true")
puts(:nil == "nil")
puts(:+ == "+")
puts(:"#{undefined}")
puts(:"#@undefined #$undefined ${undefined}")
puts(:"\#{undefined}")
puts(:"日本\u0000😀".size())
puts(:"line\nbreak")
values := {:"#{key}" => :"#{value}", label: :"\u65e5"}
puts(values["\#{key}"])
puts(values["label"])
puts("#{:inside}")
end
`
	runPortableExecutionCase(t, source, "true\ntrue\ntrue\ntrue\n#{undefined}\n#@undefined #$undefined ${undefined}\n#{undefined}\n4\nline\nbreak\n#{value}\n日\ninside\n", "")
}

func TestQuotedSymbolInvalidEscapesAreRejected(t *testing.T) {
	for _, literal := range []string{`:"\q"`, `:"\x0"`, `:"\uD800"`, `:"\U00110000"`, `:"#{"inner"}"`} {
		t.Run(literal, func(t *testing.T) {
			runPortableExecutionCase(t, "def main()\nputs("+literal+")\nend\n", "", "invalid quoted symbol escape or literal")
		})
	}
}
