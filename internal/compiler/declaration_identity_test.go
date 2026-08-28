package compiler

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/ir"
)

func TestModuleCallKeepsDispatchIdentityWhenTypeAliasUsesSameName(t *testing.T) {
	artifact, err := CompileWithOptions("module_alias_identity_collision.trb", []byte(`module Services
	def self.values(input: Array<Integer>): Array<Integer>
		return input.concurrent_map do |value|
			value
		end
	end
end

class Worker
end

alias Services = Worker

def main()
	puts(Services.values([6])[0])
	return
end
`), Options{Mode: "ruby", ModulePath: "main"})
	if err != nil {
		t.Fatal(err)
	}

	var main *ir.Method
	for _, statement := range artifact.IR.Statements {
		method, ok := statement.(*ir.Method)
		if ok && method.Name == "main" {
			main = method
			break
		}
	}
	if main == nil || len(main.Body) == 0 {
		t.Fatalf("lowered main method is missing: %#v", artifact.IR.Statements)
	}
	expression, ok := main.Body[0].(*ir.ExpressionStatement)
	if !ok {
		t.Fatalf("main first statement is %T, want expression", main.Body[0])
	}
	putsCall, ok := expression.Expression.(*ir.Call)
	if !ok || len(putsCall.Arguments) != 1 {
		t.Fatalf("lowered puts call is %#v", expression.Expression)
	}
	index, ok := putsCall.Arguments[0].Value.(*ir.Index)
	if !ok {
		t.Fatalf("puts argument is %T, want index", putsCall.Arguments[0].Value)
	}
	call, ok := index.Receiver.(*ir.Call)
	if !ok {
		t.Fatalf("indexed receiver is %T, want call", index.Receiver)
	}
	member, ok := call.Callee.(*ir.Member)
	if !ok {
		t.Fatalf("callee is %T, want member", call.Callee)
	}
	want := identity.Dispatch{
		Owner: identity.Declaration{Module: "main", Name: "Services", Kind: identity.Module},
		Name:  "values",
		Class: true,
	}
	if member.Dispatch != want {
		t.Fatalf("module call dispatch = %#v, want %#v; receiver = %#v", member.Dispatch, want, member.Receiver)
	}
}

func TestImportedAliasImplementationKeepsSemanticTargetIdentity(t *testing.T) {
	artifacts, err := CompileProject([]SourceUnit{
		{
			Filename: "contracts/worker.trb", ModulePath: "contracts/worker", Package: "contracts",
			Source: []byte(`interface Worker
	values(input: Array<Integer>): Array<Integer>
end

alias WorkerAlias = Worker
`),
		},
		{
			Filename: "main.trb", ModulePath: "main", Package: "main",
			Source: []byte(`import { WorkerAlias } from contracts/worker

module Services
	interface Worker
		local_value(): Integer
	end

	alias LocalAlias = WorkerAlias

	class EffectWorker implements LocalAlias
		def values(input: Array<Integer>): Array<Integer>
			return input.concurrent_map do |value|
				value
			end
		end
	end
end

def main()
	return
end
`),
		},
	}, Options{Mode: "go", GoModule: "example.com/declaration-identity", SourceRoot: "/project", ProjectRoot: "/project"})
	if err != nil {
		t.Fatal(err)
	}
	main := findArtifactByModule(artifacts, "main")
	if main == nil {
		t.Fatal("main artifact is missing")
	}
	var effectWorker *ir.Class
	for _, statement := range main.IR.Statements {
		module, ok := statement.(*ir.Module)
		if !ok || module.Name != "Services" {
			continue
		}
		for _, nested := range module.Body {
			class, ok := nested.(*ir.Class)
			if ok && class.Name == "EffectWorker" {
				effectWorker = class
			}
		}
	}
	if effectWorker == nil || len(effectWorker.ResolvedImplementReferences) != 1 || effectWorker.ResolvedImplementReferences[0] == nil {
		t.Fatalf("resolved interface target is missing: %#v", effectWorker)
	}
	want := identity.Declaration{Module: "contracts/worker", Name: "Worker", Kind: identity.Interface}
	if got := effectWorker.ResolvedImplementReferences[0].Declaration; got != want {
		t.Fatalf("resolved interface identity = %#v, want %#v", got, want)
	}
}

func TestTypeIdentityDistinguishesNamedImportFromNestedLeaf(t *testing.T) {
	artifacts, err := CompileProject([]SourceUnit{
		{
			Filename: "models/box.trb", ModulePath: "models/box", Package: "models",
			Source: []byte(`record Box
	remote: String
end
`),
		},
		{
			Filename: "main.trb", ModulePath: "main", Package: "main",
			Source: []byte(`import { Box } from models/box

module Services
	record Box
		local: String
	end
end

def read(box: Box): String
	return box.remote
end

def main()
	locals := [Services::Box.new(local: "local")]
	puts(read(Box.new(remote: "remote")))
	puts(locals[0].local)
	return
end
`),
		},
	}, Options{Mode: "typescript", SourceRoot: "/project", ProjectRoot: "/project"})
	if err != nil {
		t.Fatal(err)
	}
	main := findArtifactByModule(artifacts, "main")
	if main == nil {
		t.Fatal("main artifact is missing")
	}

	var read *ir.Method
	var locals *ir.Variable
	for _, statement := range main.IR.Statements {
		method, ok := statement.(*ir.Method)
		if !ok {
			continue
		}
		switch method.Name {
		case "read":
			read = method
		case "main":
			for _, body := range method.Body {
				variable, ok := body.(*ir.Variable)
				if ok && variable.Name == "locals" {
					locals = variable
				}
			}
		}
	}
	if read == nil || len(read.Parameters) != 1 {
		t.Fatalf("lowered read signature is missing: %#v", read)
	}
	wantImported := identity.Declaration{Module: "models/box", Name: "Box", Kind: identity.Record}
	if got := read.Parameters[0].Type.Declaration; got != wantImported {
		t.Fatalf("named import parameter identity = %#v, want %#v", got, wantImported)
	}
	if locals == nil || len(locals.Type.Args) != 1 {
		t.Fatalf("lowered locals binding is missing: %#v", locals)
	}
	wantNested := identity.Declaration{Module: "main", Name: "Services::Box", Kind: identity.Record}
	if got := locals.Type.Args[0].Declaration; got != wantNested {
		t.Fatalf("nested collection element identity = %#v, want %#v", got, wantNested)
	}
	output := string(main.Output)
	if !containsAll(output, "function read(box: Box): string", "const locals: Array<Services.Box>") {
		t.Fatalf("TypeScript did not render distinct semantic identities:\n%s", output)
	}
}

func TestDispatchIdentitySeparatesClassAndInstanceMethods(t *testing.T) {
	artifact, err := CompileWithOptions("dispatch_identity.trb", []byte(`class Base
	def self.values(input: Array<Integer>): Array<Integer>
		return input
	end
end

class Child < Base
	def values(input: Array<Integer>): Array<Integer>
		return input
	end
end

def main()
	Base.values([1])
	Child.new().values([2])
	return
end
`), Options{Mode: "go", ModulePath: "main", Package: "main"})
	if err != nil {
		t.Fatal(err)
	}
	classes := map[string]*ir.Class{}
	for _, statement := range artifact.IR.Statements {
		if class, ok := statement.(*ir.Class); ok {
			classes[class.Name] = class
		}
	}
	seen := map[bool]bool{}
	for name, classMember := range map[string]bool{"Base": true, "Child": false} {
		class := classes[name]
		if class == nil {
			t.Fatalf("%s class is missing", name)
		}
		for _, statement := range class.Body {
			method, ok := statement.(*ir.Method)
			if !ok || method.Name != "values" {
				continue
			}
			wantOwner := identity.Declaration{Module: "main", Name: name, Kind: identity.Class}
			want := identity.Dispatch{Owner: wantOwner, Name: "values", Class: classMember}
			if method.Dispatch != want {
				t.Fatalf("method dispatch = %#v, want %#v", method.Dispatch, want)
			}
			seen[method.Class] = true
		}
	}
	if !seen[false] || !seen[true] {
		t.Fatalf("class/instance dispatch declarations are incomplete: %#v", seen)
	}
}

func TestImportedGenericReturnKeepsDeclarationIdentityAcrossLeafCollision(t *testing.T) {
	artifacts, err := CompileProject([]SourceUnit{
		{
			Filename: "models/box.trb", ModulePath: "models/box", Package: "models",
			Source: []byte(`record Box<T>
	value: T
end

def wrap<T>(value: T): Box<T>
	return Box<T>.new(value: value)
end
`),
		},
		{
			Filename: "main.trb", ModulePath: "main", Package: "main",
			Source: []byte(`import { wrap } from models/box

module Services
	record Box<T>
		value: T
	end
end

def main()
	remote := wrap<String>("remote")
	puts(remote.value)
	return
end
`),
		},
	}, Options{Mode: "typescript", SourceRoot: "/project", ProjectRoot: "/project"})
	if err != nil {
		t.Fatal(err)
	}
	main := findArtifactByModule(artifacts, "main")
	if main == nil {
		t.Fatal("main artifact is missing")
	}
	var remote *ir.Variable
	for _, statement := range main.IR.Statements {
		method, ok := statement.(*ir.Method)
		if !ok || method.Name != "main" {
			continue
		}
		for _, body := range method.Body {
			variable, ok := body.(*ir.Variable)
			if ok && variable.Name == "remote" {
				remote = variable
			}
		}
	}
	want := identity.Declaration{Module: "models/box", Name: "Box", Kind: identity.Record}
	if remote == nil || remote.Type.Declaration != want {
		t.Fatalf("generic imported return identity = %#v, want %#v", remote, want)
	}
	if output := string(main.Output); !strings.Contains(output, "const remote: Box<string>") {
		t.Fatalf("TypeScript did not retain the imported generic return type:\n%s", output)
	}
}

func containsAll(value string, expected ...string) bool {
	for _, item := range expected {
		if !strings.Contains(value, item) {
			return false
		}
	}
	return true
}
