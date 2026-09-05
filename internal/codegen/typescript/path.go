package typescript

import (
	"fmt"

	"github.com/type-rb/type-rb/internal/codegen/effectplan"
	"github.com/type-rb/type-rb/internal/ir"
)

func validateHostPathOperations(program *ir.Program) error {
	if program.TypeScriptRuntime != "browser" {
		return nil
	}
	plan := effectplan.Analyze([]*ir.Program{program}, effectplan.Options{
		Intrinsic: func(name string) bool { return name == "trb.std.path.join" },
	})
	var first *ir.Call
	for call := range plan.Calls {
		reference := expressionReference(call.Callee)
		if reference != nil && reference.Intrinsic == "trb.std.path.join" &&
			(first == nil || call.SourceSpan().Start.Offset < first.SourceSpan().Start.Offset) {
			first = call
		}
	}
	if first != nil {
		position := first.SourceSpan().Start
		return fmt.Errorf("%s:%d:%d: Path#join requires a Node or Bun host; it is unavailable with typescript.runtime: browser", program.SourcePath, position.Line, position.Column)
	}
	return nil
}

func pathJoinExpression(parent, child, windows string) string {
	return `((parent: string, child: string, windows: boolean): string => {
		const separator = windows ? "\\" : "/";
		if (windows) child = child.split("/").join(separator);
		const drive = windows && /^[A-Za-z]:$/.test(parent);
		if (parent === "" || drive || parent.endsWith("/") || (windows && parent.endsWith("\\"))) return parent + child;
		return parent + separator + child;
	})(` + parent + ", " + child + ", " + windows + ")"
}

func hostPathWindowsExpression() string {
	return `((): boolean => {
		const host = (globalThis as { process?: { platform?: string } }).process;
		if (host?.platform === undefined) throw new Error("Path#join requires a Node or Bun host");
		return host.platform === "win32";
	})()`
}
