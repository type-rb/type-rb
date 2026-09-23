package lower

import (
	"github.com/type-rb/type-rb/internal/checker"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

func (l *lowerer) classAliasReceiver(construction checker.ClassConstruction, span token.Span) ir.Expression {
	resolved := construction.ResolvedType
	if construction.TargetBinding != nil && construction.TargetBinding.Import != nil {
		module := construction.TargetBinding.Import.RuntimePath()
		l.requireGeneratedType(resolved)
		if l.generatedValueSymbols[module] == nil {
			l.generatedValueSymbols[module] = map[string]bool{}
		}
		l.generatedValueSymbols[module][resolved.Declaration.Name] = true
	}
	name := resolved.Name
	if resolved.Declaration.Name != "" {
		name = resolved.Declaration.Name
	}
	receiver := ir.Expression(&ir.Identifier{
		ExprBase: ir.NewExprBase(span, resolved), Name: name,
		Declaration: resolved.Declaration, Reference: referenceFromBinding(construction.TargetBinding),
	})
	if len(resolved.Args) > 0 {
		receiver = &ir.TypeApply{
			ExprBase: ir.NewExprBase(span, resolved), Receiver: receiver,
			Declaration: resolved.Declaration, Arguments: append([]types.Type(nil), resolved.Args...), Kind: "class",
		}
	}
	return receiver
}
