package codegen

import (
	"fmt"

	"github.com/type-rb/type-rb/internal/codegen/effectplan"
	"github.com/type-rb/type-rb/internal/ir"
)

func anchoredIntrinsic(expression ir.Expression) string {
	switch callee := expression.(type) {
	case *ir.TypeApply:
		return anchoredIntrinsic(callee.Receiver)
	case *ir.Member:
		if callee.Reference != nil {
			return callee.Reference.Intrinsic
		}
	case *ir.Identifier:
		if callee.Reference != nil {
			return callee.Reference.Intrinsic
		}
	}
	return ""
}

func validateAnchoredDirectories(programs []*ir.Program) error {
	anchored := func(name string) bool {
		switch name {
		case "trb.std.dir.open", "trb.std.dir.open_file", "trb.std.dir.root_children", "trb.std.dir.root_create_all", "trb.std.dir.try_lock":
			return true
		}
		return false
	}
	for _, program := range programs {
		if program.Mode == "go" {
			continue
		}
		plan := effectplan.Analyze([]*ir.Program{program}, effectplan.Options{Intrinsic: anchored})
		var first *ir.Call
		for call := range plan.Calls {
			name := anchoredIntrinsic(call.Callee)
			if anchored(name) && (first == nil || call.SourceSpan().Start.Offset < first.SourceSpan().Start.Offset) {
				first = call
			}
		}
		if first != nil {
			position := first.SourceSpan().Start
			return fmt.Errorf("%s:%d:%d: anchored Dir operations require the Go native Linux/macOS adapter; %s is unsupported", program.SourcePath, position.Line, position.Column, program.Mode)
		}
	}
	return nil
}
