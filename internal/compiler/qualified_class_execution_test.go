package compiler

import "testing"

func TestQualifiedClassesProduceValidTypeScript(t *testing.T) {
	artifacts, err := CompileProject([]SourceUnit{
		{Filename: "/project/models.trb", ModulePath: "models", Source: []byte(`module Models
class Answer
VALUE := 42
def identity(): Answer
return self
end
def value(): Integer
return VALUE
end
def self.number(): Integer
return VALUE
end
end
class Child < Answer
end
end
`)},
		{Filename: "/project/main.trb", ModulePath: "main", Source: []byte(`import { Models as Library } from models
def main()
puts(Library::Answer.new().identity().value())
puts(Library::Answer.number())
puts(Library::Child.new().value())
end
`)},
	}, Options{Mode: "typescript", SourceRoot: "/project", ProjectRoot: "/project"})
	if err != nil {
		t.Fatal(err)
	}
	checkTypeScriptArtifacts(t, artifacts, "qualified_classes")
}
