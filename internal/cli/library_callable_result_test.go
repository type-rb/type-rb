package cli

import (
	"strings"
	"testing"
)

func TestLibraryCallsPreserveFunctionResults(t *testing.T) {
	source := `def main()
mut captured := 3
read := fn(): Integer
return captured
end
mut callbacks := [read, read, read]
first := callbacks.first()
last := callbacks.last()
popped := callbacks.pop()
shifted := callbacks.shift()
table := {"read" => read}
fetched := table.fetch("read")
captured = 7
puts(first() == 7)
puts(last() == 7)
puts(popped() == 7)
puts(shifted() == 7)
puts(fetched() == 7)
puts(callbacks.size() == 1)
end
`
	runPortableExecutionCase(t, source, strings.Repeat("true\n", 6), "")
}
