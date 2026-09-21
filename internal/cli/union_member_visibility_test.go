package cli

import "testing"

const unionMemberClasses = `class First
@value: Integer := 1
@_value: Integer := 11
def reveal(): Integer
return self._value
end
end
class Second
@value: Integer := 2
@_value: Integer := 22
def reveal(): Integer
return @_value
end
end
alias Choice = First | Second
`

func TestUnionPrivateMemberVisibilityAcrossTargetsAndREPL(t *testing.T) {
	for _, body := range []string{
		"puts(value._value)",
		"value._value = 3",
		"value._value += 3",
	} {
		t.Run(body, func(t *testing.T) {
			runPortableExecutionCase(t, unionMemberClasses+"def access(mut value: Choice)\n"+body+"\nend\ndef main()\nmut value: Choice := First.new()\naccess(value)\nend\n", "", "private member _value cannot be accessed externally")
		})
	}
}

func TestImportedUnionPrivateMemberVisibilityAcrossTargetsAndREPL(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"library/values.trb": unionMemberClasses,
		"main.trb": `import { First as Primary, Choice as Selected } from library/values
def access(value: Selected): Integer
return value._value
end
def main()
puts(access(Primary.new()))
end
`,
	}, "", "private member _value cannot be accessed externally")
}

func TestUnionPublicFieldsPreservePrivateOwnerAccessAcrossTargetsAndREPL(t *testing.T) {
	runPortableExecutionCase(t, unionMemberClasses+`def access(value: Choice): Integer
return value.value
end
def main()
puts(access(First.new()))
puts(access(Second.new()))
puts(First.new().reveal())
puts(Second.new().reveal())
end
`, "1\n2\n11\n22\n", "")
}
