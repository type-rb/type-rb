package cli

import "testing"

func TestUnionBindingsPreserveDeclaredStorageAcrossTargets(t *testing.T) {
	runPortableExecutionCase(t, `def text(value: Integer | String): String
case value
when Integer(number)
return number.to_s()
when String(word)
return word
end
end
def main()
mut value: Integer | String := 1
read := fn(): String; return text(value); end
puts(read())
value = "later"
puts(read())
value = 3
puts(case value
when Integer(number)
number.to_s()
when String(word)
word
end)
end
`, "1\nlater\n3\n", "")
}

func TestUnionDiscardPatternsAcrossTargets(t *testing.T) {
	runPortableExecutionCase(t, `def classify(value: Integer | String): Integer
case value
when Integer(_)
return 1
else
return 2
end
end
def main()
puts(classify(8))
puts(classify("other"))
value: Integer | String := "retained"
puts(case value
when Integer(_)
1
when String(_)
2
end)
end
`, "1\n2\n2\n", "")
}

func TestNullableUnionInjectionAndWideningAcrossTargets(t *testing.T) {
	runPortableExecutionCase(t, `alias Source = Integer | String
alias Target = Float | String | Boolean
def widen(value: Source?): Target?
return value
end
def inject(value: Source): Target?
return value
end
def text(value: Target?): String
if value != nil
case value
when Float(number)
return number.to_s()
when String(word)
return word
when Boolean(flag)
return flag.to_s()
end
end
return "absent"
end
def main()
puts(text(widen(7)))
puts(text(widen("word")))
puts(text(widen(nil)))
puts(text(inject(8)))
puts(text(9))
puts(text(false))
end
`, "7.0\nword\nabsent\n8.0\n9.0\nfalse\n", "")
}

func TestGenericUnionAliasesNormalizeAfterSubstitution(t *testing.T) {
	runPortableExecutionCase(t, `alias Scalar<T> = T | Float
alias Choice<T> = T | String
def text(value: Choice<Integer | String>): String
case value
when Integer(number)
return number.to_s()
when String(word)
return word
end
end
def main()
number: Scalar<Integer> := 2
puts(number.to_s())
puts(text(3))
puts(text("nested"))
end
`, "2.0\n3\nnested\n", "")
}
