package compiler

import "testing"

func TestParenthesizedConditionsRunAcrossBackends(t *testing.T) {
	source := []byte(`def ready(label: String): Boolean
	puts(label)
	return true
end

def main()
	if (false) || ready("if")
		puts("if-body")
	end
	if false
		puts("wrong")
	elsif (false) || ready("elsif")
		puts("elsif-body")
	end
	while (false) || ready("while")
		puts("while-body")
		break
	end
	if ((true))
		puts("grouped")
	end
	if (true) || ready("wrong")
		puts("short")
	end
	if (true) && ready("and")
		puts("and-body")
	end
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "main.trb", source, "if\nif-body\nelsif\nelsif-body\nwhile\nwhile-body\ngrouped\nshort\nand\nand-body")
		})
	}
}

func TestParenthesizedConditionsRejectTrailingTokens(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, keyword := range []string{"if", "while"} {
			for _, condition := range []string{"(true) false", "(true) || (false) true", "(false) || missing()"} {
				t.Run(mode+"/"+keyword+"/"+condition, func(t *testing.T) {
					_, err := Compile("condition.trb", []byte("def main()\n"+keyword+" "+condition+"\nputs(\"body\")\nend\nend\n"), mode)
					if err == nil {
						t.Fatal("invalid condition accepted")
					}
				})
			}
		}
	}
}
