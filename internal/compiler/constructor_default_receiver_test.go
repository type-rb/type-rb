package compiler

import (
	"strings"
	"testing"
)

func TestConstructorParameterDefaultsRejectReceiverBeforeInitialization(t *testing.T) {
	for _, expression := range []string{"self.first", "@first", "read()", "fn(): Integer; return self.first; end"} {
		parameterType := "Integer"
		if strings.HasPrefix(expression, "fn") {
			parameterType = "() -> Integer"
		}
		source := "class Value\n@first: Integer := 2\ndef read(): Integer\nreturn @first\nend\ndef initialize(value: " + parameterType + " = " + expression + ")\nend\nend\ndef main()\nValue.new()\nend\n"
		for _, mode := range []string{"go", "ruby", "typescript"} {
			t.Run(mode+"/"+expression, func(t *testing.T) {
				_, err := Compile("main.trb", []byte(source), mode)
				if err == nil || !strings.Contains(err.Error(), "constructor parameter default cannot access self before instance initialization") {
					t.Fatalf("diagnostic = %v", err)
				}
			})
		}
	}
}

func TestConstructorParameterDefaultsWithoutReceiverStillCompile(t *testing.T) {
	source := []byte(`class Value
@first: Integer := 2
def initialize(value: Integer = 3)
puts(value)
end
end
def main()
Value.new()
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		runEffectSource(t, mode, "main.trb", source, "3")
	}
}

func TestOrdinaryMethodDefaultsKeepTheirReceiver(t *testing.T) {
	source := []byte(`class Value
@first: Integer := 2
def read(value: Integer = self.first): Integer
return value
end
end
def main()
puts(Value.new().read())
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		runEffectSource(t, mode, "main.trb", source, "2")
	}
}
