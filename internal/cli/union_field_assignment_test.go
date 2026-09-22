package cli

import "testing"

func TestCommonClassUnionFieldStoresAcrossTargetsAndREPL(t *testing.T) {
	runPortableExecutionCase(t, `class First
@value: Integer := 1
end
class Second
@value: Integer := 2
end
alias Choice = First | Second
def update(mut item: Choice)
item.value = 3
item.value += 4
puts(item.value)
end
def main()
mut first: Choice := First.new()
update(first)
puts(first.value)
mut second: Choice := Second.new()
update(second)
puts(second.value)
mut current: Choice := First.new()
original := current
replace := fn(): Integer
current = Second.new()
return 9
end
current.value = replace()
puts(original.value)
puts(current.value)
end
`, "7\n7\n7\n7\n9\n2\n", "")
}

func TestImportedCommonClassUnionFieldStoresAcrossTargetsAndREPL(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"models.trb": `class First
@value: Integer := 1
end
class Second
@value: Integer := 2
end
alias Choice = First | Second
def make_first(): Choice
return First.new()
end
def make_second(): Choice
return Second.new()
end
`,
		"main.trb": `import { Choice, make_first, make_second } from models
def main()
mut first: Choice := make_first()
first.value = 8
puts(first.value)
mut second: Choice := make_second()
second.value += 5
puts(second.value)
end
`,
	}, "8\n7\n", "")
}

func TestCommonClassUnionLogicalAssignmentSkipsRightHandSide(t *testing.T) {
	runPortableExecutionCase(t, `class First
@flag: Boolean := false
end
class Second
@flag: Boolean := true
end
alias Choice = First | Second
def main()
mut item: Choice := First.new()
mut calls := 0
mark := fn(): Boolean
calls += 1
return true
end
item.flag &&= mark()
puts(calls)
item.flag ||= mark()
puts(calls)
puts(item.flag)
end
`, "0\n1\ntrue\n", "")
}
