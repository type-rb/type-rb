package effectplan

import (
	"testing"

	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/stdlib"
	"github.com/type-rb/type-rb/internal/types"
)

func TestResourceGraphRejectsUnknownAndNativeEdges(t *testing.T) {
	for name, body := range map[string]ir.Statement{
		"native statement":          &ir.Native{Text: "opaque"},
		"native block":              &ir.NativeBlock{},
		"native expression":         &ir.ExpressionStatement{Expression: &ir.NativeExpression{Text: "opaque"}},
		"unknown conversion":        &ir.ExpressionStatement{Expression: &ir.Conversion{Kind: "future_conversion"}},
		"unknown iteration":         &ir.Iterate{Operation: "future_iteration"},
		"dynamic native property":   &ir.Return{Value: &ir.Member{Receiver: &ir.Identifier{ExprBase: ir.ExprBase{Type: types.FromName("Any")}, Name: "value"}, Name: "field"}},
		"unknown intrinsic":         &ir.ExpressionStatement{Expression: &ir.Call{Callee: &ir.Identifier{Reference: &ir.Reference{Intrinsic: "trb.std.file.future_operation"}}}},
		"missing source":            &ir.ExpressionStatement{Expression: &ir.Call{Callee: &ir.Identifier{Name: "missing", Declaration: identity.Declaration{Module: "absent", Name: "missing", Kind: identity.Function}}}},
		"native claims synchronous": &ir.ExpressionStatement{Expression: &ir.Call{Callee: &ir.Identifier{Reference: &ir.Reference{Runtime: &ir.RuntimeBinding{Identity: "synthetic", MaySuspend: false}}}}},
	} {
		t.Run(name, func(t *testing.T) {
			method := &ir.Method{Name: "borrow", Parameters: []ir.Parameter{{Name: "file", Type: stdlib.FileResourceType()}}, Body: []ir.Statement{body}}
			program := &ir.Program{ModulePath: "main", SourcePath: "main.trb", Statements: []ir.Statement{method}}
			if got := ValidateResources([]*ir.Program{program}); len(got) != 1 {
				t.Fatalf("diagnostics: %#v", got)
			}
		})
	}
}

func TestResourceGraphDoesNotTreatProviderIntrinsicPrefixesAsProof(t *testing.T) {
	for _, name := range []string{"trb.std.file.future_operation", "trb.std.strings.future_operation", "trb.cli.run", "trb.web.testing.dispatch"} {
		if resourceSyncIntrinsics[name] {
			t.Fatalf("unexpected admission: %s", name)
		}
	}
}
