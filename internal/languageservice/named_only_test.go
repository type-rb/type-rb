package languageservice

import (
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

func TestNamedOnlyBoundaryAppearsInSourceSignatures(t *testing.T) {
	method := &ir.Method{
		Name: "initialize",
		Parameters: []ir.Parameter{
			{Name: "path", Type: types.FromName("String")},
			{Name: "timeout", Type: types.FromName("Integer"), NamedOnly: true, Default: &ir.Literal{}},
		},
	}
	if got := constructorSignature(method, "Client"); got != "new(path: String, *, timeout: Integer): Client" {
		t.Fatalf("constructor signature lost named-only boundary: %q", got)
	}
	method.Name = "request"
	method.ReturnType = types.FromName("String")
	if got := methodSignature(method); got != "request(path: String, *, timeout: Integer): String" {
		t.Fatalf("method signature lost named-only boundary: %q", got)
	}
	call := methodCallInfo(method)
	if len(call.Parameters) != 2 || !call.Parameters[1].NamedOnly || !call.Parameters[1].Optional {
		t.Fatalf("method call metadata lost named-only presence: %#v", call.Parameters)
	}
}
