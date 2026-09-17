package compiler

import (
	"strings"
	"testing"
)

func TestGenericIdentityAliasExecutionAcrossModes(t *testing.T) {
	source := []byte(`alias Identity<T> = T
alias Second<A, B> = B
alias Wrapped<T> = Identity<T>
alias Values<T> = Array<Wrapped<T>>
alias Maybe<T> = Identity<T>?
alias Callback = (Integer) -> Integer
record Packet
 value: String
end
def keep<T>(value: Wrapped<T>): Identity<T>
 return value
end
def select<A, B>(first: A, second: Second<A, B>): Second<A, B>
 return second
end
def main()
 values: Values<String> := ["held"]
 puts(keep<String>(values[0]))
 puts(select<Integer, String>(1, "second"))
 packet := keep<Packet>(Packet.new(value: "packet"))
 puts(packet.value)
 absent: Maybe<Integer> := nil
 puts(absent == nil)
 callback: Identity<Callback> := fn(value: Integer): Integer
  return value + 1
 end
 puts(callback(6))
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "identity_alias.trb", source, "held\nsecond\npacket\ntrue\n7")
		})
	}
}

func TestImportedGenericIdentityAliasExecutionAcrossModes(t *testing.T) {
	units := []SourceUnit{
		{Filename: "models/aliases.trb", ModulePath: "models/aliases", Package: "models", Source: []byte(`alias Identity<T> = T
alias Wrapped<T> = Identity<T>
alias Values<T> = Array<Wrapped<T>>
def keep<T>(value: Wrapped<T>): Identity<T>
 return value
end
`)},
		{Filename: "main.trb", ModulePath: "main", Package: "main", Source: []byte(`import { Identity as Same, Wrapped, Values, keep } from models/aliases
record Packet
 value: String
end
def main()
 values: Values<String> := ["held"]
 value: Same<Wrapped<String>> := keep<String>(values[0])
 puts(value)
 item: Packet := keep<Packet>(Packet.new(value: "local"))
 puts(item.value)
end
`)},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireEffectRuntime(t, mode)
			artifacts, err := CompileProject(units, Options{Mode: mode, GoModule: "example.com/identity-aliases", RubyLoader: "require_relative", SourceRoot: "/project", ProjectRoot: "/project"})
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, "example.com/identity-aliases")); got != "held\nlocal" {
				t.Fatalf("identity alias output: %q", got)
			}
		})
	}
}

func TestGenericIdentityAliasChecksUnderlyingTypeAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		_, err := Compile("identity_alias_invalid.trb", []byte("alias Identity<T> = T\ndef main()\nvalue: Identity<String> := 1\nputs(value)\nend\n"), mode)
		if err == nil || !strings.Contains(err.Error(), "cannot assign Integer to String") {
			t.Fatalf("%s identity alias did not enforce its argument: %v", mode, err)
		}
	}
}
