package repl

import (
	"iter"
	"slices"

	"github.com/type-rb/type-rb/internal/types"
)

type hashEntry struct{ Key, Value Value }

// Hash keys are checked portable scalar values. Keep their lookup index separate
// from compact, ordered slots so interactive rendering is deterministic without
// copying full compiler type metadata into every key and value.
type hashValue struct {
	index map[any]int
	slots []hashSlot
	types []*types.Type
}

type hashSlot struct {
	key, value         any
	keyType, valueType *types.Type
}

func (h *hashValue) size() int { return len(h.index) }

func sameHashType(a, b types.Type) bool {
	return a.Kind == b.Kind && a.Name == b.Name && a.Nullable == b.Nullable &&
		a.Readonly == b.Readonly && a.Declaration == b.Declaration &&
		slices.EqualFunc(a.Args, b.Args, sameHashType)
}

func (h *hashValue) internType(typ types.Type) *types.Type {
	for _, cached := range h.types {
		if sameHashType(*cached, typ) {
			return cached
		}
	}
	stored := typ
	h.types = append(h.types, &stored)
	return &stored
}

func (h *hashValue) set(key, value Value) {
	if position, ok := h.index[key.Data]; ok {
		h.slots[position].value = value.Data
		h.slots[position].valueType = h.internType(value.Type)
		return
	}
	if h.index == nil {
		h.index = make(map[any]int)
	}
	h.index[key.Data] = len(h.slots)
	h.slots = append(h.slots, hashSlot{
		key: key.Data, value: value.Data,
		keyType: h.internType(key.Type), valueType: h.internType(value.Type),
	})
}

func (h *hashValue) get(key Value) (Value, bool) {
	position, ok := h.index[key.Data]
	if !ok {
		return Value{}, false
	}
	slot := h.slots[position]
	return Value{Type: *slot.valueType, Data: slot.value}, true
}

func (h *hashValue) remove(key Value) (Value, bool) {
	value, ok := h.get(key)
	if !ok {
		return Value{}, false
	}
	position := h.index[key.Data]
	delete(h.index, key.Data)
	// Clear both payloads immediately, including slots outside the live prefix.
	h.slots[position] = hashSlot{}
	if h.size() == 0 {
		*h = hashValue{}
	} else if len(h.slots) > 32 && h.size() < len(h.slots)/2 {
		// Rebuild both stores after substantial deletion. This also releases the
		// map's high-water capacity, with amortized constant work per deletion.
		slots := make([]hashSlot, 0, h.size())
		index := make(map[any]int, h.size())
		for _, slot := range h.slots {
			if slot.valueType != nil {
				index[slot.key] = len(slots)
				slots = append(slots, slot)
			}
		}
		h.slots, h.index = slots, index
	}
	return value, true
}

// entries is for read-only traversal. Language iteration uses snapshot instead,
// because its block can insert, replace, delete, or compact the same Hash.
func (h *hashValue) entries() iter.Seq[hashEntry] {
	return func(yield func(hashEntry) bool) {
		for _, slot := range h.slots {
			if slot.valueType != nil && !yield(hashEntry{
				Key:   Value{Type: *slot.keyType, Data: slot.key},
				Value: Value{Type: *slot.valueType, Data: slot.value},
			}) {
				return
			}
		}
	}
}

func (h *hashValue) snapshot() []hashEntry {
	entries := make([]hashEntry, 0, h.size())
	for entry := range h.entries() {
		entries = append(entries, entry)
	}
	return entries
}

func (h *hashValue) copy() *hashValue {
	result := &hashValue{}
	for entry := range h.entries() {
		result.set(entry.Key, entry.Value)
	}
	return result
}
