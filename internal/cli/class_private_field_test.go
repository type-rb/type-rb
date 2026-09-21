package cli

import "testing"

const privateFieldClass = `class Value
@value: Integer := 4
@_value: Integer := 7
def public_value(): Integer
return self.value
end
def private_value(): Integer
return self._value
end
def bump()
@_value += 1
end
def direct_value(): Integer
return @_value
end
end
`

func TestClassPrivateSelfFieldAcrossTargetsAndREPL(t *testing.T) {
	runPortableExecutionCase(t, privateFieldClass+`def main()
mut value := Value.new()
puts(value.public_value())
puts(value.private_value())
value.bump()
puts(value.private_value())
puts(value.direct_value())
puts(value.public_value())
end
`, "4\n7\n8\n8\n4\n", "")
}

func TestClassPrivateSelfFieldThroughImports(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"library/value.trb": privateFieldClass,
		"main.trb": `import { Value } from library/value
def main()
puts(Value.new().private_value())
end
`,
	}, "7\n", "")
}

func TestClassPrivateSelfFieldStillRejectsExternalAccess(t *testing.T) {
	runPortableExecutionCase(t, privateFieldClass+`def main()
value := Value.new()
puts(value._value)
end
`, "", "private member _value cannot be accessed externally")
}
