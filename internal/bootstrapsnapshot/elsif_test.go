package bootstrapsnapshot

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestSnapshotElsifEvaluationAndJoins(t *testing.T) {
	helpers := "def first(value: Boolean): Boolean\nreturn value\nend\n" +
		"def second(value: Boolean): Boolean\nreturn value\nend\n" +
		"def third(value: Boolean): Boolean\nreturn value\nend\n"
	for _, version := range []int{3, 4} {
		for _, returns := range []bool{false, true} {
			for _, hasElse := range []bool{false, true} {
				t.Run(fmt.Sprintf("v%d/returns=%v/else=%v", version, returns, hasElse), func(t *testing.T) {
					body := func(value string) string {
						if returns {
							return "return " + value
						}
						return "local := " + value + "\nresult = local"
					}
					source := helpers + "def probe(a: Boolean, b: Boolean, c: Boolean): Boolean\nmut result := false\n" +
						"if first(a)\n" + body("true") + "\nelsif second(b) && !a\n" + body("false") +
						"\nelsif third(c)\n" + body("true") + "\n"
					if hasElse {
						source += "else\n" + body("false") + "\n"
					}
					source += "end\nreturn result\nend\ndef main()\nend\n"
					functions := logicalConditionFunctions(t, version, source)
					for _, a := range []bool{false, true} {
						for _, b := range []bool{false, true} {
							for _, c := range []bool{false, true} {
								want := false
								expected := []string{"first"}
								if a {
									want = true
								} else {
									expected = append(expected, "second")
									if !b {
										expected = append(expected, "third")
										want = c
									}
								}
								var calls []string
								got := runBooleanCFG(t, functions, "probe", []bool{a, b, c}, &calls)
								if got != want || !reflect.DeepEqual(calls, expected) {
									t.Fatalf("(%v,%v,%v): %v/%v, want %v/%v", a, b, c, got, calls, want, expected)
								}
							}
						}
					}
				})
			}
		}
	}
}

func TestSnapshotElsifNestedLoops(t *testing.T) {
	source := `def probe(mut active: Boolean, first: Boolean, second: Boolean): Boolean
 mut result := false
 while active
  if first
   result = true
  elsif second
   if false
    result = false
   elsif active
    result = true
   end
  else
   result = false
  end
  active = false
 end
 return result
end

def main()
end
`
	for _, version := range []int{3, 4} {
		functions := logicalConditionFunctions(t, version, source)
		for _, active := range []bool{false, true} {
			for _, first := range []bool{false, true} {
				for _, second := range []bool{false, true} {
					var calls []string
					if got := runBooleanCFG(t, functions, "probe", []bool{active, first, second}, &calls); got != (active && (first || second)) {
						t.Fatalf("v%d: wrong nested result", version)
					}
				}
			}
		}
	}
}

func TestSnapshotElsifPreservesManagedJoinsAndOrigins(t *testing.T) {
	source := "def select(a: Boolean, b: Boolean): String\nmut value := \"default\"\nif a\nlocal := \"first\"\nvalue = local\nelsif b\nlocal := \"second\"\nvalue = local\nelse\nlocal := \"last\"\nvalue = local\nend\nreturn value\nend\ndef main()\nend\n"
	functions := logicalConditionFunctions(t, 4, source)
	branches, managedJoins := 0, 0
	for _, function := range functions {
		if function.Name != "select" {
			continue
		}
		for _, block := range function.Blocks {
			if branch, ok := block.Terminator.(Branch); ok {
				branches++
				if branch.Origin.StartLine != 3 && branch.Origin.StartLine != 6 {
					t.Fatalf("wrong branch origin: %#v", branch.Origin)
				}
			}
			for _, parameter := range block.Parameters {
				if parameter.Type == "String" {
					managedJoins++
				}
			}
		}
	}
	if branches != 2 || managedJoins == 0 {
		t.Fatal("missing branch or managed join")
	}
}

func TestSnapshotElsifRejectsConditionalValuesAndKeepsV2Boundary(t *testing.T) {
	expression := "def value(a: Boolean, b: Boolean): Boolean\nresult := if a\ntrue\nelsif b\nfalse\nelse\ntrue\nend\nreturn result\nend\ndef main()\nend\n"
	artifacts := analyzeV4Program(t, expression)
	_, v3err := BuildV3(artifacts, "/project/src")
	_, v4err := BuildV4(artifacts, "/project/src")
	for _, err := range []error{v3err, v4err} {
		if err == nil || !strings.Contains(err.Error(), "value-producing if") {
			t.Fatalf("conditional value: %v", err)
		}
	}
	source := "def probe(value: Boolean): Boolean\nif value\nreturn true\nelsif false\nreturn false\nend\nreturn false\nend\ndef main()\nend\n"
	_, err := BuildV2(analyzeV4Program(t, source), "/project/src")
	if err == nil || !strings.Contains(err.Error(), "elsif") {
		t.Fatalf("v2 boundary changed: %v", err)
	}
}
