package resolver

import (
	"testing"

	"github.com/type-rb/type-rb/internal/callsignature"
	"github.com/type-rb/type-rb/internal/parser"
)

func TestCollectExportsPreservesNamedOnlyCallSignatures(t *testing.T) {
	program, diagnostics := parser.Parse([]byte(`def request(url: String, *, timeout: Integer, retries: Integer = 2): String
	return url
end
interface Client
	request(*, host: String, token: String): String
end
enum Change
	Renamed(id: Integer, *, before: String, after: String)
end
`))
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	exports := CollectExports(program.Statements)
	request := exports["request"]
	if len(request.Parameters) != 3 {
		t.Fatalf("unexpected request signature: %#v", request.Parameters)
	}
	if request.Parameters[0].Kind != callsignature.Positional || request.Parameters[0].Label != "" || request.Parameters[0].Presence != callsignature.Required {
		t.Fatalf("positional source name leaked into the call signature: %#v", request.Parameters[0])
	}
	if request.Parameters[1].Kind != callsignature.NamedOnly || request.Parameters[1].Label != "timeout" || request.Parameters[1].Presence != callsignature.Required {
		t.Fatalf("required named-only contract was lost: %#v", request.Parameters[1])
	}
	if request.Parameters[2].Label != "retries" || request.Parameters[2].Presence != callsignature.Omittable {
		t.Fatalf("omittable named-only contract was lost: %#v", request.Parameters[2])
	}
	member := exports["Client"].Members["request"]
	if len(member.Parameters) != 2 || member.Parameters[0].Label != "host" || member.Parameters[1].Label != "token" {
		t.Fatalf("interface labels were not exported: %#v", member.Parameters)
	}
	change := exports["Change"]
	if len(change.EnumVariants) != 1 || len(change.EnumVariants[0].Fields) != 3 || change.EnumVariants[0].Fields[0].NamedOnly || !change.EnumVariants[0].Fields[1].NamedOnly || !change.EnumVariants[0].Fields[2].NamedOnly {
		t.Fatalf("enum payload parameter regions were not exported: %#v", change.EnumVariants)
	}
	renamed := change.Members["Renamed"]
	if len(renamed.Parameters) != 3 || renamed.Parameters[0].Kind != callsignature.Positional || renamed.Parameters[1].Kind != callsignature.NamedOnly || renamed.Parameters[1].Label != "before" {
		t.Fatalf("enum member call signature was not exported: %#v", renamed.Parameters)
	}
}
