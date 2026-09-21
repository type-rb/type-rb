package cli

import "testing"

const initializedClassDeclarations = `enum Choice
First
Second
end
class Branch
readonly @label: String
@number: Integer := 2
def initialize(early: Boolean)
if early
@label = "early"
return
end
@label = "late"
@number += 1
leave := fn()
return
puts("unreachable lambda")
end
leave()
end
def reader(): () -> String
return fn(): String; return @label; end
end
end
class Selected
@value: Integer
def initialize(choice: Choice)
case choice
when Choice::First
@value = 4
when Choice::Second
@value = 5
end
end
end
class Stored<T>
@value: T
@copy: T
def initialize(value: T)
@value = value
@copy = @value
end
end
class Defaults
@first: Integer := 2
@second: Integer := @first + 3
end
class Nested
@value: Integer
def initialize(flag: Boolean)
label := if flag
@value = 6
"first"
else
@value = 7
"second"
end
puts(label)
end
end
class LoopReturn
@value: Integer
def initialize()
@value = 8
[1].each { |offset|
@value += offset
return
}
@value = 50
end
end
`

func TestConstructorInitializationAcrossTargetsAndREPL(t *testing.T) {
	runPortableExecutionCase(t, initializedClassDeclarations+`def main()
early := Branch.new(true)
late := Branch.new(false)
read := late.reader()
puts(early.label)
puts(early.number)
puts(read())
puts(late.number)
puts(Selected.new(Choice::First).value)
puts(Selected.new(Choice::Second).value)
puts(Stored<String>.new("kept").copy)
puts(Defaults.new().second)
puts(Nested.new(true).value)
puts(Nested.new(false).value)
puts(LoopReturn.new().value)
end
`, "early\n2\nlate\n3\n4\n5\nkept\n5\nfirst\n6\nsecond\n7\n9\n", "")
}

func TestImportedConstructorInitializationAcrossTargetsAndREPL(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"models.trb": initializedClassDeclarations,
		"main.trb": `import { Branch as Chosen, Stored } from models
def main()
puts(Chosen.new(true).label)
puts(Chosen.new(false).label)
puts(Stored<String>.new("imported").copy)
end
`,
	}, "early\nlate\nimported\n", "")
}

func TestUnsafeConstructorInitializationRejectedAcrossTargetsAndREPL(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"branch", "if flag\n@value = 1\nend", "must be initialized"},
		{"early return", "return if flag\n@value = 1", "must be initialized"},
		{"read", "puts(flag)\nputs(@value)\n@value = 1", "read before initialization"},
		{"receiver", "puts(flag)\nputs(self.read())\n@value = 1", "self cannot be used before"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := "class Value\n@value: Integer\ndef initialize(flag: Boolean)\n" + tc.body + "\nend\ndef read(): Integer\nreturn @value\nend\nend\ndef main()\nputs(Value.new(false).value)\nend\n"
			runPortableExecutionCase(t, source, "", tc.want)
		})
	}
}
