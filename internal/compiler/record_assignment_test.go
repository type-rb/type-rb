package compiler

import (
	"strings"
	"testing"
)

const recordAssignmentDeclarations = `record Sample
	value: Integer
	values: Array<Integer> = []
end

record Outer
	inner: Sample
end

record Box<T>
	value: T
end

alias SampleAlias = Sample

def sample(): Sample
	return Sample.new(value: 1)
end
`

func TestRecordFieldAssignmentsRejectedAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, operator := range []string{"=", "+=", "-=", "*=", "/="} {
			for _, test := range []struct{ name, setup, target string }{
				{"local", "mut value := Sample.new(value: 1)", "value.value"},
				{"nested", "mut value := Outer.new(inner: Sample.new(value: 1))", "value.inner.value"},
				{"generic", "mut value := Box<Integer>.new(value: 1)", "value.value"},
				{"alias", "mut value: SampleAlias := Sample.new(value: 1)", "value.value"},
				{"indexed", "mut values := [Sample.new(value: 1)]", "values[0].value"},
				{"returned", "", "sample().value"},
			} {
				t.Run(mode+"/"+operator+"/"+test.name, func(t *testing.T) {
					source := recordAssignmentDeclarations + "\ndef main()\n" + test.setup + "\n" + test.target + " " + operator + " 2\nend\n"
					_, err := Compile("main.trb", []byte(source), mode)
					if err == nil || !strings.Contains(err.Error(), "field value is readonly") {
						t.Fatalf("record write diagnostic = %v", err)
					}
				})
			}
			t.Run(mode+"/"+operator+"/parameter", func(t *testing.T) {
				source := recordAssignmentDeclarations + "\ndef update(mut value: Sample)\nvalue.value " + operator + " 2\nend\n"
				_, err := Compile("parameter.trb", []byte(source), mode)
				if err == nil || !strings.Contains(err.Error(), "field value is readonly") {
					t.Fatalf("record parameter write diagnostic = %v", err)
				}
			})
		}
	}
}

func TestImportedRecordFieldAssignmentsRejectedAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, operator := range []string{"=", "+="} {
			t.Run(mode+"/"+operator, func(t *testing.T) {
				_, err := CompileProject([]SourceUnit{
					{Filename: "sample.trb", ModulePath: "sample", Source: []byte("record Sample\nvalue: Integer\nend\n")},
					{Filename: "main.trb", ModulePath: "main", Source: []byte("import { Sample as Item } from sample\ndef main()\nmut item := Item.new(value: 1)\nitem.value " + operator + " 2\nend\n")},
				}, Options{Mode: mode})
				if err == nil || !strings.Contains(err.Error(), "field value is readonly") {
					t.Fatalf("imported record write diagnostic = %v", err)
				}
			})
		}
	}
}

func TestRecordLogicalAssignmentsRejectedAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, operator := range []string{"||=", "&&="} {
			t.Run(mode+"/"+operator, func(t *testing.T) {
				_, err := Compile("main.trb", []byte("record Flag\nvalue: Boolean\nend\ndef main()\nmut flag := Flag.new(value: true)\nflag.value "+operator+" false\nend\n"), mode)
				if err == nil || !strings.Contains(err.Error(), "field value is readonly") {
					t.Fatalf("logical record write diagnostic = %v", err)
				}
			})
		}
	}
}

func TestRecordRebindingAndContainedMutationAcrossModes(t *testing.T) {
	source := []byte(recordAssignmentDeclarations + `
def grow(mut value: Sample)
	value.values.push(3)
	value.values[0] += 4
	value = Sample.new(value: 9)
	puts(value.value)
end

def main()
	mut value := Sample.new(value: 1, values: [2])
	grow(value)
	puts(value.value)
	puts(value.values[0])
	puts(value.values[1])
	value = Sample.new(value: 7)
	puts(value.value)
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "main.trb", source, "9\n1\n6\n3\n7")
		})
	}
}
