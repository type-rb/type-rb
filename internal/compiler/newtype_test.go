package compiler

import (
	"strings"
	"testing"
)

func TestNewtypesAreNominalAcrossBackends(t *testing.T) {
	source := []byte(`newtype UserId = Integer
newtype ProductIds = Array<UserId>

def same_user(left: UserId, right: UserId): Boolean
	return left == right
end

def build_ids(): ProductIds
	id := UserId.new(7)
	_raw := id.value()
	_same := same_user(id, UserId.new(7))
	return ProductIds.new([id])
end
`)

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifact, err := Compile("newtype.trb", source, mode)
			if err != nil {
				t.Fatalf("%s rejected newtypes: %v", mode, err)
			}
			output := string(artifact.Output)
			switch mode {
			case "go":
				for _, wanted := range []string{"type UserId = int", "type ProductIds = *[]UserId", "return trbArrayReference_"} {
					if !strings.Contains(output, wanted) {
						t.Fatalf("generated Go is missing %q:\n%s", wanted, output)
					}
				}
			case "typescript":
				for _, wanted := range []string{"export type UserId = number;", "export type ProductIds = Array<UserId>;", "return [id];"} {
					if !strings.Contains(output, wanted) {
						t.Fatalf("generated TypeScript is missing %q:\n%s", wanted, output)
					}
				}
			case "ruby":
				if strings.Contains(output, "UserId.new") || strings.Contains(output, ".value") {
					t.Fatalf("Ruby did not erase newtype conversions:\n%s", output)
				}
			}
		})
	}
}

func TestNewtypeNominalBoundariesAreChecked(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		message string
	}{
		{
			name:    "base value assignment",
			source:  "newtype UserId = Integer\ndef bad(): UserId\n\treturn 1\nend\n",
			message: "return type is Integer, expected UserId",
		},
		{
			name:    "newtype passed as base",
			source:  "newtype UserId = Integer\ndef takes(value: Integer)\n\treturn\nend\ndef bad(id: UserId)\n\ttakes(id)\n\treturn\nend\n",
			message: "argument 1 to takes() has type UserId, expected Integer",
		},
		{
			name:    "different newtypes",
			source:  "newtype UserId = Integer\nnewtype OrderId = Integer\ndef bad(user: UserId, order: OrderId): Boolean\n\treturn user == order\nend\n",
			message: "operator == does not support UserId and OrderId",
		},
		{
			name:    "nullable representation",
			source:  "newtype MaybeUserId = Integer?\n",
			message: "newtype MaybeUserId representation must be non-nullable",
		},
		{
			name:    "generic declaration",
			source:  "newtype Id<T> = T\n",
			message: "generic newtype declarations are not supported yet",
		},
		{
			name:    "invalid construction",
			source:  "newtype UserId = Integer\ndef bad(): UserId\n\treturn UserId.new(\"1\")\nend\n",
			message: "argument 1 to UserId.new() has type String, expected Integer",
		},
		{
			name:    "any representation",
			source:  "newtype Opaque = Any\n",
			message: "newtype Opaque requires a concrete value representation",
		},
		{
			name:    "void representation",
			source:  "newtype Nothing = Void\n",
			message: "newtype Nothing requires a concrete value representation",
		},
		{
			name:    "alias keyword as parameter",
			source:  "def bad(alias: Integer)\n\treturn\nend\n",
			message: "alias is a reserved keyword and cannot be used as a parameter name",
		},
		{
			name:    "newtype keyword as parameter",
			source:  "def bad(newtype: Integer)\n\treturn\nend\n",
			message: "newtype is a reserved keyword and cannot be used as a parameter name",
		},
		{
			name:    "uninstantiated representation",
			source:  "newtype Values = Array\n",
			message: "newtype Values representation must be fully instantiated",
		},
		{
			name:    "direct representation cycle",
			source:  "newtype Node = Node\n",
			message: "newtype representation cycle involving Node",
		},
		{
			name:    "nested representation cycle",
			source:  "newtype Nodes = Array<Node>\nnewtype Node = Nodes\n",
			message: "newtype representation cycle involving",
		},
		{
			name:    "representation member forwarding",
			source:  "newtype UserName = String\ndef bad(name: UserName): Integer\n\treturn name.size()\nend\n",
			message: "newtype UserName has no instance member size",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Compile("invalid_newtype.trb", []byte(test.source), "go")
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("error = %v, want %q", err, test.message)
			}
		})
	}
}

func TestImportedNewtypesPreserveNominalChecksAcrossBackends(t *testing.T) {
	contracts := SourceUnit{
		Filename: "/project/contracts/index.trb", ModulePath: "contracts/index", Package: "contracts",
		Source: []byte(`newtype UserId = Integer

record User
	id: UserId
end
`),
	}
	main := SourceUnit{
		Filename: "/project/main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import { User, UserId } from contracts

def build_user(): User
	id := UserId.new(7)
	_raw := id.value()
	return User.new(id: id)
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			_, err := CompileProject([]SourceUnit{contracts, main}, Options{
				Mode: mode, GoModule: "example.com/newtypes", RubyLoader: "require_relative", ProjectRoot: "/project",
			})
			if err != nil {
				t.Fatalf("%s rejected an imported newtype: %v", mode, err)
			}
		})
	}
}

func TestImportedNewtypeConversionsDoNotLeaveUnusedGoImports(t *testing.T) {
	contracts := SourceUnit{
		Filename: "/project/contracts/index.trb", ModulePath: "contracts/index", Package: "contracts",
		Source: []byte("newtype UserId = Integer\n"),
	}
	main := SourceUnit{
		Filename: "/project/main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import { UserId } from contracts

def build_id(): Integer
	id := UserId.new(7)
	return id.value()
end
`),
	}
	artifacts, err := CompileProject([]SourceUnit{contracts, main}, Options{
		Mode: "go", GoModule: "example.com/newtypes", ProjectRoot: "/project",
	})
	if err != nil {
		t.Fatalf("compile imported newtype: %v", err)
	}
	for _, artifact := range artifacts {
		if artifact.Filename != main.Filename {
			continue
		}
		output := string(artifact.Output)
		if strings.Contains(output, `"example.com/newtypes/contracts"`) {
			t.Fatalf("generated Go retained an import erased with the newtype conversion:\n%s", output)
		}
		return
	}
	t.Fatalf("missing artifact for %s", main.Filename)
}

func TestAliasCanNameANewtypeWithoutLosingNominality(t *testing.T) {
	source := []byte(`newtype UserId = Integer
alias CurrentUserId = UserId

def build(): CurrentUserId
	id := CurrentUserId.new(7)
	_raw := id.value()
	return id
end
`)
	if _, err := Compile("newtype_alias.trb", source, "go"); err != nil {
		t.Fatalf("alias to newtype was rejected: %v", err)
	}
}

func TestTypedJSONUsesNewtypeRepresentationsAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename: "/project/main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import { JSON } from trb/std/json

newtype UserId = Integer
newtype UserIds = Array<UserId>

record Payload
	id: UserId
	ids: UserIds
	parent_id: UserId?
end

def encode_payload(): String
	id := UserId.new(7)
	payload := Payload.new(id: id, ids: UserIds.new([id]), parent_id: nil)
	return JSON.encode(payload) catch |_error|
		return ""
	end
end

def decode_id(source: String): UserId?
	payload := JSON.decode<Payload>(source) catch |_error|
		return nil
	end
	return payload.id
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			if _, err := CompileProject([]SourceUnit{source}, Options{
				Mode: mode, GoModule: "example.com/newtype-json", RubyLoader: "require_relative", ProjectRoot: "/project",
			}); err != nil {
				t.Fatalf("%s rejected newtypes in typed JSON: %v", mode, err)
			}
		})
	}
}
