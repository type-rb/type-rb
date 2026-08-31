package golang

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/cliapp"
	"github.com/type-rb/type-rb/internal/ir"
	ormintegration "github.com/type-rb/type-rb/internal/orm"
	"github.com/type-rb/type-rb/internal/types"
)

func TestCLIApplicationFailureClosesMainLifecycleBeforeExit(t *testing.T) {
	never := types.Type{Kind: types.Never, Name: "Never"}
	failure := &ir.Call{
		ExprBase: ir.NewExprBase(ir.Base{}.Span, never),
		Callee: &ir.Identifier{
			ExprBase:  ir.NewExprBase(ir.Base{}.Span, never),
			Name:      "fail",
			Reference: &ir.Reference{Intrinsic: "trb.cli.fail"},
		},
		Arguments: []ir.CallArgument{{Value: &ir.Literal{
			ExprBase: ir.NewExprBase(ir.Base{}.Span, types.FromName("String")), Kind: "string", Raw: `"stop"`,
		}}},
	}
	g := &generator{
		topMethods:     map[string]bool{"main": true},
		staticMethods:  map[string]map[string]bool{},
		records:        map[string]bool{},
		classes:        map[string]bool{},
		typeAliases:    map[string]string{},
		typeNames:      map[string]string{},
		typeKinds:      map[string]string{},
		imports:        map[string]string{},
		bindingNames:   map[string]string{},
		bindingSources: map[string]bool{},
		lexicalNames:   map[string]string{},
		moduleMethods:  map[string]bool{},
		modulePath:     "main",
		goModule:       "example.com/cli-lifecycle",
		cli:            &cliapp.Manifest{},
		cliInvocations: map[int]bool{},
		orm:            &ormintegration.Manifest{Models: []ormintegration.Model{{Name: "Record"}}},
	}
	g.topLevelMethod(&ir.Method{
		Name:       "main",
		ReturnType: types.FromName("Void"),
		Body:       []ir.Statement{&ir.ExpressionStatement{Expression: failure}},
	})
	g.cliApplicationFailureBoundarySupport()
	output := g.b.String()
	boundaryIndex := strings.Index(output, "defer trb__cliApplicationFailureBoundary_")
	closeIndex := strings.Index(output, "defer orm.TrbOrmCloseDatabase()")
	failureIndex := strings.Index(output, "panic(trb__cliApplicationFailure_")
	recoverIndex := strings.Index(output, "recovered := recover()")
	exitIndex := strings.Index(output, "os.Exit(1)")
	if boundaryIndex < 0 || closeIndex < 0 || failureIndex < 0 || recoverIndex < 0 || exitIndex < 0 {
		t.Fatalf("generated CLI lifecycle boundary is incomplete:\n%s", output)
	}
	if !(boundaryIndex < closeIndex && closeIndex < failureIndex && recoverIndex < exitIndex) {
		t.Fatalf("generated CLI lifecycle defer is outside the recover boundary:\n%s", output)
	}
}
