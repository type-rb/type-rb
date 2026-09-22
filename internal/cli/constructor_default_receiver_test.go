package cli

import "testing"

func TestConstructorParameterDefaultRejectsUnavailableReceiverAcrossTargetsAndREPL(t *testing.T) {
	runPortableExecutionCase(t, `class Value
@first: Integer := 2
def initialize(value: Integer = self.first)
puts(value)
end
end
def main()
Value.new()
end
`, "", "constructor parameter default cannot access self before instance initialization")
}
