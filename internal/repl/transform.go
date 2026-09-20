package repl

import (
	"errors"
	"fmt"
	"sort"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

func (e *Evaluator) transform(node *ir.Transform, module string, sc *scope) (Value, error) {
	source, err := e.expression(node.Source, module, sc)
	if err != nil {
		return Value{}, err
	}
	if node.Operation == "reduce" {
		accumulator, err := e.expression(node.Initial, module, sc)
		if err != nil {
			return Value{}, err
		}
		next, err := iterableCursor(source)
		if err != nil {
			return Value{}, err
		}
		for item, ok := next(); ok; item, ok = next() {
			if err := e.checkContext(); err != nil {
				return Value{}, err
			}
			iterationScope := &scope{parent: sc, values: map[string]Value{}}
			iterationScope.values[node.Accumulator] = accumulator
			iterationScope.values[node.Item] = item
			accumulator, err = e.transformResult(node, module, iterationScope)
			if err != nil {
				return Value{}, err
			}
		}
		accumulator.Type = node.ExprType()
		return accumulator, nil
	}
	if node.Operation == "concurrent_map" {
		items, err := e.materializeIterable(source)
		if err != nil {
			return Value{}, err
		}
		return e.concurrentMap(node, items, module, sc)
	}
	next, err := iterableCursor(source)
	if err != nil {
		return Value{}, err
	}
	if node.Operation == "sort_by" || node.Operation == "sort_by_descending" {
		type decoratedValue struct {
			value Value
			key   Value
		}
		decorated := []decoratedValue{}
		for item, ok := next(); ok; item, ok = next() {
			if err := e.checkContext(); err != nil {
				return Value{}, err
			}
			iterationScope := &scope{parent: sc, values: map[string]Value{node.Item: item}}
			key, err := e.transformResult(node, module, iterationScope)
			if err != nil {
				return Value{}, err
			}
			decorated = append(decorated, decoratedValue{value: item, key: key})
		}
		var compareErr error
		descending := node.Operation == "sort_by_descending"
		sort.SliceStable(decorated, func(left, right int) bool {
			compared, err := comparePortableValues(decorated[left].key, decorated[right].key, descending)
			if err != nil {
				compareErr = err
				return false
			}
			return compared < 0
		})
		if compareErr != nil {
			return Value{}, compareErr
		}
		result := &arrayValue{Items: make([]Value, len(decorated))}
		for index, item := range decorated {
			result.Items[index] = item.value
		}
		return Value{Type: node.ExprType(), Data: result}, nil
	}

	result := &arrayValue{}
	for index := 0; ; index++ {
		item, ok := next()
		if !ok {
			break
		}
		if err := e.checkContext(); err != nil {
			return Value{}, err
		}
		iterationScope := &scope{parent: sc, values: map[string]Value{node.Item: item}}
		if node.WithIndex {
			iterationScope.values[node.Index] = Value{Type: types.FromName("Integer"), Data: int64(index)}
		}
		value, err := e.transformResult(node, module, iterationScope)
		if err != nil {
			return Value{}, err
		}
		if node.Operation == "select" {
			selected, ok := value.Data.(bool)
			if !ok {
				return Value{}, errors.New("select block result must be Boolean")
			}
			if selected {
				result.Items = append(result.Items, item)
			}
			continue
		}
		if node.Operation == "any?" || node.Operation == "all?" || node.Operation == "none?" {
			matched, ok := value.Data.(bool)
			if !ok {
				return Value{}, fmt.Errorf("%s block result must be Boolean", node.Operation)
			}
			if node.Operation == "any?" && matched || node.Operation == "all?" && !matched || node.Operation == "none?" && matched {
				answer := node.Operation == "any?"
				return Value{Type: node.ExprType(), Data: answer}, nil
			}
			continue
		}
		if node.Operation == "find" || node.Operation == "find_index" {
			matched, ok := value.Data.(bool)
			if !ok {
				return Value{}, fmt.Errorf("%s block result must be Boolean", node.Operation)
			}
			if matched {
				if node.Operation == "find_index" {
					return Value{Type: node.ExprType(), Data: int64(index)}, nil
				}
				item.Type = node.ExprType()
				return item, nil
			}
			continue
		}
		result.Items = append(result.Items, value)
	}
	if node.Operation == "any?" {
		return Value{Type: node.ExprType(), Data: false}, nil
	}
	if node.Operation == "all?" || node.Operation == "none?" {
		return Value{Type: node.ExprType(), Data: true}, nil
	}
	if node.Operation == "find" || node.Operation == "find_index" {
		return Value{Type: node.ExprType(), Data: nil}, nil
	}
	return Value{Type: node.ExprType(), Data: result}, nil
}
