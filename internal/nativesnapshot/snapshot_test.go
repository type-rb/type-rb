package nativesnapshot

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
)

const gate1Program = `def sum_to(limit: Integer): Integer
	mut index := 0
	mut total := 0
	while index < limit
		index += 1
		total += index
	end
	return total
end

def scaled(value: Float): Float
	return value * 2.0
end

def countdown(mut value: Integer): Integer
	while value > 0
		if value == 1
			value -= 1
		else
			value -= 1
		end
	end
	return value
end

def main()
	answer := sum_to(5)
	if answer == 15
		if scaled(1.5) == 3.0
			if countdown(2) == 0
				puts("ok")
				return
			end
		end
	end
	puts("bad")
	return
end
`

func TestBuildLowersTheGate1ScalarControlFlowSubset(t *testing.T) {
	artifacts, err := compiler.AnalyzeProject(
		[]compiler.SourceUnit{{Filename: "/project/src/main.trb", ModulePath: "main", Package: "main", Source: []byte(gate1Program)}},
		compiler.Options{Mode: "go", GoModule: "example.com/native-snapshot", SourceRoot: "/project/src", ProjectRoot: "/project"},
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := Build(artifacts, "/project/src")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Format != Format || snapshot.Version != Version || snapshot.Module != "main" || snapshot.EntryFunction != "main#main" {
		t.Fatalf("unexpected snapshot envelope: %#v", snapshot)
	}
	if len(snapshot.Sources) != 1 || snapshot.Sources[0].Path != "main.trb" || len(snapshot.Functions) != 4 {
		t.Fatalf("unexpected snapshot inputs: %#v", snapshot)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := Build(artifacts, "/project/src")
	if err != nil {
		t.Fatal(err)
	}
	repeatedEncoded, err := json.Marshal(repeated)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != string(repeatedEncoded) {
		t.Fatalf("native snapshot output is not deterministic:\nfirst:  %s\nsecond: %s", encoded, repeatedEncoded)
	}
	text := string(encoded)
	for _, expected := range []string{
		`"op":"integer_binary"`,
		`"op":"float_binary"`,
		`"op":"call"`,
		`"op":"branch"`,
		`"op":"jump"`,
		`"op":"write_static","value":"ok\n"`,
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("snapshot is missing %s:\n%s", expected, text)
		}
	}
}

func TestBuildRejectsDynamicOutputExplicitly(t *testing.T) {
	source := []byte("def main()\n\tputs(42)\n\treturn\nend\n")
	artifacts, err := compiler.AnalyzeProject(
		[]compiler.SourceUnit{{Filename: "/project/src/main.trb", ModulePath: "main", Package: "main", Source: source}},
		compiler.Options{Mode: "go", GoModule: "example.com/native-snapshot", SourceRoot: "/project/src", ProjectRoot: "/project"},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Build(artifacts, "/project/src")
	if err == nil || !strings.Contains(err.Error(), "native snapshot v2 does not support dynamic puts() output") {
		t.Fatalf("Build() error=%v", err)
	}
}
