package orm

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

type Operator string

const (
	Equal              Operator = "="
	NotEqual           Operator = "!="
	LessThan           Operator = "<"
	LessThanOrEqual    Operator = "<="
	GreaterThan        Operator = ">"
	GreaterThanOrEqual Operator = ">="
	In                 Operator = "IN"
	RangeInclusive     Operator = "RANGE_INCLUSIVE"
	RangeExclusive     Operator = "RANGE_EXCLUSIVE"
	NotIn              Operator = "NOT_IN"
)

type Predicate struct {
	Column   string
	Operator Operator
	Value    ir.Expression
}

func Predicates(call *ir.Call) []Predicate {
	if call == nil {
		return nil
	}
	predicates := make([]Predicate, 0, len(call.Arguments))
	for _, argument := range call.Arguments {
		if argument.Name == "" {
			continue
		}
		operator := Equal
		switch value := argument.Value.(type) {
		case *ir.Range:
			operator = RangeInclusive
			if value.Exclusive {
				operator = RangeExclusive
			}
		default:
			if argument.Value.ExprType().Name == "Subquery" {
				operator = In
			} else if argument.Value.ExprType().Kind == types.Array {
				operator = In
			}
		}
		predicates = append(predicates, Predicate{Column: argument.Name, Operator: operator, Value: argument.Value})
	}
	if len(predicates) > 0 || len(call.Arguments) != 3 {
		return predicates
	}
	column, columnOK := queryString(call.Arguments[0].Value)
	operator, operatorOK := queryString(call.Arguments[1].Value)
	if !columnOK || !operatorOK {
		return nil
	}
	resolvedOperator := Operator(operator)
	if call.Arguments[2].Value.ExprType().Name == "Subquery" {
		switch resolvedOperator {
		case Equal:
			resolvedOperator = In
		case NotEqual:
			resolvedOperator = NotIn
		}
	}
	return []Predicate{{Column: column, Operator: resolvedOperator, Value: call.Arguments[2].Value}}
}

func queryString(expression ir.Expression) (string, bool) {
	literal, ok := expression.(*ir.Literal)
	if !ok || literal.Kind != "string" {
		return "", false
	}
	if value, err := strconv.Unquote(literal.Raw); err == nil {
		return value, true
	}
	return strings.Trim(literal.Raw, "'\""), true
}
