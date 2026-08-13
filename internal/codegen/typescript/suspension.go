package typescript

import (
	"fmt"

	"github.com/type-rb/type-rb/internal/codegen/effectplan"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

// SuspensionPlan is backend-owned analysis data. It deliberately does not
// make suspension part of TypeRB syntax, checking, or shared semantics.
type SuspensionPlan = effectplan.Plan

func AnalyzeSuspension(programs []*ir.Program) (*SuspensionPlan, error) {
	plan := effectplan.Analyze(programs, effectplan.Options{Intrinsic: isSuspendingIntrinsic, WebNext: true, PassToFunctions: true})
	for lambda, suspends := range plan.Lambdas {
		if suspends && lambda.SuccessType.Kind != types.Void && lambda.Fails.Kind == types.Never {
			return nil, fmt.Errorf("TypeScript function values that may suspend must omit their return type")
		}
	}
	return plan, nil
}

func isSuspendingORM(intrinsic string, fails types.Type) bool {
	return effectplan.ORMOperation(intrinsic, fails)
}

func isSuspendingIntrinsic(intrinsic string, fails types.Type) bool {
	return isSuspendingORM(intrinsic, fails) || intrinsic == "trb.web.testing.dispatch" || intrinsic == "trb.web.middleware.logger.call" || intrinsic == "trb.web.middleware.timeout.call" || intrinsic == "trb.platform.typescript.browser.request"
}
