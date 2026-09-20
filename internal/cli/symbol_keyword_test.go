package cli

import "testing"

func TestSymbolKeywordsAcrossTargetsAndREPL(t *testing.T) {
	source := `def choose(flag: Boolean, value: String = :if): String
return :return if flag
return value
end

def main()
puts(:if)
puts(:case)
puts(:end)
puts(:fn)
puts(:catch)
puts(:do)
puts(:return)
puts(:while)
puts(:unless)
puts((true ? :if : :case))
puts(choose(true))
puts(choose(false))
values := {if: :case, end: :if}
puts(values["if"])
puts(values["end"])
puts([1].map { |item| :if + item.to_s() }.join(","))
end
`
	runPortableExecutionCase(t, source, "if\ncase\nend\nfn\ncatch\ndo\nreturn\nwhile\nunless\nif\nreturn\nif\ncase\nif\nif1\n", "")
}

func TestSymbolNamesDoNotHideControlValuesAfterColons(t *testing.T) {
	source := `def identity(*, value: String): String
return value
end

def prefix(head: String, *, value: String): String
return head + value
end

def main()
values := {label: if true
:if
else
:case
end}
puts(values["label"])
puts(identity(value: if false
:if
else
:case
end))
puts(prefix(:if, value: if true
:case
else
:end
end))
end
`
	runPortableExecutionCase(t, source, "if\ncase\nifcase\n", "")
}
