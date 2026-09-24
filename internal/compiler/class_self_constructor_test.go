package compiler

import (
	"strings"
	"testing"
)

func TestClassSelfConstructorUsesDeclaredClassAcrossModes(t *testing.T) {
	source := []byte(`class Item
	@value: Integer
	def initialize(value: Integer)
		@value = value
	end
	def self.make(value: Integer): Item
		return self.new(value)
	end
end
def main()
	puts(Item.make(3).value)
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("class_self_constructor.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected self.new(): %v", mode, err)
		}
		if mode == "go" {
			output := string(artifact.Output)
			if !strings.Contains(output, "NewItem(value)") || strings.Contains(output, "NewSelf(") {
				t.Fatalf("Go constructor did not resolve class self:\n%s", output)
			}
		}
	}
}

func TestClassSelfConstructorChecksArguments(t *testing.T) {
	source := []byte(`class Item
	def initialize(value: Integer)
	end
	def self.make(): Item
		return self.new("wrong")
	end
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("class_self_constructor_args.trb", source, mode); err == nil || !strings.Contains(err.Error(), "expected Integer") {
			t.Fatalf("%s did not check self.new() arguments: %v", mode, err)
		}
	}
}
