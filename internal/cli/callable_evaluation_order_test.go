package cli

import "testing"

func TestFunctionValueSelectionPrecedesArguments(t *testing.T) {
	source := `alias Callback = (Integer) -> Integer
record Holder
callback: Callback
end
def add(value: Integer): Integer
return value + 2
end
def triple(value: Integer): Integer
return value * 3
end
def main()
mut item := Holder.new(callback: add)
replace_item := fn(): Integer
item = Holder.new(callback: triple)
return 3
end
puts(item.callback(replace_item()))
puts(item.callback(3))
mut callback := add
replace_callback := fn(): Integer
callback = triple
return 4
end
puts(callback(replace_callback()))
puts(callback(4))
mut functions := [add]
replace_element := fn(): Integer
functions[0] = triple
return 5
end
puts(functions[0](replace_element()))
mut table := {"saved" => add}
replace_entry := fn(): Integer
table["saved"] = triple
return 6
end
puts(table["saved"](replace_entry()))
mut events: Array<String> := []
factory := fn(): Callback
events.push("target")
return add
end
argument := fn(): Integer
events.push("argument")
return 7
end
puts(factory()(argument()))
puts(events.join("/"))
mut optional: Callback? := add
clear := fn(): Integer
optional = nil
return 8
end
if optional != nil
puts(optional(clear()))
end
present: Holder? := Holder.new(callback: add)
absent: Holder? := nil
mut count := 0
next_argument := fn(): Integer
count += 1
return 9
end
value := present&.callback(next_argument())
if value != nil
puts(value)
end
puts(absent&.callback(next_argument()) == nil)
puts(count)
end
`
	runPortableExecutionCase(t, source, "5\n9\n6\n12\n7\n8\n9\ntarget/argument\n10\n11\ntrue\n1\n", "")
}
