package compiler

import (
	"testing"

	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/ir"
)

func TestRecordConstructionLowersToSemanticIR(t *testing.T) {
	artifact, err := CompileWithOptions("record_construct_ir.trb", []byte(`module Services
	record Box<T>
		value: T
		label: String = "default"
	end
end

def main()
	box := Services::Box<Integer>.new(label: "chosen", value: 7)
	puts(box.value.to_s())
	return
end
`), Options{Mode: "typescript", ModulePath: "main"})
	if err != nil {
		t.Fatal(err)
	}

	construction := recordConstructionInMain(t, artifact)
	wantDeclaration := identity.Declaration{Module: "main", Name: "Services::Box", Kind: identity.Record}
	if construction.Declaration != wantDeclaration {
		t.Fatalf("record declaration = %#v, want %#v", construction.Declaration, wantDeclaration)
	}
	if len(construction.TypeArguments) != 1 || construction.TypeArguments[0].Name != "Integer" {
		t.Fatalf("record type arguments = %#v, want Integer", construction.TypeArguments)
	}
	target, ok := construction.Target.(*ir.Member)
	if !ok || !target.Namespace || target.Name != "Box" {
		t.Fatalf("record target = %#v, want qualified Services::Box member", construction.Target)
	}
	if len(construction.Fields) != 2 || construction.Fields[0].Name != "value" || construction.Fields[0].HasDefault || construction.Fields[1].Name != "label" || !construction.Fields[1].HasDefault {
		t.Fatalf("record field contract = %#v", construction.Fields)
	}
	if len(construction.Arguments) != 2 || construction.Arguments[0].Name != "label" || construction.Arguments[1].Name != "value" {
		t.Fatalf("record arguments lost authored order: %#v", construction.Arguments)
	}
}

func TestImportedRecordConstructionKeepsDeclarationAndProjectionSeparate(t *testing.T) {
	artifacts, err := CompileProject([]SourceUnit{
		{
			Filename: "models/box.trb", ModulePath: "models/box", Package: "models",
			Source: []byte(`record Box<T>
	value: T
end
`),
		},
		{
			Filename: "main.trb", ModulePath: "main", Package: "main",
			Source: []byte(`import { Box as ModelBox } from models/box

def main()
	box := ModelBox<String>.new(value: "ok")
	puts(box.value)
	return
end
`),
		},
	}, Options{Mode: "go", GoModule: "example.com/record-construct", SourceRoot: "/project", ProjectRoot: "/project"})
	if err != nil {
		t.Fatal(err)
	}
	main := findArtifactByModule(artifacts, "main")
	if main == nil {
		t.Fatal("main artifact is missing")
	}
	construction := recordConstructionInMain(t, main)
	wantDeclaration := identity.Declaration{Module: "models/box", Name: "Box", Kind: identity.Record}
	if construction.Declaration != wantDeclaration {
		t.Fatalf("imported record declaration = %#v, want %#v", construction.Declaration, wantDeclaration)
	}
	target, ok := construction.Target.(*ir.Identifier)
	if !ok || target.Name != "ModelBox" || target.Reference == nil || target.Reference.Package != "models/box" || target.Reference.Symbol != "Box" {
		t.Fatalf("imported record target projection = %#v", construction.Target)
	}
}

func recordConstructionInMain(t *testing.T, artifact *Artifact) *ir.RecordConstruct {
	t.Helper()
	for _, statement := range artifact.IR.Statements {
		method, ok := statement.(*ir.Method)
		if !ok || method.Name != "main" {
			continue
		}
		for _, body := range method.Body {
			variable, ok := body.(*ir.Variable)
			if !ok || variable.Name != "box" {
				continue
			}
			construction, ok := variable.Value.(*ir.RecordConstruct)
			if !ok {
				t.Fatalf("box value is %T, want *ir.RecordConstruct", variable.Value)
			}
			return construction
		}
	}
	t.Fatal("main box construction is missing")
	return nil
}
