package bootstrapsnapshot

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
)

func TestSnapshotLoopTransferValuesAndConditionEffects(t *testing.T) {
	for _, version := range []int{3, 4} {
		for _, transfer := range []string{"if selected\nbreak\nelse\nnext\nend", "break if selected\nnext"} {
			source := "def condition(value: Boolean): Boolean\nreturn value\nend\n" +
				"def probe(mut active: Boolean, selected: Boolean): Boolean\nmut result := false\n" +
				"while condition(active)\nactive = false\nlocal := true\nresult = local\n" + transfer +
				"\nresult = false\nend\nreturn result\nend\ndef main()\nend\n"
			functions := logicalConditionFunctions(t, version, source)
			for _, active := range []bool{false, true} {
				for _, selected := range []bool{false, true} {
					var calls []string
					got := runBooleanCFG(t, functions, "probe", []bool{active, selected}, &calls)
					wantCalls := []string{"condition"}
					if active && !selected {
						wantCalls = append(wantCalls, "condition")
					}
					if got != active || !reflect.DeepEqual(calls, wantCalls) {
						t.Fatalf("v%d active=%v selected=%v: %v/%v, want %v/%v", version, active, selected, got, calls, active, wantCalls)
					}
				}
			}
		}
	}
}

func TestSnapshotNestedLoopTargetsAndMethodReturns(t *testing.T) {
	for _, version := range []int{3, 4} {
		for _, transfer := range []string{"break", "next", "return false"} {
			t.Run(fmt.Sprintf("v%d/%s", version, transfer), func(t *testing.T) {
				source := "def probe(mut active: Boolean): Boolean\nmut result := false\nwhile active\n" +
					"active = false\nmut inner := true\nwhile inner\ninner = false\n" + transfer +
					"\nresult = false\nend\nresult = true\nnext\nresult = false\nend\nreturn result\nend\ndef main()\nend\n"
				functions := logicalConditionFunctions(t, version, source)
				for _, active := range []bool{false, true} {
					var calls []string
					want := active && transfer != "return false"
					if got := runBooleanCFG(t, functions, "probe", []bool{active}, &calls); got != want {
						t.Fatalf("active=%v: %v, want %v", active, got, want)
					}
				}
			})
		}
	}
}

func TestSnapshotLoopMixedTerminatingBranches(t *testing.T) {
	source := "def probe(active: Boolean, selected: Boolean): Boolean\nwhile active\n" +
		"if selected\nreturn true\nelse\nbreak\nend\nreturn true\nend\nreturn false\nend\ndef main()\nend\n"
	for _, version := range []int{3, 4} {
		functions := logicalConditionFunctions(t, version, source)
		for _, active := range []bool{false, true} {
			for _, selected := range []bool{false, true} {
				var calls []string
				if got := runBooleanCFG(t, functions, "probe", []bool{active, selected}, &calls); got != (active && selected) {
					t.Fatalf("v%d active=%v selected=%v: %v", version, active, selected, got)
				}
			}
		}
	}
}

func TestSnapshotLoopTransferManagedValuesAndOrigins(t *testing.T) {
	source := "def probe(mut active: Boolean, selected: Boolean): String\nmut value := \"start\"\nwhile active\n" +
		"value = \"updated\"\nif selected\nbreak\nend\nactive = false\nnext\nend\nreturn value\nend\ndef main()\nend\n"
	functions := logicalConditionFunctions(t, 4, source)
	transfers := 0
	for _, function := range functions {
		if function.Name != "probe" {
			continue
		}
		for _, block := range function.Blocks {
			jump, ok := block.Terminator.(Jump)
			if !ok || (jump.Origin.StartLine != 6 && jump.Origin.StartLine != 9) {
				continue
			}
			transfers++
			found := false
			for _, target := range function.Blocks {
				if target.ID != jump.Target {
					continue
				}
				if len(jump.Arguments) != len(target.Parameters) {
					t.Fatal("transfer omitted live loop arguments")
				}
				for _, parameter := range target.Parameters {
					found = found || parameter.Type == "String"
				}
			}
			if !found {
				t.Fatal("transfer lost its managed binding")
			}
		}
	}
	if transfers != 2 {
		t.Fatalf("got %d transfer source origins, want 2", transfers)
	}
}

func TestSnapshotLoopTransferRejectsMissingOwnerAndKeepsV2Boundary(t *testing.T) {
	for _, version := range []int{3, 4} {
		lowerer := &v3FunctionLowerer{version: version, program: &ir.Program{SourcePath: "loop.trb"}}
		for _, statement := range []ir.Statement{&ir.Break{}, &ir.Next{}} {
			if _, err := lowerer.lowerStatement(statement); err == nil || !strings.Contains(err.Error(), "loop transfer outside while") {
				t.Fatalf("v%d accepted transfer without an owner: %v", version, err)
			}
		}
	}
	artifacts := analyzeV4Program(t, "def main()\nwhile true\nbreak\nend\nend\n")
	if _, err := BuildV2(artifacts, "/project/src"); err == nil || !strings.Contains(err.Error(), "snapshot v2") {
		t.Fatalf("version 2 boundary changed: %v", err)
	}
}
