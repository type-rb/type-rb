package cli

import "testing"

func TestHashLiteralDuplicateKeysAndEvaluationOrder(t *testing.T) {
	source := `def main()
labels := {item: 1, (item): 2, "item": 3}
puts(labels["item"])
numbers := {1: 2, 1 => 3}
puts(numbers[1])
mut key := "before"
mut value := 4
mut events: Array<String> := []
change := fn(): Integer
events.push("value")
key = "after"
value = 5
return 6
end
next_key := fn(): String
events.push("key")
return "next"
end
values := {"saved": value, key => change(), next_key(): value}
puts(values["saved"])
puts(values["before"])
puts(values["next"])
puts(events.join("/"))
optional: Hash<String, Integer?> := {a: nil, b: 7, a: 9}
last := optional["a"]
if last != nil
puts(last)
end
empty: Hash<Integer, String> := {}
puts(empty.size())
nested := {a: {b: 1, b: 2}}
puts(nested["a"]["b"])
mut evaluations := 0
tick := fn(): Integer
evaluations += 1
return evaluations
end
duplicates := {"x": tick(), "x": tick()}
puts(duplicates["x"])
puts(evaluations)
end
`
	runPortableExecutionCase(t, source, "3\n3\n4\n6\n5\nvalue/key\n9\n0\n2\n2\n2\n", "")
}

func TestHashLiteralReadsEachKeyAndValueBeforeNextEntry(t *testing.T) {
	source := `def main()
mut key := "before"
mut value := 4
change := fn(): Integer
key = "after"
value = 5
return 6
end
values := {"saved": value, key => change()}
puts(values["saved"])
puts(values["before"])
end
`
	runPortableExecutionCase(t, source, "4\n6\n", "")
}
