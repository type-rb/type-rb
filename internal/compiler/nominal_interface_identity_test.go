package compiler

import "testing"

func TestNominalInterfaceApplicationsProduceValidTypeScript(t *testing.T) {
	artifacts, err := CompileProject([]SourceUnit{
		{Filename: "/project/contracts.trb", ModulePath: "contracts", Source: []byte(`interface Factory<T>
read(): T
end
`)},
		{Filename: "/project/models.trb", ModulePath: "models", Source: []byte(`import { Factory as Contract } from contracts
alias Source<T> = Contract<T>
class Holder<T> implements Source<T>
@_value: T
def initialize(value: T)
@_value = value
end
def read(): T
return @_value
end
end
`)},
		{Filename: "/project/main.trb", ModulePath: "main", Source: []byte(`import { Factory as Reader } from contracts
import { Holder as Stored } from models
class Factory
end
class Holder
end
def stored(value: Stored<String>): Stored<String>
return value
end
def describe(value: Reader<String>): String
return value.read()
end
def main()
reader: Reader<Integer> := Stored<Integer>.new(9)
puts(reader.read())
puts(describe(stored(Stored<String>.new("kept"))))
end
`)},
	}, Options{Mode: "typescript", SourceRoot: "/project", ProjectRoot: "/project"})
	if err != nil {
		t.Fatal(err)
	}
	checkTypeScriptArtifacts(t, artifacts, "nominal_interfaces")
}
