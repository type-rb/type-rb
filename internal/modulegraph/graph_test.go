package modulegraph

import (
	"reflect"
	"testing"
)

func TestComponentsPreserveAuthoredTraversalAndCollapseCycles(t *testing.T) {
	graph := map[string][]string{"root": {"z", "a"}, "z": {"shared", "y"}, "y": {"z"}, "a": {"shared", "a"}}
	got := Components([]string{"root", "a", "unused"}, func(name string) []string { return graph[name] })
	want := []Component{{Modules: []string{"shared"}}, {Modules: []string{"z", "y"}, Cyclic: true}, {Modules: []string{"a"}, Cyclic: true}, {Modules: []string{"root"}}, {Modules: []string{"unused"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v; want %#v", got, want)
	}
}
