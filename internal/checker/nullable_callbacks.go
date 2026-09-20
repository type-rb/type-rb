package checker

import (
	"maps"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/token"
)

// Flow scopes copy symbols when narrowing or assigning. Source identity keeps
// those copies tied to the same binding without conflating shadowed names.
type nullableBindingKey struct {
	name string
	span token.Span
}

func markNullableCaptures(sc *scope, names map[string]bool) {
	for name := range names {
		if value, owner, ok := sc.lookupOwner(name); ok && value.mutable {
			// Record on the declaration scope, not a temporary narrowing overlay.
			// Other functions' captures never enter this scope's call inventory.
			for parent := owner.parent; parent != nil; parent = parent.parent {
				if original, exists := parent.values[name]; exists && original.span == value.span {
					owner = parent
				}
			}
			if owner.nullableCaptures == nil {
				owner.nullableCaptures = map[nullableBindingKey]bool{}
			}
			owner.nullableCaptures[nullableBindingKey{name, value.span}] = true
		}
	}
}

func capturedNullableWrites(sc *scope) map[string]bool {
	writes := map[string]bool{}
	for current := sc; current != nil; current = current.parent {
		if current.parent == nil || current.globalStorage {
			// Source-module storage is reachable from ordinary functions as well
			// as closures. Until calls carry replacement effects, a writable
			// global cannot retain an earlier nullable or field proof.
			for name, global := range current.values {
				if global.variable != nil && global.mutable {
					if value, ok := sc.lookup(name); ok && value.span == global.span {
						writes[name] = true
					}
				}
			}
		}
		for key := range current.nullableCaptures {
			if value, ok := sc.lookup(key.name); ok && value.mutable && value.span == key.span {
				writes[key.name] = true
			}
		}
	}
	return writes
}

func invalidateCapturedNullableFacts(sc *scope) {
	invalidateNullableWrites(sc, capturedNullableWrites(sc))
}

type nullableWriteInventory struct {
	writes   map[string]bool
	captures map[string]bool
	calls    bool
}

func newNullableWriteInventory() *nullableWriteInventory {
	return &nullableWriteInventory{writes: map[string]bool{}, captures: map[string]bool{}}
}

func lambdaNullableWrites(node *ast.LambdaExpression) map[string]bool {
	hidden := map[string]bool{}
	for _, parameter := range node.Parameters {
		hidden[parameter.Name] = true
	}
	inventory := newNullableWriteInventory()
	inventory.statements(node.Body, hidden)
	maps.Copy(inventory.writes, inventory.captures)
	return inventory.writes
}

func (w *nullableWriteInventory) effects(sc *scope) map[string]bool {
	if w.calls {
		// A callback may be aliased, stored or invoked by another function. Until
		// callees have replacement-effect summaries, calls invalidate selected
		// writable captures, but never unrelated mutable or immutable bindings.
		maps.Copy(w.writes, capturedNullableWrites(sc))
		for name := range w.captures {
			if value, ok := sc.lookup(name); ok && value.mutable {
				w.writes[name] = true
			}
		}
	}
	return w.writes
}
