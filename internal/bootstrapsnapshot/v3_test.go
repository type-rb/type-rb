package bootstrapsnapshot

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
)

const v3Program = `import { Result } from trb/std/result

record Point
	x: Integer
	y: Integer
end

record Box
	point: Point
	weight: Integer
end

enum Outcome
	Ready
	Found(point: Point)
	Missing(code: Integer)
end

def shift(point: Point): Point
	return Point.new(x: point.x + 1, y: point.y + 2)
end

def inspect(outcome: Outcome): Integer
	case outcome
	when Outcome::Ready
		return 0
	when Outcome::Found(point)
		return point.x + point.y
	when Outcome::Missing(code)
		return code
	end
end

def source(success: Boolean): Result<Integer, Integer>
	if success
		return Result<Integer, Integer>::Ok(7)
	end
	return Result<Integer, Integer>::Err(41)
end

def propagated(success: Boolean): Result<Integer, Integer>
	value := try source(success)
	return Result<Integer, Integer>::Ok(value + 1)
end

def main()
	box := Box.new(point: shift(Point.new(x: 3, y: 4)), weight: 5)
	if inspect(Outcome::Ready) == 0
		if inspect(Outcome::Found(box.point)) == 10
			case propagated(true)
			when Result::Ok(value)
				if value == 8
					puts("ok")
					return
				end
			when Result::Err(_error)
				puts("bad")
				return
			end
		end
	end
	puts("bad")
	return
end
`

func TestBuildV3LowersStaticAggregatesAndResultPropagation(t *testing.T) {
	artifacts, err := compiler.AnalyzeProject(
		[]compiler.SourceUnit{{Filename: "/project/src/main.trb", ModulePath: "main", Package: "main", Source: []byte(v3Program)}},
		compiler.Options{Mode: "go", GoModule: "example.com/bootstrap-snapshot", SourceRoot: "/project/src", ProjectRoot: "/project"},
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := BuildV3(artifacts, "/project/src")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Format != Format || snapshot.Version != Version3 || snapshot.Module != "main" || snapshot.EntryFunction != "main#main" {
		t.Fatalf("unexpected snapshot envelope: %#v", snapshot)
	}
	if len(snapshot.Sources) != 1 || snapshot.Sources[0].Path != "main.trb" || len(snapshot.Functions) != 5 {
		t.Fatalf("unexpected snapshot inputs: %#v", snapshot)
	}
	if len(snapshot.Types) != 4 {
		t.Fatalf("expected Point, Box, Outcome, and Result definitions: %#v", snapshot.Types)
	}
	resultID := "trb/std/result/index#Result<Integer,Integer>"
	if !v3HasTypeDefinition(snapshot.Types, resultID) {
		t.Fatalf("snapshot is missing concrete Result type %q: %#v", resultID, snapshot.Types)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := BuildV3(artifacts, "/project/src")
	if err != nil {
		t.Fatal(err)
	}
	repeatedEncoded, err := json.Marshal(repeated)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != string(repeatedEncoded) {
		t.Fatalf("bootstrap snapshot v3 output is not deterministic:\nfirst:  %s\nsecond: %s", encoded, repeatedEncoded)
	}
	text := string(encoded)
	if strings.Contains(text, `"startLine":0`) || strings.Contains(text, `"startColumn":0`) || strings.Contains(text, `"endLine":0`) || strings.Contains(text, `"endColumn":0`) {
		t.Fatalf("snapshot contains a non-positive compiler-generated origin:\n%s", text)
	}
	for _, expected := range []string{
		`"kind":"record","id":"main#Box"`,
		`"kind":"tagged","id":"main#Outcome"`,
		`"op":"record_construct"`,
		`"op":"record_project"`,
		`"op":"variant_construct"`,
		`"op":"variant_test"`,
		`"op":"variant_project"`,
		`"op":"call"`,
		`"op":"write_static","value":"ok\n"`,
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("snapshot is missing %s:\n%s", expected, text)
		}
	}
}

func TestBuildV3RejectsDynamicRecordStorage(t *testing.T) {
	source := []byte("record Message\n\tvalue: String\nend\n\ndef main()\n\tMessage.new(value: \"dynamic\")\n\treturn\nend\n")
	artifacts, err := compiler.AnalyzeProject(
		[]compiler.SourceUnit{{Filename: "/project/src/main.trb", ModulePath: "main", Package: "main", Source: source}},
		compiler.Options{Mode: "go", GoModule: "example.com/bootstrap-snapshot", SourceRoot: "/project/src", ProjectRoot: "/project"},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = BuildV3(artifacts, "/project/src")
	if err == nil || !strings.Contains(err.Error(), "bootstrap snapshot v3 does not support value type String") {
		t.Fatalf("BuildV3() error=%v", err)
	}
}

func TestV3TypeDefinitionsKeepRequiredEmptyArrays(t *testing.T) {
	for name, definition := range map[string]TypeDefinition{
		"record": v3RecordDefinitionForTest("main#Empty"),
		"tagged": v3TaggedDefinitionForTest("main#Never"),
	} {
		t.Run(name, func(t *testing.T) {
			encoded, err := json.Marshal(definition)
			if err != nil {
				t.Fatal(err)
			}
			required := `"fields":[]`
			if name == "tagged" {
				required = `"variants":[]`
			}
			if !strings.Contains(string(encoded), required) {
				t.Fatalf("required empty array is missing: %s", encoded)
			}
		})
	}
}

func v3RecordDefinitionForTest(id string) TypeDefinition {
	fields := []Field{}
	return TypeDefinition{Kind: "record", ID: id, Fields: &fields}
}

func v3TaggedDefinitionForTest(id string) TypeDefinition {
	variants := []Variant{}
	return TypeDefinition{Kind: "tagged", ID: id, Variants: &variants}
}

func v3HasTypeDefinition(definitions []TypeDefinition, id string) bool {
	for _, definition := range definitions {
		if definition.ID == id {
			return true
		}
	}
	return false
}
