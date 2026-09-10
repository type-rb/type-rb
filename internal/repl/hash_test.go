package repl

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/type-rb/type-rb/internal/types"
)

func hashInteger(n int64) Value { return Value{Type: types.FromName("Integer"), Data: n} }
func hashString(s string) Value { return Value{Type: types.FromName("String"), Data: s} }

func TestHashIndexedMutation(t *testing.T) {
	h := &hashValue{}
	keys := []Value{hashInteger(-9007199254740991), hashInteger(9007199254740991), hashString(""), hashString("日本語\x00key")}
	for i, key := range keys {
		h.set(key, hashInteger(int64(i)))
	}
	for i, key := range keys {
		got, ok := h.get(key)
		if !ok || got.Data != int64(i) {
			t.Fatalf("get(%v) = %v, %t", key.Data, got, ok)
		}
	}
	h.set(keys[0], hashString("replacement"))
	if got, _ := h.get(keys[0]); got.Data != "replacement" || got.Type.Kind != types.String || h.size() != 4 {
		t.Fatalf("replacement = %#v; size = %d", got, h.size())
	}
	if _, ok := h.get(hashInteger(7)); ok {
		t.Fatal("missing key was found")
	}
	for _, key := range keys {
		if _, ok := h.remove(key); !ok {
			t.Fatalf("failed to delete %v", key.Data)
		}
		if _, ok := h.get(key); ok {
			t.Fatalf("deleted key %v remains", key.Data)
		}
	}
	if h.index != nil || h.slots != nil || h.types != nil {
		t.Fatal("empty Hash retained storage")
	}
	h.set(hashInteger(9), hashString("reused"))
	if got, _ := h.get(hashInteger(9)); got.Data != "reused" {
		t.Fatal(got)
	}
}

func TestHashDeleteReleasesPayloadAndCompacts(t *testing.T) {
	h := &hashValue{}
	for i := int64(0); i < 1000; i++ {
		h.set(hashInteger(i), hashString(fmt.Sprintf("value-%d", i)))
	}
	backing := h.slots
	h.remove(hashInteger(0))
	if backing[0] != (hashSlot{}) {
		t.Fatal("deleted slot retained a key, payload, or type")
	}
	for i := int64(1); i < 990; i++ {
		h.remove(hashInteger(i))
	}
	if h.size() != 10 || cap(h.slots) > 32 {
		t.Fatalf("size=%d capacity=%d", h.size(), cap(h.slots))
	}
	for i := int64(990); i < 1000; i++ {
		got, ok := h.get(hashInteger(i))
		if !ok || got.Data != fmt.Sprintf("value-%d", i) {
			t.Fatalf("compaction lost %d", i)
		}
	}
	// Repeated insertion/deletion with one survivor must not retain a growing
	// history of tombstones or obsolete map capacity.
	for i := int64(1000); i < 10000; i++ {
		h.set(hashInteger(i), hashInteger(i))
		h.remove(hashInteger(i))
	}
	if len(h.slots) > 32 || len(h.types) != 2 {
		t.Fatalf("unbounded storage: slots=%d types=%d", len(h.slots), len(h.types))
	}
}

func TestHashSnapshotCopyAndTypes(t *testing.T) {
	h := &hashValue{}
	readonly := hashInteger(1)
	readonly.Type.Readonly = true
	h.set(hashString("first"), readonly)
	h.set(hashString("second"), hashInteger(2))
	snapshot, copy := h.snapshot(), h.copy()
	h.remove(hashString("first"))
	h.set(hashString("second"), hashInteger(20))
	h.set(hashString("third"), hashInteger(3))
	if len(snapshot) != 2 || snapshot[0].Value.Data != int64(1) || snapshot[1].Value.Data != int64(2) {
		t.Fatal("mutation changed the iteration snapshot")
	}
	if got, ok := copy.get(hashString("first")); !ok || !got.Type.Readonly {
		t.Fatalf("copy lost type metadata: %#v", got)
	}
	if got, _ := copy.get(hashString("second")); got.Data != int64(2) {
		t.Fatal("copy shares mutable slots")
	}
	var names []any
	for entry := range h.entries() {
		names = append(names, entry.Key.Data)
	}
	if !reflect.DeepEqual(names, []any{"second", "third"}) {
		t.Fatal(names)
	}
}

func BenchmarkHashInsert(b *testing.B) {
	for _, count := range []int{1000, 10000, 100000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				h := &hashValue{}
				for i := 0; i < count; i++ {
					h.set(hashInteger(int64(i)), hashInteger(int64(i)))
				}
				if h.size() != count {
					b.Fatal(h.size())
				}
			}
		})
	}
}
