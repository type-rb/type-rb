package cli

import "testing"

func TestFunctionLiteralsInExpressionPositions(t *testing.T) {
	runPortableExecutionCase(t, `record Callback
apply: (Integer) -> Integer
end
def invoke(callback: (Integer) -> Integer, value: Integer): Integer
return callback(value)
end
def combine(first: () -> Integer, second: () -> Integer): Integer
return first() + second()
end
def main()
mut captured := 3
puts(invoke(fn(value: Integer): Integer; return value + captured; end, 4))
callback := Callback.new(apply: fn(value: Integer): Integer
if value > 0
return value * 2
end
return 0
end)
puts(callback.apply(3))
callbacks: Array<() -> Integer> := [fn(): Integer; return captured; end, fn(): Integer; return 9; end]
table: Hash<String, () -> Integer> := {"read" => fn(): Integer; return captured; end}
captured = 8
puts(callbacks[0]())
puts(callbacks[1]())
puts(table["read"]())
puts(combine(fn(): Integer; return 1; end, fn(): Integer; return 2; end))
end
`, "7\n6\n8\n9\n8\n3\n", "")
}

func TestNestedFunctionLiteralsKeepReturnTypeChecks(t *testing.T) {
	runPortableExecutionCase(t, `def invoke(callback: () -> Integer): Integer
return callback()
end
def main()
puts(invoke(fn(): Integer; return "wrong"; end))
end
`, "", "return type is string, expected integer")
}

func TestFunctionLiteralKeywordDoesNotCaptureMemberCalls(t *testing.T) {
	runPortableExecutionCase(t, `class Named
def fn(): Integer
return 4
end
end
def main()
value := Named.new()
value.fn()
puts(value.fn())
end
`, "4\n", "")
}
