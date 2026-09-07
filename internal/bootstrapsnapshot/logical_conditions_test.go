package bootstrapsnapshot

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// Deliberately interpret only this test's Boolean CFG. A fresh value map for
// each block rejects accidental cross-block references, not just wrong answers.
func runBooleanCFG(t *testing.T, functions []Function, name string, arguments []bool, calls *[]string) bool {
	t.Helper()
	var function Function
	for _, candidate := range functions {
		if candidate.Name == name {
			function = candidate
		}
	}
	if function.ID == "" || len(arguments) != len(function.Parameters) {
		t.Fatalf("invalid test call %s(%v)", name, arguments)
	}
	parameters := map[string]bool{}
	for index, parameter := range function.Parameters {
		parameters[parameter.ID] = arguments[index]
	}
	next := function.Entry
	var incoming []bool
	for step := 0; step < 128; step++ {
		var block Block
		for _, candidate := range function.Blocks {
			if candidate.ID == next {
				block = candidate
			}
		}
		if block.ID == "" || len(incoming) != len(block.Parameters) {
			t.Fatalf("invalid edge to %s with %v", next, incoming)
		}
		values := map[string]bool{}
		for id, value := range parameters {
			values[id] = value
		}
		for index, parameter := range block.Parameters {
			if parameter.Type != "Boolean" {
				t.Fatalf("unexpected block parameter type %s", parameter.Type)
			}
			values[parameter.ID] = incoming[index]
		}
		read := func(id string) bool {
			value, exists := values[id]
			if !exists {
				t.Fatalf("unavailable value %s in %s/%s", id, name, block.ID)
			}
			return value
		}
		readArguments := func(ids []string) []bool {
			result := make([]bool, len(ids))
			for index, id := range ids {
				result[index] = read(id)
			}
			return result
		}
		for _, instruction := range block.Instructions {
			switch node := instruction.(type) {
			case BooleanLiteral:
				values[node.Result] = node.Value
			case BooleanNot:
				values[node.Result] = !read(node.Value)
			case Call:
				callee := strings.TrimPrefix(node.Function, "main#")
				*calls = append(*calls, callee)
				values[*node.Result] = runBooleanCFG(t, functions, callee, readArguments(node.Arguments), calls)
			default:
				t.Fatalf("unexpected instruction %T", instruction)
			}
		}
		switch node := block.Terminator.(type) {
		case Return:
			return read(*node.Value)
		case Jump:
			next, incoming = node.Target, readArguments(node.Arguments)
		case Branch:
			if read(node.Condition) {
				next, incoming = node.WhenTrue, readArguments(node.TrueArguments)
			} else {
				next, incoming = node.WhenFalse, readArguments(node.FalseArguments)
			}
		default:
			t.Fatalf("unexpected terminator %T", block.Terminator)
		}
	}
	t.Fatal("Boolean CFG exceeded bounded execution budget")
	return false
}

func logicalConditionFunctions(t *testing.T, version int, source string) []Function {
	t.Helper()
	artifacts := analyzeV4Program(t, source)
	if version == 3 {
		snapshot, err := BuildV3(artifacts, "/project/src")
		if err != nil {
			t.Fatal(err)
		}
		repeated, err := BuildV3(artifacts, "/project/src")
		if err != nil || !reflect.DeepEqual(snapshot, repeated) {
			t.Fatalf("nondeterministic logical snapshot: %v", err)
		}
		return snapshot.Functions
	}
	snapshot, err := BuildV4(artifacts, "/project/src")
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := BuildV4(artifacts, "/project/src")
	if err != nil {
		t.Fatal(err)
	}
	first, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(repeated)
	if err != nil || string(first) != string(second) {
		t.Fatalf("nondeterministic logical snapshot: %v", err)
	}
	var functions []Function
	for _, function := range snapshot.Functions {
		functions = append(functions, Function{
			ID: function.ID, Name: function.Name, Parameters: function.Parameters,
			Result: function.Result, Entry: function.Entry, Blocks: function.Blocks,
		})
	}
	return functions
}

func TestSnapshotLogicalConditions(t *testing.T) {
	for _, version := range []int{3, 4} {
		for _, expression := range []string{
			"left(a) && right(b)", "left(a) || right(b)",
			"!(left(a) && right(b))", "!(left(a) || right(b))",
			"left(a) || right(b) && !a", "(left(a) || right(b)) && !a",
			"left(a) && (right(b) || !a)",
		} {
			t.Run(fmt.Sprintf("v%d/%s", version, expression), func(t *testing.T) {
				source := "def left(value: Boolean): Boolean\nreturn value\nend\n" +
					"def right(value: Boolean): Boolean\nreturn value\nend\n" +
					"def probe(a: Boolean, b: Boolean): Boolean\nif " + expression +
					"\nreturn true\nend\nreturn false\nend\ndef main()\nend\n"
				functions := logicalConditionFunctions(t, version, source)
				for _, a := range []bool{false, true} {
					for _, b := range []bool{false, true} {
						var wantedCalls, calls []string
						left := func() bool { wantedCalls = append(wantedCalls, "left"); return a }
						right := func() bool { wantedCalls = append(wantedCalls, "right"); return b }
						var want bool
						switch expression {
						case "left(a) && right(b)":
							want = left() && right()
						case "left(a) || right(b)":
							want = left() || right()
						case "!(left(a) && right(b))":
							want = !(left() && right())
						case "!(left(a) || right(b))":
							want = !(left() || right())
						case "left(a) || right(b) && !a":
							want = left() || right() && !a
						case "(left(a) || right(b)) && !a":
							want = (left() || right()) && !a
						case "left(a) && (right(b) || !a)":
							want = left() && (right() || !a)
						}
						got := runBooleanCFG(t, functions, "probe", []bool{a, b}, &calls)
						if got != want || !reflect.DeepEqual(calls, wantedCalls) {
							t.Fatalf("(%v,%v): got %v/%v, want %v/%v", a, b, got, calls, want, wantedCalls)
						}
					}
				}
			})
		}
	}
}

func TestSnapshotLogicalLoopBackedges(t *testing.T) {
	for _, version := range []int{3, 4} {
		for _, condition := range []string{"active && enabled", "!( !active || !enabled )"} {
			source := "def probe(mut active: Boolean, enabled: Boolean): Boolean\n" +
				"mut visited := false\nwhile " + condition +
				"\nvisited = true\nactive = false\nend\nreturn visited\nend\ndef main()\nend\n"
			functions := logicalConditionFunctions(t, version, source)
			for _, active := range []bool{false, true} {
				for _, enabled := range []bool{false, true} {
					var calls []string
					if got := runBooleanCFG(t, functions, "probe", []bool{active, enabled}, &calls); got != (active && enabled) {
						t.Fatalf("v%d %s (%v,%v): %v", version, condition, active, enabled, got)
					}
				}
			}
		}
	}
}

func TestSnapshotLogicalValuesRemainUnsupported(t *testing.T) {
	for _, operator := range []string{"&&", "||"} {
		for _, body := range []string{
			"return a " + operator + " b",
			"if accept(a " + operator + " b)\nreturn true\nend\nreturn false",
		} {
			artifacts := analyzeV4Program(t, "def accept(value: Boolean): Boolean\nreturn value\nend\n"+
				"def value(a: Boolean, b: Boolean): Boolean\n"+body+"\nend\ndef main()\nend\n")
			_, v3err := BuildV3(artifacts, "/project/src")
			_, v4err := BuildV4(artifacts, "/project/src")
			for _, err := range []error{v3err, v4err} {
				if err == nil || !strings.Contains(err.Error(), "binary operator "+operator) {
					t.Fatalf("unsupported logical value: %v", err)
				}
			}
		}
	}
}
