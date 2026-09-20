package stdlib

import (
	"reflect"
	"testing"

	"github.com/type-rb/type-rb/internal/types"
)

func TestLibraryInstantiationRetainsOnlyUnboundParameters(t *testing.T) {
	outer := types.FromName("T")
	symbol := Symbol{
		TypeParameters: []string{"T", "U"},
		Parameters:     []Parameter{{Name: "value", Type: types.FromName("T")}},
		Return:         types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{outer}},
	}
	resolved := Instantiate(symbol, []types.Type{outer})
	if !reflect.DeepEqual(resolved.TypeParameters, []string{"U"}) || resolved.Return.String() != "Array<T>" {
		t.Fatalf("outer T was mistaken for an unbound contract parameter: %#v", resolved)
	}
	if !reflect.DeepEqual(symbol.TypeParameters, []string{"T", "U"}) {
		t.Fatal("instantiation mutated the shared declaration")
	}
	unresolved := Instantiate(symbol, []types.Type{{Kind: types.Invalid}})
	if !reflect.DeepEqual(unresolved.TypeParameters, []string{"T", "U"}) {
		t.Fatalf("invalid input resolved contract parameters: %#v", unresolved.TypeParameters)
	}
}
