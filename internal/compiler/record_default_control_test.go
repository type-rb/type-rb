package compiler

import (
	"strings"
	"testing"
)

func TestRecordDefaultControlExpressionsAcrossModes(t *testing.T) {
	source := []byte(`enum Choice
	First
	Second
end
record Box<T>
	value: T
	choose: Boolean = true
	copy: T = if choose
		value
	else
		value
	end
	choice: Choice = Choice::First
	number: Integer = case choice
	when Choice::First
		1
	when Choice::Second
		2
	end
	tail: String = "after"
end

record Selection
	value: String
	choose: Boolean = true
	copy: String = choose ? value : "other"
end

def main()
	left := Box<String>.new(value: "held")
	puts(left.copy)
	puts(left.number)
	puts(left.tail)
	right := Box<Integer>.new(value: 7, choose: false, choice: Choice::Second)
	puts(right.copy)
	puts(right.number)
	puts(right.tail)
	puts(Selection.new(value: "selected").copy)
	puts(Selection.new(value: "ignored", choose: false).copy)
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "record_control.trb", source, "held\n1\nafter\n7\n2\nafter\nselected\nother")
		})
	}
}

func TestRecordDefaultControlExpressionsKeepDeclarationScope(t *testing.T) {
	source := []byte(`record Box<T>
	value: T
	copy: T = if true
		later
	else
		value
	end
	later: T = value
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		_, err := Compile("record_control_scope.trb", source, mode)
		if err == nil || !strings.Contains(err.Error(), "record field default cannot reference current or later field later") {
			t.Fatalf("%s: expected declaration-scope diagnostic, got %v", mode, err)
		}
	}
}
