package compiler

import (
	"strings"
	"testing"
)

const unionFieldClasses = `class First
	@value: Integer := 1
end
class Second
	@value: Integer := 2
end
alias Choice = First | Second
`

func TestCommonClassUnionFieldAssignmentAcrossBackends(t *testing.T) {
	source := []byte(unionFieldClasses + `
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
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "main.trb", source, "7\n7\n7\n7")
		})
	}
}

func TestCommonClassUnionFieldAssignmentRejectsUnsafeTargets(t *testing.T) {
	tests := []struct{ name, classes, binding, assignment, want string }{
		{"immutable binding", unionFieldClasses, "item: Choice := First.new()", "item.value = 3", "immutable"},
		{"readonly alternative", `class First
	@value: Integer := 1
end
class Second
	readonly @value: Integer := 2
end
alias Choice = First | Second
`, "mut item: Choice := First.new()", "item.value = 3", "field value is readonly"},
		{"different field types", `class First
	@value: Integer := 1
end
class Second
	@value: String := "two"
end
alias Choice = First | Second
`, "mut item: Choice := First.new()", "item.value = 3", "different types in its alternatives"},
		{"private alternatives", `class First
	@_value: Integer := 1
end
class Second
	@_value: Integer := 2
end
alias Choice = First | Second
`, "mut item: Choice := First.new()", "item._value = 3", "private member"},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			for _, operator := range []string{"=", "+="} {
				if test.name == "different field types" && operator != "=" {
					continue
				}
				t.Run(mode+"/"+test.name+"/"+operator, func(t *testing.T) {
					assignment := strings.Replace(test.assignment, " = ", " "+operator+" ", 1)
					source := test.classes + "\ndef main()\n\t" + test.binding + "\n\t" + assignment + "\nend\n"
					_, err := Compile("main.trb", []byte(source), mode)
					if err == nil || !strings.Contains(err.Error(), test.want) {
						t.Fatalf("diagnostic = %v, want %q", err, test.want)
					}
				})
			}
		}
	}
}

func TestImportedCommonClassUnionFieldRejectsReadonlyAlternative(t *testing.T) {
	units := []SourceUnit{
		{Filename: "models.trb", ModulePath: "models", Source: []byte(`class First
	@value: Integer := 1
end
class Second
	readonly @value: Integer := 2
end
alias Choice = First | Second
`)},
		{Filename: "main.trb", ModulePath: "main", Source: []byte(`import { First, Choice } from models
def main()
	mut item: Choice := First.new()
	item.value = 3
end
`)},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		directUnits := append([]SourceUnit(nil), units...)
		directUnits[1].Source = []byte(`import { Second } from models
def main()
	item := Second.new()
	puts(item.value)
end
`)
		if _, err := CompileProject(directUnits, Options{Mode: mode}); err != nil {
			t.Fatalf("%s rejected imported readonly class field read: %v", mode, err)
		}
		readUnits := append([]SourceUnit(nil), units...)
		readUnits[1].Source = []byte(strings.Replace(string(units[1].Source), "item.value = 3", "puts(item.value)", 1))
		if _, err := CompileProject(readUnits, Options{Mode: mode}); err != nil {
			t.Fatalf("%s rejected an imported readonly union field read: %v", mode, err)
		}
		_, err := CompileProject(units, Options{Mode: mode})
		if err == nil || !strings.Contains(err.Error(), "field value is readonly") {
			t.Fatalf("%s imported readonly field diagnostic = %v", mode, err)
		}
	}
}
